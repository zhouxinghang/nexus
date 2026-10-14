package httpapi_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"nexus/internal/checker"
)

type bookmarkResp struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Icon          string     `json:"icon"`
	Health        string     `json:"health"`
	LastStatus    *int32     `json:"last_status"`
	LastLatencyMs *int64     `json:"last_latency_ms"`
	LastCheckedAt *time.Time `json:"last_checked_at"`

	FetchedTitle      *string    `json:"fetched_title"`
	FetchedFaviconURL *string    `json:"fetched_favicon_url"`
	ClickCount        int64      `json:"click_count"`
	LastClickedAt     *time.Time `json:"last_clicked_at"`
}

func TestBookmarkCreateWithNameIconLink(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{
		"name": "Example",
		"url":  "https://example.com",
		"icon": "🔖",
	})
	mustStatus(t, resp, http.StatusCreated)
	var b bookmarkResp
	decode(t, resp, &b)
	if b.Name != "Example" {
		t.Errorf("name = %q", b.Name)
	}
	if b.URL != "https://example.com" {
		t.Errorf("url = %q", b.URL)
	}
	if b.Icon != "🔖" {
		t.Errorf("icon = %q", b.Icon)
	}
	if b.Health != "unknown" {
		t.Errorf("initial health = %q, want unknown", b.Health)
	}
}

func TestBookmarkNormalizesBareHost(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{
		"name": "No scheme",
		"url":  "example.org/path",
	})
	mustStatus(t, resp, http.StatusCreated)
	var b bookmarkResp
	decode(t, resp, &b)
	if b.URL != "https://example.org/path" {
		t.Errorf("url = %q, want https:// prepended", b.URL)
	}
}

func TestBookmarkEditRejectsEmptyName(t *testing.T) {
	h := newHarness(t)
	b := h.createBookmark("Named", "https://example.com")
	resp := h.do(http.MethodPatch, path("/api/bookmarks/", b.ID), map[string]string{
		"name": "   ",
		"url":  "https://example.com",
	})
	mustStatus(t, resp, http.StatusBadRequest)
}

func TestBookmarkRejectsInvalidURL(t *testing.T) {
	h := newHarness(t)
	for _, bad := range []string{"", "ftp://example.com", "   "} {
		resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{
			"name": "bad",
			"url":  bad,
		})
		mustStatus(t, resp, http.StatusBadRequest)
	}
}

func TestBookmarkListOrdering(t *testing.T) {
	h := newHarness(t)
	for _, n := range []string{"first", "second", "third"} {
		resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{
			"name": n,
			"url":  "https://" + n + ".example.com",
		})
		mustStatus(t, resp, http.StatusCreated)
	}
	resp := h.do(http.MethodGet, "/api/bookmarks", nil)
	mustStatus(t, resp, http.StatusOK)
	var list []bookmarkResp
	decode(t, resp, &list)
	if len(list) != 3 {
		t.Fatalf("expected 3 bookmarks, got %d", len(list))
	}
	if list[0].Name != "first" || list[2].Name != "third" {
		t.Errorf("expected creation order [first .. third], got [%s .. %s]", list[0].Name, list[2].Name)
	}
}

func TestBookmarkEditAndDelete(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{
		"name": "typpo",
		"url":  "https://old.example.com",
	})
	var b bookmarkResp
	decode(t, resp, &b)

	er := h.do(http.MethodPatch, path("/api/bookmarks/", b.ID), map[string]string{
		"name": "fixed",
		"url":  "https://new.example.com",
		"icon": "⭐",
	})
	mustStatus(t, er, http.StatusOK)
	var updated bookmarkResp
	decode(t, er, &updated)
	if updated.Name != "fixed" || updated.URL != "https://new.example.com" || updated.Icon != "⭐" {
		t.Errorf("edit result = %+v", updated)
	}

	dr := h.do(http.MethodDelete, path("/api/bookmarks/", b.ID), nil)
	mustStatus(t, dr, http.StatusNoContent)

	lr := h.do(http.MethodGet, "/api/bookmarks", nil)
	var list []bookmarkResp
	decode(t, lr, &list)
	for _, x := range list {
		if x.ID == b.ID {
			t.Error("deleted bookmark still present in list")
		}
	}
}

