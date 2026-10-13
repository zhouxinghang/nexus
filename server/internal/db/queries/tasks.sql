-- name: CreateProject :one
INSERT INTO projects (name)
VALUES ($1)
RETURNING *;

-- name: ListProjects :many
SELECT * FROM projects
ORDER BY created_at ASC, id ASC;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;

-- name: CreateTask :one
-- New tasks land at the bottom of their column.
INSERT INTO tasks (title, status, due_at, project_id, position)
VALUES (
    $1, $2, $3, $4,
    COALESCE((SELECT MAX(position) + 1 FROM tasks WHERE status = $2), 0)
)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasks :many
SELECT * FROM tasks
ORDER BY
    CASE status WHEN 'todo' THEN 0 WHEN 'doing' THEN 1 WHEN 'done' THEN 2 END,
    position ASC, id ASC;

-- name: UpdateTask :one
UPDATE tasks
SET title      = $2,
    status     = $3,
    due_at     = $4,
    project_id = $5,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: ReorderTasks :exec
-- Rewrite a column's ordering: every id in the list is placed in the given
-- column at its array position (used by drag-and-drop, including moves between
-- columns). Atomic over the whole list.
UPDATE tasks AS t
SET status     = sqlc.arg(status),
    position   = v.ord - 1,
    updated_at = now()
FROM unnest(sqlc.arg(ids)::bigint[]) WITH ORDINALITY AS v(id, ord)
WHERE t.id = v.id;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;
