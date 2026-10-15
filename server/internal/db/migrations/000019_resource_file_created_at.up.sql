-- Host filesystem creation time, distinct from resources.created_at (index time).
-- Existing rows are backfilled by the next Scan, not with fabricated timestamps.
ALTER TABLE resources ADD COLUMN file_created_at TIMESTAMPTZ;
