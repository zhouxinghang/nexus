-- Recreate the projects table and a nullable project_id. Rows that lived in the
-- dropped table (and each task's former linkage) cannot be resurrected.
CREATE TABLE projects (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE tasks ADD COLUMN project_id BIGINT REFERENCES projects (id) ON DELETE SET NULL;
CREATE INDEX tasks_project_idx ON tasks (project_id);
