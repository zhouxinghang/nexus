-- Bookmark enrichment: the site's own <title> and favicon URL, fetched from
-- the page. Kept separate from the Owner's name/icon, which are authoritative
-- and never overwritten by a fetch. Null until a fetch succeeds. Only the URL
-- is stored, never favicon bytes.
ALTER TABLE bookmarks
    ADD COLUMN fetched_title       TEXT,
    ADD COLUMN fetched_favicon_url TEXT;
