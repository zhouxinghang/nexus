-- Archive: a reversible tidy-up distinct from (permanent) delete. A null
-- archived_at means the Task is active on the board; a timestamp means it is
-- archived and records when. The board only ever lists/orders active Tasks.
ALTER TABLE tasks ADD COLUMN archived_at TIMESTAMPTZ;

CREATE INDEX tasks_archived_at_idx ON tasks (archived_at);
