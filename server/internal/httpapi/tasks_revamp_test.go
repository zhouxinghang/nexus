package httpapi_test

import (
	"net/http"
	"testing"
)

type labelResp struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type subtaskResp struct {
	ID       int64  `json:"id"`
	TaskID   int64  `json:"task_id"`
	Title    string `json:"title"`
	Done     bool   `json:"done"`
	Position int32  `json:"position"`
}

// listTasks fetches GET /api/tasks with an optional query string.
func (h *harness) listTasks(query string) []taskResp {
	h.t.Helper()
	r := h.do(http.MethodGet, "/api/tasks"+query, nil)
	mustStatus(h.t, r, http.StatusOK)
	var out []taskResp
	decode(h.t, r, &out)
	return out
}

func containsTask(ts []taskResp, id int64) bool {
	for _, t := range ts {
		if t.ID == id {
			return true
		}
	}
	return false
}

func (h *harness) newLabel(name, color string) labelResp {
	h.t.Helper()
	r := h.do(http.MethodPost, "/api/labels", map[string]string{"name": name, "color": color})
	mustStatus(h.t, r, http.StatusCreated)
	var l labelResp
	decode(h.t, r, &l)
	return l
}

// --- Archive ---

func TestTaskArchiveHidesFromBoard(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("keep")
	b := h.newTask("tidy away")

	r := h.do(http.MethodPost, path("/api/tasks/", b.ID)+"/archive", nil)
	mustStatus(t, r, http.StatusOK)
	var archived taskResp
	decode(t, r, &archived)
	if archived.ArchivedAt == nil {
		t.Fatal("archived_at not set")
	}

	board := h.listTasks("")
	if containsTask(board, b.ID) || !containsTask(board, a.ID) {
		t.Errorf("board after archive = %+v, want only a", board)
	}
	arch := h.listTasks("?archived=true")
	if len(arch) != 1 || arch[0].ID != b.ID {
		t.Errorf("archived list = %+v, want [b]", arch)
	}

	r = h.do(http.MethodPost, path("/api/tasks/", b.ID)+"/unarchive", nil)
	mustStatus(t, r, http.StatusOK)
	var restored taskResp
	decode(t, r, &restored)
	if restored.ArchivedAt != nil {
		t.Error("archived_at still set after unarchive")
	}
	if !containsTask(h.listTasks(""), b.ID) {
		t.Error("unarchived task not back on the board")
	}
	if len(h.listTasks("?archived=true")) != 0 {
		t.Error("archived list should be empty after unarchive")
	}
}

func TestTaskArchivedListNewestFirst(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("first archived")
	b := h.newTask("second archived")
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", a.ID)+"/archive", nil), http.StatusOK)
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", b.ID)+"/archive", nil), http.StatusOK)

	arch := h.listTasks("?archived=true")
	if len(arch) != 2 || arch[0].ID != b.ID || arch[1].ID != a.ID {
		t.Errorf("archived order = %+v, want [b a]", arch)
	}
}

func TestTaskUnarchiveLandsAtBottom(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("a")
	b := h.newTask("b")
	c := h.newTask("c")
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", a.ID)+"/archive", nil), http.StatusOK)
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", a.ID)+"/unarchive", nil), http.StatusOK)

	todo := h.tasksByStatus("todo")
	if len(todo) != 3 || todo[0].ID != b.ID || todo[1].ID != c.ID || todo[2].ID != a.ID {
		t.Errorf("todo order after unarchive = %+v, want [b c a]", todo)
	}
}

func TestTaskArchiveDoneBulk(t *testing.T) {
	h := newHarness(t)
	todo := h.newTask("todo")
	doing := h.newTask("doing")
	done1 := h.newTask("done1")
	done2 := h.newTask("done2")
	mustStatus(t, h.do(http.MethodPatch, path("/api/tasks/", doing.ID), map[string]string{"status": "doing"}), http.StatusOK)
	mustStatus(t, h.do(http.MethodPatch, path("/api/tasks/", done1.ID), map[string]string{"status": "done"}), http.StatusOK)
	mustStatus(t, h.do(http.MethodPatch, path("/api/tasks/", done2.ID), map[string]string{"status": "done"}), http.StatusOK)

	r := h.do(http.MethodPost, "/api/tasks/archive-done", nil)
	mustStatus(t, r, http.StatusOK)
	var board []taskResp
	decode(t, r, &board)
	if len(board) != 2 || !containsTask(board, todo.ID) || !containsTask(board, doing.ID) {
		t.Errorf("board after archive-done = %+v, want [todo doing]", board)
	}
	arch := h.listTasks("?archived=true")
	if len(arch) != 2 || !containsTask(arch, done1.ID) || !containsTask(arch, done2.ID) {
		t.Errorf("archived = %+v, want the two done tasks", arch)
	}
}

