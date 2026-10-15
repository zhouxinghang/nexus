package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"nexus/internal/db/dbgen"
	"nexus/internal/host"
)

// --- Scanning ---
//
// A scan walks a Managed Root's directory tree and reconciles it with the
// index. Large directories (hundreds of thousands of files) make a synchronous
// scan a long-held request, so the scan runs in the background: POST starts it
// and returns the current status; the UI polls GET for progress until done.
// This mirrors FileBrowser Quantum's background indexer, where the HTTP layer
// never blocks on a full walk.

// scanUpsertBatch caps how many Resources ride in one BulkUpsertResources
// round-trip. A whole directory could be hundreds of thousands of files;
// chunking keeps each statement's parameter arrays and the server-side memory
// bounded while still collapsing the common case into a handful of queries.
const scanUpsertBatch = 2000

// scanRunTimeout bounds one background scan so a pathological tree (or a stuck
// filesystem) can't pin a goroutine and a DB connection forever.
const scanRunTimeout = 30 * time.Minute

// scanPhase is where a scan currently is, for the status response.
type scanPhase string

const (
	scanIdle      scanPhase = "idle"      // never run (no status row yet)
	scanRunning   scanPhase = "running"   // walking / indexing
	scanCompleted scanPhase = "completed" // finished cleanly
	scanFailed    scanPhase = "failed"    // stopped on an error
)

// scanState is one root's live scan progress. It's mutated only by the
// scanManager under its lock, and copied out for the status response.
type scanState struct {
	Phase     scanPhase
	Total     int // files seen on disk so far (grows during the walk)
	Indexed   int // files upserted so far (changed/new rows)
	Removed   int // resources marked missing at reconcile
	Err       string
	StartedAt time.Time
	UpdatedAt time.Time
}

// scanManager tracks the in-flight and most-recent scan per root, so only one
// scan runs per root at a time and the UI can poll progress. It is the single
// owner of scan goroutines started from the HTTP layer.
type scanManager struct {
	mu      sync.Mutex
	states  map[int64]*scanState // rootID -> latest state (running or final)
	running map[int64]bool       // rootID -> a scan goroutine is active
}

func newScanManager() *scanManager {
	return &scanManager{
		states:  map[int64]*scanState{},
		running: map[int64]bool{},
	}
}

// begin marks a root as scanning and returns a fresh running state, or false if
// a scan is already in flight for that root.
func (m *scanManager) begin(rootID int64, now time.Time) (*scanState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running[rootID] {
		return nil, false
	}
	st := &scanState{Phase: scanRunning, StartedAt: now, UpdatedAt: now}
	m.states[rootID] = st
	m.running[rootID] = true
	return m.snapshot(rootID), true
}

// update applies mutate to the root's state under the lock and stamps it.
func (m *scanManager) update(rootID int64, now time.Time, mutate func(*scanState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.states[rootID]
	if st == nil {
		return
	}
	mutate(st)
	st.UpdatedAt = now
}

// finish records the terminal phase and clears the running flag.
func (m *scanManager) finish(rootID int64, now time.Time, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.states[rootID]
	if st != nil {
		if err != nil {
			st.Phase, st.Err = scanFailed, err.Error()
		} else {
			st.Phase = scanCompleted
		}
		st.UpdatedAt = now
	}
	delete(m.running, rootID)
}

// status returns a copy of a root's latest state, or an idle zero value if it
// has never been scanned. Callers must not retain the pointer.
func (m *scanManager) status(rootID int64) scanState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st := m.snapshot(rootID); st != nil {
		return *st
	}
	return scanState{Phase: scanIdle}
}

// snapshot returns a copy of the state under the already-held lock.
func (m *scanManager) snapshot(rootID int64) *scanState {
	st := m.states[rootID]
	if st == nil {
		return nil
	}
	cp := *st
	return &cp
}

// --- HTTP handlers ---

