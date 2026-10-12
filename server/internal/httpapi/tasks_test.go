package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

type taskResp struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Priority  string     `json:"priority"`
	DueAt     *time.Time `json:"due_at"`
	ProjectID *int64     `json:"project_id"`
}

type projectResp struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func TestTaskCreateWithTitle(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "buy milk"})
	mustStatus(t, resp, http.StatusCreated)
	var task taskResp
	decode(t, resp, &task)
	if task.Title != "buy milk" {
		t.Errorf("title = %q", task.Title)
	}
	if task.Status != "todo" {
		t.Errorf("default status = %q, want todo", task.Status)
	}
	if task.Priority != "none" {
		t.Errorf("default priority = %q, want none", task.Priority)
	}
}

func TestTaskStatusTransitions(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "ship it"})
	mustStatus(t, resp, http.StatusCreated)
	var task taskResp
	decode(t, resp, &task)

	for _, status := range []string{"doing", "done", "todo"} {
		r := h.do(http.MethodPatch, path("/api/tasks/", task.ID), map[string]string{"status": status})
		mustStatus(t, r, http.StatusOK)
		var updated taskResp
		decode(t, r, &updated)
		if updated.Status != status {
			t.Errorf("status = %q, want %q", updated.Status, status)
		}
	}
}

func TestTaskRejectsInvalidStatus(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "x"})
	var task taskResp
	decode(t, resp, &task)
	r := h.do(http.MethodPatch, path("/api/tasks/", task.ID), map[string]string{"status": "archived"})
	mustStatus(t, r, http.StatusBadRequest)
}

func TestTaskSetPriorityAndDueDate(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "deadline task"})
	var task taskResp
	decode(t, resp, &task)

	due := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	r := h.do(http.MethodPatch, path("/api/tasks/", task.ID), map[string]interface{}{
		"priority": "high",
		"due_at":   due,
	})
	mustStatus(t, r, http.StatusOK)
	var updated taskResp
	decode(t, r, &updated)
	if updated.Priority != "high" {
		t.Errorf("priority = %q, want high", updated.Priority)
	}
	if updated.DueAt == nil || !updated.DueAt.Equal(due) {
		t.Errorf("due_at = %v, want %v", updated.DueAt, due)
	}
}

func TestTaskGroupIntoProject(t *testing.T) {
	h := newHarness(t)
	pr := h.do(http.MethodPost, "/api/projects", map[string]string{"name": "Home"})
	mustStatus(t, pr, http.StatusCreated)
	var project projectResp
	decode(t, pr, &project)

	resp := h.do(http.MethodPost, "/api/tasks", map[string]interface{}{
		"title":      "mow the lawn",
		"project_id": project.ID,
	})
	mustStatus(t, resp, http.StatusCreated)
	var task taskResp
	decode(t, resp, &task)
	if task.ProjectID == nil || *task.ProjectID != project.ID {
		t.Errorf("project_id = %v, want %d", task.ProjectID, project.ID)
	}

	// List projects includes it.
	lr := h.do(http.MethodGet, "/api/projects", nil)
	mustStatus(t, lr, http.StatusOK)
	var projects []projectResp
	decode(t, lr, &projects)
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
}

func TestTaskEditTitle(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "typpo"})
	var task taskResp
	decode(t, resp, &task)
	r := h.do(http.MethodPatch, path("/api/tasks/", task.ID), map[string]string{"title": "typo fixed"})
	mustStatus(t, r, http.StatusOK)
	var updated taskResp
	decode(t, r, &updated)
	if updated.Title != "typo fixed" {
		t.Errorf("title = %q", updated.Title)
	}
}

func TestTaskDelete(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "obsolete"})
	var task taskResp
	decode(t, resp, &task)

	r := h.do(http.MethodDelete, path("/api/tasks/", task.ID), nil)
	mustStatus(t, r, http.StatusNoContent)

	lr := h.do(http.MethodGet, "/api/tasks", nil)
	var tasks []taskResp
	decode(t, lr, &tasks)
	for _, tk := range tasks {
		if tk.ID == task.ID {
			t.Error("deleted task still present in list")
		}
	}
}
