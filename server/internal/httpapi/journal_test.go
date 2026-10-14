package httpapi_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type journalEntry struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func TestJournalAppendAndTimestamp(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": "first thing happened"})
	mustStatus(t, resp, http.StatusCreated)
	var e journalEntry
	decode(t, resp, &e)
	if e.ID == 0 {
		t.Error("expected a non-zero id")
	}
	if e.Body != "first thing happened" {
		t.Errorf("body = %q", e.Body)
	}
	if e.CreatedAt.IsZero() {
		t.Error("expected an auto-set created_at timestamp")
	}
}

func TestJournalNewestFirstOrdering(t *testing.T) {
	h := newHarness(t)
	for _, body := range []string{"one", "two", "three"} {
		resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": body})
		mustStatus(t, resp, http.StatusCreated)
	}
	resp := h.do(http.MethodGet, "/api/journal/entries", nil)
	mustStatus(t, resp, http.StatusOK)
	var entries []journalEntry
	decodeItems(t, resp, &entries)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Body != "three" || entries[2].Body != "one" {
		t.Errorf("expected newest-first [three two one], got [%s %s %s]",
			entries[0].Body, entries[1].Body, entries[2].Body)
	}
}

func TestJournalFullTextSearch(t *testing.T) {
	h := newHarness(t)
	bodies := []string{
		"deployed the database migration today",
		"went for a long walk in the park",
		"migration finished without errors",
	}
	for _, b := range bodies {
		resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": b})
		mustStatus(t, resp, http.StatusCreated)
	}
	resp := h.do(http.MethodGet, "/api/journal/entries?q=migration", nil)
	mustStatus(t, resp, http.StatusOK)
	var entries []journalEntry
	decodeItems(t, resp, &entries)
	if len(entries) != 2 {
		t.Fatalf("expected 2 search hits for 'migration', got %d", len(entries))
	}
	for _, e := range entries {
		if e.Body == "went for a long walk in the park" {
			t.Error("search returned a non-matching entry")
		}
	}
}

func TestJournalSearchIsCaseInsensitive(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": "Deployed the Service"})
	mustStatus(t, resp, http.StatusCreated)

	resp = h.do(http.MethodGet, "/api/journal/entries?q=DEPLOYED", nil)
	mustStatus(t, resp, http.StatusOK)
	var entries []journalEntry
	decodeItems(t, resp, &entries)
	if len(entries) != 1 {
		t.Fatalf("expected 1 case-insensitive hit, got %d", len(entries))
	}
}

func TestJournalSearchChinese(t *testing.T) {
	h := newHarness(t)
	bodies := []string{
		"今天记录一下个人的工作总结",
		"去公园散步放松了一下",
		"【个人】目标：学习 Postgres 全文检索",
	}
	for _, b := range bodies {
		resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": b})
		mustStatus(t, resp, http.StatusCreated)
	}
	resp := h.do(http.MethodGet, "/api/journal/entries?q=%E4%B8%AA%E4%BA%BA", nil) // q=个人
	mustStatus(t, resp, http.StatusOK)
	var entries []journalEntry
	decodeItems(t, resp, &entries)
	if len(entries) != 2 {
		t.Fatalf("expected 2 search hits for '个人', got %d", len(entries))
	}
	for _, e := range entries {
		if e.Body == "去公园散步放松了一下" {
			t.Error("search returned a non-matching entry")
		}
	}
}

func TestJournalSearchEscapesWildcards(t *testing.T) {
	h := newHarness(t)
	bodies := []string{
		"got a 50% discount today",
		"nothing special happened",
	}
	for _, b := range bodies {
		resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": b})
		mustStatus(t, resp, http.StatusCreated)
	}
	// A literal "50%" must not be treated as a LIKE wildcard.
	resp := h.do(http.MethodGet, "/api/journal/entries?q=50%25", nil) // q=50%
	mustStatus(t, resp, http.StatusOK)
	var entries []journalEntry
	decodeItems(t, resp, &entries)
	if len(entries) != 1 {
		t.Fatalf("expected 1 hit for literal '50%%', got %d", len(entries))
	}
	if entries[0].Body != "got a 50% discount today" {
		t.Errorf("unexpected match: %q", entries[0].Body)
	}
}

func TestJournalHasNoEditOrDelete(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": "immutable record"})
	mustStatus(t, resp, http.StatusCreated)
	var e journalEntry
	decode(t, resp, &e)

	// No edit path.
	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		r := h.do(method, "/api/journal/entries/1", map[string]string{"body": "changed"})
		if r.StatusCode != http.StatusMethodNotAllowed && r.StatusCode != http.StatusNotFound {
			t.Errorf("%s on an entry should not be allowed, got %d", method, r.StatusCode)
		}
		r.Body.Close()
	}
	// No delete path.
	r := h.do(http.MethodDelete, "/api/journal/entries/1", nil)
	if r.StatusCode != http.StatusMethodNotAllowed && r.StatusCode != http.StatusNotFound {
		t.Errorf("DELETE on an entry should not be allowed, got %d", r.StatusCode)
	}
	r.Body.Close()
}

