ALTER TABLE bookmarks
    DROP COLUMN IF EXISTS fetched_favicon_url,
    DROP COLUMN IF EXISTS fetched_title;
