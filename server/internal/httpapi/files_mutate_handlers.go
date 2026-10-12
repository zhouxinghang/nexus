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
