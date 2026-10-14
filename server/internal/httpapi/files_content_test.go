package httpapi_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type previewResp struct {
	Type     string `json:"type"`
	Mime     string `json:"mime"`
	Size     int64  `json:"size"`
	Editable bool   `json:"editable"`
	Reason   string `json:"reason"`
}
type textContentResp struct {
	Content  string `json:"content"`
	Revision string `json:"revision"`
	Encoding string `json:"encoding"`
}

func TestResourcePreviewTypes(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	files := []struct {
		name, data, kind string
		editable         bool
	}{
		{"readme.md", "# Hello\n你好\n", "text", true},
		{"source.ts", "const greeting: string = '你好';\n", "text", true},
		{"long.ts", strings.Repeat(" ", 511) + "// 你好\n", "text", true},
		{"Dockerfile", "FROM alpine\n", "text", true},
		{"empty.txt", "", "text", true},
		{"index.html", "<script>window.attack = true</script>", "text", true},
		{"vector.svg", "<svg><script>bad()</script></svg>", "text", true},
		{"photo.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "image", false},
		{"movie.mp4", "\x00\x00\x00\x18ftypmp42", "video", false},
		{"song.mp3", "ID3\x00\x01\x02", "audio", false},
		{"paper.pdf", "%PDF-1.4\n1 0 obj\n", "pdf", false},
		{"archive.zip", "PK\x03\x04\x00\x00\x00\x00", "unsupported", false},
		{"utf16.txt", "\xff\xfeh\x00i\x00", "unsupported", false},
		{"disguised.txt", strings.Repeat("a", 600) + "\x00", "unsupported", false},
		{"too-large.txt", strings.Repeat("x", (2<<20)+1), "unsupported", false},
	}
	for _, f := range files {
		writeFile(t, filepath.Join(dir, f.name), f.data)
	}
	root := h.createRoot(t, dir)
	indexed := h.scan(t, root.ID)
	byName := map[string]int64{}
	for _, res := range indexed {
		byName[res.Name] = res.ID
	}
	for _, file := range files {
		t.Run(file.name, func(t *testing.T) {
			resp := h.do(http.MethodGet, path("/api/files/resources/", byName[file.name])+"/preview", nil)
			mustStatus(t, resp, http.StatusOK)
			if resp.Header.Get("Cache-Control") != "no-store" {
				t.Error("preview cached")
			}
			var p previewResp
			decode(t, resp, &p)
			if p.Type != file.kind || p.Editable != file.editable {
				t.Fatalf("preview = %+v, want %s editable=%v", p, file.kind, file.editable)
			}
			resp = h.do(http.MethodGet, path("/api/files/resources/", byName[file.name])+"/content", nil)
			if file.kind == "text" {
				mustStatus(t, resp, http.StatusOK)
				if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
					t.Fatal("HTML/text must not be served as executable content")
				}
				var content textContentResp
				decode(t, resp, &content)
				if content.Content != file.data || len(content.Revision) != 64 || content.Encoding != "UTF-8" {
					t.Fatalf("content = %+v", content)
				}
			} else if file.kind == "unsupported" {
				mustStatus(t, resp, http.StatusUnsupportedMediaType)
				resp.Body.Close()
			} else {
				mustStatus(t, resp, http.StatusOK)
				if !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "inline") || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
					t.Error("unsafe media headers")
				}
				data, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if !bytes.Equal(data, []byte(file.data)) {
					t.Fatalf("media bytes changed: %q", data)
				}
			}
		})
	}
}

func TestResourceTextSaveAndConflict(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	writeFile(t, file, "\uFEFForiginal\r\n你好\r\n")
	root := h.createRoot(t, dir)
	id := h.scan(t, root.ID)[0].ID
	base := path("/api/files/resources/", id)
	h.do(http.MethodPost, base+"/tags", map[string]string{"tag": "keep"}).Body.Close()
	var original textContentResp
	decode(t, h.do(http.MethodGet, base+"/content", nil), &original)
	body := "\uFEFFupdated\r\n世界\r\n"
	resp := h.do(http.MethodPut, base+"/content", map[string]string{"content": body, "revision": original.Revision})
	mustStatus(t, resp, http.StatusOK)
	var saved textContentResp
	decode(t, resp, &saved)
	if saved.Content != body || saved.Revision == original.Revision {
		t.Fatalf("saved = %+v", saved)
	}
	data, _ := os.ReadFile(file)
	if string(data) != body {
		t.Fatalf("disk bytes = %q", data)
	}
	var res resourceResp
	decode(t, h.do(http.MethodGet, base, nil), &res)
	if res.ID != id || res.Size != int64(len([]byte(body))) || len(res.Tags) != 1 {
		t.Fatalf("index metadata lost/stale: %+v", res)
	}
	var log []auditResp
	decode(t, h.do(http.MethodGet, "/api/files/audit", nil), &log)
	if len(log) != 1 || log[0].Action != "edit" || log[0].OldPath != file || log[0].NewPath != file {
		t.Fatalf("edit audit = %+v", log)
	}
	resp = h.do(http.MethodPut, base+"/content", map[string]string{"content": "stale draft", "revision": original.Revision})
	mustStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
	data, _ = os.ReadFile(file)
	if string(data) != body {
		t.Fatal("stale editor overwrote current content")
	}
	// External changes of the same length and timestamp must also conflict.
	stat, _ := os.Stat(file)
	external := strings.Replace(body, "updated", "outside", 1)
	os.WriteFile(file, []byte(external), 0o644)
	os.Chtimes(file, stat.ModTime(), stat.ModTime())
	resp = h.do(http.MethodPut, base+"/content", map[string]string{"content": "my draft", "revision": saved.Revision})
	mustStatus(t, resp, http.StatusConflict)
	resp.Body.Close()
	// The latest version can deliberately be saved as an empty file.
	decode(t, h.do(http.MethodGet, base+"/content", nil), &saved)
	resp = h.do(http.MethodPut, base+"/content", map[string]string{"content": "", "revision": saved.Revision})
	mustStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	data, _ = os.ReadFile(file)
	if len(data) != 0 {
		t.Fatal("empty content was not saved")
	}
}

