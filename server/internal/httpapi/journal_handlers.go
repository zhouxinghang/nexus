package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"nexus/internal/db/dbgen"
)

// Journal page sizes: the default page and the hard cap on ?limit=.
const (
	journalPageDefault = 50
	journalPageMax     = 200
)

func (s *Server) registerJournalRoutes(r chi.Router) {
	r.Post("/journal/entries", s.handleCreateJournalEntry)
	r.Get("/journal/entries", s.handleListJournalEntries)
}

type journalEntryResponse struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type createJournalEntryRequest struct {
	Body string `json:"body"`
}

func (s *Server) handleCreateJournalEntry(w http.ResponseWriter, r *http.Request) {
	var req createJournalEntryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "body is required")
		return
	}
	entry, err := s.queries.CreateJournalEntry(r.Context(), req.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create entry")
		return
	}
	writeJSON(w, http.StatusCreated, journalEntryResponse{
		ID:        entry.ID,
		Body:      entry.Body,
		CreatedAt: entry.CreatedAt,
	})
}

// handleListJournalEntries returns one keyset page of the stream (or of the
// ?q= search matches), newest first: {"items": [...], "next_cursor": ...}.
// One extra row is fetched to tell whether another page follows.
func (s *Server) handleListJournalEntries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	limit, err := parseLimit(q, journalPageDefault, journalPageMax)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	cursor, hasCursor, err := parseCursor(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid cursor")
		return
	}
	var cursorAt *time.Time
	var cursorID pgtype.Int8
	if hasCursor {
		cursorAt = &cursor.CreatedAt
		cursorID = pgtype.Int8{Int64: cursor.ID, Valid: true}
	}

	var rows []dbgen.JournalEntry
	if query != "" {
		rows, err = s.queries.SearchJournalEntries(r.Context(), dbgen.SearchJournalEntriesParams{
			Pattern:         likeContains(query),
			CursorCreatedAt: cursorAt,
			CursorID:        cursorID,
			PageLimit:       limit + 1,
		})
	} else {
		rows, err = s.queries.ListJournalEntries(r.Context(), dbgen.ListJournalEntriesParams{
			CursorCreatedAt: cursorAt,
			CursorID:        cursorID,
			PageLimit:       limit + 1,
		})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list entries")
		return
	}

	page := keysetPage[journalEntryResponse]{Items: []journalEntryResponse{}}
	if len(rows) > int(limit) {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next := timeIDCursor{CreatedAt: last.CreatedAt, ID: last.ID}.encode()
		page.NextCursor = &next
	}
	for _, e := range rows {
		page.Items = append(page.Items, journalEntryResponse{ID: e.ID, Body: e.Body, CreatedAt: e.CreatedAt})
	}
	writeJSON(w, http.StatusOK, page)
}

// likeContains builds a case-insensitive "contains" pattern for an ILIKE
// query, escaping the LIKE wildcards (% and _) and the escape character (\)
// in the user's term so they match literally. The term is matched as a
// substring, which lets Chinese and other non-space-delimited text be found.
func likeContains(term string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(term) + "%"
}
