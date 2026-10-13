-- Enrich Tasks with industry-standard card content: a free-text description and
-- a priority flag. Priority here is metadata (a visual flag, like Linear/Todoist
-- labels), NOT an ordering key — drag-and-drop `position` still governs column
-- order (see migration 000006). This restores the priority the original spec
-- asked for (user story 50) without reintroducing priority-based sorting.
CREATE TYPE task_priority AS ENUM ('none', 'low', 'medium', 'high', 'urgent');

ALTER TABLE tasks
    ADD COLUMN description TEXT          NOT NULL DEFAULT '',
    ADD COLUMN priority    task_priority NOT NULL DEFAULT 'none';

CREATE INDEX tasks_priority_idx ON tasks (priority);
