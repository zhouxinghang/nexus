package httpapi_test

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"nexus/internal/checker"
)

func (h *harness) getBookmark(id int64) bookmarkResp {
	h.t.Helper()
	for _, b := range h.listBookmarks() {
		if b.ID == id {
			return b
		}
	}
	h.t.Fatalf("bookmark %d not found", id)
	return bookmarkResp{}
}

// --- Auto-fetch title + favicon ---

func TestBookmarkBlankNameFilledFromFetchedTitle(t *testing.T) {
	h := newHarness(t)
	h.fetcher.SetResult("https://go.dev", checker.Meta{Title: "The Go Programming Language", FaviconURL: "https://go.dev/images/favicon.svg"})

	resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{"url": "go.dev"})
	mustStatus(t, resp, http.StatusCreated)
	var b bookmarkResp
	decode(t, resp, &b)
	if b.Name != "The Go Programming Language" {
		t.Errorf("name = %q, want fetched title", b.Name)
	}
	if b.Icon != "" {
		t.Errorf("owner icon was auto-populated: %q", b.Icon)
	}
	if b.FetchedTitle == nil || *b.FetchedTitle != "The Go Programming Language" {
		t.Errorf("fetched_title = %v", b.FetchedTitle)
	}
	if b.FetchedFaviconURL == nil || *b.FetchedFaviconURL != "https://go.dev/images/favicon.svg" {
		t.Errorf("fetched_favicon_url = %v", b.FetchedFaviconURL)
	}
}

func TestBookmarkBlankNameFallsBackToHost(t *testing.T) {
	h := newHarness(t)
	// Fetch fails outright…
	h.fetcher.SetError("https://down.example.com:8443/x", errors.New("connection refused"))
	resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{"url": "https://down.example.com:8443/x"})
	mustStatus(t, resp, http.StatusCreated)
	var b bookmarkResp
	decode(t, resp, &b)
	if b.Name != "down.example.com:8443" {
		t.Errorf("name = %q, want host fallback", b.Name)
	}
	if b.FetchedTitle != nil || b.FetchedFaviconURL != nil {
		t.Errorf("fetched fields set despite failed fetch: %+v", b)
	}

	// …or succeeds with an empty title.
	h.fetcher.SetResult("https://untitled.example.com", checker.Meta{FaviconURL: "https://untitled.example.com/favicon.ico"})
	resp = h.do(http.MethodPost, "/api/bookmarks", map[string]string{"url": "https://untitled.example.com"})
	mustStatus(t, resp, http.StatusCreated)
	decode(t, resp, &b)
	if b.Name != "untitled.example.com" {
		t.Errorf("name = %q, want host fallback", b.Name)
	}
	if b.FetchedFaviconURL == nil {
		t.Error("favicon from an untitled page should still be stored")
	}
}

func TestBookmarkOwnerFieldsWinOverFetch(t *testing.T) {
	h := newHarness(t)
	h.fetcher.SetResult("https://example.com", checker.Meta{Title: "Example Domain", FaviconURL: "https://example.com/favicon.ico"})

	resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{
		"name": "My Example",
		"url":  "https://example.com",
		"icon": "🧪",
	})
	mustStatus(t, resp, http.StatusCreated)
	var b bookmarkResp
	decode(t, resp, &b)
	h.server.WaitBackgroundForTest()

	got := h.getBookmark(b.ID)
	if got.Name != "My Example" || got.Icon != "🧪" {
		t.Errorf("owner fields overwritten: name=%q icon=%q", got.Name, got.Icon)
	}
	if got.FetchedTitle == nil || *got.FetchedTitle != "Example Domain" {
		t.Errorf("background fetch did not store fetched_title: %v", got.FetchedTitle)
	}
}

func TestBookmarkFetchErrorStillCreates(t *testing.T) {
	h := newHarness(t)
	h.fetcher.SetDefault(checker.Meta{}, errors.New("timeout"))
	b := h.createBookmark("Offline", "https://offline.example.com")
	h.server.WaitBackgroundForTest()
	got := h.getBookmark(b.ID)
	if got.Name != "Offline" || got.FetchedTitle != nil || got.FetchedFaviconURL != nil {
		t.Errorf("after failed background fetch: %+v", got)
	}
}

