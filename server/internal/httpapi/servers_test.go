package httpapi_test

import (
	"net/http"
	"testing"

	"nexus/internal/host"
)

type currentResp struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemTotal   uint64  `json:"mem_total"`
	MemUsed    uint64  `json:"mem_used"`
	Load1      float64 `json:"load1"`
	UptimeSecs uint64  `json:"uptime_secs"`
	TopProcs   []struct {
		PID  int32  `json:"pid"`
		Name string `json:"name"`
	} `json:"top_procs"`
	Docker struct {
		Enabled    bool `json:"enabled"`
		Containers []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Image string `json:"image"`
			State string `json:"state"`
		} `json:"containers"`
	} `json:"docker"`
}

func TestServersCurrentServesCannedVitals(t *testing.T) {
	h := newHarness(t)
	h.host.SetVitals(cannedVitals())
	h.host.SetDocker(host.DockerStatus{
		Enabled: true,
		Containers: []host.Container{
			{ID: "abc123", Name: "/postgres", Image: "postgres:16", State: "running"},
		},
	})

	resp := h.do(http.MethodGet, "/api/servers/current", nil)
	mustStatus(t, resp, http.StatusOK)
	var out currentResp
	decode(t, resp, &out)

	if out.CPUPercent != 42.5 {
		t.Errorf("cpu = %v, want 42.5", out.CPUPercent)
	}
	if out.MemTotal != 16<<30 {
		t.Errorf("mem_total = %v", out.MemTotal)
	}
	if out.UptimeSecs != 3600 {
		t.Errorf("uptime = %v", out.UptimeSecs)
	}
	if len(out.TopProcs) != 1 || out.TopProcs[0].Name != "init" {
		t.Errorf("top procs = %+v", out.TopProcs)
	}
	if !out.Docker.Enabled || len(out.Docker.Containers) != 1 {
		t.Errorf("docker = %+v", out.Docker)
	}
	if out.Docker.Containers[0].Image != "postgres:16" {
		t.Errorf("container image = %q", out.Docker.Containers[0].Image)
	}
}

func TestServersDockerAbsentDegrades(t *testing.T) {
	h := newHarness(t)
	h.host.SetVitals(cannedVitals())
	// Docker disabled by default.
	resp := h.do(http.MethodGet, "/api/servers/current", nil)
	mustStatus(t, resp, http.StatusOK)
	var out currentResp
	decode(t, resp, &out)
	if out.Docker.Enabled {
		t.Error("expected docker to be reported as not enabled")
	}
	if out.Docker.Containers == nil {
		t.Error("expected an empty (non-null) containers slice")
	}
}
