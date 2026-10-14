DROP INDEX IF EXISTS tasks_archived_at_idx;
ALTER TABLE tasks DROP COLUMN IF EXISTS archived_at;
