// Package checker runs the background health checker that periodically probes
// each Bookmark's URL and records whether the site is reachable. It mirrors the
// Servers sampler: a long-running goroutine on a fixed interval, plus exported
// methods so the HTTP seam can drive a check synchronously in tests.
package checker

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"nexus/internal/db/dbgen"
)

// Interval is how often every Bookmark is health-checked.
const Interval = 5 * time.Minute

// probeTimeout bounds a single HTTP probe so one slow site can't stall the run.
const probeTimeout = 10 * time.Second

// ProbeResult is the outcome of probing a single URL.
type ProbeResult struct {
	// Up is true when the site answered with a non-5xx status.
	Up bool
	// Status is the HTTP status code, or 0 when the request never completed.
	Status int
	// Latency is the round-trip time of the probe.
	Latency time.Duration
}

// Prober is the seam through which the checker reaches the network. A real
// implementation makes HTTP requests; a fake is injected in tests so no live
// network calls happen.
type Prober interface {
	Probe(ctx context.Context, url string) ProbeResult
}

// HTTPProber is the production Prober. It issues a GET and treats any response
// below 500 as "up" (a 401/403/404 still proves the host is reachable).
type HTTPProber struct {
	client *http.Client
}

// NewHTTPProber builds an HTTPProber that follows redirects with a bounded
// per-request timeout.
func NewHTTPProber() *HTTPProber {
	return &HTTPProber{client: &http.Client{Timeout: probeTimeout}}
}

func (p *HTTPProber) Probe(ctx context.Context, url string) ProbeResult {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{Up: false}
	}
	req.Header.Set("User-Agent", "Nexus-Bookmark-Checker/1.0")

	start := time.Now()
	resp, err := p.client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return ProbeResult{Up: false, Latency: latency}
	}
	defer resp.Body.Close()
	return ProbeResult{
		Up:      resp.StatusCode < 500,
		Status:  resp.StatusCode,
		Latency: latency,
	}
}

var _ Prober = (*HTTPProber)(nil)

// Checker probes Bookmarks and persists the results.
type Checker struct {
	queries *dbgen.Queries
	prober  Prober
}

// New builds a Checker.
func New(queries *dbgen.Queries, p Prober) *Checker {
	return &Checker{queries: queries, prober: p}
}

// CheckOne probes a single Bookmark and records the result, returning the
// updated row.
func (c *Checker) CheckOne(ctx context.Context, b dbgen.Bookmark) (dbgen.Bookmark, error) {
	res := c.prober.Probe(ctx, b.Url)

	health := dbgen.BookmarkHealthDown
	if res.Up {
		health = dbgen.BookmarkHealthUp
	}

	params := dbgen.RecordBookmarkCheckParams{ID: b.ID, Health: health}
	if res.Status > 0 {
		params.LastStatus = pgtype.Int4{Int32: int32(res.Status), Valid: true}
	}
	if res.Latency > 0 {
		params.LastLatencyMs = pgtype.Int8{Int64: res.Latency.Milliseconds(), Valid: true}
	}
	return c.queries.RecordBookmarkCheck(ctx, params)
}

// CheckAll probes every Bookmark once. Exposed so the HTTP seam and tests can
// run a full sweep on demand without waiting for the ticker.
func (c *Checker) CheckAll(ctx context.Context) error {
	bookmarks, err := c.queries.ListBookmarks(ctx)
	if err != nil {
		return err
	}
	for _, b := range bookmarks {
		if _, err := c.CheckOne(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

// Run blocks, checking every Bookmark on each Interval tick until the context
// is cancelled. Intended to be launched in a goroutine on startup. It checks
// once immediately so freshly added Nexus instances get health promptly.
func (c *Checker) Run(ctx context.Context) {
	_ = c.CheckAll(ctx)

	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.CheckAll(ctx)
		}
	}
}
