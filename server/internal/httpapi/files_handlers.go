package httpapi

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"nexus/internal/db/dbgen"
)

func (s *Server) registerFileRoutes(r chi.Router) {
	// Managed Roots
	r.Post("/files/roots", s.handleCreateRoot)
	r.Get("/files/roots", s.handleListRoots)
	r.Delete("/files/roots/{id}", s.handleDeleteRoot)
	r.Post("/files/roots/{id}/scan", s.handleScanRoot)

	// Resources
	r.Get("/files/resources", s.handleListResources)
	r.Get("/files/resources/{id}", s.handleGetResource)
	r.Post("/files/resources/register", s.handleRegisterResource)
	r.Delete("/files/resources/{id}", s.handleClearResource)
	r.Get("/files/resources/{id}/download", s.handleDownloadResource)

	// Metadata
	r.Put("/files/resources/{id}/note", s.handleSetNote)
	r.Post("/files/resources/{id}/tags", s.handleAddTag)
	r.Delete("/files/resources/{id}/tags/{tag}", s.handleRemoveTag)

	// Mutations (Issue 07)
	r.Post("/files/resources/{id}/rename", s.handleRenameResource)
	r.Post("/files/resources/{id}/move", s.handleMoveResource)
	r.Delete("/files/resources/{id}/file", s.handleDeleteResourceFile)

	// Audit log
	r.Get("/files/audit", s.handleListAudit)
}

// --- response shapes ---

