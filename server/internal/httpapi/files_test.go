package httpapi_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type resourceResp struct {
	ID            int64      `json:"id"`
	RootID        int64      `json:"root_id"`
	Path          string     `json:"path"`
	Name          string     `json:"name"`
	Size          int64      `json:"size"`
	IsDir         bool       `json:"is_dir"`
	ModifiedAt    *time.Time `json:"modified_at"`
	FileCreatedAt *time.Time `json:"file_created_at"`
	State         string     `json:"state"`
	Kind          string     `json:"kind"`
	Tags          []string   `json:"tags"`
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

// scan starts a background scan and blocks until it finishes, then returns the
// root's indexed Resources. Scanning is asynchronous (POST 202 + poll), so the
// helper hides the polling the real UI does.
func (h *harness) scan(t *testing.T, rootID int64) []resourceResp {
	t.Helper()
	h.scanAndWait(t, rootID)
	resp := h.do(http.MethodGet, fmt.Sprintf("/api/files/resources?root_id=%d&limit=1000", rootID), nil)
	mustStatus(t, resp, http.StatusOK)
	var resources []resourceResp
	decodeItems(t, resp, &resources)
	return resources
}

// scanAndWait POSTs a scan then polls the status endpoint until the scan leaves
// the running phase, returning the terminal status. It fails the test on a
// failed scan or if the scan doesn't settle within a generous timeout.
func (h *harness) scanAndWait(t *testing.T, rootID int64) scanStatusResp {
	t.Helper()
	start := h.do(http.MethodPost, path("/api/files/roots/", rootID)+"/scan", nil)
	mustStatus(t, start, http.StatusAccepted)
	start.Body.Close()

	deadline := time.Now().Add(30 * time.Second)
	for {
		resp := h.do(http.MethodGet, path("/api/files/roots/", rootID)+"/scan", nil)
		mustStatus(t, resp, http.StatusOK)
		var st scanStatusResp
		decode(t, resp, &st)
		switch st.Phase {
		case "completed":
			return st
		case "failed":
			t.Fatalf("scan failed: %s", st.Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("scan did not complete within timeout (phase=%s)", st.Phase)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// scanStatusResp mirrors the server's scan status envelope.
type scanStatusResp struct {
	RootID  int64  `json:"root_id"`
	Phase   string `json:"phase"`
	Total   int    `json:"total"`
	Indexed int    `json:"indexed"`
	Removed int    `json:"removed"`
	Error   string `json:"error"`
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
	decodeItems(t, lr, &remaining)
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
	decodeItems(t, resp, &resources)
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
	decodeItems(t, fr, &filtered)
	if len(filtered) != 1 || filtered[0].Name != "x.txt" {
		t.Errorf("tag filter wrong: %+v", filtered)
	}
}

func TestNotesRemovedAndTagsDoNotAlterContent(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "n.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	nr := h.do(http.MethodPut, path("/api/files/resources/", id)+"/note", map[string]string{"note": "context about this file"})
	mustStatus(t, nr, http.StatusNotFound)
	nr.Body.Close()
	tr := h.do(http.MethodPost, path("/api/files/resources/", id)+"/tags", map[string]string{"tag": "context"})
	mustStatus(t, tr, http.StatusOK)
	var res map[string]any
	decode(t, tr, &res)
	if _, exists := res["note"]; exists {
		t.Fatal("Resource response still exposes note")
	}

	// The underlying file is never altered by metadata.
	data, _ := os.ReadFile(f)
	if string(data) != "x" {
		t.Errorf("tag write altered the file contents: %q", string(data))
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

// resourcePage is the offset envelope returned by GET /api/files/resources.
type resourcePage struct {
	Items       []resourceResp   `json:"items"`
	Total       int64            `json:"total"`
	Limit       int32            `json:"limit"`
	Offset      int32            `json:"offset"`
	Kinds       map[string]int64 `json:"kinds"`
	Directories []directoryResp  `json:"directories"`
}

func (h *harness) resourcePage(query string) resourcePage {
	h.t.Helper()
	r := h.do(http.MethodGet, "/api/files/resources"+query, nil)
	mustStatus(h.t, r, http.StatusOK)
	var p resourcePage
	decode(h.t, r, &p)
	return p
}

func TestResourceOffsetPagination(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	for i := 0; i < 7; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), "x")
	}
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)

	var got []string
	for offset := 0; ; offset += 3 {
		p := h.resourcePage(fmt.Sprintf("?limit=3&offset=%d", offset))
		if p.Total != 7 || p.Limit != 3 || p.Offset != int32(offset) {
			t.Fatalf("envelope = total %d limit %d offset %d", p.Total, p.Limit, p.Offset)
		}
		for _, r := range p.Items {
			got = append(got, r.Name)
		}
		if offset+len(p.Items) >= int(p.Total) {
			if len(p.Items) != 1 {
				t.Errorf("final page has %d items, want 1", len(p.Items))
			}
			break
		}
		if len(p.Items) != 3 {
			t.Fatalf("page at offset %d has %d items", offset, len(p.Items))
		}
	}
	if len(got) != 7 {
		t.Fatalf("walked %d resources, want 7", len(got))
	}
	for i, name := range got {
		if want := fmt.Sprintf("f%02d.txt", i); name != want {
			t.Errorf("position %d = %s, want %s (path ASC)", i, name, want)
		}
	}

	// Past the end: empty items, same total.
	end := h.resourcePage("?limit=3&offset=30")
	if len(end.Items) != 0 || end.Total != 7 {
		t.Errorf("past-the-end page = %+v", end)
	}
}

func TestResourceTagFilterCountedInSQL(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("top%d.txt", i)), "x")
		writeFile(t, filepath.Join(sub, fmt.Sprintf("deep%d.txt", i)), "x")
	}
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)

	// Tag 3 top-level files and 2 nested ones.
	tagged := 0
	for _, r := range resources {
		if (strings.HasPrefix(r.Name, "top") && tagged < 3) ||
			(r.Name == "deep0.txt" || r.Name == "deep1.txt") {
			ar := h.do(http.MethodPost, path("/api/files/resources/", r.ID)+"/tags", map[string]string{"tag": "keep"})
			mustStatus(t, ar, http.StatusOK)
			ar.Body.Close()
			if strings.HasPrefix(r.Name, "top") {
				tagged++
			}
		}
	}

	p := h.resourcePage("?tag=keep&limit=2")
	if p.Total != 5 || len(p.Items) != 2 {
		t.Errorf("tag page = total %d, %d items; want 5, 2", p.Total, len(p.Items))
	}
	for _, r := range p.Items {
		if len(r.Tags) != 1 || r.Tags[0] != "keep" {
			t.Errorf("untagged resource in tag page: %+v", r)
		}
	}

	// Tag + directory prefix + root together.
	q := fmt.Sprintf("?tag=keep&root_id=%d&dir=%s", root.ID, url.QueryEscape(sub+"/"))
	p = h.resourcePage(q)
	if p.Total != 2 || len(p.Items) != 2 {
		t.Errorf("tag+dir page = total %d, %d items; want 2, 2", p.Total, len(p.Items))
	}

	// A directory prefix containing LIKE wildcards matches literally.
	p = h.resourcePage("?dir=" + url.QueryEscape(dir+"/%"))
	if p.Total != 0 {
		t.Errorf("wildcard dir matched %d resources, want 0", p.Total)
	}
}

func TestResourcePagingRejectsBadParams(t *testing.T) {
	h := newHarness(t)
	for _, q := range []string{"?limit=0", "?limit=x", "?offset=-1", "?offset=y"} {
		r := h.do(http.MethodGet, "/api/files/resources"+q, nil)
		mustStatus(t, r, http.StatusBadRequest)
		r.Body.Close()
	}
}
