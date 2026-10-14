package checker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestParseMeta(t *testing.T) {
	base := mustURL(t, "https://example.com/docs/page")
	cases := []struct {
		name, html, title, icon string
	}{
		{
			name:  "title and relative icon",
			html:  `<html><head><title> Example &amp; Co </title><link rel="icon" href="/static/fav.png"></head>`,
			title: "Example & Co",
			icon:  "https://example.com/static/fav.png",
		},
		{
			name:  "shortcut icon, single quotes, unquoted rel order",
			html:  `<LINK href='img/i.ico' REL='shortcut icon'><TITLE>Upper</TITLE>`,
			title: "Upper",
			icon:  "https://example.com/docs/img/i.ico",
		},
		{
			name:  "icon preferred over apple-touch-icon",
			html:  `<link rel="apple-touch-icon" href="/apple.png"><link rel="icon" href="/fav.svg"><title>x</title>`,
			title: "x",
			icon:  "https://example.com/fav.svg",
		},
		{
			name: "apple-touch-icon when no icon",
			html: `<link rel="apple-touch-icon" href="https://cdn.example.net/a.png">`,
			icon: "https://cdn.example.net/a.png",
		},
		{
			name:  "fallback favicon.ico, whitespace collapsed",
			html:  "<title>\n  Multi\n  line\n</title>",
			title: "Multi line",
			icon:  "https://example.com/favicon.ico",
		},
		{
			name: "commented-out tags ignored, data URI skipped",
			html: `<!-- <title>Old</title><link rel="icon" href="/old.ico"> -->` +
				`<link rel="icon" href="data:image/png;base64,AAAA"><title>New</title>`,
			title: "New",
			icon:  "https://example.com/favicon.ico",
		},
		{
			name:  "CJK title",
			html:  `<title>个人工作台</title>`,
			title: "个人工作台",
			icon:  "https://example.com/favicon.ico",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := ParseMeta([]byte(c.html), base)
			if m.Title != c.title || m.FaviconURL != c.icon {
				t.Errorf("got {%q %q}, want {%q %q}", m.Title, m.FaviconURL, c.title, c.icon)
			}
		})
	}
}

func TestParseMetaDropsNonUTF8Title(t *testing.T) {
	// GBK-encoded "中文" — not valid UTF-8.
	m := ParseMeta([]byte("<title>\xd6\xd0\xce\xc4</title>"), mustURL(t, "https://cn.example.com"))
	if m.Title != "" {
		t.Errorf("non-UTF-8 title kept: %q", m.Title)
	}
}

func TestHTTPFetcher(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Hello</title><link rel="icon" href="fav.png"></head></html>`))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/sub/page", http.StatusFound)
	})
	mux.HandleFunc("/sub/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<title>Landed</title><link rel="icon" href="i.ico">`))
	})
	mux.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG"))
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := NewHTTPFetcher()
	ctx := context.Background()

	m, err := f.FetchMeta(ctx, srv.URL+"/page")
	if err != nil || m.Title != "Hello" || m.FaviconURL != srv.URL+"/fav.png" {
		t.Errorf("page: %+v, %v", m, err)
	}

	// Relative icons resolve against the post-redirect URL.
	m, err = f.FetchMeta(ctx, srv.URL+"/moved")
	if err != nil || m.Title != "Landed" || m.FaviconURL != srv.URL+"/sub/i.ico" {
		t.Errorf("redirect: %+v, %v", m, err)
	}

	m, err = f.FetchMeta(ctx, srv.URL+"/image")
	if err != nil || m != (Meta{}) {
		t.Errorf("non-HTML: %+v, %v (want empty, nil)", m, err)
	}

	if _, err := f.FetchMeta(ctx, srv.URL+"/missing"); !errors.Is(err, ErrFetch) {
		t.Errorf("404: err = %v, want ErrFetch", err)
	}
}
