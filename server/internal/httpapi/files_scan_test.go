package httpapi_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// scanTimes rescans a root to completion and returns each file's updated_at,
// so a test can tell which rows the scan actually wrote.
func (h *harness) scanTimes(t *testing.T, rootID int64) map[string]time.Time {
	t.Helper()
	h.scanAndWait(t, rootID)
	resp := h.do(http.MethodGet, fmt.Sprintf("/api/files/resources?root_id=%d&limit=1000", rootID), nil)
	mustStatus(t, resp, http.StatusOK)
	var rows []struct {
		Name      string    `json:"name"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	decodeItems(t, resp, &rows)
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		out[r.Name] = r.UpdatedAt
	}
	return out
}

// A rescan should only write the rows that changed: an untouched file keeps its
// updated_at, while an edited file's row is rewritten. This is the modtime-based
// "quick scan" that keeps rescanning a large, mostly-static directory cheap.
func TestScanSkipsUnchangedFiles(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	stable := filepath.Join(dir, "stable.txt")
	edited := filepath.Join(dir, "edited.txt")
	writeFile(t, stable, "unchanging")
	writeFile(t, edited, "before")
	root := h.createRoot(t, dir)

	first := h.scanTimes(t, root.ID)
	if _, ok := first["stable.txt"]; !ok {
		t.Fatalf("stable.txt missing from first scan: %+v", first)
	}

	// Change one file's size (and mtime). A size change is always detectable.
	writeFile(t, edited, "after-and-longer")

	second := h.scanTimes(t, root.ID)
	if !second["stable.txt"].Equal(first["stable.txt"]) {
		t.Errorf("stable.txt was rewritten: %v -> %v (should be skipped)",
			first["stable.txt"], second["stable.txt"])
	}
	if !second["edited.txt"].After(first["edited.txt"]) {
		t.Errorf("edited.txt updated_at = %v, want after %v",
			second["edited.txt"], first["edited.txt"])
	}
}

// A directory far larger than one upsert batch must index completely across the
// chunked BulkUpsertResources calls, with nothing dropped at a batch boundary.
func TestScanLargeDirectoryBatches(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	const n = 2500 // > scanUpsertBatch (2000), so the scan flushes more than once
	for i := 0; i < n; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%04d.txt", i)), "x")
	}
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)

	page := h.resourcePage("?limit=1")
	if page.Total != n {
		t.Fatalf("indexed %d files, want %d", page.Total, n)
	}
}

// Deleting many files between scans marks them all Missing in the single
// set-based reconcile, leaving the survivors Present.
func TestScanReconcilesMissingInBulk(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	const n = 50
	for i := 0; i < n; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), "x")
	}
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)

	// Remove the even-numbered files out from under Nexus.
	for i := 0; i < n; i += 2 {
		if err := os.Remove(filepath.Join(dir, fmt.Sprintf("f%02d.txt", i))); err != nil {
			t.Fatalf("remove: %v", err)
		}
	}
	h.scan(t, root.ID)

	page := h.resourcePage("?limit=1000")
	missing, present := 0, 0
	for _, r := range page.Items {
		switch r.State {
		case "missing":
			missing++
		case "present":
			present++
		}
	}
	if missing != n/2 || present != n/2 {
		t.Fatalf("after bulk delete: %d missing, %d present; want %d, %d",
			missing, present, n/2, n/2)
	}
}

// The status endpoint reports a terminal "completed" phase with the files
// seen and indexed, so the UI can show progress and know when to refresh.
func TestScanStatusReportsProgress(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	const n = 12
	for i := 0; i < n; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), "x")
	}
	root := h.createRoot(t, dir)

	// Before any scan, status is idle.
	resp := h.do(http.MethodGet, path("/api/files/roots/", root.ID)+"/scan", nil)
	mustStatus(t, resp, http.StatusOK)
	var idle scanStatusResp
	decode(t, resp, &idle)
	if idle.Phase != "idle" {
		t.Fatalf("pre-scan phase = %q, want idle", idle.Phase)
	}

	st := h.scanAndWait(t, root.ID)
	if st.Phase != "completed" {
		t.Fatalf("phase = %q, want completed", st.Phase)
	}
	if st.Total != n || st.Indexed != n {
		t.Errorf("status total=%d indexed=%d, want %d/%d", st.Total, st.Indexed, n, n)
	}

	// A second scan finds nothing changed: everything is skipped, so indexed
	// drops to zero while total still reflects the files on disk.
	st2 := h.scanAndWait(t, root.ID)
	if st2.Total != n || st2.Indexed != 0 {
		t.Errorf("rescan total=%d indexed=%d, want %d/0", st2.Total, st2.Indexed, n)
	}
}

// Status for a root that doesn't exist is a 404, not an idle status.
func TestScanStatusUnknownRoot(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodGet, "/api/files/roots/999999/scan", nil)
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

