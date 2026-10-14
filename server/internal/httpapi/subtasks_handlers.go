package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"nexus/internal/db/dbgen"
)

func (s *Server) registerSubtaskRoutes(r chi.Router) {
	r.Post("/tasks/{id}/subtasks", s.handleCreateSubtask)
	r.Post("/tasks/{id}/subtasks/reorder", s.handleReorderSubtasks)
	r.Patch("/subtasks/{id}", s.handleUpdateSubtask)
	r.Delete("/subtasks/{id}", s.handleDeleteSubtask)
}

type subtaskResponse struct {
	ID        int64     `json:"id"`
	TaskID    int64     `json:"task_id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	Position  int32     `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toSubtaskResponse(st dbgen.Subtask) subtaskResponse {
	return subtaskResponse{
		ID:        st.ID,
		TaskID:    st.TaskID,
		Title:     st.Title,
		Done:      st.Done,
		Position:  st.Position,
		CreatedAt: st.CreatedAt,
		UpdatedAt: st.UpdatedAt,
	}
}

type createSubtaskRequest struct {
	Title string `json:"title"`
}

// handleCreateSubtask appends a checklist item to the bottom of a task's list
// and returns the updated parent task.
func (s *Server) handleCreateSubtask(w http.ResponseWriter, r *http.Request) {
	t, ok := s.loadTask(w, r)
	if !ok {
		return
	}
	var req createSubtaskRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if _, err := s.queries.CreateSubtask(r.Context(), dbgen.CreateSubtaskParams{
		TaskID: t.ID,
		Title:  strings.TrimSpace(req.Title),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create subtask")
		return
	}
	s.writeTask(w, r, http.StatusCreated, t)
}

type reorderSubtasksRequest struct {
	IDs []int64 `json:"ids"`
}

// handleReorderSubtasks rewrites a task's checklist order (the ids are the
// full, final top-to-bottom order) and returns the updated parent task.
func (s *Server) handleReorderSubtasks(w http.ResponseWriter, r *http.Request) {
	t, ok := s.loadTask(w, r)
	if !ok {
		return
	}
	var req reorderSubtasksRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.queries.ReorderSubtasks(r.Context(), dbgen.ReorderSubtasksParams{
		TaskID: t.ID,
		Ids:    req.IDs,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not reorder subtasks")
		return
	}
	s.writeTask(w, r, http.StatusOK, t)
}

// handleUpdateSubtask edits a subtask's title and/or toggles done; absent
// fields are kept. Toggling never changes the parent task's status. Returns
// the updated parent task so the card's progress resyncs in one round trip.
func (s *Server) handleUpdateSubtask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := s.queries.GetSubtask(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "subtask not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load subtask")
		return
	}
	raw := map[string]json.RawMessage{}
	if err := decodeJSONRaw(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	params := dbgen.UpdateSubtaskParams{ID: existing.ID, Title: existing.Title, Done: existing.Done}
	if msg, ok := raw["title"]; ok {
		var title string
		if err := json.Unmarshal(msg, &title); err != nil || strings.TrimSpace(title) == "" {
			writeError(w, http.StatusBadRequest, "title cannot be empty")
			return
		}
		params.Title = strings.TrimSpace(title)
	}
	if msg, ok := raw["done"]; ok {
		var done bool
		if err := json.Unmarshal(msg, &done); err != nil {
			writeError(w, http.StatusBadRequest, "invalid done")
			return
		}
		params.Done = done
	}
	if _, err := s.queries.UpdateSubtask(r.Context(), params); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update subtask")
		return
	}
	s.writeParentTask(w, r, existing.TaskID)
}

func (s *Server) handleDeleteSubtask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := s.queries.GetSubtask(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "subtask not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load subtask")
		return
	}
	if _, err := s.queries.DeleteSubtask(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete subtask")
		return
	}
	s.writeParentTask(w, r, existing.TaskID)
}

func (s *Server) writeParentTask(w http.ResponseWriter, r *http.Request, taskID int64) {
	t, err := s.queries.GetTask(r.Context(), taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load task")
		return
	}
	s.writeTask(w, r, http.StatusOK, t)
}
