-- Labels: named, colored, first-class entities attachable to any Task
-- (many-to-many). Modeled after resource_tags, but with a label row so rename
-- and recolor work. Deleting a label or a Task cascades out of task_labels;
-- neither deletes the other side. Labels are a filter/visual, never an
-- ordering key.
CREATE TABLE labels (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE,
    color      TEXT        NOT NULL DEFAULT 'gray',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE task_labels (
    task_id  BIGINT NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    label_id BIGINT NOT NULL REFERENCES labels (id) ON DELETE CASCADE,
    PRIMARY KEY (task_id, label_id)
);

CREATE INDEX task_labels_label_idx ON task_labels (label_id);
