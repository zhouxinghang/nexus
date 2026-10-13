# 04: Subtasks

Status: ready-for-agent

**What to build:** An ordered checklist of subtasks under a Task, each with a
title and a done flag, with the card showing completion progress ("2/5").

**Blocked by:** 01 (remove projects) — recommended, not strict

**Spec:** `.scratch/tasks-revamp/spec.md`

## Acceptance criteria

- [ ] Migration adds a `subtasks` table: `id`, `task_id` (FK → tasks, cascade
      delete), `title TEXT NOT NULL`, `done BOOLEAN NOT NULL DEFAULT false`,
      `position INTEGER NOT NULL DEFAULT 0`, `created_at`, `updated_at`. Ordered
      by `position` within a Task.
- [ ] Endpoints: `POST /api/tasks/{id}/subtasks` (create at bottom of the Task's
      list), `PATCH /api/subtasks/{id}` (edit title and/or toggle `done`),
      `POST /api/tasks/{id}/subtasks/reorder` (rewrite order, mirror
      `ReorderTasks`'s `unnest … WITH ORDINALITY` approach), `DELETE
      /api/subtasks/{id}`. 404 for missing ids.
- [ ] The task response includes its subtasks (ordered) **and/or** a progress
      summary `{done, total}`.
- [ ] Deleting a Task cascades its subtasks.
- [ ] Frontend: a checklist editor in the Task's edit modal (add / toggle /
      rename / reorder / delete) and a "done/total" progress indicator on the
      card.
- [ ] Tests: add/edit/toggle/reorder/delete subtasks; assert ordering and the
      progress summary; assert deleting a Task cascades its subtasks.

## Notes

- One level of checklist only — no nested subtasks, no per-subtask due
  dates/assignees (out of scope per the spec).
- Reuse the `tasks.position` ordinal pattern and the batch-reorder SQL shape so
  drag-ordering subtasks matches how the board reorders Tasks.
- Toggling `done` on all subtasks does **not** auto-complete the parent Task;
  status stays an explicit Owner action.
