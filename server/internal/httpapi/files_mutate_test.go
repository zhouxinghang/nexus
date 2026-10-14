package httpapi_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

type auditResp struct {
	Action  string `json:"action"`
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

func TestRenameUnderlyingFile(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "old.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	rr := h.do(http.MethodPost, path("/api/files/resources/", id)+"/rename", map[string]string{"name": "new.txt"})
	mustStatus(t, rr, http.StatusOK)
	var res resourceResp
	decode(t, rr, &res)
	if res.Name != "new.txt" {
		t.Errorf("name = %q, want new.txt", res.Name)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err != nil {
		t.Errorf("renamed file not on disk: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("old file still exists after rename")
	}
}

func TestMoveUnderlyingFile(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	writeFile(t, f, "x")
	// A subdirectory within the same root is a valid move target.
	sub := filepath.Join(dir, "archive")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	dest := filepath.Join(sub, "a.txt")
	mr := h.do(http.MethodPost, path("/api/files/resources/", id)+"/move", map[string]string{"dest": dest})
	mustStatus(t, mr, http.StatusOK)
	var res resourceResp
	decode(t, mr, &res)
	if res.Path != filepath.Clean(dest) {
		t.Errorf("path = %q, want %q", res.Path, dest)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("moved file not at dest: %v", err)
	}
}

func TestMoveOutsideRootRejected(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	outside := filepath.Join(t.TempDir(), "a.txt")
	mr := h.do(http.MethodPost, path("/api/files/resources/", id)+"/move", map[string]string{"dest": outside})
	mustStatus(t, mr, http.StatusBadRequest)

	// The file must not have moved.
	if _, err := os.Stat(f); err != nil {
		t.Errorf("original file disturbed by rejected move: %v", err)
	}
}

func TestDeletePermanent(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "doomed.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	dr := h.do(http.MethodDelete, path("/api/files/resources/", id)+"/file", nil)
	mustStatus(t, dr, http.StatusNoContent)

	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file still on disk after permanent delete")
	}
	// Resource cleaned up — no dangling entry.
	lr := h.do(http.MethodGet, "/api/files/resources", nil)
	var remaining []resourceResp
	decodeItems(t, lr, &remaining)
	for _, r := range remaining {
		if r.ID == id {
			t.Error("resource still present after byte deletion")
		}
	}
}

func TestTagsFollowRenameAndMove(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "orig.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	h.do(http.MethodPost, path("/api/files/resources/", id)+"/tags", map[string]string{"tag": "keep"}).Body.Close()

	// Rename.
	rr := h.do(http.MethodPost, path("/api/files/resources/", id)+"/rename", map[string]string{"name": "renamed.txt"})
	mustStatus(t, rr, http.StatusOK)
	var afterRename resourceResp
	decode(t, rr, &afterRename)
	if len(afterRename.Tags) != 1 || afterRename.Tags[0] != "keep" {
		t.Errorf("tags lost on rename: %+v", afterRename)
	}

	// Move into a subdir.
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	dest := filepath.Join(sub, "renamed.txt")
	mr := h.do(http.MethodPost, path("/api/files/resources/", id)+"/move", map[string]string{"dest": dest})
	mustStatus(t, mr, http.StatusOK)
	var afterMove resourceResp
	decode(t, mr, &afterMove)
	if len(afterMove.Tags) != 1 || afterMove.Tags[0] != "keep" {
		t.Errorf("tags lost on move: %+v", afterMove)
	}
}

func TestAuditLogRecordsMutations(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "x.txt")
	writeFile(t, f, "x")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	h.do(http.MethodPost, path("/api/files/resources/", id)+"/rename", map[string]string{"name": "y.txt"}).Body.Close()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	h.do(http.MethodPost, path("/api/files/resources/", id)+"/move", map[string]string{"dest": filepath.Join(sub, "y.txt")}).Body.Close()
	h.do(http.MethodDelete, path("/api/files/resources/", id)+"/file", nil).Body.Close()

	resp := h.do(http.MethodGet, "/api/files/audit", nil)
	mustStatus(t, resp, http.StatusOK)
	var log []auditResp
	decode(t, resp, &log)
	actions := map[string]bool{}
	for _, a := range log {
		actions[a.Action] = true
	}
	for _, want := range []string{"rename", "move", "delete"} {
		if !actions[want] {
			t.Errorf("audit log missing %q action; log=%+v", want, log)
		}
	}
}

type batchResultResp struct {
	Moved   int      `json:"moved"`
	Tagged  int      `json:"tagged"`
	Deleted int      `json:"deleted"`
	Failed  []string `json:"failed"`
}

func TestBatchMoveMovesManyFiles(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "a")
	writeFile(t, filepath.Join(dir, "b.txt"), "b")
	writeFile(t, filepath.Join(dir, "c.txt"), "c")
	sub := filepath.Join(dir, "archive")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)

	ids := []int64{}
	for _, r := range resources {
		if !r.IsDir {
			ids = append(ids, r.ID)
		}
	}

	resp := h.do(http.MethodPost, "/api/files/resources/batch/move", map[string]any{
		"ids":  ids,
		"dest": sub,
	})
	mustStatus(t, resp, http.StatusOK)
	var res batchResultResp
	decode(t, resp, &res)
	if res.Moved != len(ids) {
		t.Errorf("moved = %d, want %d", res.Moved, len(ids))
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if _, err := os.Stat(filepath.Join(sub, name)); err != nil {
			t.Errorf("%s not moved into archive: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s still at source after move", name)
		}
	}
}