func TestResourceContentRange(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	data := "%PDF-1.4\n" + strings.Repeat("a", 1000)
	writeFile(t, filepath.Join(dir, "a.pdf"), data)
	root := h.createRoot(t, dir)
	id := h.scan(t, root.ID)[0].ID
	req, _ := http.NewRequest(http.MethodGet, h.ts.URL+path("/api/files/resources/", id)+"/content", nil)
	req.AddCookie(h.cookie)
	req.Header.Set("Range", "bytes=9-18")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	mustStatus(t, resp, http.StatusPartialContent)
	got, _ := io.ReadAll(resp.Body)
	if string(got) != data[9:19] || resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("range = %q headers=%v", got, resp.Header)
	}
}

func TestResourceContentRejections(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	writeFile(t, file, "original")
	root := h.createRoot(t, dir)
	id := h.scan(t, root.ID)[0].ID
	base := path("/api/files/resources/", id)
	var content textContentResp
	decode(t, h.do(http.MethodGet, base+"/content", nil), &content)
	for name, body := range map[string]any{
		"missing revision": map[string]string{"content": "x"},
		"missing content":  map[string]string{"revision": content.Revision},
		"invalid revision": map[string]string{"content": "x", "revision": strings.Repeat("g", 64)},
		"unknown field":    map[string]string{"content": "x", "revision": content.Revision, "force": "true"},
	} {
		t.Run(name, func(t *testing.T) {
			resp := h.do(http.MethodPut, base+"/content", body)
			mustStatus(t, resp, http.StatusBadRequest)
			resp.Body.Close()
		})
	}
	resp := h.do(http.MethodPut, base+"/content", map[string]string{"content": "binary\x00", "revision": content.Revision})
	mustStatus(t, resp, http.StatusUnsupportedMediaType)
	resp.Body.Close()
	resp = h.do(http.MethodPut, base+"/content", map[string]string{"content": strings.Repeat("x", (2<<20)+1), "revision": content.Revision})
	mustStatus(t, resp, http.StatusRequestEntityTooLarge)
	resp.Body.Close()
	resp = h.doWithCookie(http.MethodGet, base+"/content", nil, nil)
	mustStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
	resp = h.doWithCookie(http.MethodPut, base+"/content", map[string]string{"content": "x", "revision": content.Revision}, nil)
	mustStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
	// Resolve disk state on every access, not just indexed state.
	os.Remove(file)
	for _, suffix := range []string{"/preview", "/content", "/download"} {
		resp = h.do(http.MethodGet, base+suffix, nil)
		mustStatus(t, resp, http.StatusNotFound)
		resp.Body.Close()
	}
	resp = h.do(http.MethodPut, base+"/content", map[string]string{"content": "x", "revision": content.Revision})
	mustStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("save recreated a vanished file")
	}
}

func TestResourceContentSymlinkEscape(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	writeFile(t, file, "original")
	root := h.createRoot(t, dir)
	id := h.scan(t, root.ID)[0].ID
	base := path("/api/files/resources/", id)
	var content textContentResp
	decode(t, h.do(http.MethodGet, base+"/content", nil), &content)
	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, outside, "secret")
	os.Remove(file)
	os.Symlink(outside, file)
	for _, suffix := range []string{"/preview", "/content", "/download"} {
		resp := h.do(http.MethodGet, base+suffix, nil)
		mustStatus(t, resp, http.StatusForbidden)
		resp.Body.Close()
	}
	resp := h.do(http.MethodPut, base+"/content", map[string]string{"content": "bad", "revision": content.Revision})
	mustStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()
	data, _ := os.ReadFile(outside)
	if string(data) != "secret" {
		t.Fatal("out-of-root target modified")
	}
}

func TestResourceContentConcurrentSaves(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "original")
	root := h.createRoot(t, dir)
	id := h.scan(t, root.ID)[0].ID
	base := path("/api/files/resources/", id)
	var content textContentResp
	decode(t, h.do(http.MethodGet, base+"/content", nil), &content)
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for _, draft := range []string{"first", "second"} {
		wg.Add(1)
		go func(draft string) {
			defer wg.Done()
			resp := h.do(http.MethodPut, base+"/content", map[string]string{"content": draft, "revision": content.Revision})
			statuses <- resp.StatusCode
			resp.Body.Close()
		}(draft)
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("concurrent saves = %v", counts)
	}
}
