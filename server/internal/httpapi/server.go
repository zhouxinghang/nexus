// Package httpapi wires the chi router, authentication, and all module
// handlers into a single http.Handler. Every module is tested through this
// seam, driving the real router via httptest against an ephemeral Postgres.
package httpapi

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"nexus/internal/auth"
	"nexus/internal/checker"
	"nexus/internal/db/dbgen"
	"nexus/internal/host"
	"nexus/internal/sampler"
)

// Server bundles the dependencies shared across all handlers.
type Server struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
	auth    *auth.Authenticator
	host    host.Host
	sampler *sampler.Sampler
	checker *checker.Checker
	fetcher checker.Fetcher
	// scans tracks per-root background scans so the HTTP layer never blocks on
	// a full directory walk and the UI can poll progress.
	scans *scanManager
	// bg tracks fire-and-forget work started by handlers (metadata fetches)
	// so tests can wait for it deterministically.
	bg sync.WaitGroup
	// Striped locks bound memory while serializing concurrent editor saves.
	fileEdits [64]sync.Mutex
}

// NewServer builds a Server from its dependencies.
func NewServer(pool *pgxpool.Pool, authn *auth.Authenticator, h host.Host, prober checker.Prober, fetcher checker.Fetcher) *Server {
	queries := dbgen.New(pool)
	return &Server{
		pool:    pool,
		queries: queries,
		auth:    authn,
		host:    h,
		sampler: sampler.New(queries, h),
		checker: checker.New(queries, prober),
		fetcher: fetcher,
		scans:   newScanManager(),
	}
}

// goBackground runs fn on its own goroutine, tracked by s.bg.
func (s *Server) goBackground(fn func()) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		fn()
	}()
}

// Sampler exposes the background sampler so callers (main, tests) can run it.
func (s *Server) Sampler() *sampler.Sampler { return s.sampler }

// Router returns the fully-wired http.Handler: the SPA, the public login
// endpoint, and the authenticated API under /api.
func (s *Server) Router(spa http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	r.Route("/api", func(api chi.Router) {
		// Public endpoints.
		api.Post("/auth/login", s.handleLogin)

		// Authenticated endpoints.
		api.Group(func(pr chi.Router) {
			pr.Use(s.auth.Middleware)

			pr.Get("/auth/me", s.handleMe)
			pr.Post("/auth/logout", s.handleLogout)
			pr.Get("/overview", s.handleOverview)

			s.registerJournalRoutes(pr)
			s.registerTaskRoutes(pr)
			s.registerServerRoutes(pr)
			s.registerFileRoutes(pr)
			s.registerBookmarkRoutes(pr)
		})
	})

	if spa != nil {
		r.NotFound(spa.ServeHTTP)
		r.Get("/", spa.ServeHTTP)
	}
	return r
}

// --- shared JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// decodeJSONRaw decodes into a map of raw messages so handlers can distinguish
// an absent field from one explicitly set to null.
func decodeJSONRaw(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// inTx runs fn inside a single database transaction, committing on success and
// rolling back on any error. Handlers use it to make multi-statement batch
// mutations atomic in the index.
func (s *Server) inTx(r *http.Request, fn func(q *dbgen.Queries) error) error {
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if err := fn(s.queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(r.Context())
}
