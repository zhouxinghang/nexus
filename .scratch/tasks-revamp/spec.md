# Tasks Revamp Spec

Status: ready-for-agent

## Problem Statement

The Tasks board works (three fixed columns Todo/Doing/Done, drag-ordered, with a
description, a priority flag, and a due date), but three gaps and one piece of
dead weight keep it from matching how people actually run a personal board
(Trello / Linear / Todoist):

1. **Done grows forever.** Completed Tasks pile up in the Done column with no way
   to clear them out while keeping the record. Comparable boards *archive* done
   cards: hidden from the board, retained and recoverable.
2. **No labels.** A Task can't be categorized across the board. There's no way to
   mark something as `work` / `home` / `bug` and filter to it — a core board
   feature.
3. **No subtasks.** A Task is a single title + description; there's no checklist
   of smaller steps with visible progress (e.g. "2/5 done").
4. **Projects are unused weight.** The `projects` table + `project_id` + the
   `/api/projects` endpoints + the project filter exist, but `CONTEXT.md`'s Task
   definition never mentions a project, and grouping-by-list isn't carrying its
   weight for a single-Owner board. It should be removed so labels become the
   one grouping mechanism.

## Solution

Revamp Tasks on the existing architecture (single Go process, chi + sqlc +
golang-migrate, embedded SPA):

- **Remove Projects** entirely: drop the table, the `project_id` column, the
  `/api/projects` routes, and the project filter/composer UI. Labels replace the
  grouping need.
