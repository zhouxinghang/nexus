-- Journal Entries: append-only, immutable, full-text searchable.
CREATE TABLE journal_entries (
    id          BIGSERIAL PRIMARY KEY,
    body        TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    search      TSVECTOR    GENERATED ALWAYS AS (to_tsvector('english', body)) STORED
);

CREATE INDEX journal_entries_created_at_idx ON journal_entries (created_at DESC);
CREATE INDEX journal_entries_search_idx ON journal_entries USING GIN (search);
