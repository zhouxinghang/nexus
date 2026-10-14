package httpapi

import (
	"net/http"
	"time"
)

// overviewJournalLimit is how many recent Journal entries Overview shows.
const overviewJournalLimit = 5

// overviewThreshold is the usage percent at or above which a vital is flagged.
// Flags are visual only; nothing is ever sent anywhere.
const overviewThreshold = 90.0

type overviewResponse struct {
	Host      overviewHost           `json:"host"`
	Tasks     overviewTasks          `json:"tasks"`
	Journal   []journalEntryResponse `json:"journal"`
	Bookmarks overviewBookmarks      `json:"bookmarks"`
	Files     overviewFiles          `json:"files"`
}

type overviewHost struct {
	// Available is false when the live sample failed; the numbers are then
	// zero and the other panels still render.
	Available     bool          `json:"available"`
	CPUPercent    float64       `json:"cpu_percent"`
	MemTotal      uint64        `json:"mem_total"`
	MemUsed       uint64        `json:"mem_used"`
	MemPercent    float64       `json:"mem_percent"`
	DiskTotal     uint64        `json:"disk_total"`
	DiskUsed      uint64        `json:"disk_used"`
	DiskPercent   float64       `json:"disk_percent"`
	Load1         float64       `json:"load1"`
	Load5         float64       `json:"load5"`
	Load15        float64       `json:"load15"`
	UptimeSecs    uint64        `json:"uptime_secs"`
	DockerEnabled bool          `json:"docker_enabled"`
	Containers    int           `json:"containers"`
	Flags         overviewFlags `json:"flags"`
}

type overviewFlags struct {
	CPU  bool `json:"cpu"`
	Mem  bool `json:"mem"`
	Disk bool `json:"disk"`
}

type overviewTasks struct {
	Todo     int64 `json:"todo"`
	Doing    int64 `json:"doing"`
	Done     int64 `json:"done"`
	Overdue  int64 `json:"overdue"`
	DueToday int64 `json:"due_today"`
}

type overviewBookmarks struct {
	Up       int64                  `json:"up"`
	Down     int64                  `json:"down"`
	Unknown  int64                  `json:"unknown"`
	DownList []overviewDownBookmark `json:"down_list"`
}

type overviewDownBookmark struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
}

type overviewFiles struct {
	Total   int64 `json:"total"`
	Missing int64 `json:"missing"`
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

// handleOverview composes the workbench summary in one round trip. Vitals come
// from the live Host sample (like /servers/current), so Overview reflects "now"
// without depending on the sampler having run; everything else is small
// count/summary SQL rather than re-listing whole tables.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var out overviewResponse

	if v, err := s.host.Sample(); err == nil {
		h := overviewHost{
			Available:   true,
			CPUPercent:  v.CPUPercent,
			MemTotal:    v.MemTotal,
			MemUsed:     v.MemUsed,
			MemPercent:  percent(v.MemUsed, v.MemTotal),
			DiskTotal:   v.DiskTotal,
			DiskUsed:    v.DiskUsed,
			DiskPercent: percent(v.DiskUsed, v.DiskTotal),
			Load1:       v.Load1,
			Load5:       v.Load5,
			Load15:      v.Load15,
			UptimeSecs:  v.UptimeSecs,
		}
		h.Flags = overviewFlags{
			CPU:  h.CPUPercent >= overviewThreshold,
			Mem:  h.MemPercent >= overviewThreshold,
			Disk: h.DiskPercent >= overviewThreshold,
		}
		out.Host = h
	}
	docker := s.host.Status()
	out.Host.DockerEnabled = docker.Enabled
	out.Host.Containers = len(docker.Containers)

	tasks, err := s.queries.TaskSummary(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not summarize tasks")
		return
	}
	out.Tasks = overviewTasks{
		Todo: tasks.Todo, Doing: tasks.Doing, Done: tasks.Done,
		Overdue: tasks.Overdue, DueToday: tasks.DueToday,
	}

	entries, err := s.queries.LatestJournalEntries(ctx, overviewJournalLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load journal")
		return
	}
	out.Journal = []journalEntryResponse{}
	for _, e := range entries {
		out.Journal = append(out.Journal, journalEntryResponse{ID: e.ID, Body: e.Body, CreatedAt: e.CreatedAt})
	}

	health, err := s.queries.BookmarkHealthSummary(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not summarize bookmarks")
		return
	}
	down, err := s.queries.ListDownBookmarks(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list down bookmarks")
		return
	}
	out.Bookmarks = overviewBookmarks{
		Up: health.Up, Down: health.Down, Unknown: health.Unknown,
		DownList: []overviewDownBookmark{},
	}
	for _, b := range down {
		out.Bookmarks.DownList = append(out.Bookmarks.DownList, overviewDownBookmark{
			ID: b.ID, Name: b.Name, URL: b.Url, LastCheckedAt: b.LastCheckedAt,
		})
	}

	files, err := s.queries.ResourceSummary(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not summarize files")
		return
	}
	out.Files = overviewFiles{Total: files.Total, Missing: files.Missing}

	writeJSON(w, http.StatusOK, out)
}
