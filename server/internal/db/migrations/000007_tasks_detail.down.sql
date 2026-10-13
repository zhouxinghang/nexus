-- Reverse the description/priority enrichment.
DROP INDEX IF EXISTS tasks_priority_idx;

ALTER TABLE tasks
    DROP COLUMN IF EXISTS priority,
    DROP COLUMN IF EXISTS description;

DROP TYPE IF EXISTS task_priority;
