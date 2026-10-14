package httpapi

import (
	"context"
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
	r.Post("/tasks", s.handleCreateTask)
	r.Get("/tasks", s.handleListTasks)
	r.Post("/tasks/reorder", s.handleReorderTasks)
	r.Post("/tasks/archive-done", s.handleArchiveDoneTasks)
	r.Patch("/tasks/{id}", s.handleUpdateTask)
	r.Delete("/tasks/{id}", s.handleDeleteTask)
	r.Post("/tasks/{id}/archive", s.handleArchiveTask)
	r.Post("/tasks/{id}/unarchive", s.handleUnarchiveTask)

	s.registerLabelRoutes(r)
	s.registerSubtaskRoutes(r)
}

type taskResponse struct {
	ID              int64             `json:"id"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Status          string            `json:"status"`
	Priority        string            `json:"priority"`
	Position        int32             `json:"position"`
	DueAt           *time.Time        `json:"due_at"`
	ArchivedAt      *time.Time        `json:"archived_at"`
	Labels          []labelResponse   `json:"labels"`
	Subtasks        []subtaskResponse `json:"subtasks"`
	SubtaskProgress subtaskProgress   `json:"subtask_progress"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type subtaskProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// toTaskResponses decorates tasks with their labels and subtasks, loading both
// in one batch query each rather than per task.
func toTaskResponses(ctx context.Context, q *dbgen.Queries, ts []dbgen.Task) ([]taskResponse, error) {
	out := make([]taskResponse, 0, len(ts))
	if len(ts) == 0 {
		return out, nil
	}
	ids := make([]int64, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	labelRows, err := q.ListLabelsForTasks(ctx, ids)
	if err != nil {
		return nil, err
	}
	subtaskRows, err := q.ListSubtasksForTasks(ctx, ids)
	if err != nil {
		return nil, err
	}
	labels := map[int64][]labelResponse{}
	for _, l := range labelRows {
		labels[l.TaskID] = append(labels[l.TaskID], labelResponse{ID: l.ID, Name: l.Name, Color: l.Color})
	}
	subtasks := map[int64][]subtaskResponse{}
	for _, st := range subtaskRows {
		subtasks[st.TaskID] = append(subtasks[st.TaskID], toSubtaskResponse(st))
	}

	for _, t := range ts {
		resp := taskResponse{
			ID:          t.ID,
			Title:       t.Title,
			Description: t.Description,
			Status:      string(t.Status),
			Priority:    string(t.Priority),
			Position:    t.Position,
			DueAt:       t.DueAt,
			ArchivedAt:  t.ArchivedAt,
			Labels:      labels[t.ID],
			Subtasks:    subtasks[t.ID],
			CreatedAt:   t.CreatedAt,
			UpdatedAt:   t.UpdatedAt,
		}
		if resp.Labels == nil {
			resp.Labels = []labelResponse{}
		}
		if resp.Subtasks == nil {
			resp.Subtasks = []subtaskResponse{}
		}
		for _, st := range resp.Subtasks {
			if st.Done {
				resp.SubtaskProgress.Done++
			}
		}
		resp.SubtaskProgress.Total = len(resp.Subtasks)
		out = append(out, resp)
	}
	return out, nil
}

// writeTask writes a single decorated task.
func (s *Server) writeTask(w http.ResponseWriter, r *http.Request, status int, t dbgen.Task) {
	out, err := toTaskResponses(r.Context(), s.queries, []dbgen.Task{t})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load task details")
		return
	}
	writeJSON(w, status, out[0])
}

// writeActiveTasks writes the full active board (no label filter). Mutations
// that reshuffle the board return it so the client can resync in one round
// trip.
func (s *Server) writeActiveTasks(w http.ResponseWriter, r *http.Request) {
	ts, err := s.queries.ListTasks(r.Context(), pgtype.Int8{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list tasks")
		return
	}
	out, err := toTaskResponses(r.Context(), s.queries, ts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list tasks")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// loadTask fetches a task by the {id} URL param, writing a 400/404/500 and
// returning ok=false on failure.
func (s *Server) loadTask(w http.ResponseWriter, r *http.Request) (dbgen.Task, bool) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return dbgen.Task{}, false
	}
	t, err := s.queries.GetTask(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return dbgen.Task{}, false
		}
		writeError(w, http.StatusInternalServerError, "could not load task")
		return dbgen.Task{}, false
	}
	return t, true
}

// --- Tasks ---

type taskWriteRequest struct {
	Title       *string    `json:"title"`
	Description *string    `json:"description"`
	Status      *string    `json:"status"`
	Priority    *string    `json:"priority"`
	DueAt       *time.Time `json:"due_at"`
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
	case dbgen.TaskPriorityNone, dbgen.TaskPriorityLow, dbgen.TaskPriorityMedium,
		dbgen.TaskPriorityHigh, dbgen.TaskPriorityUrgent:
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
	if req.Description != nil {
		params.Description = *req.Description
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
	t, err := s.queries.CreateTask(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create task")
		return
	}
	s.writeTask(w, r, http.StatusCreated, t)
}

// handleListTasks returns the active board by default, or the archived list
// (newest-archived first) with ?archived=true. ?label_id= narrows either to
// tasks carrying that label; the filter runs in SQL.
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var labelID pgtype.Int8
	if v := q.Get("label_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid label_id")
			return
		}
		labelID = pgtype.Int8{Int64: id, Valid: true}
	}
	archived := false
	if v := q.Get("archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid archived")
			return
		}
		archived = b
	}

	var ts []dbgen.Task
	var err error
	if archived {
		ts, err = s.queries.ListArchivedTasks(r.Context(), labelID)
	} else {
		ts, err = s.queries.ListTasks(r.Context(), labelID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list tasks")
		return
	}
	out, err := toTaskResponses(r.Context(), s.queries, ts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list tasks")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type reorderRequest struct {
	Status string  `json:"status"`
	IDs    []int64 `json:"ids"`
}

// handleReorderTasks rewrites one column's ordering. The ids are the full,
// final top-to-bottom order of the column; tasks dragged in from another
// column are moved into this status at their new position. Archived tasks are
// ignored. Returns the full active board so the client can resync in one
// round trip.
func (s *Server) handleReorderTasks(w http.ResponseWriter, r *http.Request) {
	var req reorderRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validStatus(req.Status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := s.queries.ReorderTasks(r.Context(), dbgen.ReorderTasksParams{
		Status: dbgen.TaskStatus(req.Status),
		Ids:    req.IDs,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "could not reorder tasks")
		return
	}
	s.writeActiveTasks(w, r)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	existing, ok := s.loadTask(w, r)
	if !ok {
		return
	}

	// Decode into a raw map first so we can tell which fields were present
	// (a present null clears due_at; an absent field leaves it).
	raw := map[string]json.RawMessage{}
	if err := decodeJSONRaw(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := dbgen.UpdateTaskParams{
		ID:          existing.ID,
		Title:       existing.Title,
		Description: existing.Description,
		Status:      existing.Status,
		Priority:    existing.Priority,
		DueAt:       existing.DueAt,
	}

	if msg, ok := raw["title"]; ok {
		var title string
		if err := json.Unmarshal(msg, &title); err != nil || strings.TrimSpace(title) == "" {
			writeError(w, http.StatusBadRequest, "title cannot be empty")
			return
		}
		params.Title = title
	}
	if msg, ok := raw["description"]; ok {
		var description string
		if err := json.Unmarshal(msg, &description); err != nil {
			writeError(w, http.StatusBadRequest, "invalid description")
			return
		}
		params.Description = description
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

	t, err := s.queries.UpdateTask(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update task")
		return
	}
	s.writeTask(w, r, http.StatusOK, t)
}

// handleDeleteTask permanently removes a task (and, by cascade, its subtasks
// and label links). Unlike archive, this is not reversible.
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

// --- Archive ---

// handleArchiveTask hides a task from the board without deleting it.
func (s *Server) handleArchiveTask(w http.ResponseWriter, r *http.Request) {
	existing, ok := s.loadTask(w, r)
	if !ok {
		return
	}
	t, err := s.queries.ArchiveTask(r.Context(), existing.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not archive task")
		return
	}
	s.writeTask(w, r, http.StatusOK, t)
}

// handleUnarchiveTask returns an archived task to the bottom of its column.
func (s *Server) handleUnarchiveTask(w http.ResponseWriter, r *http.Request) {
	existing, ok := s.loadTask(w, r)
	if !ok {
		return
	}
	t, err := s.queries.UnarchiveTask(r.Context(), existing.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not unarchive task")
		return
	}
	s.writeTask(w, r, http.StatusOK, t)
}

// handleArchiveDoneTasks archives every active task in the Done column and
// returns the refreshed active board.
func (s *Server) handleArchiveDoneTasks(w http.ResponseWriter, r *http.Request) {
	if _, err := s.queries.ArchiveDoneTasks(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "could not archive done tasks")
		return
	}
	s.writeActiveTasks(w, r)
}

// --- helpers ---

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}