func TestTaskArchiveMissingIs404(t *testing.T) {
	h := newHarness(t)
	mustStatus(t, h.do(http.MethodPost, "/api/tasks/9999/archive", nil), http.StatusNotFound)
	mustStatus(t, h.do(http.MethodPost, "/api/tasks/9999/unarchive", nil), http.StatusNotFound)
}

func TestTaskDeleteStaysPermanent(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("gone for good")
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", a.ID)+"/archive", nil), http.StatusOK)
	mustStatus(t, h.do(http.MethodDelete, path("/api/tasks/", a.ID), nil), http.StatusNoContent)

	if containsTask(h.listTasks(""), a.ID) || containsTask(h.listTasks("?archived=true"), a.ID) {
		t.Error("deleted task still listed")
	}
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", a.ID)+"/unarchive", nil), http.StatusNotFound)
}

func TestTaskReorderIgnoresArchived(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("a")
	b := h.newTask("b")
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", b.ID)+"/archive", nil), http.StatusOK)

	r := h.do(http.MethodPost, "/api/tasks/reorder", map[string]interface{}{
		"status": "doing",
		"ids":    []int64{b.ID, a.ID},
	})
	mustStatus(t, r, http.StatusOK)
	arch := h.listTasks("?archived=true")
	if len(arch) != 1 || arch[0].Status != "todo" {
		t.Errorf("archived task was moved by reorder: %+v", arch)
	}
}

// --- Labels ---

func TestLabelCRUD(t *testing.T) {
	h := newHarness(t)
	l := h.newLabel("work", "blue")
	if l.Name != "work" || l.Color != "blue" {
		t.Errorf("created label = %+v", l)
	}

	// Uniqueness.
	mustStatus(t, h.do(http.MethodPost, "/api/labels", map[string]string{"name": "work"}), http.StatusConflict)
	// Validation.
	mustStatus(t, h.do(http.MethodPost, "/api/labels", map[string]string{"name": "  "}), http.StatusBadRequest)
	mustStatus(t, h.do(http.MethodPost, "/api/labels", map[string]string{"name": "x", "color": "chartreuse"}), http.StatusBadRequest)

	// Rename + recolor.
	r := h.do(http.MethodPatch, path("/api/labels/", l.ID), map[string]string{"name": "job", "color": "red"})
	mustStatus(t, r, http.StatusOK)
	var updated labelResp
	decode(t, r, &updated)
	if updated.Name != "job" || updated.Color != "red" {
		t.Errorf("updated label = %+v", updated)
	}

	// Renaming onto an existing name conflicts.
	other := h.newLabel("home", "green")
	mustStatus(t, h.do(http.MethodPatch, path("/api/labels/", other.ID), map[string]string{"name": "job"}), http.StatusConflict)

	r = h.do(http.MethodGet, "/api/labels", nil)
	mustStatus(t, r, http.StatusOK)
	var all []labelResp
	decode(t, r, &all)
	if len(all) != 2 {
		t.Errorf("labels = %+v, want 2", all)
	}

	mustStatus(t, h.do(http.MethodDelete, path("/api/labels/", l.ID), nil), http.StatusNoContent)
	mustStatus(t, h.do(http.MethodDelete, path("/api/labels/", l.ID), nil), http.StatusNotFound)
	mustStatus(t, h.do(http.MethodPatch, path("/api/labels/", l.ID), map[string]string{"name": "x"}), http.StatusNotFound)
}

func TestTaskAttachDetachLabel(t *testing.T) {
	h := newHarness(t)
	task := h.newTask("fix bug")
	bug := h.newLabel("bug", "red")

	r := h.do(http.MethodPost, path("/api/tasks/", task.ID)+"/labels", map[string]int64{"label_id": bug.ID})
	mustStatus(t, r, http.StatusOK)
	var withLabel taskResp
	decode(t, r, &withLabel)
	if len(withLabel.Labels) != 1 || withLabel.Labels[0].ID != bug.ID || withLabel.Labels[0].Color != "red" {
		t.Errorf("labels after attach = %+v", withLabel.Labels)
	}

	// Attach is idempotent.
	r = h.do(http.MethodPost, path("/api/tasks/", task.ID)+"/labels", map[string]int64{"label_id": bug.ID})
	mustStatus(t, r, http.StatusOK)
	decode(t, r, &withLabel)
	if len(withLabel.Labels) != 1 {
		t.Errorf("labels after second attach = %+v", withLabel.Labels)
	}

	// 404s for missing task / label.
	mustStatus(t, h.do(http.MethodPost, "/api/tasks/9999/labels", map[string]int64{"label_id": bug.ID}), http.StatusNotFound)
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", task.ID)+"/labels", map[string]int64{"label_id": 9999}), http.StatusNotFound)

	r = h.do(http.MethodDelete, path("/api/tasks/", task.ID)+path("/labels/", bug.ID), nil)
	mustStatus(t, r, http.StatusOK)
	var without taskResp
	decode(t, r, &without)
	if len(without.Labels) != 0 {
		t.Errorf("labels after detach = %+v", without.Labels)
	}
	mustStatus(t, h.do(http.MethodDelete, path("/api/tasks/", task.ID)+"/labels/9999", nil), http.StatusNotFound)
}

