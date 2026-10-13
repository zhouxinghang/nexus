package httpapi

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"nexus/internal/db/dbgen"
)

// copyTo is a tiny indirection so download handler and tests share one copy.
func copyTo(w io.Writer, r io.Reader) (int64, error) {
	return io.Copy(w, r)
}

type renameRequest struct {
	Name string `json:"name"` // new base name within the same directory
}

func (s *Server) handleRenameResource(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req renameRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || strings.ContainsRune(name, filepath.Separator) {
		writeError(w, http.StatusBadRequest, "name must be a bare filename")
		return
	}
	res, err := s.queries.GetResource(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	newPath := filepath.Join(filepath.Dir(res.Path), name)
	s.doMove(w, r, res, newPath, "rename")
}

type moveRequest struct {
	Dest string `json:"dest"` // destination path (absolute) or directory
}

func (s *Server) handleMoveResource(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req moveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	dest := strings.TrimSpace(req.Dest)
	if dest == "" || !filepath.IsAbs(dest) {
		writeError(w, http.StatusBadRequest, "absolute dest is required")
		return
	}
	res, err := s.queries.GetResource(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	dest = filepath.Clean(dest)
	// If dest is an existing directory, move into it keeping the name.
	if fi, statErr := s.host.Stat(dest); statErr == nil && fi.IsDir {
		dest = filepath.Join(dest, res.Name)
	}
	s.doMove(w, r, res, dest, "move")
}

// doMove performs the shared rename/move: validate the target is inside a
// Managed Root, perform the real syscall, update the Resource (tags/notes
// follow via the row), and record the audit entry.
func (s *Server) doMove(w http.ResponseWriter, r *http.Request, res dbgen.Resource, newPath, action string) {
	newPath = filepath.Clean(newPath)

	root, ok := s.rootForPath(r, newPath)
	if !ok {
		writeError(w, http.StatusBadRequest, "target is outside every Managed Root")
		return
	}

	// Ensure the destination directory exists.
	if err := s.host.MkdirAll(filepath.Dir(newPath)); err != nil {
		writeError(w, http.StatusInternalServerError, "could not prepare destination")
		return
	}
	if err := s.host.Rename(res.Path, newPath); err != nil {
		writeError(w, http.StatusInternalServerError, "could not move file")
		return
	}

	updated, err := s.queries.UpdateResourcePath(r.Context(), dbgen.UpdateResourcePathParams{
		ID:     res.ID,
		Path:   newPath,
		Name:   filepath.Base(newPath),
		RootID: root.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update resource")
		return
	}

	_, _ = s.queries.InsertAuditLog(r.Context(), dbgen.InsertAuditLogParams{
		Action:     action,
		ResourceID: pgtype.Int8{Int64: res.ID, Valid: true},
		OldPath:    res.Path,
		NewPath:    newPath,
	})

	writeJSON(w, http.StatusOK, s.toResourceResponse(r, updated))
}

func (s *Server) handleDeleteResourceFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := s.queries.GetResource(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	// Permanent delete — no trash.
	if err := s.host.Remove(res.Path); err != nil {
		// If the file is already gone, continue to clean up the Resource.
		writeError(w, http.StatusInternalServerError, "could not delete file")
		return
	}
	if err := s.queries.DeleteResource(r.Context(), res.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not remove resource")
		return
	}
	_, _ = s.queries.InsertAuditLog(r.Context(), dbgen.InsertAuditLogParams{
		Action:     "delete",
		ResourceID: pgtype.Int8{Int64: res.ID, Valid: true},
		OldPath:    res.Path,
		NewPath:    "",
	})
	w.WriteHeader(http.StatusNoContent)
}

// --- Batch mutations ---------------------------------------------------------
//
// These collapse what used to be N client round-trips into one request. The DB
// work (path rewrites, tag inserts, row deletes, audit rows) runs in a single
// transaction so the index never ends up half-updated. The real filesystem
// syscalls (rename/remove) can't join a DB transaction, so we run them first
// and only commit the rows for files that actually moved or were deleted.

type batchIDsRequest struct {
	IDs []int64 `json:"ids"`
}

type batchMoveRequest struct {
	IDs  []int64 `json:"ids"`
	Dest string  `json:"dest"`
}

type batchTagRequest struct {
	IDs []int64 `json:"ids"`
	Tag string  `json:"tag"`
}

// batchResult reports per-request outcomes so the UI can surface partial
// failures (e.g. a file that vanished mid-move).
type batchResult struct {
	Moved   int      `json:"moved,omitempty"`
	Tagged  int      `json:"tagged,omitempty"`
	Deleted int      `json:"deleted,omitempty"`
	Failed  []string `json:"failed,omitempty"`
}

// loadBatch fetches every requested resource in one query.
func (s *Server) loadBatch(w http.ResponseWriter, r *http.Request, ids []int64) ([]dbgen.Resource, bool) {
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return nil, false
	}
	resources, err := s.queries.ListResourcesByIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load resources")
		return nil, false
	}
	return resources, true
}

func (s *Server) handleBatchMove(w http.ResponseWriter, r *http.Request) {
	var req batchMoveRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	destDir := filepath.Clean(strings.TrimSpace(req.Dest))
	if strings.TrimSpace(req.Dest) == "" || !filepath.IsAbs(destDir) {
		writeError(w, http.StatusBadRequest, "absolute dest is required")
		return
	}
	// A batch dest must be a directory we move each file into; a single-file
	// target makes no sense for many sources.
	if fi, statErr := s.host.Stat(destDir); statErr != nil || !fi.IsDir {
		writeError(w, http.StatusBadRequest, "dest must be an existing directory")
		return
	}
	root, ok := s.rootForPath(r, destDir)
	if !ok {
		writeError(w, http.StatusBadRequest, "target is outside every Managed Root")
		return
	}

	resources, ok := s.loadBatch(w, r, req.IDs)
	if !ok {
		return
	}
	if err := s.host.MkdirAll(destDir); err != nil {
		writeError(w, http.StatusInternalServerError, "could not prepare destination")
		return
	}

	// Phase 1: perform the real moves, collecting the rows that succeeded.
	type moved struct {
		id      int64
		oldPath string
		newPath string
		name    string
	}
	var done []moved
	result := batchResult{}
	for _, res := range resources {
		if res.State == dbgen.ResourceStateMissing {
			result.Failed = append(result.Failed, res.Path)
			continue
		}
		newPath := filepath.Join(destDir, res.Name)
		if newPath == res.Path {
			continue // already there; nothing to do
		}
		if err := s.host.Rename(res.Path, newPath); err != nil {
			result.Failed = append(result.Failed, res.Path)
			continue
		}
		done = append(done, moved{id: res.ID, oldPath: res.Path, newPath: newPath, name: res.Name})
	}

	// Phase 2: persist the index + audit rows for the files that moved, in one
	// transaction.
	if len(done) > 0 {
		if err := s.inTx(r, func(q *dbgen.Queries) error {
			for _, m := range done {
				if _, err := q.UpdateResourcePath(r.Context(), dbgen.UpdateResourcePathParams{
					ID:     m.id,
					Path:   m.newPath,
					Name:   m.name,
					RootID: root.ID,
				}); err != nil {
					return err
				}
				if _, err := q.InsertAuditLog(r.Context(), dbgen.InsertAuditLogParams{
					Action:     "move",
					ResourceID: pgtype.Int8{Int64: m.id, Valid: true},
					OldPath:    m.oldPath,
					NewPath:    m.newPath,
				}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update index")
			return
		}
	}
	result.Moved = len(done)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleBatchAddTag(w http.ResponseWriter, r *http.Request) {
	var req batchTagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tag := strings.TrimSpace(req.Tag)
	if tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}
	// One INSERT ... SELECT unnest(ids) tags every row and skips duplicates via
	// ON CONFLICT — no per-id round-trips.
	if err := s.queries.AddTagToResources(r.Context(), dbgen.AddTagToResourcesParams{
		Tag: tag,
		Ids: req.IDs,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not add tag")
		return
	}
	writeJSON(w, http.StatusOK, batchResult{Tagged: len(req.IDs)})
}

func (s *Server) handleBatchDelete(w http.ResponseWriter, r *http.Request) {
	var req batchIDsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	resources, ok := s.loadBatch(w, r, req.IDs)
	if !ok {
		return
	}

	// Phase 1: remove the on-disk bytes (missing resources have none), keeping
	// the ids whose index rows we can now safely drop.
	result := batchResult{}
	var toDelete []int64
	var audit []dbgen.Resource
	for _, res := range resources {
		if res.State != dbgen.ResourceStateMissing {
			if err := s.host.Remove(res.Path); err != nil {
				result.Failed = append(result.Failed, res.Path)
				continue
			}
			audit = append(audit, res)
		}
		toDelete = append(toDelete, res.ID)
	}

	// Phase 2: drop all index rows in one DELETE ... = ANY($1) and record audit
	// rows for the files we actually removed, in a single transaction.
	if len(toDelete) > 0 {
		if err := s.inTx(r, func(q *dbgen.Queries) error {
			if err := q.DeleteResourcesByIDs(r.Context(), toDelete); err != nil {
				return err
			}
			for _, res := range audit {
				if _, err := q.InsertAuditLog(r.Context(), dbgen.InsertAuditLogParams{
					Action:     "delete",
					ResourceID: pgtype.Int8{Int64: res.ID, Valid: true},
					OldPath:    res.Path,
					NewPath:    "",
				}); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "could not remove resources")
			return
		}
	}
	result.Deleted = len(toDelete)
	writeJSON(w, http.StatusOK, result)
}
