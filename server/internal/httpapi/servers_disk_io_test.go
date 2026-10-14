package httpapi_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"nexus/internal/host"
)

type diskIORates struct {
	Read      *float64 `json:"disk_read_rate"`
	Write     *float64 `json:"disk_write_rate"`
	ReadIOPS  *float64 `json:"disk_read_iops"`
	WriteIOPS *float64 `json:"disk_write_iops"`
}

func TestServersDiskIOCurrentAndHistory(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	for _, rate := range []*host.DiskRate{
		{ReadBytes: 4096, WriteBytes: 2048, ReadOps: 8, WriteOps: 4},
		{ReadBytes: 8192, WriteBytes: 4096, ReadOps: 16, WriteOps: 8},
		nil, {},
	} {
		v := cannedVitals()
		v.DiskRate = rate
		h.host.SetVitals(v)
		resp := h.do(http.MethodGet, "/api/servers/current", nil)
		mustStatus(t, resp, http.StatusOK)
		var current struct {
			DiskRate *host.DiskRate `json:"disk_rate"`
		}
		decode(t, resp, &current)
		if !reflect.DeepEqual(current.DiskRate, rate) {
			t.Fatalf("current disk_rate=%+v, want %+v", current.DiskRate, rate)
		}
		if err := h.server.CaptureSampleForTest(ctx); err != nil {
			t.Fatal(err)
		}
	}
	resp := h.do(http.MethodGet, "/api/servers/samples", nil)
	mustStatus(t, resp, http.StatusOK)
	var raw []diskIORates
	decode(t, resp, &raw)
	if len(raw) != 4 {
		t.Fatalf("history has %d rows, want 4", len(raw))
	}
	if !eq(raw[0].Read, 4096) || !eq(raw[1].Write, 4096) || !eq(raw[1].ReadIOPS, 16) || !eq(raw[0].WriteIOPS, 4) {
		t.Fatalf("persisted rates = %+v", raw)
	}
	if raw[2] != (diskIORates{}) {
		t.Fatalf("unavailable rates must remain null, got %+v", raw[2])
	}
	if !eq(raw[3].Read, 0) || !eq(raw[3].Write, 0) || !eq(raw[3].ReadIOPS, 0) || !eq(raw[3].WriteIOPS, 0) {
		t.Fatalf("idle rates must remain real zero, got %+v", raw[3])
	}
	now := time.Now().UTC()
	query := "?from=" + now.Add(-time.Minute).Format(time.RFC3339) + "&to=" + now.Add(time.Minute).Format(time.RFC3339) + "&max_points=1"
	resp = h.do(http.MethodGet, "/api/servers/samples"+query, nil)
	mustStatus(t, resp, http.StatusOK)
	var buckets []diskIORates
	decode(t, resp, &buckets)
	// NULL observation is ignored, measured idle zero contributes to averages.
	if len(buckets) != 1 || !eq(buckets[0].Read, 4096) || !eq(buckets[0].Write, 2048) || !eq(buckets[0].ReadIOPS, 8) || !eq(buckets[0].WriteIOPS, 4) {
		t.Fatalf("bucketed I/O rates = %+v", buckets)
	}
}

func TestDiskIOMigrationKeepsLegacySamplesNull(t *testing.T) {
	h := newHarnessSeeded(t, 17, func(ctx context.Context, pool *pgxpool.Pool) {
		_, err := pool.Exec(ctx, `INSERT INTO metric_samples
		  (cpu_percent,mem_total,mem_used,disk_total,disk_used,net_rx,net_tx,load1,load5,load15,uptime_secs)
		  VALUES (25,100,50,1000,500,0,0,1,1,1,3600)`)
		if err != nil {
			t.Fatal(err)
		}
	})
	for _, query := range []string{"", "?max_points=1"} {
		resp := h.do(http.MethodGet, "/api/servers/samples"+query, nil)
		mustStatus(t, resp, http.StatusOK)
		var samples []struct {
			diskIORates
			DiskUsed int64   `json:"disk_used"`
			CPU      float64 `json:"cpu_percent"`
		}
		decode(t, resp, &samples)
		if len(samples) != 1 || samples[0].diskIORates != (diskIORates{}) || samples[0].DiskUsed != 500 || samples[0].CPU != 25 {
			t.Fatalf("legacy sample changed for %q: %+v", query, samples)
		}
	}
}