func TestTaskFilterByLabel(t *testing.T) {
	h := newHarness(t)
	work := h.newLabel("work", "blue")
	home := h.newLabel("home", "green")
	a := h.newTask("report")
	b := h.newTask("dishes")
	c := h.newTask("archived report")
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", a.ID)+"/labels", map[string]int64{"label_id": work.ID}), http.StatusOK)
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", b.ID)+"/labels", map[string]int64{"label_id": home.ID}), http.StatusOK)
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", c.ID)+"/labels", map[string]int64{"label_id": work.ID}), http.StatusOK)
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", c.ID)+"/archive", nil), http.StatusOK)

	got := h.listTasks(path("?label_id=", work.ID))
	if len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("work filter = %+v, want [a] (archived c excluded)", got)
	}
	got = h.listTasks(path("?archived=true&label_id=", work.ID))
	if len(got) != 1 || got[0].ID != c.ID {
		t.Errorf("archived work filter = %+v, want [c]", got)
	}
	mustStatus(t, h.do(http.MethodGet, "/api/tasks?label_id=abc", nil), http.StatusBadRequest)
}

func TestDeleteLabelKeepsTasks(t *testing.T) {
	h := newHarness(t)
	l := h.newLabel("temp", "gray")
	task := h.newTask("labelled")
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", task.ID)+"/labels", map[string]int64{"label_id": l.ID}), http.StatusOK)

	mustStatus(t, h.do(http.MethodDelete, path("/api/labels/", l.ID), nil), http.StatusNoContent)

	board := h.listTasks("")
	if len(board) != 1 || board[0].ID != task.ID {
		t.Fatalf("task was removed with its label: %+v", board)
	}
	if len(board[0].Labels) != 0 {
		t.Errorf("deleted label still attached: %+v", board[0].Labels)
	}
}

// --- Subtasks ---

func TestSubtaskLifecycle(t *testing.T) {
	h := newHarness(t)
	task := h.newTask("release")

	var tr taskResp
	for _, title := range []string{"write notes", "tag build", "announce"} {
		r := h.do(http.MethodPost, path("/api/tasks/", task.ID)+"/subtasks", map[string]string{"title": title})
		mustStatus(t, r, http.StatusCreated)
		decode(t, r, &tr)
	}
	if len(tr.Subtasks) != 3 || tr.Subtasks[0].Title != "write notes" || tr.Subtasks[2].Title != "announce" {
		t.Fatalf("subtasks = %+v", tr.Subtasks)
	}
	if tr.SubtaskProgress["done"] != 0 || tr.SubtaskProgress["total"] != 3 {
		t.Errorf("progress = %+v, want 0/3", tr.SubtaskProgress)
	}
	first, second, third := tr.Subtasks[0], tr.Subtasks[1], tr.Subtasks[2]

	// Toggle done.
	r := h.do(http.MethodPatch, path("/api/subtasks/", first.ID), map[string]bool{"done": true})
	mustStatus(t, r, http.StatusOK)
	decode(t, r, &tr)
	if !tr.Subtasks[0].Done || tr.SubtaskProgress["done"] != 1 {
		t.Errorf("after toggle: %+v progress %+v", tr.Subtasks, tr.SubtaskProgress)
	}
	if tr.Status != "todo" {
		t.Errorf("parent status changed to %q by subtask toggle", tr.Status)
	}

	// Rename.
	r = h.do(http.MethodPatch, path("/api/subtasks/", second.ID), map[string]string{"title": "tag v1.0"})
	mustStatus(t, r, http.StatusOK)
	decode(t, r, &tr)
	if tr.Subtasks[1].Title != "tag v1.0" || !tr.Subtasks[0].Done {
		t.Errorf("after rename: %+v", tr.Subtasks)
	}
	mustStatus(t, h.do(http.MethodPatch, path("/api/subtasks/", second.ID), map[string]string{"title": " "}), http.StatusBadRequest)

	// Reorder to third, first, second.
	r = h.do(http.MethodPost, path("/api/tasks/", task.ID)+"/subtasks/reorder", map[string][]int64{
		"ids": {third.ID, first.ID, second.ID},
	})
	mustStatus(t, r, http.StatusOK)
	decode(t, r, &tr)
	got := []int64{tr.Subtasks[0].ID, tr.Subtasks[1].ID, tr.Subtasks[2].ID}
	want := []int64{third.ID, first.ID, second.ID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order after reorder = %v, want %v", got, want)
		}
	}

	// Delete.
	r = h.do(http.MethodDelete, path("/api/subtasks/", third.ID), nil)
	mustStatus(t, r, http.StatusOK)
	decode(t, r, &tr)
	if len(tr.Subtasks) != 2 || tr.SubtaskProgress["total"] != 2 || tr.SubtaskProgress["done"] != 1 {
		t.Errorf("after delete: %+v progress %+v", tr.Subtasks, tr.SubtaskProgress)
	}

	// Board list carries the subtasks too.
	board := h.listTasks("")
	if len(board) != 1 || len(board[0].Subtasks) != 2 {
		t.Errorf("board subtasks = %+v", board)
	}
}

