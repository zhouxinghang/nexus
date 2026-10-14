-- name: CreateTask :one
-- New tasks land at the bottom of their column (among active tasks).
INSERT INTO tasks (title, description, status, priority, due_at, position)
VALUES (
    $1, $2, $3, $4, $5,
    COALESCE((SELECT MAX(position) + 1 FROM tasks WHERE status = $3 AND archived_at IS NULL), 0)
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasks :many
-- The board: active (non-archived) tasks only, optionally narrowed to those
-- carrying one label.
SELECT * FROM tasks
WHERE archived_at IS NULL
  AND (sqlc.narg(label_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM task_labels tl
        WHERE tl.task_id = tasks.id AND tl.label_id = sqlc.narg(label_id)::bigint))
ORDER BY
    CASE status WHEN 'todo' THEN 0 WHEN 'doing' THEN 1 WHEN 'done' THEN 2 END,
    position ASC, id ASC;

-- name: ListArchivedTasks :many
-- The archived view, newest-archived first, with the same optional label filter.
SELECT * FROM tasks
WHERE archived_at IS NOT NULL
  AND (sqlc.narg(label_id)::bigint IS NULL OR EXISTS (
        SELECT 1 FROM task_labels tl
        WHERE tl.task_id = tasks.id AND tl.label_id = sqlc.narg(label_id)::bigint))
ORDER BY archived_at DESC, id DESC;

-- name: UpdateTask :one
UPDATE tasks
SET title       = $2,
    description = $3,
    status      = $4,
    priority    = $5,
    due_at      = $6,
    updated_at  = now()
WHERE id = $1
RETURNING *;

-- name: ReorderTasks :exec
-- Rewrite a column's ordering: every id in the list is placed in the given
-- column at its array position (used by drag-and-drop, including moves between
-- columns). Atomic over the whole list. Archived tasks are never repositioned.
UPDATE tasks AS t
SET status     = sqlc.arg(status),
    position   = v.ord - 1,
    updated_at = now()
FROM unnest(sqlc.arg(ids)::bigint[]) WITH ORDINALITY AS v(id, ord)
WHERE t.id = v.id AND t.archived_at IS NULL;

-- name: ArchiveTask :one
-- Archiving an already-archived task keeps its original archived_at.
UPDATE tasks
SET archived_at = COALESCE(archived_at, now()),
    updated_at  = now()
WHERE id = $1
RETURNING *;

-- name: UnarchiveTask :one
-- An unarchived task returns to the bottom of its column.
UPDATE tasks AS t
SET position = CASE
        WHEN t.archived_at IS NULL THEN t.position
        ELSE COALESCE((SELECT MAX(t2.position) + 1 FROM tasks t2
                       WHERE t2.status = t.status AND t2.archived_at IS NULL), 0)
    END,
    archived_at = NULL,
    updated_at  = now()
WHERE t.id = $1
RETURNING *;

-- name: ArchiveDoneTasks :execrows
UPDATE tasks
SET archived_at = now(),
    updated_at  = now()
WHERE status = 'done' AND archived_at IS NULL;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;

-- name: ListLabels :many
SELECT * FROM labels
ORDER BY name ASC, id ASC;

-- name: GetLabel :one
SELECT * FROM labels WHERE id = $1;

-- name: CreateLabel :one
INSERT INTO labels (name, color)
VALUES ($1, $2)
RETURNING *;

-- name: UpdateLabel :one
UPDATE labels
SET name = $2, color = $3
WHERE id = $1
RETURNING *;

-- name: DeleteLabel :execrows
DELETE FROM labels WHERE id = $1;

-- name: AttachTaskLabel :exec
INSERT INTO task_labels (task_id, label_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DetachTaskLabel :exec
DELETE FROM task_labels WHERE task_id = $1 AND label_id = $2;

-- name: ListLabelsForTasks :many
SELECT tl.task_id, l.id, l.name, l.color
FROM task_labels tl
JOIN labels l ON l.id = tl.label_id
WHERE tl.task_id = ANY(sqlc.arg(task_ids)::bigint[])
ORDER BY l.name ASC, l.id ASC;

-- name: CreateSubtask :one
-- New subtasks land at the bottom of their task's checklist.
INSERT INTO subtasks (task_id, title, position)
VALUES (
    $1, $2,
    COALESCE((SELECT MAX(position) + 1 FROM subtasks WHERE task_id = $1), 0)
)
RETURNING *;

-- name: GetSubtask :one
SELECT * FROM subtasks WHERE id = $1;

-- name: UpdateSubtask :one
UPDATE subtasks
SET title      = $2,
    done       = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSubtask :execrows
DELETE FROM subtasks WHERE id = $1;

-- name: ReorderSubtasks :exec
-- Rewrite one task's checklist order (same shape as ReorderTasks). Ids that
-- belong to a different task are ignored.
UPDATE subtasks AS s
SET position   = v.ord - 1,
    updated_at = now()
FROM unnest(sqlc.arg(ids)::bigint[]) WITH ORDINALITY AS v(id, ord)
WHERE s.id = v.id AND s.task_id = sqlc.arg(task_id);

-- name: ListSubtasksForTasks :many
SELECT * FROM subtasks
WHERE task_id = ANY(sqlc.arg(task_ids)::bigint[])
ORDER BY task_id ASC, position ASC, id ASC;

-- name: TaskSummary :one
-- Overview counts over active (non-archived) tasks. Due dates are calendar
-- days stored as midnight, so "today" is judged by the DB clock's current day:
-- overdue = due before today, due today = due within today. Done tasks are
-- never counted as overdue or due.
SELECT
    count(*) FILTER (WHERE status = 'todo')  AS todo,
    count(*) FILTER (WHERE status = 'doing') AS doing,
    count(*) FILTER (WHERE status = 'done')  AS done,
    count(*) FILTER (WHERE status <> 'done' AND due_at < date_trunc('day', now())) AS overdue,
    count(*) FILTER (WHERE status <> 'done'
                       AND due_at >= date_trunc('day', now())
                       AND due_at <  date_trunc('day', now()) + interval '1 day') AS due_today
FROM tasks
WHERE archived_at IS NULL;
