ALTER TABLE bookmarks
    DROP COLUMN IF EXISTS last_clicked_at,
    DROP COLUMN IF EXISTS click_count;