func TestSubtaskNotFound(t *testing.T) {
	h := newHarness(t)
	mustStatus(t, h.do(http.MethodPost, "/api/tasks/9999/subtasks", map[string]string{"title": "x"}), http.StatusNotFound)
	mustStatus(t, h.do(http.MethodPatch, "/api/subtasks/9999", map[string]bool{"done": true}), http.StatusNotFound)
	mustStatus(t, h.do(http.MethodDelete, "/api/subtasks/9999", nil), http.StatusNotFound)
	mustStatus(t, h.do(http.MethodPost, "/api/tasks/9999/subtasks/reorder", map[string][]int64{"ids": {}}), http.StatusNotFound)
}

func TestSubtaskReorderIgnoresOtherTasks(t *testing.T) {
	h := newHarness(t)
	a := h.newTask("a")
	b := h.newTask("b")
	var ta, tb taskResp
	r := h.do(http.MethodPost, path("/api/tasks/", a.ID)+"/subtasks", map[string]string{"title": "a1"})
	decode(t, r, &ta)
	r = h.do(http.MethodPost, path("/api/tasks/", b.ID)+"/subtasks", map[string]string{"title": "b1"})
	decode(t, r, &tb)
	r = h.do(http.MethodPost, path("/api/tasks/", b.ID)+"/subtasks", map[string]string{"title": "b2"})
	decode(t, r, &tb)

	// Try to pull a's subtask into b's ordering.
	r = h.do(http.MethodPost, path("/api/tasks/", b.ID)+"/subtasks/reorder", map[string][]int64{
		"ids": {ta.Subtasks[0].ID, tb.Subtasks[1].ID, tb.Subtasks[0].ID},
	})
	mustStatus(t, r, http.StatusOK)
	decode(t, r, &tb)
	if len(tb.Subtasks) != 2 || tb.Subtasks[0].Title != "b2" {
		t.Errorf("b subtasks = %+v", tb.Subtasks)
	}
}

func TestDeleteTaskCascadesSubtasksAndLabels(t *testing.T) {
	h := newHarness(t)
	task := h.newTask("parent")
	l := h.newLabel("keep-me", "teal")
	var tr taskResp
	r := h.do(http.MethodPost, path("/api/tasks/", task.ID)+"/subtasks", map[string]string{"title": "child"})
	decode(t, r, &tr)
	mustStatus(t, h.do(http.MethodPost, path("/api/tasks/", task.ID)+"/labels", map[string]int64{"label_id": l.ID}), http.StatusOK)

	mustStatus(t, h.do(http.MethodDelete, path("/api/tasks/", task.ID), nil), http.StatusNoContent)

	mustStatus(t, h.do(http.MethodPatch, path("/api/subtasks/", tr.Subtasks[0].ID), map[string]bool{"done": true}), http.StatusNotFound)

	var links int
	if err := h.pool.QueryRow(t.Context(), `SELECT count(*) FROM task_labels WHERE task_id = $1`, task.ID).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Errorf("task_labels rows left = %d", links)
	}
	// The label itself survives.
	r = h.do(http.MethodGet, "/api/labels", nil)
	var all []labelResp
	decode(t, r, &all)
	if len(all) != 1 {
		t.Errorf("labels = %+v, want the label to survive", all)
	}
}
