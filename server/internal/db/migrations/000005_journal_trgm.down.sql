-- Revert to the english tsvector full-text search column and index.
DROP INDEX IF EXISTS journal_entries_body_trgm_idx;

ALTER TABLE journal_entries
    ADD COLUMN search TSVECTOR
    GENERATED ALWAYS AS (to_tsvector('english', body)) STORED;

CREATE INDEX journal_entries_search_idx ON journal_entries USING GIN (search);
