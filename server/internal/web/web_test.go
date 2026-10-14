package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testFiles(t *testing.T) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{
		"index.html":              {Data: []byte("<html><body>Nexus</body></html>")},
		"assets/index-Abc123.js":  {Data: []byte(strings.Repeat("const nexus = 'workbench';\n", 100))},
		"assets/index-Abc123.css": {Data: []byte(strings.Repeat("body { color: black; }\n", 100))},
		"favicon.svg":             {Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")},
	}
	for name, file := range files {
		if strings.HasSuffix(name, ".gz") {
			continue
		}
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(file.Data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		files[name+".gz"] = &fstest.MapFile{Data: compressed.Bytes()}
	}
	return files
}

func serve(h http.Handler, method, path, encoding string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if encoding != "" {
		req.Header.Set("Accept-Encoding", encoding)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestStaticGzipNegotiation(t *testing.T) {
	files := testFiles(t)
	for _, path := range []string{"/assets/index-Abc123.js", "/assets/index-Abc123.css", "/favicon.svg", "/", "/index.html", "/files"} {
		t.Run(path, func(t *testing.T) {
			h := handler(files)
			plain := serve(h, http.MethodGet, path, "identity")
			compressed := serve(h, http.MethodGet, path, "gzip, deflate, br")
			if compressed.Code != http.StatusOK {
				t.Fatalf("status = %d", compressed.Code)
			}
			if got := compressed.Header().Get("Content-Encoding"); got != "gzip" {
				t.Fatalf("Content-Encoding = %q, want gzip", got)
			}
			if got := compressed.Header().Get("Content-Type"); got != plain.Header().Get("Content-Type") {
				t.Fatalf("compressed Content-Type = %q, plain = %q", got, plain.Header().Get("Content-Type"))
			}
			reader, err := gzip.NewReader(compressed.Body)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			body, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(body, plain.Body.Bytes()) {
				t.Fatal("gzip response changed the original content")
			}
			for _, rec := range []*httptest.ResponseRecorder{plain, compressed} {
				if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
					t.Fatal("all negotiated variants must vary by Accept-Encoding")
				}
			}
			head := serve(h, http.MethodHead, path, "gzip")
			if head.Body.Len() != 0 || head.Header().Get("Content-Encoding") != "gzip" {
				t.Fatal("HEAD must advertise gzip without a response body")
			}
		})
	}
}

func TestStaticEncodingExclusionsAndFallback(t *testing.T) {
	files := testFiles(t)
	h := handler(files)
	for _, encoding := range []string{"", "identity", "br", "gzip;q=0", "gzip;q=0, *;q=1", "*;q=0", "notgzip", "gzip;q=invalid"} {
		rec := serve(h, http.MethodGet, "/assets/index-Abc123.js", encoding)
		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Accept-Encoding %q: unexpected encoding %q", encoding, got)
		}
	}
	for _, encoding := range []string{"gzip;q=0.5", "GZIP", "*;q=1"} {
		rec := serve(h, http.MethodGet, "/assets/index-Abc123.js", encoding)
		if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
			t.Errorf("Accept-Encoding %q: encoding = %q, want gzip", encoding, got)
		}
	}
	delete(files, "assets/index-Abc123.js.gz")
	rec := serve(h, http.MethodGet, "/assets/index-Abc123.js", "gzip")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("a build without a gzip sibling must remain usable")
	}
}

func TestStaticCachePolicyAndMissingAssets(t *testing.T) {
	h := handler(testFiles(t))
	for _, path := range []string{"/assets/index-Abc123.js", "/assets/index-Abc123.css"} {
		rec := serve(h, http.MethodGet, path, "gzip")
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Errorf("%s: Cache-Control = %q", path, got)
		}
	}
	for _, path := range []string{"/", "/index.html", "/files", "/files/nested", "/favicon.svg"} {
		rec := serve(h, http.MethodGet, path, "gzip")
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", path, got)
		}
	}
	rec := serve(h, http.MethodGet, "/assets/old-build.js", "gzip")
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "<html>") {
		t.Fatal("a missing lazy chunk must return 404, not SPA HTML")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("missing asset Cache-Control = %q, want no-store", got)
	}
}

func TestStaticRangeUsesOriginalBytes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/assets/index-Abc123.js", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-9")
	rec := httptest.NewRecorder()
	handler(testFiles(t)).ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Header().Get("Content-Encoding") != "" {
		t.Fatal("byte ranges must refer to the original representation")
	}
	if got := rec.Body.String(); got != "const nexu" {
		t.Fatalf("range body = %q", got)
	}
}
