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

	"nexus/internal/checker"
	"nexus/internal/db/dbgen"
)

// contextWithCheckTimeout bounds the fire-and-forget health check and
// metadata fetch kicked off when a Bookmark is created, so they can't outlive
// a reasonable window even if the request context is already gone.
func contextWithCheckTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}

// blankNameFetchTimeout bounds the one synchronous fetch on the request path:
// a URL-only create waits this long at most for the page title before falling
// back to the host as the name.
const blankNameFetchTimeout = 5 * time.Second

func (s *Server) registerBookmarkRoutes(r chi.Router) {
	r.Post("/bookmarks", s.handleCreateBookmark)
	r.Get("/bookmarks", s.handleListBookmarks)
	r.Patch("/bookmarks/{id}", s.handleUpdateBookmark)
	r.Delete("/bookmarks/{id}", s.handleDeleteBookmark)
	r.Post("/bookmarks/{id}/check", s.handleCheckBookmark)
	r.Post("/bookmarks/{id}/refetch", s.handleRefetchBookmark)
	r.Post("/bookmarks/{id}/click", s.handleClickBookmark)
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
	// Fetched from the site itself; separate from the Owner's name/icon.
	FetchedTitle      *string    `json:"fetched_title"`
	FetchedFaviconURL *string    `json:"fetched_favicon_url"`
	ClickCount        int64      `json:"click_count"`
	LastClickedAt     *time.Time `json:"last_clicked_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func toBookmarkResponse(b dbgen.Bookmark) bookmarkResponse {
	resp := bookmarkResponse{
		ID:            b.ID,
		Name:          b.Name,
		URL:           b.Url,
		Icon:          b.Icon,
		Health:        string(b.Health),
		LastCheckedAt: b.LastCheckedAt,
		ClickCount:    b.ClickCount,
		LastClickedAt: b.LastClickedAt,
		CreatedAt:     b.CreatedAt,
		UpdatedAt:     b.UpdatedAt,
	}
	if b.FetchedTitle.Valid {
		v := b.FetchedTitle.String
		resp.FetchedTitle = &v
	}
	if b.FetchedFaviconUrl.Valid {
		v := b.FetchedFaviconUrl.String
		resp.FetchedFaviconURL = &v
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

// handleCreateBookmark adds a Bookmark. A blank name is allowed: the page
// title is fetched synchronously (briefly bounded) to fill it, falling back to
// the URL host. With a name given, the metadata fetch runs in the background.
// The Owner's icon is never auto-populated; fetched values live in their own
// columns.
func (s *Server) handleCreateBookmark(w http.ResponseWriter, r *http.Request) {
	var req bookmarkWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	normalized, ok := normalizeURL(req.URL)
	if !ok {
		writeError(w, http.StatusBadRequest, "a valid http(s) url is required")
		return
	}

	name := strings.TrimSpace(req.Name)
	var meta *checker.Meta
	if name == "" {
		ctx, cancel := context.WithTimeout(r.Context(), blankNameFetchTimeout)
		m, err := s.fetcher.FetchMeta(ctx, normalized)
		cancel()
		if err == nil {
			meta = &m
			name = m.Title
		}
		if name == "" {
			name = hostOfURL(normalized)
		}
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
	if meta != nil {
		// Already fetched on the request path; store it now so the response
		// carries the favicon too.
		if updated, err := s.recordMeta(r.Context(), b.ID, *meta); err == nil {
			b = updated
		}
	}

	// Kick off a health check (and, if not already done, the metadata fetch)
	// immediately so the new Bookmark fills in without waiting for the next
	// ticker, but don't block the response on them.
	s.goBackground(func() {
		ctx, cancel := contextWithCheckTimeout()
		defer cancel()
		_, _ = s.checker.CheckOne(ctx, b)
		if meta == nil {
			s.fetchAndRecordMeta(ctx, b)
		}
	})

	writeJSON(w, http.StatusCreated, toBookmarkResponse(b))
}

// recordMeta stores fetched metadata through the query that only writes the
// fetched_* columns, never the Owner's name or icon.
func (s *Server) recordMeta(ctx context.Context, id int64, m checker.Meta) (dbgen.Bookmark, error) {
	return s.queries.RecordBookmarkMeta(ctx, dbgen.RecordBookmarkMetaParams{
		ID:                id,
		FetchedTitle:      m.Title,
		FetchedFaviconUrl: m.FaviconURL,
	})
}

// fetchAndRecordMeta fetches a Bookmark's page metadata and stores it. A
// failed fetch leaves any previous values untouched.
func (s *Server) fetchAndRecordMeta(ctx context.Context, b dbgen.Bookmark) {
	m, err := s.fetcher.FetchMeta(ctx, b.Url)
	if err != nil {
		return
	}
	_, _ = s.recordMeta(ctx, b.ID, m)
}

// hostOfURL returns a normalized URL's host (with any non-default port), the
// last-resort name for a URL-only create.
func hostOfURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

// handleListBookmarks lists the board. ?sort=created (default) keeps
// creation order; ?sort=clicks puts the most-visited first.
func (s *Server) handleListBookmarks(w http.ResponseWriter, r *http.Request) {
	var bs []dbgen.Bookmark
	var err error
	switch r.URL.Query().Get("sort") {
	case "", "created":
		bs, err = s.queries.ListBookmarks(r.Context())
	case "clicks":
		bs, err = s.queries.ListBookmarksByClicks(r.Context())
	default:
		writeError(w, http.StatusBadRequest, "invalid sort (want created or clicks)")
		return
	}
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
	// A new link means the fetched title/favicon describe the old site.
	if b.Url != existing.Url {
		s.goBackground(func() {
			ctx, cancel := contextWithCheckTimeout()
			defer cancel()
			s.fetchAndRecordMeta(ctx, b)
		})
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

// handleRefetchBookmark refreshes a Bookmark's fetched title and favicon on
// demand and returns the updated row. Only the fetched_* columns change; a
// failed fetch is reported as 502 and leaves the stored values as they were.
func (s *Server) handleRefetchBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	b, err := s.queries.GetBookmark(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bookmark not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load bookmark")
		return
	}
	m, err := s.fetcher.FetchMeta(r.Context(), b.Url)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not fetch the site's page")
		return
	}
	updated, err := s.recordMeta(r.Context(), b.ID, m)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save fetched metadata")
		return
	}
	writeJSON(w, http.StatusOK, toBookmarkResponse(updated))
}

// handleClickBookmark counts one open of a Bookmark. It is cheap and
// side-effect-only: no body in, none out.
func (s *Server) handleClickBookmark(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	n, err := s.queries.RecordBookmarkClick(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record click")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "bookmark not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
