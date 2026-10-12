package httpapi_test

import (
	"net/http"
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
	decode(t, resp, &entries)
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
	decode(t, resp, &entries)
	if len(entries) != 2 {
		t.Fatalf("expected 2 search hits for 'migration', got %d", len(entries))
	}
	for _, e := range entries {
		if e.Body == "went for a long walk in the park" {
			t.Error("search returned a non-matching entry")
		}
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
