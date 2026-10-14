package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nexus/internal/db/dbgen"
)

func (s *Server) registerLabelRoutes(r chi.Router) {
	r.Get("/labels", s.handleListLabels)
	r.Post("/labels", s.handleCreateLabel)
	r.Patch("/labels/{id}", s.handleUpdateLabel)
	r.Delete("/labels/{id}", s.handleDeleteLabel)

	r.Post("/tasks/{id}/labels", s.handleAttachTaskLabel)
	r.Delete("/tasks/{id}/labels/{labelId}", s.handleDetachTaskLabel)
}

type labelResponse struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func toLabelResponse(l dbgen.Label) labelResponse {
	return labelResponse{ID: l.ID, Name: l.Name, Color: l.Color}
}

// labelColors is the fixed palette of named color tokens; the UI maps each to
// a chip style. Kept small so labels stay legible in both themes.
var labelColors = map[string]bool{
	"gray": true, "red": true, "orange": true, "amber": true, "green": true,
	"teal": true, "blue": true, "violet": true, "pink": true,
}

const defaultLabelColor = "gray"

// isUniqueViolation reports whether err is a Postgres unique-constraint error.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	ls, err := s.queries.ListLabels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list labels")
		return
	}
	out := []labelResponse{}
	for _, l := range ls {
		out = append(out, toLabelResponse(l))
	}
	writeJSON(w, http.StatusOK, out)
}

type labelWriteRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func (s *Server) handleCreateLabel(w http.ResponseWriter, r *http.Request) {
	var req labelWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	color := defaultLabelColor
	if req.Color != nil {
		if !labelColors[*req.Color] {
			writeError(w, http.StatusBadRequest, "invalid color")
			return
		}
		color = *req.Color
	}
	l, err := s.queries.CreateLabel(r.Context(), dbgen.CreateLabelParams{
		Name:  strings.TrimSpace(*req.Name),
		Color: color,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a label with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create label")
		return
	}
	writeJSON(w, http.StatusCreated, toLabelResponse(l))
}

// handleUpdateLabel renames and/or recolors a label; absent fields are kept.
func (s *Server) handleUpdateLabel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req labelWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	existing, err := s.queries.GetLabel(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "label not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load label")
		return
	}
	params := dbgen.UpdateLabelParams{ID: existing.ID, Name: existing.Name, Color: existing.Color}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			writeError(w, http.StatusBadRequest, "name cannot be empty")
			return
		}
		params.Name = strings.TrimSpace(*req.Name)
	}
	if req.Color != nil {
		if !labelColors[*req.Color] {
			writeError(w, http.StatusBadRequest, "invalid color")
			return
		}
		params.Color = *req.Color
	}
	l, err := s.queries.UpdateLabel(r.Context(), params)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "a label with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not update label")
		return
	}
	writeJSON(w, http.StatusOK, toLabelResponse(l))
}

// handleDeleteLabel removes a label; the cascade detaches it from every Task
// without deleting any Task.
func (s *Server) handleDeleteLabel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	n, err := s.queries.DeleteLabel(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete label")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "label not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type attachLabelRequest struct {
	LabelID int64 `json:"label_id"`
}

// handleAttachTaskLabel attaches a label to a task (idempotent) and returns
// the updated task.
func (s *Server) handleAttachTaskLabel(w http.ResponseWriter, r *http.Request) {
	t, ok := s.loadTask(w, r)
	if !ok {
		return
	}
	var req attachLabelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := s.queries.GetLabel(r.Context(), req.LabelID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "label not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load label")
		return
	}
	if err := s.queries.AttachTaskLabel(r.Context(), dbgen.AttachTaskLabelParams{
		TaskID:  t.ID,
		LabelID: req.LabelID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not attach label")
		return
	}
	s.writeTask(w, r, http.StatusOK, t)
}

// handleDetachTaskLabel removes a label from a task and returns the updated
// task. Detaching a label the task doesn't carry is a no-op.
func (s *Server) handleDetachTaskLabel(w http.ResponseWriter, r *http.Request) {
	t, ok := s.loadTask(w, r)
	if !ok {
		return
	}
	labelID, err := strconv.ParseInt(chi.URLParam(r, "labelId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid label id")
		return
	}
	if _, err := s.queries.GetLabel(r.Context(), labelID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "label not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load label")
		return
	}
	if err := s.queries.DetachTaskLabel(r.Context(), dbgen.DetachTaskLabelParams{
		TaskID:  t.ID,
		LabelID: labelID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not detach label")
		return
	}
	s.writeTask(w, r, http.StatusOK, t)
}