- **Archive Done**: a Task can be archived (individually, or "archive all in
  Done" in bulk). Archived Tasks are hidden from the board by default, listable
  in an archived view, and unarchivable. Archiving is reversible; it is distinct
  from delete (which stays permanent).
- **Labels**: named, colored labels attachable to any Task (many-to-many), with
  the board filterable by label.
- **Subtasks**: an ordered checklist of items under a Task, each with its own
  title and done flag; the card shows completion progress.

Single-Owner, single-Host, no new runtime dependencies.

## User Stories

### Remove Projects

1. As the Owner, I no longer want a project/list concept on Tasks, so that the
   board is simpler and labels are the one way to group work.
2. As the Owner, I want existing Tasks to survive the removal (their title,
   status, order, description, priority, due date intact), so that nothing is
   lost when projects go away.

### Archive Done

3. As the Owner, I want to archive a Task, so that I can clear finished work off
   the board without deleting it.
4. As the Owner, I want to archive every Task in the Done column at once, so
   that I can tidy up in one action.
5. As the Owner, I want archived Tasks hidden from the board by default, so that
   the board shows only active work.
6. As the Owner, I want to view my archived Tasks, so that I can find a past one.
7. As the Owner, I want to unarchive a Task, so that I can bring it back to the
   board.
8. As the Owner, I want archive to be distinct from delete (delete stays
   permanent), so that archiving is a safe, reversible tidy-up.

### Labels

9. As the Owner, I want to create labels with a name and a color, so that I can
   categorize Tasks.
10. As the Owner, I want to attach one or more labels to a Task, so that I can
    classify it across columns.
11. As the Owner, I want to see a Task's labels on its card, so that its category
    is visible at a glance.
12. As the Owner, I want to filter the board by label, so that I can focus on one
    kind of work.
13. As the Owner, I want to rename/recolor/delete a label, so that I can keep my
    label set tidy; deleting a label removes it from its Tasks (not the Tasks).

### Subtasks

14. As the Owner, I want to add subtasks (checklist items) to a Task, so that I
    can break it into smaller steps.
15. As the Owner, I want to check/uncheck a subtask, so that I can track partial
    progress.
16. As the Owner, I want to reorder, edit, and delete subtasks, so that the
    checklist stays accurate.
17. As the Owner, I want the card to show subtask progress (e.g. "2/5"), so that
    I can see how far along a Task is.

## Implementation Decisions

### Remove Projects

- Migration drops the `project_id` column, its index, the `projects` table, and
  the FK. Tasks are preserved (only the project linkage disappears). Provide a
  working `down` migration (recreate `projects` + nullable `project_id`), while
  noting data in dropped tables can't be resurrected.
- Delete the `/api/projects` routes and `handleCreateProject` /
  `handleListProjects` / `handleDeleteProject`; drop `project_id` from
  `CreateTask` / `UpdateTask` params, the task response, and the `taskWriteRequest`.
- Remove the project composer, project filter, and `projectName` from
  `web/src/pages/Tasks.tsx`.
- This is a deliberately breaking API change, acceptable for a single-Owner app;
  call it out in the ticket.

### Archive Done

- Add `archived_at TIMESTAMPTZ` (null = active) to `tasks`. Archiving sets it;
  unarchiving nulls it. (A nullable timestamp doubles as "when archived".)
- `GET /api/tasks` returns only active (non-archived) Tasks by default;
  `?archived=true` returns the archived list (newest-archived first). The board
  reorder/positioning logic ignores archived Tasks.
- Endpoints: `POST /api/tasks/{id}/archive`, `POST /api/tasks/{id}/unarchive`,
  and `POST /api/tasks/archive-done` (bulk-archive every Task currently in the
  Done column). Each returns the refreshed active list (or the affected row) for
  a one-round-trip resync, consistent with `handleReorderTasks`.
- Archive is **not** delete; `DELETE /api/tasks/{id}` stays permanent.

### Labels

- `labels` table: `id`, `name` (unique), `color` (a short token/hex), `created_at`.
  Join table `task_labels (task_id, label_id)` with a composite PK and cascade
  delete, mirroring the Files `resource_tags` pattern but with a first-class
  colored label entity.
- Endpoints: `GET/POST /api/labels`, `PATCH/DELETE /api/labels/{id}`;
  `POST /api/tasks/{id}/labels` (attach by label id) and
  `DELETE /api/tasks/{id}/labels/{labelId}` (detach). Deleting a label cascades
  out of `task_labels` (Tasks keep existing).
- The task response includes its labels (`[{id,name,color}]`).
- `GET /api/tasks?label_id=` filters the board to Tasks carrying that label
  (combinable with the default active filter). Keep the filter in SQL.

### Subtasks

- `subtasks` table: `id`, `task_id` (FK cascade), `title`, `done BOOLEAN NOT NULL
  DEFAULT false`, `position INTEGER`, timestamps. Ordered by `position` within
  a Task (same ordinal pattern as `tasks.position`).
- Endpoints under the parent Task: `POST /api/tasks/{id}/subtasks` (create at
  bottom), `PATCH /api/subtasks/{id}` (edit title / toggle done),
  `POST /api/tasks/{id}/subtasks/reorder` (rewrite order, mirror
  `ReorderTasks`), `DELETE /api/subtasks/{id}`.
- The task response includes its subtasks (ordered) or at least a progress
  summary `{done, total}`; the card renders "done/total".

### API contract (shape, not paths)

- All endpoints stay under the authenticated `/api` group with the existing JSON
  error convention, status codes, and cookie session.
- The task response object changes: `project_id` is **removed**; `labels`,
  `subtasks` (or a subtask progress summary), and `archived_at` are **added**.
  Update the frontend types and existing tests to match.

## Testing Decisions

Same seam as the rest: drive the real chi router via `httptest` against an
ephemeral Postgres, authenticate for the cookie.

- **Remove Projects:** assert `/api/projects` routes are gone (404), task
  create/update no longer accept `project_id`, and the task response omits it;
  assert existing-task fields survive the migration.
- **Archive:** create Tasks, archive one and assert it drops off the default
  list but appears under `?archived=true`; unarchive restores it; `archive-done`
  archives exactly the Done-column Tasks and leaves others; delete is still
  permanent and separate from archive.
- **Labels:** create/rename/recolor/delete labels; attach/detach to a Task and
  assert the task response reflects it; `?label_id=` filters correctly; deleting
  a label removes it from Tasks without deleting the Tasks.
- **Subtasks:** add/edit/toggle/reorder/delete subtasks; assert ordering and the
  progress summary; deleting a Task cascades its subtasks and label links.

## Out of Scope

- Reintroducing projects/lists under another name, or custom board columns
  (still fixed Todo/Doing/Done).
- Auto-archiving Done after a timeout, or scheduled cleanup — archiving is a
  manual action.
- Nested subtasks (one level of checklist only), subtask due dates/assignees.
- Label-based ordering of the board (labels are a filter/visual, not an ordering
  key — same posture as priority).
- Reminders/notifications (unchanged from v1: frontend due-date highlight only).

## Further Notes

- Vocabulary: `CONTEXT.md`'s **Task** definition currently reads "a fixed status
  of Todo, Doing, or Done, plus an optional free-text description, a priority
  flag … and an optional due date." This revamp changes the Task's shape — it
  adds **Label**, **Subtask**, and an **archived** state, and drops any project
  linkage. Update the Task definition in `CONTEXT.md` and add glossary entries
  for **Label** and **Subtask** (and note archived as a Task state), rather than
  letting code and docs diverge.
- There is no existing ADR about projects; removing them is a scope decision
  recorded here. If the archive/delete distinction warrants it, a short ADR
  ("Tasks are archivable but Done is still permanent-deletable") can capture the
  reversibility trade-off.
- Tickets: `issues/01-remove-projects.md`, `issues/02-archive-done.md`,
  `issues/03-labels.md`, `issues/04-subtasks.md`. **Recommended order: 01 first**
  (it simplifies the Task model the others touch), then 02–04 in any order; all
  are otherwise independent.
