-- Tasks drop the project/list concept: labels (000011) become the one grouping
-- mechanism. Only the project linkage disappears; every other Task field
-- (title, status, position, description, priority, due_at, timestamps) stays.
DROP INDEX IF EXISTS tasks_project_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS project_id;
DROP TABLE IF EXISTS projects;
