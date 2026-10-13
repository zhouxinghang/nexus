package httpapi_test

import (
	"net/http"
	"testing"
	"time"
)

type taskResp struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Priority    string     `json:"priority"`
	Position    int32      `json:"position"`
	DueAt       *time.Time `json:"due_at"`
	ProjectID   *int64     `json:"project_id"`
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
	if task.Description != "" {
		t.Errorf("default description = %q, want empty", task.Description)
	}
}

func TestTaskCreateWithDescriptionAndPriority(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]interface{}{
		"title":       "write spec",
		"description": "cover the edge cases",
		"priority":    "high",
	})
	mustStatus(t, resp, http.StatusCreated)
	var task taskResp
	decode(t, resp, &task)
	if task.Description != "cover the edge cases" {
		t.Errorf("description = %q", task.Description)
	}
	if task.Priority != "high" {
		t.Errorf("priority = %q, want high", task.Priority)
	}
}

func TestTaskSetPriority(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "triage"})
	var task taskResp
	decode(t, resp, &task)

	for _, p := range []string{"low", "medium", "high", "urgent", "none"} {
		r := h.do(http.MethodPatch, path("/api/tasks/", task.ID), map[string]string{"priority": p})
		mustStatus(t, r, http.StatusOK)
		var updated taskResp
		decode(t, r, &updated)
		if updated.Priority != p {
			t.Errorf("priority = %q, want %q", updated.Priority, p)
		}
	}
}

func TestTaskRejectsInvalidPriority(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "x"})
	var task taskResp
	decode(t, resp, &task)
	r := h.do(http.MethodPatch, path("/api/tasks/", task.ID), map[string]string{"priority": "critical"})
	mustStatus(t, r, http.StatusBadRequest)
}

func TestTaskEditDescription(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "note taking"})
	var task taskResp
	decode(t, resp, &task)
	r := h.do(http.MethodPatch, path("/api/tasks/", task.ID), map[string]string{
		"description": "flesh out the details here",
	})
	mustStatus(t, r, http.StatusOK)
	var updated taskResp
	decode(t, r, &updated)
	if updated.Description != "flesh out the details here" {
		t.Errorf("description = %q", updated.Description)
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

func TestTaskSetDueDate(t *testing.T) {
	h := newHarness(t)
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": "deadline task"})
	var task taskResp
	decode(t, resp, &task)

	due := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	r := h.do(http.MethodPatch, path("/api/tasks/", task.ID), map[string]interface{}{
		"due_at": due,
	})
	mustStatus(t, r, http.StatusOK)
	var updated taskResp
	decode(t, r, &updated)
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

// newTask is a small helper that creates a task and returns it.
func (h *harness) newTask(title string) taskResp {
	h.t.Helper()
	resp := h.do(http.MethodPost, "/api/tasks", map[string]string{"title": title})
	mustStatus(h.t, resp, http.StatusCreated)
	var task taskResp
	decode(h.t, resp, &task)
	return task
}

// tasksByStatus filters the current task list down to one column.
func (h *harness) tasksByStatus(status string) []taskResp {
	h.t.Helper()
	lr := h.do(http.MethodGet, "/api/tasks", nil)
	mustStatus(h.t, lr, http.StatusOK)
	var all []taskResp
	decode(h.t, lr, &all)
	out := []taskResp{}
	for _, tk := range all {
		if tk.Status == status {
			out = append(out, tk)
		}
	}
	return out
}

func TestTaskReorderWithinColumn(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("a")
	b := h.newTask("b")
	c := h.newTask("c")

	// Created in order a,b,c — their positions should already be ascending.
	todo := h.tasksByStatus("todo")
	if len(todo) != 3 || todo[0].ID != a.ID || todo[2].ID != c.ID {
		t.Fatalf("initial order = %+v", todo)
	}

	// Reorder to c,a,b.
	r := h.do(http.MethodPost, "/api/tasks/reorder", map[string]interface{}{
		"status": "todo",
		"ids":    []int64{c.ID, a.ID, b.ID},
	})
	mustStatus(t, r, http.StatusOK)

	todo = h.tasksByStatus("todo")
	gotOrder := []int64{todo[0].ID, todo[1].ID, todo[2].ID}
	wantOrder := []int64{c.ID, a.ID, b.ID}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("order after reorder = %v, want %v", gotOrder, wantOrder)
		}
	}
}

func TestTaskReorderMovesBetweenColumns(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("a")
	b := h.newTask("b")

	// Move b into the "doing" column at the top.
	r := h.do(http.MethodPost, "/api/tasks/reorder", map[string]interface{}{
		"status": "doing",
		"ids":    []int64{b.ID},
	})
	mustStatus(t, r, http.StatusOK)

	doing := h.tasksByStatus("doing")
	if len(doing) != 1 || doing[0].ID != b.ID {
		t.Fatalf("doing column = %+v, want [b]", doing)
	}
	if doing[0].Status != "doing" {
		t.Errorf("moved task status = %q, want doing", doing[0].Status)
	}

	todo := h.tasksByStatus("todo")
	if len(todo) != 1 || todo[0].ID != a.ID {
		t.Fatalf("todo column = %+v, want [a]", todo)
	}
}

func TestTaskReorderRejectsInvalidStatus(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("a")
	r := h.do(http.MethodPost, "/api/tasks/reorder", map[string]interface{}{
		"status": "archived",
		"ids":    []int64{a.ID},
	})
	mustStatus(t, r, http.StatusBadRequest)
}
