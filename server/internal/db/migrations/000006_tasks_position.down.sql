-- Restore the priority enum/column and drop position ordering.
CREATE TYPE task_priority AS ENUM ('none', 'low', 'medium', 'high');
ALTER TABLE tasks ADD COLUMN priority task_priority NOT NULL DEFAULT 'none';

DROP INDEX IF EXISTS tasks_status_position_idx;
CREATE INDEX tasks_status_idx ON tasks (status);

ALTER TABLE tasks DROP COLUMN position;
