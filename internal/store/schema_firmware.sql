-- firmware area tables (applied after schema.sql).

CREATE TABLE IF NOT EXISTS firmware_packages (
  id TEXT PRIMARY KEY,
  vendor TEXT NOT NULL DEFAULT '',
  models TEXT NOT NULL DEFAULT '[]',
  version TEXT NOT NULL,
  min_version TEXT NOT NULL DEFAULT '',
  file_name TEXT NOT NULL,
  size INTEGER NOT NULL DEFAULT 0,
  sha256 TEXT NOT NULL,
  storage_path TEXT NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

-- package_id is not a foreign key on purpose: finished campaigns keep their
-- history after a package is deleted; the service refuses to delete a package
-- an unfinished campaign still references.
CREATE TABLE IF NOT EXISTS firmware_campaigns (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  package_id TEXT NOT NULL,
  group_id TEXT NOT NULL DEFAULT '',
  device_ids TEXT NOT NULL DEFAULT '[]',
  canary_count INTEGER NOT NULL DEFAULT 0,
  concurrency INTEGER NOT NULL DEFAULT 0,
  window TEXT,
  failure_threshold INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  wave INTEGER NOT NULL DEFAULT 0,
  confirmed INTEGER NOT NULL DEFAULT 0,
  dry_run INTEGER NOT NULL DEFAULT 0,
  allow_downgrade INTEGER NOT NULL DEFAULT 0,
  verify_timeout_secs INTEGER NOT NULL DEFAULT 0,
  pause_reason TEXT NOT NULL DEFAULT '',
  job_id TEXT NOT NULL DEFAULT '',
  scheduled_at TEXT,
  started_at TEXT,
  finished_at TEXT,
  created_by TEXT NOT NULL DEFAULT '',
  started_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS firmware_campaigns_status ON firmware_campaigns(status);
CREATE INDEX IF NOT EXISTS firmware_campaigns_package ON firmware_campaigns(package_id);

-- device_id is not a foreign key: the row is the update history and must
-- outlive the device record.
CREATE TABLE IF NOT EXISTS firmware_campaign_devices (
  campaign_id TEXT NOT NULL REFERENCES firmware_campaigns(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL,
  wave INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  from_version TEXT NOT NULL DEFAULT '',
  to_version TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  started_at TEXT,
  finished_at TEXT,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (campaign_id, device_id)
);
CREATE INDEX IF NOT EXISTS firmware_campaign_devices_device ON firmware_campaign_devices(device_id);
