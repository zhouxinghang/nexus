package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"nexus/internal/db/dbgen"
)

func (s *Server) registerFileRoutes(r chi.Router) {
	// Managed Roots
	r.Post("/files/roots", s.handleCreateRoot)
	r.Get("/files/roots", s.handleListRoots)
	r.Delete("/files/roots/{id}", s.handleDeleteRoot)
	r.Post("/files/roots/{id}/scan", s.handleScanRoot)
	r.Get("/files/roots/{id}/scan", s.handleScanStatus)

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

	// Batch mutations: one request operates on many Resources so the client
	// never fans out N round-trips. The static "batch" segment is matched
	// ahead of the "{id}" wildcard by chi.
	r.Post("/files/resources/batch/move", s.handleBatchMove)
	r.Post("/files/resources/batch/tag", s.handleBatchAddTag)
	r.Post("/files/resources/batch/delete", s.handleBatchDelete)

	// Audit log
	r.Get("/files/audit", s.handleListAudit)
}

// --- response shapes ---

// resourcePage is the Files offset envelope. It extends the shared offset
// page with per-kind counts so the UI's type-filter chips can show totals for
// the whole result set rather than only the loaded rows.
type resourcePage struct {
	offsetPage[resourceResponse]
	Kinds map[string]int64 `json:"kinds"`
}

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
	Kind       string     `json:"kind"`
	Note       string     `json:"note"`
	Tags       []string   `json:"tags"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// toResourceResponses converts a page of Resources, loading every row's tags
// in one query rather than one per row.
func (s *Server) toResourceResponses(r *http.Request, rs []dbgen.Resource) ([]resourceResponse, error) {
	out := make([]resourceResponse, 0, len(rs))
	if len(rs) == 0 {
		return out, nil
	}
	ids := make([]int64, len(rs))
	for i, res := range rs {
		ids[i] = res.ID
	}
	rows, err := s.queries.ListTagsForResources(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	tags := map[int64][]string{}
	for _, t := range rows {
		tags[t.ResourceID] = append(tags[t.ResourceID], t.Tag)
	}
	for _, res := range rs {
		out = append(out, newResourceResponse(res, tags[res.ID]))
	}
	return out, nil
}

func (s *Server) toResourceResponse(r *http.Request, res dbgen.Resource) resourceResponse {
	tags, _ := s.queries.ListTagsForResource(r.Context(), res.ID)
	return newResourceResponse(res, tags)
}

func newResourceResponse(res dbgen.Resource, tags []string) resourceResponse {
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
		Kind:       res.Kind,
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
//
// The scan implementation (start/status handlers, the background runner, and
// its batching/diffing helpers) lives in files_scan.go.

// --- Resources ---

// Files page sizes: the default page and the hard cap on ?limit=.
const (
	resourcePageDefault = 200
	resourcePageMax     = 1000
)

// handleListResources returns one offset page of Resources, ordered by path,
// for the optional root_id / dir prefix / tag / kind filters:
// {"items": [...], "total": n, "limit": n, "offset": n, "kinds": {...}}. Every
// filter, the slice, the count, and the per-kind counts run in SQL.
//
// Offset (not keyset) is a deliberate first pass: the optional prefix + tag
// combinations make a stable keyset cursor awkward, and offset is adequate
// for one Host's file counts. Revisit with a (path) keyset if lists grow
// large enough for deep offsets to hurt.
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var rootID pgtype.Int8
	if rootStr := q.Get("root_id"); rootStr != "" {
		id, err := strconv.ParseInt(rootStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid root_id")
			return
		}
		rootID = pgtype.Int8{Int64: id, Valid: true}
	}
	s.writeResourcePage(w, r, rootID)
}

// writeResourcePage writes the paged resource list for the request's dir /
// tag / kind / limit / offset params, scoped to rootID when it is set. The
// envelope carries per-kind counts (for the other active filters) so the UI's
// type chips reflect the full result set, not just the loaded page.
func (s *Server) writeResourcePage(w http.ResponseWriter, r *http.Request, rootID pgtype.Int8) {
	q := r.URL.Query()
	limit, err := parseLimit(q, resourcePageDefault, resourcePageMax)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit")
		return
	}
	offset, err := parseOffset(q)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid offset")
		return
	}
	var dir, tag, kind pgtype.Text
	if v := q.Get("dir"); v != "" {
		dir = pgtype.Text{String: likePrefix(v), Valid: true}
	}
	if v := q.Get("tag"); v != "" {
		tag = pgtype.Text{String: v, Valid: true}
	}
	if v := q.Get("kind"); v != "" {
		if !validKind(v) {
			writeError(w, http.StatusBadRequest, "invalid kind")
			return
		}
		kind = pgtype.Text{String: v, Valid: true}
	}

	total, err := s.queries.CountResources(r.Context(), dbgen.CountResourcesParams{
		RootID: rootID, DirPattern: dir, Tag: tag, Kind: kind,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not count resources")
		return
	}
	// Per-kind counts ignore the kind filter so every chip stays visible and
	// clickable while one kind is selected.
	kindRows, err := s.queries.CountResourcesByKind(r.Context(), dbgen.CountResourcesByKindParams{
		RootID: rootID, DirPattern: dir, Tag: tag,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not count resources by kind")
		return
	}
	kinds := map[string]int64{}
	for _, kr := range kindRows {
		kinds[kr.Kind] = kr.Count
	}
	resources, err := s.queries.ListResourcesPage(r.Context(), dbgen.ListResourcesPageParams{
		RootID: rootID, DirPattern: dir, Tag: tag, Kind: kind,
		PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list resources")
		return
	}
	items, err := s.toResourceResponses(r, resources)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load tags")
		return
	}
	writeJSON(w, http.StatusOK, resourcePage{
		offsetPage: offsetPage[resourceResponse]{
			Items: items, Total: total, Limit: limit, Offset: offset,
		},
		Kinds: kinds,
	})
}

// resourceKinds is the fixed Files taxonomy, matching the resource_kind SQL
// function and the client's chips. Used to validate the ?kind= filter.
var resourceKinds = map[string]bool{
	"folder": true, "document": true, "image": true, "video": true,
	"audio": true, "spreadsheet": true, "presentation": true, "code": true,
	"archive": true, "other": true,
}

func validKind(k string) bool { return resourceKinds[k] }

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
