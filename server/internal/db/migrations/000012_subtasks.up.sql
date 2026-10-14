-- Subtasks: a one-level, ordered checklist under a Task. `position` is the
-- same per-list ordinal as tasks.position. Completing every subtask does not
-- change the parent Task's status; that stays an explicit Owner action.
CREATE TABLE subtasks (
    id         BIGSERIAL PRIMARY KEY,
    task_id    BIGINT      NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    title      TEXT        NOT NULL,
    done       BOOLEAN     NOT NULL DEFAULT false,
    position   INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX subtasks_task_position_idx ON subtasks (task_id, position);
