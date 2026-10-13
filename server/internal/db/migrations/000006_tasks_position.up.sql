-- Tasks move to drag-and-drop ordering and drop the priority concept.
-- `position` is a per-board ordinal (lower = higher in its column); the board
-- orders by (status, position). Existing rows are seeded from created_at so
-- their current order is preserved.
ALTER TABLE tasks ADD COLUMN position INTEGER NOT NULL DEFAULT 0;

UPDATE tasks t
SET position = seq.rn
FROM (
    SELECT id,
           row_number() OVER (PARTITION BY status ORDER BY created_at ASC, id ASC) - 1 AS rn
    FROM tasks
) seq
WHERE t.id = seq.id;

CREATE INDEX tasks_status_position_idx ON tasks (status, position);

-- Priority is removed entirely.
DROP INDEX IF EXISTS tasks_status_idx;
ALTER TABLE tasks DROP COLUMN priority;
DROP TYPE task_priority;
