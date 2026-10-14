-- Click tracking: a running count of how often the Owner opened each Bookmark
-- plus when they last did, for a "most visited" sort. No per-click history.
ALTER TABLE bookmarks
    ADD COLUMN click_count     BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN last_clicked_at TIMESTAMPTZ;
