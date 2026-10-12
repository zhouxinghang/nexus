-- Files module: Managed Roots, Resources, tags, notes, and an audit log.
CREATE TABLE managed_roots (
    id         BIGSERIAL PRIMARY KEY,
    path       TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE resource_state AS ENUM ('present', 'missing');

CREATE TABLE resources (
    id          BIGSERIAL PRIMARY KEY,
    root_id     BIGINT      NOT NULL REFERENCES managed_roots (id) ON DELETE CASCADE,
    path        TEXT        NOT NULL,            -- absolute path on the Host
    name        TEXT        NOT NULL,
    size        BIGINT      NOT NULL DEFAULT 0,
    is_dir      BOOLEAN     NOT NULL DEFAULT false,
    modified_at TIMESTAMPTZ,
    state       resource_state NOT NULL DEFAULT 'present',
    note        TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (path)
);

CREATE INDEX resources_root_idx ON resources (root_id);
CREATE INDEX resources_state_idx ON resources (state);

CREATE TABLE resource_tags (
    resource_id BIGINT NOT NULL REFERENCES resources (id) ON DELETE CASCADE,
    tag         TEXT   NOT NULL,
    PRIMARY KEY (resource_id, tag)
);

CREATE INDEX resource_tags_tag_idx ON resource_tags (tag);

CREATE TABLE file_audit_log (
    id          BIGSERIAL PRIMARY KEY,
    action      TEXT        NOT NULL,            -- rename | move | delete
    resource_id BIGINT,                          -- nullable: resource may be gone
    old_path    TEXT        NOT NULL,
    new_path    TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX file_audit_log_created_at_idx ON file_audit_log (created_at DESC);
