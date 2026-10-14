package checker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Meta is the metadata extracted from a Bookmark's page: its <title> and the
// absolute URL of its declared (or conventional) favicon. Either may be empty.
type Meta struct {
	Title      string
	FaviconURL string
}

// Fetcher is the seam through which Nexus reads a site's title and favicon.
// A real implementation makes an HTTP request; a fake is injected in tests so
// no live network calls happen (mirroring Prober).
type Fetcher interface {
	FetchMeta(ctx context.Context, url string) (Meta, error)
}

// fetchTimeout bounds a single metadata fetch, matching the probe posture.
const fetchTimeout = probeTimeout

// maxHeadBytes caps how much of the page is read looking for <title> and
// <link rel=icon>; both live in <head>, near the top.
const maxHeadBytes = 512 << 10

// maxTitleRunes keeps an absurd <title> from becoming an absurd name.
const maxTitleRunes = 200

// ErrFetch is returned when the page could not be fetched at all (transport
// failure, timeout, or a non-2xx status). Callers treat it as "no metadata"
// and never let it block creating a Bookmark.
var ErrFetch = errors.New("could not fetch page")

// HTTPFetcher is the production Fetcher. It parses with the standard library
// (a small tolerant scanner over the page head), not a full HTML parser.
type HTTPFetcher struct {
	client *http.Client
}

// NewHTTPFetcher builds an HTTPFetcher that follows redirects with a bounded
// per-request timeout.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{client: &http.Client{Timeout: fetchTimeout}}
}

func (f *HTTPFetcher) FetchMeta(ctx context.Context, rawURL string) (Meta, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Meta{}, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	req.Header.Set("User-Agent", "Nexus-Bookmark-Fetcher/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")

	resp, err := f.client.Do(req)
	if err != nil {
		return Meta{}, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Meta{}, fmt.Errorf("%w: status %d", ErrFetch, resp.StatusCode)
	}

	head, err := io.ReadAll(io.LimitReader(resp.Body, maxHeadBytes))
	if err != nil && len(head) == 0 {
		return Meta{}, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	if !isHTML(resp.Header.Get("Content-Type"), head) {
		// Reachable but not a page (an image, a PDF, JSON…): nothing to read.
		return Meta{}, nil
	}
	// Resolve relative hrefs against where the redirects actually landed.
	return ParseMeta(head, resp.Request.URL), nil
}

var _ Fetcher = (*HTTPFetcher)(nil)

func isHTML(contentType string, head []byte) bool {
	if contentType == "" {
		contentType = http.DetectContentType(head)
	}
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "text/html" || mt == "application/xhtml+xml"
}

var (
	titleRe = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title\s*>`)
	linkRe  = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	attrRe  = regexp.MustCompile(`(?is)([a-z][a-z0-9_:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+))`)
	spaceRe = regexp.MustCompile(`\s+`)
	// commentRe strips HTML comments so a commented-out <title> or <link>
	// is not picked up.
	commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// ParseMeta extracts the title and best favicon from a page head. base is
// the page's final URL; the favicon falls back to <origin>/favicon.ico when
// the page declares none. A title that isn't valid UTF-8 (e.g. a GBK page)
// is dropped rather than stored garbled.
func ParseMeta(head []byte, base *url.URL) Meta {
	doc := commentRe.ReplaceAll(head, nil)
	var m Meta

	if t := titleRe.FindSubmatch(doc); t != nil && utf8.Valid(t[1]) {
		title := html.UnescapeString(string(t[1]))
		title = strings.TrimSpace(spaceRe.ReplaceAllString(title, " "))
		if utf8.RuneCountInString(title) > maxTitleRunes {
			title = string([]rune(title)[:maxTitleRunes])
		}
		m.Title = title
	}

	m.FaviconURL = bestIcon(doc, base)
	if m.FaviconURL == "" && base != nil {
		m.FaviconURL = (&url.URL{Scheme: base.Scheme, Host: base.Host, Path: "/favicon.ico"}).String()
	}
	return m
}

// bestIcon picks the first rel="icon" / "shortcut icon" link, falling back to
// the first apple-touch-icon. Only http(s) results are kept (no data: URIs —
// we store a URL, never icon bytes).
func bestIcon(doc []byte, base *url.URL) string {
	bestRank := 99
	best := ""
	for _, tag := range linkRe.FindAll(doc, -1) {
		attrs := parseAttrs(tag)
		href := strings.TrimSpace(attrs["href"])
		if href == "" {
			continue
		}
		rank := iconRank(attrs["rel"])
		if rank >= bestRank {
			continue
		}
		abs := resolve(base, href)
		if abs == "" {
			continue
		}
		bestRank, best = rank, abs
	}
	return best
}

func iconRank(rel string) int {
	rank := 99
	for _, tok := range strings.Fields(strings.ToLower(rel)) {
		switch tok {
		case "icon":
			return 0
		case "apple-touch-icon", "apple-touch-icon-precomposed":
			rank = 1
		}
	}
	return rank
}

func parseAttrs(tag []byte) map[string]string {
	out := map[string]string{}
	// Skip the tag name so "<link" isn't read as an attribute.
	if i := bytes.IndexAny(tag, " \t\r\n/"); i >= 0 {
		tag = tag[i:]
	}
	for _, m := range attrRe.FindAllSubmatch(tag, -1) {
		name := strings.ToLower(string(m[1]))
		if _, seen := out[name]; seen {
			continue
		}
		val := m[2]
		if val == nil {
			val = m[3]
		}
		if val == nil {
			val = m[4]
		}
		out[name] = html.UnescapeString(string(val))
	}
	return out
}

func resolve(base *url.URL, href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	abs := ref
	if base != nil {
		abs = base.ResolveReference(ref)
	}
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	return abs.String()
}