func TestBookmarkHealthCheckMarksUpAndDown(t *testing.T) {
	h := newHarness(t)

	// A reachable site and an unreachable one, pinned on the fake prober.
	h.prober.SetResult("https://up.example.com", checker.ProbeResult{Up: true, Status: 200, Latency: 5 * time.Millisecond})
	h.prober.SetResult("https://down.example.com", checker.ProbeResult{Up: false, Status: 0})

	up := h.createBookmark("Up site", "https://up.example.com")
	down := h.createBookmark("Down site", "https://down.example.com")

	if err := h.server.CheckBookmarksForTest(context.Background()); err != nil {
		t.Fatalf("check all: %v", err)
	}

	list := h.listBookmarks()
	got := map[int64]bookmarkResp{}
	for _, b := range list {
		got[b.ID] = b
	}
	if got[up.ID].Health != "up" {
		t.Errorf("up site health = %q, want up", got[up.ID].Health)
	}
	if got[up.ID].LastStatus == nil || *got[up.ID].LastStatus != 200 {
		t.Errorf("up site last_status = %v, want 200", got[up.ID].LastStatus)
	}
	if got[up.ID].LastCheckedAt == nil {
		t.Error("up site last_checked_at should be set")
	}
	if got[down.ID].Health != "down" {
		t.Errorf("down site health = %q, want down", got[down.ID].Health)
	}
}

func TestBookmarkManualRecheck(t *testing.T) {
	h := newHarness(t)
	h.prober.SetDefault(checker.ProbeResult{Up: false, Status: 503, Latency: time.Millisecond})
	b := h.createBookmark("Flaky", "https://flaky.example.com")

	// First recheck: the site is down (503 is >= 500, so "down").
	r := h.do(http.MethodPost, path("/api/bookmarks/", b.ID)+"/check", nil)
	mustStatus(t, r, http.StatusOK)
	var checked bookmarkResp
	decode(t, r, &checked)
	if checked.Health != "down" {
		t.Fatalf("health after 503 = %q, want down", checked.Health)
	}

	// The site recovers; a manual recheck flips it back to up.
	h.prober.SetDefault(checker.ProbeResult{Up: true, Status: 200, Latency: time.Millisecond})
	r2 := h.do(http.MethodPost, path("/api/bookmarks/", b.ID)+"/check", nil)
	mustStatus(t, r2, http.StatusOK)
	var rechecked bookmarkResp
	decode(t, r2, &rechecked)
	if rechecked.Health != "up" {
		t.Errorf("health after recovery = %q, want up", rechecked.Health)
	}
}

func TestBookmarkCheckAllEndpoint(t *testing.T) {
	h := newHarness(t)
	h.prober.SetDefault(checker.ProbeResult{Up: true, Status: 200, Latency: time.Millisecond})
	h.createBookmark("A", "https://a.example.com")
	h.createBookmark("B", "https://b.example.com")

	r := h.do(http.MethodPost, "/api/bookmarks/check", nil)
	mustStatus(t, r, http.StatusOK)
	var list []bookmarkResp
	decode(t, r, &list)
	if len(list) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(list))
	}
	for _, b := range list {
		if b.Health != "up" {
			t.Errorf("bookmark %q health = %q, want up", b.Name, b.Health)
		}
	}
}

func TestBookmarkCheckMissingReturns404(t *testing.T) {
	h := newHarness(t)
	r := h.do(http.MethodPost, "/api/bookmarks/999999/check", nil)
	mustStatus(t, r, http.StatusNotFound)
}

// --- helpers ---

func (h *harness) createBookmark(name, url string) bookmarkResp {
	h.t.Helper()
	resp := h.do(http.MethodPost, "/api/bookmarks", map[string]string{"name": name, "url": url})
	mustStatus(h.t, resp, http.StatusCreated)
	var b bookmarkResp
	decode(h.t, resp, &b)
	return b
}

func (h *harness) listBookmarks() []bookmarkResp {
	h.t.Helper()
	resp := h.do(http.MethodGet, "/api/bookmarks", nil)
	mustStatus(h.t, resp, http.StatusOK)
	var list []bookmarkResp
	decode(h.t, resp, &list)
	return list
}
