package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"nexus/internal/db/dbgen"
	"nexus/internal/host"
)

func (s *Server) registerServerRoutes(r chi.Router) {
	r.Get("/servers/current", s.handleServerCurrent)
	r.Get("/servers/samples", s.handleServerSamples)
}

type vitalsResponse struct {
	CPUPercent float64         `json:"cpu_percent"`
	MemTotal   uint64          `json:"mem_total"`
	MemUsed    uint64          `json:"mem_used"`
	DiskTotal  uint64          `json:"disk_total"`
	DiskUsed   uint64          `json:"disk_used"`
	NetRx      uint64          `json:"net_rx"`
	NetTx      uint64          `json:"net_tx"`
	Load1      float64         `json:"load1"`
	Load5      float64         `json:"load5"`
	Load15     float64         `json:"load15"`
	UptimeSecs uint64          `json:"uptime_secs"`
	TopProcs   []host.Process  `json:"top_procs"`
	Docker     host.DockerStatus `json:"docker"`
}

func (s *Server) handleServerCurrent(w http.ResponseWriter, r *http.Request) {
	v, err := s.host.Sample()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not sample host")
		return
	}
	resp := vitalsResponse{
		CPUPercent: v.CPUPercent,
		MemTotal:   v.MemTotal,
		MemUsed:    v.MemUsed,
		DiskTotal:  v.DiskTotal,
		DiskUsed:   v.DiskUsed,
		NetRx:      v.NetRx,
		NetTx:      v.NetTx,
		Load1:      v.Load1,
		Load5:      v.Load5,
		Load15:     v.Load15,
		UptimeSecs: v.UptimeSecs,
		TopProcs:   v.TopProcs,
		Docker:     s.host.Status(),
	}
	if resp.TopProcs == nil {
		resp.TopProcs = []host.Process{}
	}
	if resp.Docker.Containers == nil {
		resp.Docker.Containers = []host.Container{}
	}
	writeJSON(w, http.StatusOK, resp)
}

type sampleResponse struct {
	ID         int64            `json:"id"`
	TakenAt    time.Time        `json:"taken_at"`
	CPUPercent float64          `json:"cpu_percent"`
	MemTotal   int64            `json:"mem_total"`
	MemUsed    int64            `json:"mem_used"`
	DiskTotal  int64            `json:"disk_total"`
	DiskUsed   int64            `json:"disk_used"`
	NetRx      int64            `json:"net_rx"`
	NetTx      int64            `json:"net_tx"`
	Load1      float64          `json:"load1"`
	Load5      float64          `json:"load5"`
	Load15     float64          `json:"load15"`
	UptimeSecs int64            `json:"uptime_secs"`
	TopProcs   []host.Process   `json:"top_procs"`
	Containers []host.Container `json:"containers"`
}

func (s *Server) handleServerSamples(w http.ResponseWriter, r *http.Request) {
	// Default range: last 24h. The upper bound is nudged slightly into the
	// future so sub-second clock skew between the DB (which stamps taken_at
	// via now()) and this process doesn't drop the most recent sample.
	// Overridable with ?from=&to= RFC3339.
	now := time.Now()
	from := now.Add(-24 * time.Hour)
	to := now.Add(1 * time.Hour)
	q := r.URL.Query()
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from timestamp")
			return
		}
		from = t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to timestamp")
			return
		}
		to = t
	}

	rows, err := s.queries.ListMetricSamplesInRange(r.Context(), dbgen.ListMetricSamplesInRangeParams{
		TakenAt:   from,
		TakenAt_2: to,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list samples")
		return
	}
	out := make([]sampleResponse, 0, len(rows))
	for _, s := range rows {
		var procs []host.Process
		var conts []host.Container
		_ = json.Unmarshal(s.TopProcs, &procs)
		_ = json.Unmarshal(s.Containers, &conts)
		if procs == nil {
			procs = []host.Process{}
		}
		if conts == nil {
			conts = []host.Container{}
		}
		out = append(out, sampleResponse{
			ID:         s.ID,
			TakenAt:    s.TakenAt,
			CPUPercent: s.CpuPercent,
			MemTotal:   s.MemTotal,
			MemUsed:    s.MemUsed,
			DiskTotal:  s.DiskTotal,
			DiskUsed:   s.DiskUsed,
			NetRx:      s.NetRx,
			NetTx:      s.NetTx,
			Load1:      s.Load1,
			Load5:      s.Load5,
			Load15:     s.Load15,
			UptimeSecs: s.UptimeSecs,
			TopProcs:   procs,
			Containers: conts,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
