package httpapi_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type resourceResp struct {
	ID         int64      `json:"id"`
	RootID     int64      `json:"root_id"`
	Path       string     `json:"path"`
	Name       string     `json:"name"`
	Size       int64      `json:"size"`
	IsDir      bool       `json:"is_dir"`
	ModifiedAt *time.Time `json:"modified_at"`
	State      string     `json:"state"`
	Note       string     `json:"note"`
	Tags       []string   `json:"tags"`
}

type rootResp struct {
	ID   int64  `json:"id"`
	Path string `json:"path"`
}

// writeFile creates a file with content under dir, returning its full path.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func (h *harness) createRoot(t *testing.T, dir string) rootResp {
	t.Helper()
	resp := h.do(http.MethodPost, "/api/files/roots", map[string]string{"path": dir})
	mustStatus(t, resp, http.StatusCreated)
	var root rootResp
	decode(t, resp, &root)
	return root
}

func (h *harness) scan(t *testing.T, rootID int64) []resourceResp {
	t.Helper()
	resp := h.do(http.MethodPost, path("/api/files/roots/", rootID)+"/scan", nil)
	mustStatus(t, resp, http.StatusOK)
	var resources []resourceResp
	decode(t, resp, &resources)
	return resources
}

func TestRootRejectsNonexistentPath(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/files/roots", map[string]string{"path": "/no/such/dir/really"})
	mustStatus(t, resp, http.StatusBadRequest)
}

func TestRootRejectsFilePath(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "a-file.txt")
	writeFile(t, f, "hello")
	resp := h.do(http.MethodPost, "/api/files/roots", map[string]string{"path": f})
	mustStatus(t, resp, http.StatusBadRequest)
}

func TestRootCreateListRemove(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	root := h.createRoot(t, dir)
	if root.Path != filepath.Clean(dir) {
		t.Errorf("root path = %q, want %q", root.Path, dir)
	}

	lr := h.do(http.MethodGet, "/api/files/roots", nil)
	mustStatus(t, lr, http.StatusOK)
	var roots []rootResp
	decode(t, lr, &roots)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}

	dr := h.do(http.MethodDelete, path("/api/files/roots/", root.ID), nil)
	mustStatus(t, dr, http.StatusNoContent)

	lr2 := h.do(http.MethodGet, "/api/files/roots", nil)
	var roots2 []rootResp
	decode(t, lr2, &roots2)
	if len(roots2) != 0 {
		t.Errorf("expected 0 roots after delete, got %d", len(roots2))
	}
}

func TestScanIndexesFiles(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "aaa")
	writeFile(t, filepath.Join(dir, "sub", "b.txt"), "bbbbb")
	root := h.createRoot(t, dir)

	resources := h.scan(t, root.ID)
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
	names := map[string]int64{}
	for _, r := range resources {
		names[r.Name] = r.Size
		if r.State != "present" {
			t.Errorf("resource %s state = %q, want present", r.Name, r.State)
		}
	}
	if names["a.txt"] != 3 || names["b.txt"] != 5 {
		t.Errorf("unexpected sizes: %+v", names)
	}
}

func TestScanDetectsNewUpdatedMissing(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	fa := filepath.Join(dir, "a.txt")
	writeFile(t, fa, "aaa")
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)

	// New file appears.
	fb := filepath.Join(dir, "b.txt")
	writeFile(t, fb, "bb")
	// Existing file changes size.
	writeFile(t, fa, "aaaaaaaa")

	resources := h.scan(t, root.ID)
	byName := map[string]resourceResp{}
	for _, r := range resources {
		byName[r.Name] = r
	}
	if byName["a.txt"].Size != 8 {
		t.Errorf("a.txt updated size = %d, want 8", byName["a.txt"].Size)
	}
	if _, ok := byName["b.txt"]; !ok {
		t.Error("expected new b.txt resource")
	}

	// Delete a file outside Nexus → marked Missing, not removed.
	if err := os.Remove(fb); err != nil {
		t.Fatalf("remove: %v", err)
	}
	resources2 := h.scan(t, root.ID)
	var bState string
	for _, r := range resources2 {
		if r.Name == "b.txt" {
			bState = r.State
		}
	}
	if bState != "missing" {
		t.Errorf("b.txt state = %q, want missing", bState)
	}
}

