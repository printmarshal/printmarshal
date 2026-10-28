-- config area tables (PostgreSQL; mirrors schema_config.sql).

CREATE TABLE IF NOT EXISTS config_profiles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  settings JSONB NOT NULL DEFAULT '[]',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS config_snapshots (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  document JSONB NOT NULL,
  at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS config_snapshots_device ON config_snapshots(device_id, at);

CREATE TABLE IF NOT EXISTS config_writes (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  job_id TEXT NOT NULL DEFAULT '',
  profile_id TEXT NOT NULL DEFAULT '',
  revert_of TEXT NOT NULL DEFAULT '',
  actor TEXT NOT NULL DEFAULT '',
  dry_run BOOLEAN NOT NULL DEFAULT FALSE,
  changes JSONB NOT NULL DEFAULT '[]',
  result JSONB NOT NULL,
  written BIGINT NOT NULL DEFAULT 0,
  failed BIGINT NOT NULL DEFAULT 0,
  skipped BIGINT NOT NULL DEFAULT 0,
  unchanged BIGINT NOT NULL DEFAULT 0,
  at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS config_writes_device ON config_writes(device_id, at);
CREATE INDEX IF NOT EXISTS config_writes_job ON config_writes(job_id);

CREATE TABLE IF NOT EXISTS config_drift (
  device_id TEXT NOT NULL,
  profile_id TEXT NOT NULL,
  report JSONB NOT NULL,
  at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (device_id, profile_id)
);
