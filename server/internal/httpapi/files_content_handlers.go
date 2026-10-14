package httpapi

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"nexus/internal/db/dbgen"
	"nexus/internal/host"
)

type previewResponse struct {
	Type       string    `json:"type"`
	Mime       string    `json:"mime"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
	Editable   bool      `json:"editable"`
	Reason     string    `json:"reason,omitempty"`
}

type textContentResponse struct {
	Content  string `json:"content"`
	Revision string `json:"revision"`
	Encoding string `json:"encoding"`
}

// openResourceContent uses the indexed id only for locating a Resource. Disk
// metadata and root confinement are rechecked on every preview/read/save.
func (s *Server) openResourceContent(w http.ResponseWriter, r *http.Request) (dbgen.Resource, dbgen.ManagedRoot, io.ReadSeekCloser, host.FileInfo, bool) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return dbgen.Resource{}, dbgen.ManagedRoot{}, nil, host.FileInfo{}, false
	}
	res, err := s.queries.GetResource(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "resource not found")
		return res, dbgen.ManagedRoot{}, nil, host.FileInfo{}, false
	}
	root, err := s.queries.GetManagedRoot(r.Context(), res.RootID)
	if err != nil || !pathInRoot(res.Path, root.Path) {
		writeError(w, http.StatusForbidden, "resource is outside its Managed Root")
		return res, root, nil, host.FileInfo{}, false
	}
	f, fi, editable, err := s.host.OpenWithinRoot(root.Path, res.Path)
	if err != nil {
		writeContentError(w, err)
		return res, root, nil, fi, false
	}
	return res, root, f, fi, editable
}

func writeContentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, host.ErrFileChanged):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, host.ErrTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, host.ErrUnsafePath), errors.Is(err, host.ErrSymlink):
		writeError(w, http.StatusForbidden, "file cannot be accessed safely for this operation within its Managed Root")
	case errors.Is(err, os.ErrPermission):
		writeError(w, http.StatusForbidden, "file permissions do not allow this operation")
	case errors.Is(err, host.ErrNotRegular):
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, io.EOF):
		writeError(w, http.StatusBadRequest, "could not read file")
	case errors.Is(err, os.ErrNotExist):
		writeError(w, http.StatusNotFound, "underlying file is unavailable")
	default:
		writeError(w, http.StatusInternalServerError, "could not read or save the underlying file")
	}
}

// Preview classification is independent of the listing taxonomy: a .docx
// isn't text just because it is a "document", and extensionless UTF-8 is text.
// Only browser-safe formats are served inline; HTML/XML always stay source.
func describePreview(name string, fi host.FileInfo, editable bool, data []byte) previewResponse {
	p := previewResponse{Type: "unsupported", Mime: "application/octet-stream", Size: fi.Size, ModifiedAt: fi.ModifiedAt}
	ext := strings.ToLower(filepath.Ext(name))
	mediaType := mime.TypeByExtension(ext)
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}
	mediaType, _, _ = mime.ParseMediaType(mediaType)
	// The OS commonly maps ".ts" to MPEG transport streams. In a workbench,
	// UTF-8 TypeScript source must not be mistaken for a video.
	if ext == ".ts" {
		mediaType = "text/plain"
	}
	switch {
	case ext == ".pdf" && bytes.HasPrefix(data, []byte("%PDF-")):
		p.Type, p.Mime = "pdf", "application/pdf"
	case strings.HasPrefix(mediaType, "image/") && mediaType != "image/svg+xml":
		p.Type, p.Mime = "image", mediaType
	case strings.HasPrefix(mediaType, "video/"):
		p.Type, p.Mime = "video", mediaType
	case strings.HasPrefix(mediaType, "audio/"):
		p.Type, p.Mime = "audio", mediaType
	default:
		if fi.Size > host.MaxTextFileSize {
			p.Reason = "Text preview and editing are limited to 2 MiB. Download this file to open it locally."
		} else if isText(data) {
			p.Type, p.Mime, p.Editable = "text", "text/plain; charset=utf-8", editable
			if !editable {
				p.Reason = "This file is read-only (a symbolic link or no write permission)."
			}
		} else {
			p.Reason = "This format or text encoding cannot be previewed. Download it to open it locally."
		}
	}
	return p
}

func isText(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, b := range data {
		// UTF-16, binary payloads and non-printing control bytes are not text.
		if b < 32 && b != '\t' && b != '\n' && b != '\r' && b != '\f' {
			return false
		}
	}
	return true
}

func readPreview(f io.ReadSeeker, name string, fi host.FileInfo, editable bool) (previewResponse, []byte, error) {
	// Sample first so media previews don't buffer whole videos. Text validation
	// is full-file and bounded: a binary byte beyond the header is still rejected.
	header := make([]byte, 512)
	n, err := io.ReadFull(f, header)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return previewResponse{}, nil, err
	}
	p := describePreview(name, fi, editable, header[:n])
	if p.Type == "text" || (p.Type == "unsupported" && fi.Size <= host.MaxTextFileSize) {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return p, nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, host.MaxTextFileSize+1))
		if err != nil {
			return p, nil, err
		}
		fi.Size = int64(len(data))
		p = describePreview(name, fi, editable, data)
		return p, data, nil
	}
	return p, nil, nil
}

func (s *Server) handleResourcePreview(w http.ResponseWriter, r *http.Request) {
	res, _, f, fi, editable := s.openResourceContent(w, r)
	if f == nil {
		return
	}
	defer f.Close()
	p, _, err := readPreview(f, res.Name, fi, editable)
	if err != nil {
		writeContentError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleResourceContent(w http.ResponseWriter, r *http.Request) {
	res, _, f, fi, editable := s.openResourceContent(w, r)
	if f == nil {
		return
	}
	defer f.Close()
	p, data, err := readPreview(f, res.Name, fi, editable)
	if err != nil {
		writeContentError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if p.Type == "text" {
		writeJSON(w, http.StatusOK, textContentResponse{Content: string(data), Revision: host.ContentRevision(data), Encoding: "UTF-8"})
		return
	}
	if p.Type == "unsupported" {
		writeError(w, http.StatusUnsupportedMediaType, p.Reason)
		return
	}
	w.Header().Set("Content-Type", p.Mime)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": res.Name}))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'self'")
	if p.Type != "pdf" {
		// A sandboxed response disables browser plugins, including native PDF
		// viewers. PDF is identified by its magic and served only as PDF.
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	}
	// ServeContent supports Range/HEAD seeking for audio, video and PDF viewers.
	http.ServeContent(w, r, res.Name, fi.ModifiedAt, f)
}

func (s *Server) handleSaveResourceContent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Content  *string `json:"content"`
		Revision string  `json:"revision"`
	}
	// JSON escaping can expand a byte sixfold; enforce both the wire limit and
	// decoded UTF-8 byte limit. A missing content field is not an empty file.
	r.Body = http.MaxBytesReader(w, r.Body, 6*host.MaxTextFileSize+1024)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body is too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return
	}
	defer r.Body.Close()
	_, revisionErr := hex.DecodeString(req.Revision)
	if dec.Decode(new(any)) != io.EOF || req.Content == nil || len(req.Revision) != 64 || revisionErr != nil {
		writeError(w, http.StatusBadRequest, "content and a valid revision are required")
		return
	}
	data := []byte(*req.Content)
	if len(data) > host.MaxTextFileSize {
		writeContentError(w, host.ErrTooLarge)
		return
	}
	if !isText(data) {
		writeError(w, http.StatusUnsupportedMediaType, "only UTF-8 text can be saved")
		return
	}
	lock := &s.fileEdits[id%int64(len(s.fileEdits))]
	lock.Lock()
	defer lock.Unlock()
	res, root, f, fi, editable := s.openResourceContent(w, r)
	if f == nil {
		return
	}
	p, _, err := readPreview(f, res.Name, fi, editable)
	f.Close()
	if err != nil {
		writeContentError(w, err)
		return
	}
	if !p.Editable {
		writeError(w, http.StatusUnsupportedMediaType, "only writable UTF-8 text files up to 2 MiB can be edited")
		return
	}
	fi, err = s.host.WriteText(root.Path, res.Path, data, req.Revision)
	if err != nil {
		writeContentError(w, err)
		return
	}
	// Refresh only the content metadata of this row; tags/id survive.
	err = s.inTx(r, func(q *dbgen.Queries) error {
		if _, err := q.UpsertResource(r.Context(), dbgen.UpsertResourceParams{
			RootID: res.RootID, Path: res.Path, Name: res.Name,
			Size: fi.Size, ModifiedAt: &fi.ModifiedAt,
		}); err != nil {
			return err
		}
		_, err := q.InsertAuditLog(r.Context(), dbgen.InsertAuditLogParams{
			Action: "edit", ResourceID: pgtype.Int8{Int64: res.ID, Valid: true},
			OldPath: res.Path, NewPath: res.Path,
		})
		return err
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "file was saved, but the index could not be updated; reload or scan the Managed Root")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, textContentResponse{Content: string(data), Revision: host.ContentRevision(data), Encoding: "UTF-8"})
}
