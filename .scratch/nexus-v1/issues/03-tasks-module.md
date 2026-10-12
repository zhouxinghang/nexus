# 03: Tasks module

**What to build:** The Owner manages to-dos on a three-column Kanban board
(Todo / Doing / Done). They create a Task with a title, move it between columns
to change its status, set a priority and a due date, and group Tasks into
projects/lists. Tasks approaching or past their due date are visually
highlighted. Tasks can be edited and deleted.

**Blocked by:** 01 (walking skeleton + auth)

**Status:** done

- [x] The Owner can create a Task with a title through the API and UI.
- [x] Each Task has a fixed status enum of `todo` / `doing` / `done`.
- [x] The board shows three fixed columns (Todo/Doing/Done); columns are not customizable.
- [x] The Owner can move a Task between columns to change its status.
- [x] The Owner can set a priority on a Task.
- [x] The Owner can set a due date on a Task.
- [x] Tasks approaching or past their due date are highlighted in the frontend (visual only — no reminders/notifications).
- [x] The Owner can group Tasks into projects/lists.
- [x] The Owner can edit and delete a Task.
- [x] No recurring tasks.
- [x] Tests (through the HTTP API seam) assert: create, status transitions across all three columns, priority and due-date set, grouping into a project, edit, and delete.

## Comments

- Schema: `task_status` enum (`todo`/`doing`/`done`), `task_priority` enum (`none`/`low`/`medium`/`high`), `projects` table, and `tasks` with optional `due_at` and `project_id` (FK `ON DELETE SET NULL`).
- API: `POST/GET /api/tasks`, `PATCH /api/tasks/{id}` (partial update — raw-JSON decode so a present-null clears `due_at`/`project_id`), `DELETE /api/tasks/{id}`, plus `POST/GET/DELETE /api/projects`. UI board in `web/src/pages/Tasks.tsx` with per-task priority/due-date controls, project grouping, and due-date highlight (overdue = red, within 2 days = amber).
- Tests in `internal/httpapi/tasks_test.go` cover create defaults, all status transitions, invalid-status rejection, priority + due-date, project grouping, edit, and delete.
