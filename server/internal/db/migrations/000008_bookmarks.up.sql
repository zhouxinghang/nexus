-- Bookmarks module: a website-navigation board. Each Bookmark is a named,
-- clickable link to an external site with an optional icon, plus a periodic
-- health check that records whether the site is reachable. This is a separate
-- tool from Files (which stays local-only, see CONTEXT.md); the Files "no
-- external URLs" rule is unaffected.
CREATE TYPE bookmark_health AS ENUM ('unknown', 'up', 'down');

CREATE TABLE bookmarks (
    id              BIGSERIAL PRIMARY KEY,
    name            TEXT            NOT NULL,
    url             TEXT            NOT NULL,
    icon            TEXT            NOT NULL DEFAULT '',   -- emoji, image URL, or empty (favicon fallback)
    health          bookmark_health NOT NULL DEFAULT 'unknown',
    last_status     INTEGER,                              -- last HTTP status code, null until first check
    last_latency_ms BIGINT,                               -- last probe round-trip in ms, null until first check
    last_checked_at TIMESTAMPTZ,                          -- when the last health check ran
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT now()
);

CREATE INDEX bookmarks_created_at_idx ON bookmarks (created_at ASC);
