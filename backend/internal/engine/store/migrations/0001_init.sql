CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;

CREATE TABLE projects (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    remote_url     TEXT NOT NULL,
    default_branch TEXT NOT NULL,
    setup_script   TEXT NOT NULL DEFAULT '',
    run_script     TEXT NOT NULL DEFAULT '',
    sandbox_image  TEXT NOT NULL DEFAULT '',
    merge_mode     TEXT NOT NULL CHECK (merge_mode IN ('pr', 'local')),
    code_remote    TEXT NOT NULL CHECK (code_remote IN ('project', 'tower')),
    created_at     INTEGER NOT NULL
) STRICT;

CREATE TABLE workspaces (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id),
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('created', 'active', 'idle', 'archived')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (project_id, slug)
) STRICT;

CREATE TABLE chats (
    id                  TEXT PRIMARY KEY,
    workspace_id        TEXT NOT NULL REFERENCES workspaces (id),
    title               TEXT NOT NULL,
    provider_session_id TEXT,
    created_at          INTEGER NOT NULL
) STRICT;

CREATE INDEX chats_workspace_id ON chats (workspace_id);

CREATE TABLE events (
    id         TEXT PRIMARY KEY,
    chat_id    TEXT NOT NULL REFERENCES chats (id),
    seq        INTEGER,
    author     TEXT NOT NULL,
    machine_id TEXT NOT NULL,
    type       TEXT NOT NULL,
    payload    TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (chat_id, seq)
) STRICT;

CREATE INDEX events_chat_id ON events (chat_id);

CREATE TRIGGER events_immutable
BEFORE UPDATE ON events
WHEN OLD.seq IS NOT NULL
    OR NEW.seq IS NULL
    OR NEW.id IS NOT OLD.id
    OR NEW.chat_id IS NOT OLD.chat_id
    OR NEW.author IS NOT OLD.author
    OR NEW.machine_id IS NOT OLD.machine_id
    OR NEW.type IS NOT OLD.type
    OR NEW.payload IS NOT OLD.payload
    OR NEW.created_at IS NOT OLD.created_at
BEGIN
    SELECT RAISE(ABORT, 'events are immutable');
END;

CREATE TRIGGER events_append_only
BEFORE DELETE ON events
BEGIN
    SELECT RAISE(ABORT, 'events are append-only');
END;
