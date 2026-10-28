-- config area tables (applied after schema.sql).

CREATE TABLE IF NOT EXISTS config_profiles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  settings TEXT NOT NULL DEFAULT '[]',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS config_snapshots (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  document TEXT NOT NULL,
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS config_snapshots_device ON config_snapshots(device_id, at);

CREATE TABLE IF NOT EXISTS config_writes (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  job_id TEXT NOT NULL DEFAULT '',
  profile_id TEXT NOT NULL DEFAULT '',
  revert_of TEXT NOT NULL DEFAULT '',
  actor TEXT NOT NULL DEFAULT '',
  dry_run INTEGER NOT NULL DEFAULT 0,
  changes TEXT NOT NULL DEFAULT '[]',
  result TEXT NOT NULL,
  written INTEGER NOT NULL DEFAULT 0,
  failed INTEGER NOT NULL DEFAULT 0,
  skipped INTEGER NOT NULL DEFAULT 0,
  unchanged INTEGER NOT NULL DEFAULT 0,
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS config_writes_device ON config_writes(device_id, at);
CREATE INDEX IF NOT EXISTS config_writes_job ON config_writes(job_id);

CREATE TABLE IF NOT EXISTS config_drift (
  device_id TEXT NOT NULL,
  profile_id TEXT NOT NULL,
  report TEXT NOT NULL,
  at TEXT NOT NULL,
  PRIMARY KEY (device_id, profile_id)
);