func TestJournalRequiresBody(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": "   "})
	mustStatus(t, resp, http.StatusBadRequest)
}

// journalPage is the keyset envelope returned by GET /api/journal/entries.
type journalPage struct {
	Items      []journalEntry `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

func (h *harness) journalPage(query string) journalPage {
	h.t.Helper()
	r := h.do(http.MethodGet, "/api/journal/entries"+query, nil)
	mustStatus(h.t, r, http.StatusOK)
	var p journalPage
	decode(h.t, r, &p)
	return p
}

// walkJournal follows next_cursor from the first page and returns every
// entry in order plus the page count.
func (h *harness) walkJournal(base string) ([]journalEntry, int) {
	h.t.Helper()
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	var all []journalEntry
	pages := 0
	p := h.journalPage(base)
	for {
		pages++
		all = append(all, p.Items...)
		if p.NextCursor == nil {
			return all, pages
		}
		if pages > 50 {
			h.t.Fatal("cursor never terminated")
		}
		p = h.journalPage(base + sep + "cursor=" + url.QueryEscape(*p.NextCursor))
	}
}

func TestJournalKeysetPagination(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 7; i++ {
		resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": fmt.Sprintf("entry %d", i)})
		mustStatus(t, resp, http.StatusCreated)
		resp.Body.Close()
	}

	first := h.journalPage("?limit=3")
	if len(first.Items) != 3 || first.NextCursor == nil {
		t.Fatalf("first page = %d items, cursor %v", len(first.Items), first.NextCursor)
	}
	if first.Items[0].Body != "entry 6" {
		t.Errorf("first item = %q, want newest", first.Items[0].Body)
	}

	all, pages := h.walkJournal("?limit=3")
	if pages != 3 || len(all) != 7 {
		t.Fatalf("walked %d pages / %d entries, want 3 / 7", pages, len(all))
	}
	seen := map[int64]bool{}
	for i, e := range all {
		if seen[e.ID] {
			t.Errorf("duplicate entry %d across pages", e.ID)
		}
		seen[e.ID] = true
		if want := fmt.Sprintf("entry %d", 6-i); e.Body != want {
			t.Errorf("position %d = %q, want %q", i, e.Body, want)
		}
	}

	// An exactly-full final page signals the end.
	full := h.journalPage("?limit=7")
	if len(full.Items) != 7 || full.NextCursor != nil {
		t.Errorf("limit=7 page = %d items, cursor %v; want 7, null", len(full.Items), full.NextCursor)
	}
}

func TestJournalPaginationStableAcrossAppends(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 4; i++ {
		resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": fmt.Sprintf("old %d", i)})
		resp.Body.Close()
	}
	first := h.journalPage("?limit=2")
	// A new entry lands between page fetches; the next page must neither skip
	// nor repeat anything from the original stream.
	resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": "new"})
	resp.Body.Close()
	second := h.journalPage("?limit=2&cursor=" + url.QueryEscape(*first.NextCursor))
	if len(second.Items) != 2 || second.Items[0].Body != "old 1" || second.Items[1].Body != "old 0" {
		t.Errorf("second page = %+v, want [old 1, old 0]", second.Items)
	}
	if second.NextCursor != nil {
		t.Error("expected the last page to have no next cursor")
	}
}

func TestJournalSearchPagination(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < 5; i++ {
		resp := h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": fmt.Sprintf("迁移 note %d", i)})
		resp.Body.Close()
		resp = h.do(http.MethodPost, "/api/journal/entries", map[string]string{"body": fmt.Sprintf("other %d", i)})
		resp.Body.Close()
	}
	all, pages := h.walkJournal("?q=" + url.QueryEscape("迁移") + "&limit=2")
	if len(all) != 5 || pages != 3 {
		t.Fatalf("search walk = %d entries over %d pages, want 5 over 3", len(all), pages)
	}
	for _, e := range all {
		if !strings.Contains(e.Body, "迁移") {
			t.Errorf("non-matching entry in results: %q", e.Body)
		}
	}
}

func TestJournalPagingRejectsBadParams(t *testing.T) {
	h := newHarness(t)
	for _, q := range []string{"?limit=0", "?limit=-1", "?limit=abc", "?cursor=!!!", "?cursor=" + url.QueryEscape("bm90LWEtY3Vyc29y")} {
		r := h.do(http.MethodGet, "/api/journal/entries"+q, nil)
		mustStatus(t, r, http.StatusBadRequest)
		r.Body.Close()
	}
	// An oversized limit is clamped rather than rejected.
	p := h.journalPage("?limit=100000")
	if p.Items == nil {
		t.Error("expected an (empty) items array")
	}
}
