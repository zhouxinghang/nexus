-- Tasks module: projects (lists) and tasks on a three-column board.
CREATE TABLE projects (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE task_status AS ENUM ('todo', 'doing', 'done');
CREATE TYPE task_priority AS ENUM ('none', 'low', 'medium', 'high');

CREATE TABLE tasks (
    id         BIGSERIAL PRIMARY KEY,
    title      TEXT          NOT NULL,
    status     task_status   NOT NULL DEFAULT 'todo',
    priority   task_priority NOT NULL DEFAULT 'none',
    due_at     TIMESTAMPTZ,
    project_id BIGINT        REFERENCES projects (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX tasks_status_idx ON tasks (status);
CREATE INDEX tasks_project_idx ON tasks (project_id);
