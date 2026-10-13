# 03: Labels

Status: ready-for-agent

**What to build:** Named, colored labels attachable to any Task (many-to-many),
with the board filterable by label. Labels become the one grouping mechanism now
that projects are gone (ticket 01).

**Blocked by:** 01 (remove projects) — recommended, not strict

**Spec:** `.scratch/tasks-revamp/spec.md`

## Acceptance criteria

- [ ] Migration adds a `labels` table (`id`, `name` UNIQUE, `color` short
      token/hex, `created_at`) and a join table `task_labels (task_id, label_id)`
      with a composite PK and cascade delete on both FKs.
- [ ] Label CRUD: `GET /api/labels`, `POST /api/labels` (name + color),
      `PATCH /api/labels/{id}` (rename/recolor), `DELETE /api/labels/{id}`
      (cascades out of `task_labels`; Tasks are not deleted). Validate non-empty
      name and uniqueness (409/400 on conflict).
- [ ] Attach/detach: `POST /api/tasks/{id}/labels` (attach by label id, idempotent)
      and `DELETE /api/tasks/{id}/labels/{labelId}` (detach). 404 for missing
      task/label.
- [ ] The task response includes its labels as `[{id,name,color}]`.
- [ ] `GET /api/tasks?label_id=` filters the board to Tasks carrying that label,
      combinable with the default active filter; the filter is done in SQL (join
      `task_labels`), not in Go.
- [ ] Frontend: label management (create/rename/recolor/delete), attach/detach on
      a Task (in its edit modal), label chips on the card, and a label filter on
      the board.
- [ ] Tests: label CRUD incl. uniqueness; attach/detach reflected in the task
      response; `?label_id=` filters correctly; deleting a label removes it from
      Tasks without deleting the Tasks.

## Notes

- Model labels after the Files `resource_tags` pattern, but as a first-class
  colored entity (`labels` + `task_labels`) rather than a bare string column, so
  color and rename work.
- Labels are a filter/visual only — never a board ordering key (same posture as
  priority).
- Keep `color` as a small set of named tokens or validated hex; the UI maps it
  to a chip style.
