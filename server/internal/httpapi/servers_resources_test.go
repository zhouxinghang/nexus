package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"nexus/internal/host"
)

// The Bazel-profiler-style resource set: core count, network throughput,
// pressure-stall information, and Nexus's own footprint.

func richVitals() host.Vitals {
	v := cannedVitals()
	v.CPUCores = 8
	v.NetRate = &host.NetRate{RxBytes: 1500, TxBytes: 250, RxPackets: 12, TxPackets: 3}
	v.Pressure = &host.Pressure{CPUSome: 4, MemorySome: 2.5, MemoryFull: 1, IOSome: 30, IOFull: 20}
	v.Self = &host.SelfUsage{CPU: 1.5, MemRSS: 64 << 20}
	return v
}

type resourcesCurrentResp struct {
	CPUCores int             `json:"cpu_cores"`
	NetRate  *host.NetRate   `json:"net_rate"`
	Pressure *host.Pressure  `json:"pressure"`
	Self     *host.SelfUsage `json:"self"`
}

func TestServersCurrentResourceSet(t *testing.T) {
	h := newHarness(t)
	h.host.SetVitals(richVitals())
	r := h.do(http.MethodGet, "/api/servers/current", nil)
	mustStatus(t, r, http.StatusOK)
	var out resourcesCurrentResp
	decode(t, r, &out)
	if out.CPUCores != 8 {
		t.Errorf("cpu_cores = %d, want 8", out.CPUCores)
	}
	if out.NetRate == nil || *out.NetRate != *richVitals().NetRate {
		t.Errorf("net_rate = %+v", out.NetRate)
	}
	if out.Pressure == nil || *out.Pressure != *richVitals().Pressure {
		t.Errorf("pressure = %+v", out.Pressure)
	}
	if out.Self == nil || out.Self.MemRSS != 64<<20 || out.Self.CPU != 1.5 {
		t.Errorf("self = %+v", out.Self)
	}

	// A Host that can't provide them (first reading, not Linux) reports null.
	h.host.SetVitals(cannedVitals())
	r = h.do(http.MethodGet, "/api/servers/current", nil)
	mustStatus(t, r, http.StatusOK)
	out = resourcesCurrentResp{}
	decode(t, r, &out)
	if out.NetRate != nil || out.Pressure != nil || out.Self != nil {
		t.Errorf("absent readings should be null, got %+v", out)
	}
}

type resourcesSampleResp struct {
	TakenAt    time.Time `json:"taken_at"`
	CPUCores   *int32    `json:"cpu_cores"`
	NetRxRate  *float64  `json:"net_rx_rate"`
	NetTxRate  *float64  `json:"net_tx_rate"`
	NetRxPps   *float64  `json:"net_rx_pps"`
	NetTxPps   *float64  `json:"net_tx_pps"`
	PsiCPUSome *float64  `json:"psi_cpu_some"`
	PsiMemSome *float64  `json:"psi_mem_some"`
	PsiMemFull *float64  `json:"psi_mem_full"`
	PsiIOSome  *float64  `json:"psi_io_some"`
	PsiIOFull  *float64  `json:"psi_io_full"`
	SelfCPU    *float64  `json:"self_cpu"`
	SelfRSS    *int64    `json:"self_rss"`
}

func (h *harness) resourceSamples(extra string) []resourcesSampleResp {
	h.t.Helper()
	r := h.do(http.MethodGet, "/api/servers/samples?"+extra, nil)
	mustStatus(h.t, r, http.StatusOK)
	var out []resourcesSampleResp
	decode(h.t, r, &out)
	return out
}

func eq(p *float64, want float64) bool { return p != nil && *p == want }

func TestSamplesPersistResourceSet(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.host.SetVitals(richVitals())
	if err := h.server.CaptureSampleForTest(ctx); err != nil {
		t.Fatalf("capture: %v", err)
	}
	h.host.SetVitals(cannedVitals()) // no rate/pressure/self this time
	if err := h.server.CaptureSampleForTest(ctx); err != nil {
		t.Fatalf("capture: %v", err)
	}

	got := h.resourceSamples("")
	if len(got) != 2 {
		t.Fatalf("samples = %d, want 2", len(got))
	}
	s := got[0]
	if s.CPUCores == nil || *s.CPUCores != 8 {
		t.Errorf("cpu_cores = %v, want 8", s.CPUCores)
	}
	if !eq(s.NetRxRate, 1500) || !eq(s.NetTxRate, 250) || !eq(s.NetRxPps, 12) || !eq(s.NetTxPps, 3) {
		t.Errorf("net rates = %v %v %v %v", s.NetRxRate, s.NetTxRate, s.NetRxPps, s.NetTxPps)
	}
	if !eq(s.PsiCPUSome, 4) || !eq(s.PsiMemSome, 2.5) || !eq(s.PsiMemFull, 1) || !eq(s.PsiIOSome, 30) || !eq(s.PsiIOFull, 20) {
		t.Errorf("psi = %v %v %v %v %v", s.PsiCPUSome, s.PsiMemSome, s.PsiMemFull, s.PsiIOSome, s.PsiIOFull)
	}
	if !eq(s.SelfCPU, 1.5) || s.SelfRSS == nil || *s.SelfRSS != 64<<20 {
		t.Errorf("self = %v %v", s.SelfCPU, s.SelfRSS)
	}

	bare := got[1]
	if bare.CPUCores != nil || bare.NetRxRate != nil || bare.PsiCPUSome != nil || bare.SelfCPU != nil || bare.SelfRSS != nil {
		t.Errorf("a sample without the readings should carry nulls, got %+v", bare)
	}
}

func TestSamplesBucketedResourceSet(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	end := time.Now().UTC().Truncate(time.Minute)
	// Two hours of legacy rows (no new columns) followed by fresh captures.
	h.seedSamples(end.Add(-time.Hour), 60)
	for _, rx := range []float64{1000, 3000} {
		v := richVitals()
		v.NetRate.RxBytes = rx
		h.host.SetVitals(v)
		if err := h.server.CaptureSampleForTest(ctx); err != nil {
			t.Fatalf("capture: %v", err)
		}
	}

	from := end.Add(-2 * time.Hour)
	to := time.Now().UTC().Add(time.Minute)
	got := h.resourceSamples("from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339) + "&max_points=4")
	if len(got) < 2 {
		t.Fatalf("buckets = %d, want legacy + fresh", len(got))
	}
	legacy, fresh := got[0], got[len(got)-1]
	if legacy.CPUCores != nil || legacy.NetRxRate != nil || legacy.PsiIOSome != nil || legacy.SelfRSS != nil {
		t.Errorf("legacy-only bucket should be null, got %+v", legacy)
	}
	if !eq(fresh.NetRxRate, 2000) {
		t.Errorf("bucketed net_rx_rate = %v, want avg 2000", fresh.NetRxRate)
	}
	if fresh.CPUCores == nil || *fresh.CPUCores != 8 || !eq(fresh.PsiIOSome, 30) || !eq(fresh.SelfCPU, 1.5) {
		t.Errorf("fresh bucket = %+v", fresh)
	}
	if fresh.SelfRSS == nil || *fresh.SelfRSS != 64<<20 {
		t.Errorf("bucketed self_rss = %v", fresh.SelfRSS)
	}
}
