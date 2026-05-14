DROP TABLE IF EXISTS conflicts;
DROP TABLE IF EXISTS sync_plan_actions;
DROP TABLE IF EXISTS staged_uploads;
DROP TABLE IF EXISTS sync_sessions;
DROP TABLE IF EXISTS sync_locks;
DROP TABLE IF EXISTS tombstones;
DROP TABLE IF EXISTS file_revisions;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS server_state;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS web_sessions;
DROP TABLE IF EXISTS oauth_states;
DROP TABLE IF EXISTS vaults;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    google_sub TEXT UNIQUE,
    email TEXT NOT NULL COLLATE NOCASE UNIQUE,
    first_name TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL CHECK (role IN ('ADMIN', 'USER')),
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'DISABLED')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_login_at TEXT
);

CREATE INDEX idx_users_email ON users(email);

CREATE TABLE web_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_web_sessions_token_hash ON web_sessions(token_hash);
CREATE INDEX idx_web_sessions_user_id ON web_sessions(user_id);

CREATE TABLE oauth_states (
    id TEXT PRIMARY KEY,
    state_hash TEXT NOT NULL UNIQUE,
    redirect_to TEXT NOT NULL DEFAULT '/vault-dashboard',
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_oauth_states_state_hash ON oauth_states(state_hash);

CREATE TABLE api_keys (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    key_prefix TEXT NOT NULL,
    key_value TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    rotated_at TEXT
);

CREATE TABLE vaults (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('ACTIVE', 'DELETED')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE INDEX idx_vaults_user_status ON vaults(user_id, status);

CREATE TABLE files (
    vault_id TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    current_hash TEXT,
    previous_hash TEXT,
    size INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL DEFAULT 0,
    deleted INTEGER NOT NULL DEFAULT 0 CHECK (deleted IN (0, 1)),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (vault_id, path)
);

CREATE INDEX idx_files_vault_revision ON files(vault_id, revision);

CREATE TABLE file_revisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    vault_id TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    hash TEXT,
    size INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL,
    kind TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX idx_file_revisions_vault_path ON file_revisions(vault_id, path, revision);

CREATE TABLE tombstones (
    vault_id TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    revision INTEGER NOT NULL,
    deleted_at TEXT NOT NULL,
    client_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    PRIMARY KEY (vault_id, path)
);

CREATE TABLE sync_locks (
    vault_id TEXT PRIMARY KEY REFERENCES vaults(id) ON DELETE CASCADE,
    session_id TEXT,
    client_id TEXT,
    client_name TEXT,
    status TEXT NOT NULL,
    acquired_at TEXT,
    heartbeat_at TEXT,
    expires_at TEXT
);

CREATE TABLE sync_sessions (
    session_id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    vault_id TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    client_id TEXT NOT NULL,
    client_name TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    base_server_revision INTEGER NOT NULL,
    commit_server_revision INTEGER,
    error_code TEXT,
    error_message TEXT
);

CREATE INDEX idx_sync_sessions_vault_status ON sync_sessions(vault_id, status);

CREATE TABLE staged_uploads (
    session_id TEXT NOT NULL REFERENCES sync_sessions(session_id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    expected_hash TEXT NOT NULL,
    actual_hash TEXT,
    size INTEGER NOT NULL DEFAULT 0,
    staging_path TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    validated_at TEXT,
    PRIMARY KEY (session_id, path, expected_hash)
);

CREATE TABLE sync_plan_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sync_sessions(session_id) ON DELETE CASCADE,
    action_type TEXT NOT NULL,
    path TEXT NOT NULL,
    expected_hash TEXT,
    remote_hash TEXT,
    base_hash TEXT,
    size INTEGER NOT NULL DEFAULT 0,
    revision INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE INDEX idx_sync_plan_actions_session ON sync_plan_actions(session_id);

CREATE TABLE conflicts (
    id TEXT PRIMARY KEY,
    vault_id TEXT NOT NULL REFERENCES vaults(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    session_id TEXT NOT NULL REFERENCES sync_sessions(session_id) ON DELETE CASCADE,
    local_hash TEXT,
    remote_hash TEXT,
    base_hash TEXT,
    kind TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    resolved_at TEXT
);

CREATE INDEX idx_conflicts_vault_status ON conflicts(vault_id, status);
