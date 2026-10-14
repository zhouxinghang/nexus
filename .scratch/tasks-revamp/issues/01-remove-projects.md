# 01: Remove Projects

Status: done

**What to build:** Remove the project/list concept from Tasks. Drop the
`projects` table, the `tasks.project_id` column and FK, the `/api/projects`
routes, and the project composer/filter in the UI. Labels (ticket 03) replace
the grouping need. `CONTEXT.md`'s Task definition never mentioned projects, so
this aligns code with the domain.

**Blocked by:** None

**Spec:** `.scratch/tasks-revamp/spec.md`

## Acceptance criteria

- [x] Migration drops `tasks.project_id` (and `tasks_project_idx`), the
      `projects` table, and the FK — **preserving all other Task fields**
      (title, status, position, description, priority, due_at, timestamps). A
      working `down` migration recreates `projects` + a nullable `project_id`
      (note: data in dropped tables can't be resurrected).
- [x] Remove `/api/projects` routes and `handleCreateProject` /
      `handleListProjects` / `handleDeleteProject`.
- [x] Drop `project_id` from `CreateTask` / `UpdateTask` sqlc params and the
      regenerated code, from `taskWriteRequest`, from the `UpdateTask` raw-field
      handling, and from `taskResponse` (`toTaskResponse`).
- [x] Remove the project composer, project filter, `projectName`, and the
      `Project` type / `projectId` / `projectFilter` state from
      `web/src/pages/Tasks.tsx`.
- [x] Update existing tasks tests that reference projects; assert `/api/projects`
      is gone (404), task create/update reject/ignore `project_id`, and the task
      response omits it. Assert existing-task fields survive the migration.
- [x] `CONTEXT.md`'s Task definition no longer implies a project linkage (final
      wording can land with ticket 03's glossary update, but don't leave it
      claiming projects exist).

## Notes

- This is a deliberately breaking API change (`project_id` removed), acceptable
  for a single-Owner app. Do it first — it simplifies the Task model that
  archive/labels/subtasks all touch.
- `go test ./...` and `sqlc generate` must be clean after regeneration.

## Comments

- Implemented: migration `000009_tasks_drop_projects` (working down migration
  recreates `projects` + nullable `project_id`; dropped rows aren't
  resurrected). `/api/projects` routes and handlers removed; `project_id`
  gone from sqlc params, request, and response. Create rejects `project_id`
  (unknown field → 400); PATCH ignores it.
- Migration survival is tested by seeding a project-linked task at schema v8
  (`newHarnessSeeded` + `db.MigrateTo`) and asserting its fields after the
  remaining migrations (`TestTaskFieldsSurviveProjectRemoval`).
- `CONTEXT.md` Task definition updated (no project linkage).