type rootResponse struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type resourceResponse struct {
	ID         int64      `json:"id"`
	RootID     int64      `json:"root_id"`
	Path       string     `json:"path"`
	Name       string     `json:"name"`
	Size       int64      `json:"size"`
	IsDir      bool       `json:"is_dir"`
	ModifiedAt *time.Time `json:"modified_at"`
	State      string     `json:"state"`
	Note       string     `json:"note"`
	Tags       []string   `json:"tags"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (s *Server) toResourceResponse(r *http.Request, res dbgen.Resource) resourceResponse {
	tags, _ := s.queries.ListTagsForResource(r.Context(), res.ID)
	if tags == nil {
		tags = []string{}
	}
	return resourceResponse{
		ID:         res.ID,
		RootID:     res.RootID,
		Path:       res.Path,
		Name:       res.Name,
		Size:       res.Size,
		IsDir:      res.IsDir,
		ModifiedAt: res.ModifiedAt,
		State:      string(res.State),
		Note:       res.Note,
		Tags:       tags,
		CreatedAt:  res.CreatedAt,
		UpdatedAt:  res.UpdatedAt,
	}
}

// --- Managed Roots ---

type createRootRequest struct {
	Path string `json:"path"`
}

func (s *Server) handleCreateRoot(w http.ResponseWriter, r *http.Request) {
	var req createRootRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p := strings.TrimSpace(req.Path)
	if p == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !filepath.IsAbs(p) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	// Reject a path that doesn't exist or isn't a directory.
	fi, err := s.host.Stat(p)
	if err != nil || !fi.IsDir {
		writeError(w, http.StatusBadRequest, "path does not exist or is not a directory")
		return
	}
	root, err := s.queries.CreateManagedRoot(r.Context(), filepath.Clean(p))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not create root (already exists?)")
		return
	}
	writeJSON(w, http.StatusCreated, rootResponse{ID: root.ID, Path: root.Path, CreatedAt: root.CreatedAt})
}

func (s *Server) handleListRoots(w http.ResponseWriter, r *http.Request) {
	roots, err := s.queries.ListManagedRoots(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list roots")
		return
	}
	out := []rootResponse{}
	for _, root := range roots {
		out = append(out, rootResponse{ID: root.ID, Path: root.Path, CreatedAt: root.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteRoot(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.queries.DeleteManagedRoot(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete root")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Scanning ---

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

	// Walk the real filesystem under the root.
	files, err := s.host.Walk(root.Path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not scan root")
		return
	}

	seen := map[string]bool{}
	for _, f := range files {
		seen[f.Path] = true
		mod := f.ModifiedAt
		_, err := s.queries.UpsertResource(r.Context(), dbgen.UpsertResourceParams{
			RootID:     root.ID,
			Path:       f.Path,
			Name:       f.Name,
			Size:       f.Size,
			IsDir:      f.IsDir,
			ModifiedAt: &mod,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not index file")
			return
		}
	}

	// Any present Resource under this root not seen on disk → mark Missing.
	existing, err := s.queries.ListResourceIDsByRoot(r.Context(), root.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not reconcile root")
		return
	}
	for _, e := range existing {
		if !seen[e.Path] {
			if err := s.queries.MarkResourceMissing(r.Context(), e.ID); err != nil {
				writeError(w, http.StatusInternalServerError, "could not mark missing")
				return
			}
		}
	}

	s.handleListResources(w, r)
}

// --- Resources ---

func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var resources []dbgen.Resource
	var err error

	if tag := q.Get("tag"); tag != "" {
		ids, idErr := s.queries.ListResourceIDsByTag(r.Context(), tag)
		if idErr != nil {
			writeError(w, http.StatusInternalServerError, "could not filter by tag")
			return
		}
		idset := map[int64]bool{}
		for _, id := range ids {
			idset[id] = true
		}
		all, allErr := s.queries.ListResources(r.Context())
		if allErr != nil {
			writeError(w, http.StatusInternalServerError, "could not list resources")
			return
		}
		for _, res := range all {
			if idset[res.ID] {
				resources = append(resources, res)
			}
		}
	} else if rootStr := q.Get("root_id"); rootStr != "" {
		rootID, convErr := strconv.ParseInt(rootStr, 10, 64)
		if convErr != nil {
			writeError(w, http.StatusBadRequest, "invalid root_id")
			return
		}
		resources, err = s.queries.ListResourcesByRoot(r.Context(), rootID)
	} else {
		resources, err = s.queries.ListResources(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list resources")
		return
	}

	// Optional subdirectory prefix filter.
	prefix := q.Get("dir")

	out := []resourceResponse{}
	for _, res := range resources {
		if prefix != "" && !strings.HasPrefix(res.Path, prefix) {
			continue
		}
		out = append(out, s.toResourceResponse(r, res))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetResource(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, s.toResourceResponse(r, res))
}

type registerRequest struct {
	Path string `json:"path"`
}

func (s *Server) handleRegisterResource(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p := filepath.Clean(strings.TrimSpace(req.Path))
	if p == "" || !filepath.IsAbs(p) {
		writeError(w, http.StatusBadRequest, "absolute path is required")
		return
	}
	// The file must fall inside a Managed Root.
	root, ok := s.rootForPath(r, p)
	if !ok {
		writeError(w, http.StatusBadRequest, "path is not inside any Managed Root")
		return
	}
	fi, err := s.host.Stat(p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "file does not exist")
		return
	}
	mod := fi.ModifiedAt
	res, err := s.queries.UpsertResource(r.Context(), dbgen.UpsertResourceParams{
		RootID:     root.ID,
		Path:       p,
		Name:       fi.Name,
		Size:       fi.Size,
		IsDir:      fi.IsDir,
		ModifiedAt: &mod,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not register resource")
		return
	}
	writeJSON(w, http.StatusCreated, s.toResourceResponse(r, res))
}

// handleClearResource manually removes a Missing Resource from the index.
func (s *Server) handleClearResource(w http.ResponseWriter, r *http.Request) {
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
	if res.State != dbgen.ResourceStateMissing {
		writeError(w, http.StatusBadRequest, "only Missing resources can be cleared")
		return
	}
	if err := s.queries.DeleteResource(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not clear resource")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDownloadResource(w http.ResponseWriter, r *http.Request) {
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
	rc, err := s.host.Open(res.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "underlying file not available")
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+res.Name+"\"")
	_, _ = copyTo(w, rc)
}

// --- Metadata ---

type noteRequest struct {
	Note string `json:"note"`
}

func (s *Server) handleSetNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req noteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := s.queries.UpdateResourceNote(r.Context(), dbgen.UpdateResourceNoteParams{ID: id, Note: req.Note})
	if err != nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeJSON(w, http.StatusOK, s.toResourceResponse(r, res))
}

type tagRequest struct {
	Tag string `json:"tag"`
}

func (s *Server) handleAddTag(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req tagRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tag := strings.TrimSpace(req.Tag)
	if tag == "" {
		writeError(w, http.StatusBadRequest, "tag is required")
		return
	}
	if _, err := s.queries.GetResource(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	if err := s.queries.AddResourceTag(r.Context(), dbgen.AddResourceTagParams{ResourceID: id, Tag: tag}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not add tag")
		return
	}
	res, _ := s.queries.GetResource(r.Context(), id)
	writeJSON(w, http.StatusOK, s.toResourceResponse(r, res))
}

func (s *Server) handleRemoveTag(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tag := chi.URLParam(r, "tag")
	if err := s.queries.RemoveResourceTag(r.Context(), dbgen.RemoveResourceTagParams{ResourceID: id, Tag: tag}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not remove tag")
		return
	}
	res, err := s.queries.GetResource(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	writeJSON(w, http.StatusOK, s.toResourceResponse(r, res))
}

// --- Audit log ---

type auditResponse struct {
	ID         int64     `json:"id"`
	Action     string    `json:"action"`
	ResourceID *int64    `json:"resource_id"`
	OldPath    string    `json:"old_path"`
	NewPath    string    `json:"new_path"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	rows, err := s.queries.ListAuditLog(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list audit log")
		return
	}
	out := []auditResponse{}
	for _, a := range rows {
		var rid *int64
		if a.ResourceID.Valid {
			v := a.ResourceID.Int64
			rid = &v
		}
		out = append(out, auditResponse{
			ID:         a.ID,
			Action:     a.Action,
			ResourceID: rid,
			OldPath:    a.OldPath,
			NewPath:    a.NewPath,
			CreatedAt:  a.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// --- helpers shared with mutation handlers ---

// rootForPath returns the Managed Root that contains p, if any.
func (s *Server) rootForPath(r *http.Request, p string) (dbgen.ManagedRoot, bool) {
	roots, err := s.queries.ListManagedRoots(r.Context())
	if err != nil {
		return dbgen.ManagedRoot{}, false
	}
	clean := filepath.Clean(p)
	for _, root := range roots {
		if pathInRoot(clean, root.Path) {
			return root, true
		}
	}
	return dbgen.ManagedRoot{}, false
}

// pathInRoot reports whether path is equal to or beneath root.
func pathInRoot(path, root string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
