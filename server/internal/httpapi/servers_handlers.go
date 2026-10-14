package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"nexus/internal/db/dbgen"
	"nexus/internal/host"
)

// maxSamplePoints caps ?max_points= on /servers/samples.
const maxSamplePoints = 2000

func (s *Server) registerServerRoutes(r chi.Router) {
	r.Get("/servers/current", s.handleServerCurrent)
	r.Get("/servers/samples", s.handleServerSamples)
	// Read-only drill-ins; monitoring only, no process/container actions.
	r.Get("/servers/processes/{pid}", s.handleProcessDetail)
	r.Get("/servers/containers/{id}", s.handleContainerDetail)
}

type vitalsResponse struct {
	CPUPercent float64           `json:"cpu_percent"`
	MemTotal   uint64            `json:"mem_total"`
	MemUsed    uint64            `json:"mem_used"`
	DiskTotal  uint64            `json:"disk_total"`
	DiskUsed   uint64            `json:"disk_used"`
	Disks      []host.DiskUsage  `json:"disks"`
	NetRx      uint64            `json:"net_rx"`
	NetTx      uint64            `json:"net_tx"`
	Load1      float64           `json:"load1"`
	Load5      float64           `json:"load5"`
	Load15     float64           `json:"load15"`
	UptimeSecs uint64            `json:"uptime_secs"`
	TopProcs   []host.Process    `json:"top_procs"`
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
		Disks:      v.Disks,
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
	if resp.Disks == nil {
		resp.Disks = []host.DiskUsage{}
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

// handleServerSamples returns the Metric Samples in [from, to] (default: the
// last 24h), oldest first. With ?max_points=N the rows are averaged into at
// most N equal time buckets in SQL so a 30-day chart stays light. That is
// query-time aggregation for display only: the stored samples and their raw
// 30-day retention are unchanged, and without the param raw rows come back
// exactly as before. Bucketed rows carry the bucket start as taken_at and
// empty top_procs/containers.
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

	if v := q.Get("max_points"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "invalid max_points")
			return
		}
		if n > maxSamplePoints {
			n = maxSamplePoints
		}
		if !to.After(from) {
			writeError(w, http.StatusBadRequest, "from must be before to")
			return
		}
		s.writeBucketedSamples(w, r, from, to, n)
		return
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

// writeBucketedSamples averages [from, to] into at most n buckets. The width
// is span/n plus one microsecond so the sample exactly at `to` still lands in
// bucket n-1, never an (n+1)th.
func (s *Server) writeBucketedSamples(w http.ResponseWriter, r *http.Request, from, to time.Time, n int) {
	span := to.Sub(from).Microseconds()
	width := span/int64(n) + 1
	rows, err := s.queries.ListMetricSamplesBucketed(r.Context(), dbgen.ListMetricSamplesBucketedParams{
		Bucket: pgtype.Interval{Microseconds: width, Valid: true},
		FromAt: from,
		ToAt:   to,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list samples")
		return
	}
	out := make([]sampleResponse, 0, len(rows))
	for _, b := range rows {
		out = append(out, sampleResponse{
			ID:         b.ID,
			TakenAt:    b.BucketAt,
			CPUPercent: b.CpuPercent,
			MemTotal:   b.MemTotal,
			MemUsed:    b.MemUsed,
			DiskTotal:  b.DiskTotal,
			DiskUsed:   b.DiskUsed,
			NetRx:      b.NetRx,
			NetTx:      b.NetTx,
			Load1:      b.Load1,
			Load5:      b.Load5,
			Load15:     b.Load15,
			UptimeSecs: b.UptimeSecs,
			TopProcs:   []host.Process{},
			Containers: []host.Container{},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleProcessDetail returns one process's read-only details. A pid that has
// exited is a 404, so a click on a stale row degrades cleanly.
func (s *Server) handleProcessDetail(w http.ResponseWriter, r *http.Request) {
	pid, err := strconv.ParseInt(chi.URLParam(r, "pid"), 10, 32)
	if err != nil || pid <= 0 {
		writeError(w, http.StatusBadRequest, "invalid pid")
		return
	}
	d, err := s.host.ProcessDetail(int32(pid))
	if err != nil {
		if errors.Is(err, host.ErrNotFound) {
			writeError(w, http.StatusNotFound, "process is no longer running")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not read process")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleContainerDetail returns one container's read-only details: 404 when
// it's gone, 503 when Docker itself isn't available.
func (s *Server) handleContainerDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.host.ContainerDetail(chi.URLParam(r, "id"))
	if err != nil {
		switch {
		case errors.Is(err, host.ErrNotFound):
			writeError(w, http.StatusNotFound, "container is no longer present")
		case errors.Is(err, host.ErrDockerUnavailable):
			writeError(w, http.StatusServiceUnavailable, "docker is not available on this host")
		default:
			writeError(w, http.StatusInternalServerError, "could not inspect container")
		}
		return
	}
	if d.Ports == nil {
		d.Ports = []host.PortBinding{}
	}
	if d.Mounts == nil {
		d.Mounts = []host.Mount{}
	}
	writeJSON(w, http.StatusOK, d)
}
