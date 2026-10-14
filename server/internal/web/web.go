// Package web embeds the built React SPA so the single Go binary serves both
// the UI and the JSON API. The embedded files live under dist/ and are
// produced by `vite build`. A tiny placeholder exists so the embed compiles
// before the frontend is built.
package web

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded SPA and build-time gzip siblings. Unknown page
// paths fall back to index.html; missing assets remain 404s.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return handler(sub)
}

func handler(sub fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Set("Cache-Control", "no-cache")
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || name == "" || name == "index.html" {
			serveIndex(sub, w, r)
			return
		}
		info, err := fs.Stat(sub, name)
		asset := strings.HasPrefix(name, "assets/") || name == "assets"
		if err != nil || info.IsDir() {
			if asset {
				w.Header().Set("Cache-Control", "no-store")
				http.NotFound(w, r)
				return
			}
			serveIndex(sub, w, r)
			return
		}
		if asset {
			// Vite places content-hashed files under assets/. HTML must not
			// inherit this policy, or it could keep referencing old chunks.
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		served := encodedPath(sub, name, w, r)
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + served
		fileServer.ServeHTTP(w, r2)
	})
}

func serveIndex(sub fs.FS, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	name := encodedPath(sub, "index.html", w, r)
	data, err := fs.ReadFile(sub, name)
	if err != nil {
		http.Error(w, "ui not built", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func encodedPath(sub fs.FS, name string, w http.ResponseWriter, r *http.Request) string {
	// Range offsets must stay relative to the uncompressed representation.
	if r.Header.Get("Range") == "" && acceptsGzip(r.Header.Get("Accept-Encoding")) {
		if info, err := fs.Stat(sub, name+".gz"); err == nil && !info.IsDir() {
			w.Header().Set("Content-Encoding", "gzip")
			return name + ".gz"
		}
	}
	return name
}

func acceptsGzip(header string) bool {
	wildcard := false
	for _, token := range strings.Split(strings.ToLower(header), ",") {
		parts := strings.Split(token, ";")
		encoding := strings.TrimSpace(parts[0])
		if encoding != "gzip" && encoding != "*" {
			continue
		}
		quality := 1.0
		for _, parameter := range parts[1:] {
			if value, ok := strings.CutPrefix(strings.TrimSpace(parameter), "q="); ok {
				number, err := strconv.ParseFloat(value, 64)
				if err != nil || number < 0 || number > 1 {
					quality = 0
				} else {
					quality = number
				}
			}
		}
		if encoding == "gzip" {
			return quality > 0
		}
		wildcard = quality > 0
	}
	return wildcard
}
