package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"nexus/internal/db/dbgen"
)

// contextWithCheckTimeout bounds the fire-and-forget health check kicked off
// when a Bookmark is created, so it can't outlive a reasonable window even if
// the request context is already gone.
func contextWithCheckTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

func (s *Server) registerBookmarkRoutes(r chi.Router) {
	r.Post("/bookmarks", s.handleCreateBookmark)
	r.Get("/bookmarks", s.handleListBookmarks)
	r.Patch("/bookmarks/{id}", s.handleUpdateBookmark)
	r.Delete("/bookmarks/{id}", s.handleDeleteBookmark)
	r.Post("/bookmarks/{id}/check", s.handleCheckBookmark)
	r.Post("/bookmarks/check", s.handleCheckAllBookmarks)
}

type bookmarkResponse struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	Icon          string     `json:"icon"`
	Health        string     `json:"health"`
	LastStatus    *int32     `json:"last_status"`
	LastLatencyMs *int64     `json:"last_latency_ms"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func toBookmarkResponse(b dbgen.Bookmark) bookmarkResponse {
	resp := bookmarkResponse{
		ID:            b.ID,
		Name:          b.Name,
		URL:           b.Url,
		Icon:          b.Icon,
		Health:        string(b.Health),
		LastCheckedAt: b.LastCheckedAt,
		CreatedAt:     b.CreatedAt,
		UpdatedAt:     b.UpdatedAt,
	}
	if b.LastStatus.Valid {
		v := b.LastStatus.Int32
		resp.LastStatus = &v
	}
	if b.LastLatencyMs.Valid {
		v := b.LastLatencyMs.Int64
		resp.LastLatencyMs = &v
	}
	return resp
}

type bookmarkWriteRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Icon string `json:"icon"`
}

// normalizeURL validates the Owner-supplied link and prepends https:// when no
// scheme is present, so a bare "example.com" still becomes a clickable,
// probeable URL. Only http(s) links are accepted.
func normalizeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	if u.Host == "" {
		return "", false
	}
	return u.String(), true
}

func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	var req bookmarkWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	normalized, ok := normalizeURL(req.URL)
	if !ok {
		writeError(w, http.StatusBadRequest, "a valid http(s) url is required")
		return
	}
	b, err := s.queries.CreateBookmark(r.Context(), dbgen.CreateBookmarkParams{
		Name: name,
		Url:  normalized,
		Icon: strings.TrimSpace(req.Icon),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create bookmark")
		return
	}

	// Kick off a health check immediately so the new Bookmark shows a status
	// without waiting for the next ticker, but don't block the response on it.
	go func(b dbgen.Bookmark) {
		ctx, cancel := contextWithCheckTimeout()
		defer cancel()
		_, _ = s.checker.CheckOne(ctx, b)
	}(b)

	writeJSON(w, http.StatusCreated, toBookmarkResponse(b))
}

func (s *Server) handleListBookmarks(w http.ResponseWriter, r *http.Request) {
	bs, err := s.queries.ListBookmarks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list bookmarks")
		return
	}
	out := []bookmarkResponse{}
	for _, b := range bs {
		out = append(out, toBookmarkResponse(b))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := s.queries.GetBookmark(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load bookmark")
		return
	}

	var req bookmarkWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	normalized, ok := normalizeURL(req.URL)
	if !ok {
		writeError(w, http.StatusBadRequest, "a valid http(s) url is required")
		return
	}

	b, err := s.queries.UpdateBookmark(r.Context(), dbgen.UpdateBookmarkParams{
		ID:   existing.ID,
		Name: name,
		Url:  normalized,
		Icon: strings.TrimSpace(req.Icon),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update bookmark")
		return
	}
	writeJSON(w, http.StatusOK, toBookmarkResponse(b))
}

func (s *Server) handleDeleteBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.queries.DeleteBookmark(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete bookmark")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCheckBookmark probes a single Bookmark on demand and returns the
// updated row, so the Owner can force a recheck from the UI.
func (s *Server) handleCheckBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	b, err := s.queries.GetBookmark(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	updated, err := s.checker.CheckOne(r.Context(), b)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check bookmark")
		return
	}
	writeJSON(w, http.StatusOK, toBookmarkResponse(updated))
}

// handleCheckAllBookmarks probes every Bookmark on demand, then returns the
// refreshed list.
func (s *Server) handleCheckAllBookmarks(w http.ResponseWriter, r *http.Request) {
	if err := s.checker.CheckAll(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not check bookmarks")
		return
	}
	s.handleListBookmarks(w, r)
}