func TestBookmarkRefetchUpdatesOnlyFetchedFields(t *testing.T) {
	h := newHarness(t)
	h.fetcher.SetResult("https://site.example.com", checker.Meta{Title: "Old title", FaviconURL: "https://site.example.com/old.png"})
	resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{
		"name": "Mine", "url": "https://site.example.com", "icon": "⭐",
	})
	var b bookmarkResp
	decode(t, resp, &b)
	h.server.WaitBackgroundForTest()

	h.fetcher.SetResult("https://site.example.com", checker.Meta{Title: "New title", FaviconURL: "https://site.example.com/new.png"})
	r := h.do(http.MethodPost, path("/api/bookmarks/", b.ID)+"/refetch", nil)
	mustStatus(t, r, http.StatusOK)
	var updated bookmarkResp
	decode(t, r, &updated)
	if updated.Name != "Mine" || updated.Icon != "⭐" {
		t.Errorf("refetch touched owner fields: %+v", updated)
	}
	if updated.FetchedTitle == nil || *updated.FetchedTitle != "New title" ||
		updated.FetchedFaviconURL == nil || *updated.FetchedFaviconURL != "https://site.example.com/new.png" {
		t.Errorf("refetch result = %+v", updated)
	}

	// A failed refetch reports 502 and keeps the stored values.
	h.fetcher.SetError("https://site.example.com", errors.New("down"))
	r = h.do(http.MethodPost, path("/api/bookmarks/", b.ID)+"/refetch", nil)
	mustStatus(t, r, http.StatusBadGateway)
	r.Body.Close()
	if got := h.getBookmark(b.ID); got.FetchedTitle == nil || *got.FetchedTitle != "New title" {
		t.Errorf("failed refetch cleared fetched_title: %v", got.FetchedTitle)
	}

	mustStatus(t, h.do(http.MethodPost, "/api/bookmarks/999999/refetch", nil), http.StatusNotFound)
}

func TestBookmarkURLChangeRefetches(t *testing.T) {
	h := newHarness(t)
	h.fetcher.SetResult("https://a.example.com", checker.Meta{Title: "A"})
	h.fetcher.SetResult("https://b.example.com", checker.Meta{Title: "B"})
	b := h.createBookmark("Link", "https://a.example.com")
	h.server.WaitBackgroundForTest()

	r := h.do(http.MethodPatch, path("/api/bookmarks/", b.ID), map[string]string{"name": "Link", "url": "https://b.example.com"})
	mustStatus(t, r, http.StatusOK)
	r.Body.Close()
	h.server.WaitBackgroundForTest()
	if got := h.getBookmark(b.ID); got.FetchedTitle == nil || *got.FetchedTitle != "B" {
		t.Errorf("fetched_title after URL change = %v, want B", got.FetchedTitle)
	}
}

// --- Click count ---

func TestBookmarkClickIncrements(t *testing.T) {
	h := newHarness(t)
	b := h.createBookmark("Clicky", "https://clicky.example.com")
	if b.ClickCount != 0 || b.LastClickedAt != nil {
		t.Fatalf("fresh bookmark = %+v", b)
	}

	before := time.Now().Add(-time.Minute)
	for i := 0; i < 3; i++ {
		r := h.do(http.MethodPost, path("/api/bookmarks/", b.ID)+"/click", nil)
		mustStatus(t, r, http.StatusNoContent)
		r.Body.Close()
	}
	got := h.getBookmark(b.ID)
	if got.ClickCount != 3 {
		t.Errorf("click_count = %d, want 3", got.ClickCount)
	}
	if got.LastClickedAt == nil || got.LastClickedAt.Before(before) {
		t.Errorf("last_clicked_at = %v", got.LastClickedAt)
	}

	mustStatus(t, h.do(http.MethodPost, "/api/bookmarks/999999/click", nil), http.StatusNotFound)
}

func TestBookmarkSortByClicks(t *testing.T) {
	h := newHarness(t)
	a := h.createBookmark("A", "https://a.example.com")
	b := h.createBookmark("B", "https://b.example.com")
	c := h.createBookmark("C", "https://c.example.com")
	d := h.createBookmark("D", "https://d.example.com")
	click := func(id int64, n int) {
		for i := 0; i < n; i++ {
			r := h.do(http.MethodPost, path("/api/bookmarks/", id)+"/click", nil)
			mustStatus(t, r, http.StatusNoContent)
			r.Body.Close()
		}
	}
	// C and A tie on count; A was clicked more recently, so it wins the tie.
	click(c.ID, 2)
	click(b.ID, 3)
	click(a.ID, 2)

	r := h.do(http.MethodGet, "/api/bookmarks?sort=clicks", nil)
	mustStatus(t, r, http.StatusOK)
	var list []bookmarkResp
	decode(t, r, &list)
	got := []int64{list[0].ID, list[1].ID, list[2].ID, list[3].ID}
	want := []int64{b.ID, a.ID, c.ID, d.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sort=clicks order = %v, want %v", got, want)
		}
	}

	// The default (and explicit "created") order is unchanged.
	for _, q := range []string{"", "?sort=created"} {
		r := h.do(http.MethodGet, "/api/bookmarks"+q, nil)
		var list []bookmarkResp
		decode(t, r, &list)
		if list[0].ID != a.ID || list[3].ID != d.ID {
			t.Errorf("%q order changed: %v", q, list)
		}
	}

	mustStatus(t, h.do(http.MethodGet, "/api/bookmarks?sort=popular", nil), http.StatusBadRequest)
}
