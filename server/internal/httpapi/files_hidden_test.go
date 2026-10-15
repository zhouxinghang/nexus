package httpapi_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestScanIgnoresDotPrefixedEntriesBeforeIndexing(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	visible := []string{"a.txt", filepath.Join("sub", "b.txt"), "with.dot.txt"}
	hidden := []string{
		".env",
		".DS_Store",
		"..hidden",
		filepath.Join("sub", ".secret"),
		filepath.Join(".git", "config"),
		filepath.Join("sub", ".cache", "data.txt"),
	}
	for _, name := range append(append([]string{}, visible...), hidden...) {
		writeFile(t, filepath.Join(dir, name), "x")
	}
	root := h.createRoot(t, dir)
	st := h.scanAndWait(t, root.ID)
	if st.Total != len(visible) || st.Indexed != len(visible) || st.Removed != 0 {
		t.Fatalf("scan counted hidden entries: %+v", st)
	}
	// Assert persisted state, not merely a filter on the list response.
	var count int
	if err := h.pool.QueryRow(context.Background(), "SELECT count(*) FROM resources WHERE root_id=$1", root.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != len(visible) {
		t.Fatalf("persisted %d Resources, want %d", count, len(visible))
	}
	page := h.resourcePage(browseQuery(root.ID, dir))
	if len(page.Directories) != 1 || page.Directories[0].Name != "sub" {
		t.Fatalf("hidden directories exposed: %+v", page.Directories)
	}
	page = h.resourcePage(browseQuery(root.ID, filepath.Join(dir, "sub")))
	if len(page.Directories) != 0 {
		t.Fatalf("hidden nested directories exposed: %+v", page.Directories)
	}
	writeFile(t, filepath.Join(dir, ".new"), "hidden")
	writeFile(t, filepath.Join(dir, ".git", "new.txt"), "hidden")
	st = h.scanAndWait(t, root.ID)
	if st.Total != len(visible) || st.Indexed != 0 || st.Removed != 0 {
		t.Fatalf("rescan indexed hidden entries: %+v", st)
	}
	for _, name := range hidden {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("ignored Host file was changed: %v", err)
		}
	}
}

func TestRegisterIgnoresDotPrefixedEntries(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(t.TempDir(), ".ancestor", ".root")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	h.createRoot(t, dir)
	for _, name := range []string{".env", filepath.Join(".git", "config"), filepath.Join("sub", ".secret")} {
		file := filepath.Join(dir, name)
		writeFile(t, file, "x")
		resp := h.do(http.MethodPost, "/api/files/resources/register", map[string]string{"path": file})
		mustStatus(t, resp, http.StatusBadRequest)
		resp.Body.Close()
	}
	var count int
	if err := h.pool.QueryRow(context.Background(), "SELECT count(*) FROM resources").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("registered %d hidden Resources, want 0", count)
	}
	file := filepath.Join(dir, "visible.txt")
	writeFile(t, file, "x")
	resp := h.do(http.MethodPost, "/api/files/resources/register", map[string]string{"path": file})
	mustStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}
