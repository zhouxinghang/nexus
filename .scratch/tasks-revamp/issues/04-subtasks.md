# 04: Subtasks

Status: done

**What to build:** An ordered checklist of subtasks under a Task, each with a
title and a done flag, with the card showing completion progress ("2/5").

**Blocked by:** 01 (remove projects) — recommended, not strict

**Spec:** `.scratch/tasks-revamp/spec.md`

## Acceptance criteria

- [x] Migration adds a `subtasks` table: `id`, `task_id` (FK → tasks, cascade
      delete), `title TEXT NOT NULL`, `done BOOLEAN NOT NULL DEFAULT false`,
      `position INTEGER NOT NULL DEFAULT 0`, `created_at`, `updated_at`. Ordered
      by `position` within a Task.
- [x] Endpoints: `POST /api/tasks/{id}/subtasks` (create at bottom of the Task's
      list), `PATCH /api/subtasks/{id}` (edit title and/or toggle `done`),
      `POST /api/tasks/{id}/subtasks/reorder` (rewrite order, mirror
      `ReorderTasks`'s `unnest … WITH ORDINALITY` approach), `DELETE
      /api/subtasks/{id}`. 404 for missing ids.
- [x] The task response includes its subtasks (ordered) **and/or** a progress
      summary `{done, total}`.
- [x] Deleting a Task cascades its subtasks.
- [x] Frontend: a checklist editor in the Task's edit modal (add / toggle /
      rename / reorder / delete) and a "done/total" progress indicator on the
      card.
- [x] Tests: add/edit/toggle/reorder/delete subtasks; assert ordering and the
      progress summary; assert deleting a Task cascades its subtasks.

## Notes

- One level of checklist only — no nested subtasks, no per-subtask due
  dates/assignees (out of scope per the spec).
- Reuse the `tasks.position` ordinal pattern and the batch-reorder SQL shape so
  drag-ordering subtasks matches how the board reorders Tasks.
- Toggling `done` on all subtasks does **not** auto-complete the parent Task;
  status stays an explicit Owner action.

## Comments

- Implemented: migration `000012_subtasks`; create, patch, reorder, and
  delete endpoints as specified. Each returns the updated parent task so the
  card's progress resyncs in one round trip. The task response carries both
  ordered `subtasks` and `subtask_progress: {done,total}`.
- Reorder uses the `unnest … WITH ORDINALITY` shape, scoped to the parent
  task (ids from another task are ignored).
- UI: checklist in the task editor (add, toggle, inline rename, drag by the
  grip handle, delete) plus a done/total chip on the card. Toggling never
  changes the parent's status.
- Glossary: **Subtask** added to `CONTEXT.md`.
