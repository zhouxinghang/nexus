# 03: Tasks module

**What to build:** The Owner manages to-dos on a three-column Kanban board
(Todo / Doing / Done). They create a Task with a title, drag it between columns
to change its status, drag to reorder Tasks within a column, set a due date,
and group Tasks into projects/lists. Tasks approaching or past their due date
are visually highlighted. Tasks can be edited and deleted.

**Blocked by:** 01 (walking skeleton + auth)

**Status:** done

- [x] The Owner can create a Task with a title through the API and UI.
- [x] Each Task has a fixed status enum of `todo` / `doing` / `done`.
- [x] The board shows three fixed columns (Todo/Doing/Done); columns are not customizable.
- [x] The Owner can drag a Task between columns to change its status.
- [x] The Owner can drag to reorder Tasks within a column; the order is persisted.
- [x] The Owner can set a due date on a Task.
- [x] Tasks approaching or past their due date are highlighted in the frontend (visual only — no reminders/notifications).
- [x] The Owner can group Tasks into projects/lists.
- [x] The Owner can edit and delete a Task.
- [x] No recurring tasks.
- [x] Tests (through the HTTP API seam) assert: create, status transitions across all three columns, due-date set, reorder within a column, move between columns via reorder, grouping into a project, edit, and delete.

## Comments

- Schema: `task_status` enum (`todo`/`doing`/`done`), `projects` table, and `tasks` with optional `due_at` and `project_id` (FK `ON DELETE SET NULL`), plus a `position` integer giving each Task its per-column order. The board orders by `(status, position)`. Priority was dropped in migration `000006` (drag-and-drop ordering replaces it).
- API: `POST/GET /api/tasks`, `PATCH /api/tasks/{id}` (partial update — raw-JSON decode so a present-null clears `due_at`/`project_id`), `POST /api/tasks/reorder` (`{status, ids}` — rewrites one column's full top-to-bottom order and moves dragged-in Tasks into that status; returns the full list), `DELETE /api/tasks/{id}`, plus `POST/GET/DELETE /api/projects`. UI board in `web/src/pages/Tasks.tsx` uses native HTML5 drag-and-drop with an insertion indicator, project grouping, and due-date highlight (overdue = red, within 2 days = amber).
- Tests in `internal/httpapi/tasks_test.go` cover create defaults, all status transitions, invalid-status rejection, due-date, reorder within a column, cross-column move via reorder, invalid reorder status, project grouping, edit, and delete.

## Follow-up: richer task content + UI polish (2026-10-13)

- Migration `000007_tasks_detail` adds `description TEXT NOT NULL DEFAULT ''` and reintroduces a `task_priority` enum (`none`/`low`/`medium`/`high`/`urgent`) as a `priority` column on `tasks`. Priority is **metadata/visual flag only** — drag-and-drop `position` still owns column order (per migration `000006`), so this does not reintroduce priority-based sorting. This restores spec user story 50 (set a priority on a Task).
- API: `POST /api/tasks` and `PATCH /api/tasks/{id}` now accept `description` and `priority`; both are validated (`validPriority`) and surface in `taskResponse`. `PATCH` keeps raw-JSON partial semantics.
- UI (`web/src/pages/Tasks.tsx`): cards now show a left priority accent bar, a priority flag chip, a 2-line description preview, a relative due label ("Today"/"Tomorrow"/"3d overdue"), and strike-through titles in Done. Clicking a card opens a `Modal` editor (new shared `Modal` in `components/ui.tsx`) for title/description/status/priority/project/due. The composer gained a priority select, and a project filter chip row filters the board. New tests: `TestTaskCreateWithDescriptionAndPriority`, `TestTaskSetPriority`, `TestTaskRejectsInvalidPriority`, `TestTaskEditDescription`.
