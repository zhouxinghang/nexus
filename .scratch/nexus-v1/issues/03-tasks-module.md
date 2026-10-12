# 03: Tasks module

**What to build:** The Owner manages to-dos on a three-column Kanban board
(Todo / Doing / Done). They create a Task with a title, move it between columns
to change its status, set a priority and a due date, and group Tasks into
projects/lists. Tasks approaching or past their due date are visually
highlighted. Tasks can be edited and deleted.

**Blocked by:** 01 (walking skeleton + auth)

**Status:** ready-for-agent

- [ ] The Owner can create a Task with a title through the API and UI.
- [ ] Each Task has a fixed status enum of `todo` / `doing` / `done`.
- [ ] The board shows three fixed columns (Todo/Doing/Done); columns are not customizable.
- [ ] The Owner can move a Task between columns to change its status.
- [ ] The Owner can set a priority on a Task.
- [ ] The Owner can set a due date on a Task.
- [ ] Tasks approaching or past their due date are highlighted in the frontend (visual only — no reminders/notifications).
- [ ] The Owner can group Tasks into projects/lists.
- [ ] The Owner can edit and delete a Task.
- [ ] No recurring tasks.
- [ ] Tests (through the HTTP API seam) assert: create, status transitions across all three columns, priority and due-date set, grouping into a project, edit, and delete.