func TestClearMissingResource(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "gone.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	os.Remove(f)
	h.scan(t, root.ID)

	// Clearing a present resource is rejected; clearing a missing one works.
	dr := h.do(http.MethodDelete, path("/api/files/resources/", id), nil)
	mustStatus(t, dr, http.StatusNoContent)

	lr := h.do(http.MethodGet, "/api/files/resources", nil)
	var remaining []resourceResp
	decode(t, lr, &remaining)
	if len(remaining) != 0 {
		t.Errorf("expected 0 resources after clear, got %d", len(remaining))
	}
}

func TestMissingKeepsTagsUntilCleared(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "tagged.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	h.do(http.MethodPost, path("/api/files/resources/", id)+"/tags", map[string]string{"tag": "important"}).Body.Close()

	os.Remove(f)
	after := h.scan(t, root.ID)
	if after[0].State != "missing" {
		t.Fatalf("expected missing, got %q", after[0].State)
	}
	if len(after[0].Tags) != 1 || after[0].Tags[0] != "important" {
		t.Errorf("tags lost on missing: %+v", after[0].Tags)
	}
}

func TestRegisterSingleFile(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "single.txt")
	writeFile(t, f, "solo")
	root := h.createRoot(t, dir)

	resp := h.do(http.MethodPost, "/api/files/resources/register", map[string]string{"path": f})
	mustStatus(t, resp, http.StatusCreated)
	var res resourceResp
	decode(t, resp, &res)
	if res.Name != "single.txt" || res.RootID != root.ID {
		t.Errorf("unexpected resource %+v", res)
	}
}

func TestRegisterOutsideRootRejected(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	h.createRoot(t, dir)
	other := t.TempDir()
	f := filepath.Join(other, "outside.txt")
	writeFile(t, f, "x")
	resp := h.do(http.MethodPost, "/api/files/resources/register", map[string]string{"path": f})
	mustStatus(t, resp, http.StatusBadRequest)
}

func TestBrowseByRootAndMetadata(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "doc.md"), "# title")
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)

	resp := h.do(http.MethodGet, path("/api/files/resources?root_id=", root.ID), nil)
	mustStatus(t, resp, http.StatusOK)
	var resources []resourceResp
	decode(t, resp, &resources)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	r := resources[0]
	if r.Name != "doc.md" || r.Size != 7 || r.ModifiedAt == nil {
		t.Errorf("metadata incomplete: %+v", r)
	}
}

func TestTagAddAndFilter(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.txt"), "x")
	writeFile(t, filepath.Join(dir, "y.txt"), "y")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)

	var xID int64
	for _, r := range resources {
		if r.Name == "x.txt" {
			xID = r.ID
		}
	}
	ar := h.do(http.MethodPost, path("/api/files/resources/", xID)+"/tags", map[string]string{"tag": "red"})
	mustStatus(t, ar, http.StatusOK)

	fr := h.do(http.MethodGet, "/api/files/resources?tag=red", nil)
	mustStatus(t, fr, http.StatusOK)
	var filtered []resourceResp
	decode(t, fr, &filtered)
	if len(filtered) != 1 || filtered[0].Name != "x.txt" {
		t.Errorf("tag filter wrong: %+v", filtered)
	}
}

func TestWriteNote(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "n.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	nr := h.do(http.MethodPut, path("/api/files/resources/", id)+"/note", map[string]string{"note": "context about this file"})
	mustStatus(t, nr, http.StatusOK)
	var res resourceResp
	decode(t, nr, &res)
	if res.Note != "context about this file" {
		t.Errorf("note = %q", res.Note)
	}

	// The underlying file is never altered by metadata.
	data, _ := os.ReadFile(f)
	if string(data) != "x" {
		t.Errorf("note write altered the file contents: %q", string(data))
	}
}

func TestDownloadResource(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "data.bin")
	writeFile(t, f, "payload-bytes")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	resp := h.do(http.MethodGet, path("/api/files/resources/", id)+"/download", nil)
	mustStatus(t, resp, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "payload-bytes" {
		t.Errorf("download body = %q", string(body))
	}
}
