package httpapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nexus/internal/checker"
	"nexus/internal/host"
)

type overviewResp struct {
	Host struct {
		Available   bool    `json:"available"`
		CPUPercent  float64 `json:"cpu_percent"`
		MemPercent  float64 `json:"mem_percent"`
		DiskPercent float64 `json:"disk_percent"`
		UptimeSecs  uint64  `json:"uptime_secs"`
		Containers  int     `json:"containers"`
		Flags       struct {
			CPU  bool `json:"cpu"`
			Mem  bool `json:"mem"`
			Disk bool `json:"disk"`
		} `json:"flags"`
	} `json:"host"`
	Tasks struct {
		Todo     int64 `json:"todo"`
		Doing    int64 `json:"doing"`
		Done     int64 `json:"done"`
		Overdue  int64 `json:"overdue"`
		DueToday int64 `json:"due_today"`
	} `json:"tasks"`
	Journal   []journalEntry `json:"journal"`
	Bookmarks struct {
		Up       int64 `json:"up"`
		Down     int64 `json:"down"`
		Unknown  int64 `json:"unknown"`
		DownList []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"down_list"`
	} `json:"bookmarks"`
	Files struct {
		Total   int64 `json:"total"`
		Missing int64 `json:"missing"`
	} `json:"files"`
}

func (h *harness) overview() overviewResp {
	h.t.Helper()
	r := h.do(http.MethodGet, "/api/overview", nil)
	mustStatus(h.t, r, http.StatusOK)
	var o overviewResp
	decode(h.t, r, &o)
	return o
}

func TestOverviewSummarizesWorkbench(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// Host: CPU and disk over threshold, memory under.
	v := cannedVitals()
	v.CPUPercent = 95
	v.DiskTotal, v.DiskUsed = 100<<30, 92<<30
	h.host.SetVitals(v)
	h.host.SetDocker(host.DockerStatus{Enabled: true, Containers: []host.Container{{ID: "a"}, {ID: "b"}}})

	// Tasks, judged against the DB's calendar day (due dates are stored as
	// midnight of the chosen day).
	var today time.Time
	if err := h.pool.QueryRow(ctx, `SELECT date_trunc('day', now())`).Scan(&today); err != nil {
		t.Fatal(err)
	}
	yesterday := today.Add(-24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)
	mk := func(title, status string, due *time.Time) taskResp {
		body := map[string]interface{}{"title": title, "status": status}
		if due != nil {
			body["due_at"] = due
		}
		r := h.do(http.MethodPost, "/api/tasks", body)
		mustStatus(t, r, http.StatusCreated)
		var tk taskResp
		decode(t, r, &tk)
		return tk
	}
	mk("overdue todo", "todo", &yesterday)
	mk("overdue doing", "doing", &yesterday)
	mk("due today", "todo", &today)
	mk("due tomorrow", "todo", &tomorrow)
	mk("done but late", "done", &yesterday) // done is never overdue
	archived := mk("archived late", "todo", &yesterday)
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", archived.ID)+"/archive", nil), http.StatusOK)

	// Journal: more entries than Overview shows.
	for i := 0; i < 7; i++ {
		r := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": fmt.Sprintf("note %d", i)})
		r.Body.Close()
	}

	// Bookmarks: two up, one down (via the fake Prober), one never checked.
	h.prober.SetResult("https://down.example.com", checker.ProbeResult{Up: false})
	h.createBookmark("Up A", "https://a.example.com")
	h.createBookmark("Up B", "https://b.example.com")
	down := h.createBookmark("Broken", "https://down.example.com")
	if err := h.server.CheckBookmarksForTest(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx, `INSERT INTO bookmarks (name, url) VALUES ('Fresh', 'https://fresh.example.com')`); err != nil {
		t.Fatal(err)
	}

	// Files: three indexed, one then deleted out from under Nexus → Missing.
	dir := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		writeFile(t, filepath.Join(dir, n), "x")
	}
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)
	if err := os.Remove(filepath.Join(dir, "c.txt")); err != nil {
		t.Fatal(err)
	}
	h.scan(t, root.ID)

	o := h.overview()

	if !o.Host.Available || o.Host.CPUPercent != 95 || o.Host.UptimeSecs != 3600 || o.Host.Containers != 2 {
		t.Errorf("host = %+v", o.Host)
	}
	if !o.Host.Flags.CPU || o.Host.Flags.Mem || !o.Host.Flags.Disk {
		t.Errorf("flags = %+v, want cpu+disk only", o.Host.Flags)
	}
	if o.Host.MemPercent != 50 || o.Host.DiskPercent != 92 {
		t.Errorf("percents mem=%v disk=%v", o.Host.MemPercent, o.Host.DiskPercent)
	}

	if o.Tasks.Todo != 3 || o.Tasks.Doing != 1 || o.Tasks.Done != 1 {
		t.Errorf("task counts = %+v, want todo 3 doing 1 done 1", o.Tasks)
	}
	if o.Tasks.Overdue != 2 || o.Tasks.DueToday != 1 {
		t.Errorf("overdue/due today = %d/%d, want 2/1", o.Tasks.Overdue, o.Tasks.DueToday)
	}

	if len(o.Journal) != 5 || o.Journal[0].Body != "note 6" || o.Journal[4].Body != "note 2" {
		t.Errorf("journal = %+v, want the 5 newest", o.Journal)
	}

	if o.Bookmarks.Up != 2 || o.Bookmarks.Down != 1 || o.Bookmarks.Unknown != 1 {
		t.Errorf("bookmark counts = %+v", o.Bookmarks)
	}
	if len(o.Bookmarks.DownList) != 1 || o.Bookmarks.DownList[0].ID != down.ID ||
		o.Bookmarks.DownList[0].URL != "https://down.example.com" {
		t.Errorf("down list = %+v", o.Bookmarks.DownList)
	}

	if o.Files.Total != 3 || o.Files.Missing != 1 {
		t.Errorf("files = %+v, want 3 total / 1 missing", o.Files)
	}
}

func TestOverviewEmptyWorkbench(t *testing.T) {
	h := newHarness(t)
	h.host.SetVitals(cannedVitals())
	o := h.overview()
	if o.Tasks.Todo != 0 || o.Tasks.Overdue != 0 || o.Files.Total != 0 || o.Bookmarks.Down != 0 {
		t.Errorf("non-zero counts on an empty workbench: %+v", o)
	}
	if o.Journal == nil || o.Bookmarks.DownList == nil {
		t.Error("expected empty arrays, not null")
	}
	if o.Host.Flags.CPU || o.Host.Flags.Mem || o.Host.Flags.Disk {
		t.Errorf("flags raised for healthy vitals: %+v", o.Host.Flags)
	}
}

func TestOverviewSurvivesHostSampleFailure(t *testing.T) {
	h := newHarness(t)
	h.host.SampleErr = errors.New("sensor offline")
	o := h.overview()
	if o.Host.Available {
		t.Error("host should be reported unavailable when sampling fails")
	}
}

func TestOverviewRequiresAuth(t *testing.T) {
	h := newHarness(t)
	r := h.doWithCookie(http.MethodGet, "/api/overview", nil, nil)
	mustStatus(t, r, http.StatusUnauthorized)
}