type scanStatusResponse struct {
	RootID    int64      `json:"root_id"`
	Phase     scanPhase  `json:"phase"`
	Total     int        `json:"total"`
	Indexed   int        `json:"indexed"`
	Removed   int        `json:"removed"`
	Error     string     `json:"error,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func newScanStatusResponse(rootID int64, st scanState) scanStatusResponse {
	resp := scanStatusResponse{
		RootID:  rootID,
		Phase:   st.Phase,
		Total:   st.Total,
		Indexed: st.Indexed,
		Removed: st.Removed,
		Error:   st.Err,
	}
	if !st.StartedAt.IsZero() {
		started := st.StartedAt
		resp.StartedAt = &started
	}
	if !st.UpdatedAt.IsZero() {
		updated := st.UpdatedAt
		resp.UpdatedAt = &updated
	}
	return resp
}

// handleScanRoot starts a background scan of a Managed Root and returns 202
// with the initial status. A scan already in flight for the root is left to
// run and reported as-is (also 202), so a double-click never launches two.
func (s *Server) handleScanRoot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	root, err := s.queries.GetManagedRoot(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "root not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load root")
		return
	}

	init, started := s.scans.begin(root.ID, time.Now())
	if !started {
		// Already scanning: report the live status instead of starting again.
		writeJSON(w, http.StatusAccepted, newScanStatusResponse(root.ID, s.scans.status(root.ID)))
		return
	}

	s.goBackground(func() {
		ctx, cancel := context.WithTimeout(context.Background(), scanRunTimeout)
		defer cancel()
		err := s.runScan(ctx, root)
		s.scans.finish(root.ID, time.Now(), err)
	})

	writeJSON(w, http.StatusAccepted, newScanStatusResponse(root.ID, *init))
}

// handleScanStatus reports a root's current (or most recent) scan progress.
func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := s.queries.GetManagedRoot(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "root not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load root")
		return
	}
	writeJSON(w, http.StatusOK, newScanStatusResponse(id, s.scans.status(id)))
}

// runScan performs the walk + incremental index + reconcile for one root,
// updating the scan state as it goes. It uses its own context (not a request's)
// because it outlives the POST that started it.
func (s *Server) runScan(ctx context.Context, root dbgen.ManagedRoot) error {
	// Walk the real filesystem under the root.
	files, err := s.host.Walk(root.Path)
	if err != nil {
		return err
	}
	s.scans.update(root.ID, time.Now(), func(st *scanState) { st.Total = len(files) })

	// Load the index's current metadata for this root once, so a rescan
	// only writes the files that are new or actually changed — a modtime-based
	// "quick scan" rather than re-upserting every row every time.
	digestRows, err := s.queries.ListResourceDigestByRoot(ctx, root.ID)
	if err != nil {
		return err
	}
	known := make(map[string]fileDigest, len(digestRows))
	for _, d := range digestRows {
		known[d.Path] = fileDigest{size: d.Size, mod: d.ModifiedAt, created: d.FileCreatedAt}
	}

	// Collect the paths seen on disk (for reconciliation) and upsert the subset
	// whose size or modtime changed (the only ones worth writing).
	seen := make([]string, 0, len(files))
	indexed := 0
	batch := newUpsertBatch(root.ID)
	flush := func() error {
		if len(batch.Paths) == 0 {
			return nil
		}
		if err := s.queries.BulkUpsertResources(ctx, batch); err != nil {
			return err
		}
		indexed += len(batch.Paths)
		s.scans.update(root.ID, time.Now(), func(st *scanState) { st.Indexed = indexed })
		batch = newUpsertBatch(root.ID)
		return nil
	}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		seen = append(seen, f.Path)
		if d, ok := known[f.Path]; ok && d.unchanged(f) {
			continue // present and identical: skip the write
		}
		appendToBatch(&batch, f)
		if len(batch.Paths) >= scanUpsertBatch {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	if err := flush(); err != nil {
		return err
	}

	// Any present Resource under this root not seen on disk → mark Missing, in
	// a single set-based statement rather than one UPDATE per vanished row.
	removed, err := s.queries.MarkResourcesMissingUnderRoot(ctx, dbgen.MarkResourcesMissingUnderRootParams{
		RootID:    root.ID,
		SeenPaths: seen,
	})
	if err != nil {
		return err
	}
	s.scans.update(root.ID, time.Now(), func(st *scanState) { st.Removed = int(removed) })
	return nil
}

// fileDigest is the subset of an indexed Resource a rescan compares against the
// live file to decide whether anything changed.
type fileDigest struct {
	size    int64
	mod     *time.Time
	created *time.Time
}

// unchanged reports whether f matches this digest in size and modification
// time. Postgres stores timestamptz at microsecond resolution, so the live
// mtime is truncated to the same granularity before comparing — otherwise a
// nanosecond tail would make every file look changed on the next scan.
func (d fileDigest) unchanged(f host.FileInfo) bool {
	if d.size != f.Size {
		return false
	}
	if d.mod == nil {
		return false
	}
	// Fill creation times for legacy rows even when size/mtime are unchanged.
	// An unavailable observation does not erase an earlier known birth time.
	if f.CreatedAt != nil && (d.created == nil || !d.created.Equal(f.CreatedAt.Truncate(time.Microsecond))) {
		return false
	}
	return d.mod.Equal(f.ModifiedAt.Truncate(time.Microsecond))
}

// newUpsertBatch starts an empty BulkUpsertResources batch for a root.
func newUpsertBatch(rootID int64) dbgen.BulkUpsertResourcesParams {
	return dbgen.BulkUpsertResourcesParams{RootID: rootID}
}

// appendToBatch adds one file's columns to the batch's parallel arrays. The
// mtime is truncated to microseconds so it round-trips through Postgres's
// timestamptz unchanged, keeping later quick-scan comparisons stable.
func appendToBatch(b *dbgen.BulkUpsertResourcesParams, f host.FileInfo) {
	mod := f.ModifiedAt.Truncate(time.Microsecond)
	b.Paths = append(b.Paths, f.Path)
	b.Names = append(b.Names, f.Name)
	b.Sizes = append(b.Sizes, f.Size)
	b.IsDirs = append(b.IsDirs, f.IsDir)
	b.ModifiedAts = append(b.ModifiedAts, mod)
	created := time.Time{}
	if f.CreatedAt != nil {
		created = f.CreatedAt.Truncate(time.Microsecond)
	}
	b.FileCreatedAts = append(b.FileCreatedAts, created)
	b.FileCreatedKnown = append(b.FileCreatedKnown, f.CreatedAt != nil)
}
