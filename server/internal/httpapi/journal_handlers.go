package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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

func (s *Server) handleListJournalEntries(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	out := []journalEntryResponse{}
	if query != "" {
		rows, err := s.queries.SearchJournalEntries(r.Context(), likeContains(query))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not search entries")
			return
		}
		for _, e := range rows {
			out = append(out, journalEntryResponse{ID: e.ID, Body: e.Body, CreatedAt: e.CreatedAt})
		}
	} else {
		rows, err := s.queries.ListJournalEntries(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not list entries")
			return
		}
		for _, e := range rows {
			out = append(out, journalEntryResponse{ID: e.ID, Body: e.Body, CreatedAt: e.CreatedAt})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// likeContains builds a case-insensitive "contains" pattern for an ILIKE
// query, escaping the LIKE wildcards (% and _) and the escape character (\)
// in the user's term so they match literally. The term is matched as a
// substring, which lets Chinese and other non-space-delimited text be found.
func likeContains(term string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(term) + "%"
}
