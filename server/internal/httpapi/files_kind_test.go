package httpapi_test

import (
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
)

// A sampling of the Files taxonomy: name -> expected server-classified kind.
// Covers extension, extension-less code names, and the
// multi-dot archive case, all of which the resource_kind SQL function handles.
var kindFixtures = map[string]string{
	"report.PDF":       "document",
	"photo.jpeg":       "image",
	"clip.mkv":         "video",
	"song.flac":        "audio",
	"sheet.csv":        "spreadsheet",
	"deck.pptx":        "presentation",
	"main.go":          "code",
	"Dockerfile":       "code",
	"bundle.tar.gz":    "archive",
	"README":           "other",
	"weird.unknownext": "other",
}

func TestResourceKindClassifiedInSQL(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	for name := range kindFixtures {
		writeFile(t, filepath.Join(dir, name), "x")
	}
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)

	got := map[string]string{}
	for _, r := range resources {
		got[r.Name] = r.Kind
	}
	for name, want := range kindFixtures {
		if got[name] != want {
			t.Errorf("kind(%q) = %q, want %q", name, got[name], want)
		}
	}
}

func TestResourceKindFilterAndCounts(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	// Two images, one document, one code file, plus the scan-created dir.
	writeFile(t, filepath.Join(dir, "a.png"), "x")
	writeFile(t, filepath.Join(dir, "b.jpg"), "x")
	writeFile(t, filepath.Join(dir, "notes.md"), "x")
	writeFile(t, filepath.Join(dir, "sub", "main.go"), "x")
	root := h.createRoot(t, dir)
	h.scan(t, root.ID)

	// Per-kind counts in the envelope reflect the whole result set, with no
	// kind filter applied. The scan indexes only files (no "sub" folder row).
	all := h.resourcePage("")
	if all.Total != 4 {
		t.Fatalf("total = %d, want 4", all.Total)
	}
	wantCounts := map[string]int64{"image": 2, "document": 1, "code": 1}
	for kind, want := range wantCounts {
		if all.Kinds[kind] != want {
			t.Errorf("kinds[%q] = %d, want %d", kind, all.Kinds[kind], want)
		}
	}

	// Filtering by kind slices the list and total in SQL, but the per-kind
	// counts stay full so the chips remain visible while a kind is selected.
	imgs := h.resourcePage("?kind=image")
	if imgs.Total != 2 || len(imgs.Items) != 2 {
		t.Fatalf("image page = total %d, %d items; want 2, 2", imgs.Total, len(imgs.Items))
	}
	for _, r := range imgs.Items {
		if r.Kind != "image" {
			t.Errorf("non-image %q in image page (kind=%q)", r.Name, r.Kind)
		}
	}
	if imgs.Kinds["document"] != 1 || imgs.Kinds["code"] != 1 {
		t.Errorf("kind counts changed under filter: %+v", imgs.Kinds)
	}

	// Kind composes with the directory-prefix filter.
	codeInSub := h.resourcePage("?kind=code&dir=" + url.QueryEscape(filepath.Join(dir, "sub")+"/"))
	if codeInSub.Total != 1 || len(codeInSub.Items) != 1 || codeInSub.Items[0].Name != "main.go" {
		t.Errorf("code+dir page = %+v", codeInSub)
	}

	// An unknown kind value is rejected.
	bad := h.do(http.MethodGet, "/api/files/resources?kind=bogus", nil)
	mustStatus(t, bad, http.StatusBadRequest)
	bad.Body.Close()
}
