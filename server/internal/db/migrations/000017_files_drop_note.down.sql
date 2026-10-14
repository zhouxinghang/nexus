-- Removed note text cannot be recovered by rolling back.
ALTER TABLE resources ADD COLUMN note TEXT NOT NULL DEFAULT '';
