# 03: Labels

Status: done

**What to build:** Named, colored labels attachable to any Task (many-to-many),
with the board filterable by label. Labels become the one grouping mechanism now
that projects are gone (ticket 01).

**Blocked by:** 01 (remove projects) — recommended, not strict

**Spec:** `.scratch/tasks-revamp/spec.md`

## Acceptance criteria

- [x] Migration adds a `labels` table (`id`, `name` UNIQUE, `color` short
      token/hex, `created_at`) and a join table `task_labels (task_id, label_id)`
      with a composite PK and cascade delete on both FKs.
- [x] Label CRUD: `GET /api/labels`, `POST /api/labels` (name + color),
      `PATCH /api/labels/{id}` (rename/recolor), `DELETE /api/labels/{id}`
      (cascades out of `task_labels`; Tasks are not deleted). Validate non-empty
      name and uniqueness (409/400 on conflict).
- [x] Attach/detach: `POST /api/tasks/{id}/labels` (attach by label id, idempotent)
      and `DELETE /api/tasks/{id}/labels/{labelId}` (detach). 404 for missing
      task/label.
- [x] The task response includes its labels as `[{id,name,color}]`.
- [x] `GET /api/tasks?label_id=` filters the board to Tasks carrying that label,
      combinable with the default active filter; the filter is done in SQL (join
      `task_labels`), not in Go.
- [x] Frontend: label management (create/rename/recolor/delete), attach/detach on
      a Task (in its edit modal), label chips on the card, and a label filter on
      the board.
- [x] Tests: label CRUD incl. uniqueness; attach/detach reflected in the task
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

## Comments

- Implemented: migration `000011_task_labels`; label CRUD under
  `/api/labels` (unique name → 409, blank → 400); attach/detach return the
  updated task; task response carries `labels: [{id,name,color}]`.
- `color` is one of a fixed set of named tokens (gray, red, orange, amber,
  green, teal, blue, violet, pink), validated server-side and mapped to chip
  styles in the UI.
- `?label_id=` filters in SQL (an `EXISTS` over `task_labels`), and it
  combines with `?archived=true`. The board UI filters the already-loaded
  list on the client, so a drag inside a filtered column can still write back
  the full column order. The SQL filter is there for API callers.
- Glossary: **Label** added to `CONTEXT.md` (avoid "tag", which is the
  Files term).
