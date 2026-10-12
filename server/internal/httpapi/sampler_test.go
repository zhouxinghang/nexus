package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type sampleResp struct {
	ID         int64     `json:"id"`
	TakenAt    time.Time `json:"taken_at"`
	CPUPercent float64   `json:"cpu_percent"`
	MemUsed    int64     `json:"mem_used"`
	TopProcs   []struct {
		Name string `json:"name"`
	} `json:"top_procs"`
}

func TestSamplesAccumulate(t *testing.T) {
	h := newHarness(t)
	h.host.SetVitals(cannedVitals())

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := h.server.CaptureSampleForTest(ctx); err != nil {
			t.Fatalf("capture: %v", err)
		}
	}

	resp := h.do(http.MethodGet, "/api/servers/samples", nil)
	mustStatus(t, resp, http.StatusOK)
	var samples []sampleResp
	decode(t, resp, &samples)
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
	if samples[0].CPUPercent != 42.5 {
		t.Errorf("sample cpu = %v", samples[0].CPUPercent)
	}
	if len(samples[0].TopProcs) != 1 || samples[0].TopProcs[0].Name != "init" {
		t.Errorf("sample top procs = %+v", samples[0].TopProcs)
	}
}

func TestSamplesTimeRangeQuery(t *testing.T) {
	h := newHarness(t)
	h.host.SetVitals(cannedVitals())
	ctx := context.Background()
	if err := h.server.CaptureSampleForTest(ctx); err != nil {
		t.Fatalf("capture: %v", err)
	}

	// A window entirely in the past should yield nothing.
	from := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	resp := h.do(http.MethodGet, "/api/servers/samples?from="+from+"&to="+to, nil)
	mustStatus(t, resp, http.StatusOK)
	var samples []sampleResp
	decode(t, resp, &samples)
	if len(samples) != 0 {
		t.Errorf("expected 0 samples in past window, got %d", len(samples))
	}

	// A window including now should include the sample.
	from2 := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	to2 := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	resp2 := h.do(http.MethodGet, "/api/servers/samples?from="+from2+"&to="+to2, nil)
	mustStatus(t, resp2, http.StatusOK)
	var samples2 []sampleResp
	decode(t, resp2, &samples2)
	if len(samples2) != 1 {
		t.Errorf("expected 1 sample in current window, got %d", len(samples2))
	}
}

func TestSamplesRetentionPrune(t *testing.T) {
	h := newHarness(t)
	h.host.SetVitals(cannedVitals())
	ctx := context.Background()

	// One fresh sample and one aged 40 days.
	if err := h.server.CaptureSampleForTest(ctx); err != nil {
		t.Fatalf("capture: %v", err)
	}
	if err := h.server.InsertAgedSampleForTest(ctx, 40); err != nil {
		t.Fatalf("insert aged: %v", err)
	}

	removed, err := h.server.PruneSamplesForTest(ctx)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected to prune 1 aged sample, removed %d", removed)
	}

	// Only the fresh sample remains.
	from := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	to := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	resp := h.do(http.MethodGet, "/api/servers/samples?from="+from+"&to="+to, nil)
	var samples []sampleResp
	decode(t, resp, &samples)
	if len(samples) != 1 {
		t.Errorf("expected 1 remaining sample, got %d", len(samples))
	}
}