func TestBatchMoveRejectsOutsideRoot(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "a")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)
	id := resources[0].ID

	outside := t.TempDir() // a directory outside every Managed Root
	resp := h.do(http.MethodPost, "/api/files/resources/batch/move", map[string]any{
		"ids":  []int64{id},
		"dest": outside,
	})
	mustStatus(t, resp, http.StatusBadRequest)
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); err != nil {
		t.Errorf("source disturbed by rejected batch move: %v", err)
	}
}

func TestBatchAddTagTagsManyFiles(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "a")
	writeFile(t, filepath.Join(dir, "b.txt"), "b")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)

	ids := []int64{resources[0].ID, resources[1].ID}
	resp := h.do(http.MethodPost, "/api/files/resources/batch/tag", map[string]any{
		"ids": ids,
		"tag": "shared",
	})
	mustStatus(t, resp, http.StatusOK)

	lr := h.do(http.MethodGet, "/api/files/resources", nil)
	var all []resourceResp
	decodeItems(t, lr, &all)
	tagged := 0
	for _, r := range all {
		for _, tg := range r.Tags {
			if tg == "shared" {
				tagged++
			}
		}
	}
	if tagged != 2 {
		t.Errorf("tagged %d resources, want 2", tagged)
	}
}

func TestBatchDeleteRemovesManyFiles(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.txt"), "a")
	writeFile(t, filepath.Join(dir, "b.txt"), "b")
	root := h.createRoot(t, dir)
	resources := h.scan(t, root.ID)

	ids := []int64{resources[0].ID, resources[1].ID}
	resp := h.do(http.MethodPost, "/api/files/resources/batch/delete", map[string]any{
		"ids": ids,
	})
	mustStatus(t, resp, http.StatusOK)
	var res batchResultResp
	decode(t, resp, &res)
	if res.Deleted != 2 {
		t.Errorf("deleted = %d, want 2", res.Deleted)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s still on disk after batch delete", name)
		}
	}
	lr := h.do(http.MethodGet, "/api/files/resources", nil)
	var remaining []resourceResp
	decodeItems(t, lr, &remaining)
	if len(remaining) != 0 {
		t.Errorf("resources remain after batch delete: %+v", remaining)
	}
}
