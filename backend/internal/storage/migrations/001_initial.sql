CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS server_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS api_keys (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  key_prefix TEXT NOT NULL DEFAULT 'noxsync_',
  key_value TEXT NOT NULL,
  key_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  rotated_at TEXT
);

CREATE TABLE IF NOT EXISTS server_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  revision INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS files (
  path TEXT PRIMARY KEY,
  current_hash TEXT,
  previous_hash TEXT,
  size INTEGER NOT NULL DEFAULT 0,
  revision INTEGER NOT NULL DEFAULT 0,
  deleted INTEGER NOT NULL DEFAULT 0 CHECK (deleted IN (0, 1)),
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS file_revisions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path TEXT NOT NULL,
  hash TEXT,
  size INTEGER NOT NULL DEFAULT 0,
  revision INTEGER NOT NULL,
  kind TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_file_revisions_path_revision
  ON file_revisions(path, revision);

CREATE TABLE IF NOT EXISTS tombstones (
  path TEXT PRIMARY KEY,
  revision INTEGER NOT NULL,
  deleted_at TEXT NOT NULL,
  client_id TEXT,
  session_id TEXT
);

CREATE TABLE IF NOT EXISTS sync_locks (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  session_id TEXT,
  client_id TEXT,
  client_name TEXT,
  vault_id TEXT,
  status TEXT NOT NULL,
  acquired_at TEXT,
  heartbeat_at TEXT,
  expires_at TEXT
);

CREATE TABLE IF NOT EXISTS sync_sessions (
  session_id TEXT PRIMARY KEY,
  client_id TEXT NOT NULL,
  client_name TEXT NOT NULL,
  vault_id TEXT NOT NULL,
  status TEXT NOT NULL,
  started_at TEXT NOT NULL,
  completed_at TEXT,
  base_server_revision INTEGER NOT NULL,
  commit_server_revision INTEGER,
  error_code TEXT,
  error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_sync_sessions_status
  ON sync_sessions(status);

CREATE TABLE IF NOT EXISTS staged_uploads (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  path TEXT NOT NULL,
  expected_hash TEXT NOT NULL,
  actual_hash TEXT,
  size INTEGER NOT NULL DEFAULT 0,
  staging_path TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  validated_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_staged_uploads_session
  ON staged_uploads(session_id);

CREATE TABLE IF NOT EXISTS conflicts (
  id TEXT PRIMARY KEY,
  path TEXT NOT NULL,
  session_id TEXT,
  local_hash TEXT,
  remote_hash TEXT,
  base_hash TEXT,
  kind TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  resolved_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_conflicts_status
  ON conflicts(status);
