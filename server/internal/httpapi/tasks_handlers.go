package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"nexus/internal/db/dbgen"
)

func (s *Server) registerTaskRoutes(r chi.Router) {
	r.Post("/projects", s.handleCreateProject)
	r.Get("/projects", s.handleListProjects)
	r.Delete("/projects/{id}", s.handleDeleteProject)

	r.Post("/tasks", s.handleCreateTask)
	r.Get("/tasks", s.handleListTasks)
	r.Patch("/tasks/{id}", s.handleUpdateTask)
	r.Delete("/tasks/{id}", s.handleDeleteTask)
}

type projectResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type taskResponse struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Priority  string     `json:"priority"`
	DueAt     *time.Time `json:"due_at"`
	ProjectID *int64     `json:"project_id"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func toTaskResponse(t dbgen.Task) taskResponse {
	var pid *int64
	if t.ProjectID.Valid {
		v := t.ProjectID.Int64
		pid = &v
	}
	return taskResponse{
		ID:        t.ID,
		Title:     t.Title,
		Status:    string(t.Status),
		Priority:  string(t.Priority),
		DueAt:     t.DueAt,
		ProjectID: pid,
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
	}
}

// --- Projects ---

type createProjectRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	p, err := s.queries.CreateProject(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create project")
		return
	}
	writeJSON(w, http.StatusCreated, projectResponse{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.queries.ListProjects(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list projects")
		return
	}
	out := []projectResponse{}
	for _, p := range ps {
		out = append(out, projectResponse{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.queries.DeleteProject(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete project")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Tasks ---

type taskWriteRequest struct {
	Title     *string    `json:"title"`
	Status    *string    `json:"status"`
	Priority  *string    `json:"priority"`
	DueAt     *time.Time `json:"due_at"`
	ProjectID *int64     `json:"project_id"`
}

func validStatus(s string) bool {
	switch dbgen.TaskStatus(s) {
	case dbgen.TaskStatusTodo, dbgen.TaskStatusDoing, dbgen.TaskStatusDone:
		return true
	}
	return false
}

func validPriority(s string) bool {
	switch dbgen.TaskPriority(s) {
	case dbgen.TaskPriorityNone, dbgen.TaskPriorityLow, dbgen.TaskPriorityMedium, dbgen.TaskPriorityHigh:
		return true
	}
	return false
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var req taskWriteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == nil || strings.TrimSpace(*req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	params := dbgen.CreateTaskParams{
		Title:    *req.Title,
		Status:   dbgen.TaskStatusTodo,
		Priority: dbgen.TaskPriorityNone,
	}
	if req.Status != nil {
		if !validStatus(*req.Status) {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		params.Status = dbgen.TaskStatus(*req.Status)
	}
	if req.Priority != nil {
		if !validPriority(*req.Priority) {
			writeError(w, http.StatusBadRequest, "invalid priority")
			return
		}
		params.Priority = dbgen.TaskPriority(*req.Priority)
	}
	params.DueAt = req.DueAt
	if req.ProjectID != nil {
		params.ProjectID = pgtype.Int8{Int64: *req.ProjectID, Valid: true}
	}
	t, err := s.queries.CreateTask(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create task")
		return
	}
	writeJSON(w, http.StatusCreated, toTaskResponse(t))
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	ts, err := s.queries.ListTasks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list tasks")
		return
	}
	out := []taskResponse{}
	for _, t := range ts {
		out = append(out, toTaskResponse(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := s.queries.GetTask(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not load task")
		return
	}

	// Decode into a raw map first so we can tell which fields were present
	// (a present null clears due_at/project_id; an absent field leaves it).
	raw := map[string]json.RawMessage{}
	if err := decodeJSONRaw(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := dbgen.UpdateTaskParams{
		ID:        existing.ID,
		Title:     existing.Title,
		Status:    existing.Status,
		Priority:  existing.Priority,
		DueAt:     existing.DueAt,
		ProjectID: existing.ProjectID,
	}

	if msg, ok := raw["title"]; ok {
		var title string
		if err := json.Unmarshal(msg, &title); err != nil || strings.TrimSpace(title) == "" {
			writeError(w, http.StatusBadRequest, "title cannot be empty")
			return
		}
		params.Title = title
	}
	if msg, ok := raw["status"]; ok {
		var status string
		if err := json.Unmarshal(msg, &status); err != nil || !validStatus(status) {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		params.Status = dbgen.TaskStatus(status)
	}
	if msg, ok := raw["priority"]; ok {
		var priority string
		if err := json.Unmarshal(msg, &priority); err != nil || !validPriority(priority) {
			writeError(w, http.StatusBadRequest, "invalid priority")
			return
		}
		params.Priority = dbgen.TaskPriority(priority)
	}
	if msg, ok := raw["due_at"]; ok {
		var due *time.Time
		if err := json.Unmarshal(msg, &due); err != nil {
			writeError(w, http.StatusBadRequest, "invalid due_at")
			return
		}
		params.DueAt = due
	}
	if msg, ok := raw["project_id"]; ok {
		var pid *int64
		if err := json.Unmarshal(msg, &pid); err != nil {
			writeError(w, http.StatusBadRequest, "invalid project_id")
			return
		}
		if pid != nil {
			params.ProjectID = pgtype.Int8{Int64: *pid, Valid: true}
		} else {
			params.ProjectID = pgtype.Int8{Valid: false}
		}
	}

	t, err := s.queries.UpdateTask(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update task")
		return
	}
	writeJSON(w, http.StatusOK, toTaskResponse(t))
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := s.queries.DeleteTask(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete task")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}
