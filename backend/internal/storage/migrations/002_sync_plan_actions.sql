CREATE TABLE IF NOT EXISTS sync_plan_actions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  action_type TEXT NOT NULL,
  path TEXT NOT NULL,
  expected_hash TEXT,
  remote_hash TEXT,
  base_hash TEXT,
  size INTEGER NOT NULL DEFAULT 0,
  revision INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'PENDING',
  created_at TEXT NOT NULL,
  completed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_sync_plan_actions_session
  ON sync_plan_actions(session_id);

CREATE INDEX IF NOT EXISTS idx_sync_plan_actions_session_path
  ON sync_plan_actions(session_id, path);
