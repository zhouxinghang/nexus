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
INSERT INTO tasks (title, status, priority, due_at, project_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasks :many
SELECT * FROM tasks
ORDER BY
    CASE status WHEN 'todo' THEN 0 WHEN 'doing' THEN 1 WHEN 'done' THEN 2 END,
    created_at ASC, id ASC;

-- name: UpdateTask :one
UPDATE tasks
SET title      = $2,
    status     = $3,
    priority   = $4,
    due_at     = $5,
    project_id = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTask :exec
DELETE FROM tasks WHERE id = $1;
