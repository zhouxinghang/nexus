# 02: Archive Done

Status: ready-for-agent

**What to build:** Let the Owner archive Tasks so finished work leaves the board
without being deleted. Archived Tasks are hidden by default, listable in an
archived view, and unarchivable. Archive is reversible and distinct from delete
(which stays permanent). Includes a bulk "archive all in Done".

**Blocked by:** 01 (remove projects) — recommended, not strict

**Spec:** `.scratch/tasks-revamp/spec.md`

## Acceptance criteria

- [ ] Migration adds `archived_at TIMESTAMPTZ` (null = active) to `tasks`.
- [ ] `GET /api/tasks` returns only **active** (non-archived) Tasks by default;
      `?archived=true` returns archived Tasks, newest-archived first. Board
      ordering/reorder logic ignores archived Tasks.
- [ ] `POST /api/tasks/{id}/archive` sets `archived_at = now()`;
      `POST /api/tasks/{id}/unarchive` nulls it. Each returns the affected row
      or the refreshed active list (one-round-trip resync, like
      `handleReorderTasks`). 404 for a missing id.
- [ ] `POST /api/tasks/archive-done` archives every Task currently in the Done
      column and leaves other columns untouched; returns the refreshed active
      list.
- [ ] `DELETE /api/tasks/{id}` stays permanent and separate from archive.
- [ ] `archived_at` added to the task response.
- [ ] Frontend: an archive action on a card (and/or in its edit modal), an
      "Archive all done" action on the Done column, and an archived view
      (e.g. a toggle/drawer) with unarchive. Archived Tasks don't appear on the
      board.
- [ ] Tests: archive drops a Task from the default list but it appears under
      `?archived=true`; unarchive restores it to the board; `archive-done`
      archives exactly the Done Tasks; delete still permanently removes.

## Notes

- Reuse the existing new-tasks-land-at-bottom / `position` semantics for active
  Tasks; unarchiving can place the Task at the bottom of its column.
- Keep the archive/delete distinction obvious in the UI so the Owner doesn't
  confuse the reversible tidy-up with permanent deletion.
