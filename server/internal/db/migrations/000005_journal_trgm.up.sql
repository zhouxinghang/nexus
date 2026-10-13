-- Switch Journal search from an english-only tsvector (which can't tokenize
-- CJK text) to trigram substring matching, which is language-agnostic and
-- supports Chinese search. See docs/adr/0004-journal-trigram-search.md.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- The english tsvector and its index are no longer used.
DROP INDEX IF EXISTS journal_entries_search_idx;
ALTER TABLE journal_entries DROP COLUMN IF EXISTS search;

-- GIN trigram index backs case-insensitive ILIKE '%term%' substring search.
CREATE INDEX journal_entries_body_trgm_idx
    ON journal_entries USING GIN (body gin_trgm_ops);
