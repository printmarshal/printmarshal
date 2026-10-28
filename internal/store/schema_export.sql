-- Printer syslog lines received by the ingest listener (docs/export.md).
CREATE TABLE IF NOT EXISTS device_logs (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id TEXT NOT NULL,
  at        TEXT NOT NULL,
  facility  INTEGER NOT NULL DEFAULT 1,
  severity  INTEGER NOT NULL DEFAULT 6,
  message   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS device_logs_device ON device_logs(device_id, id);
CREATE INDEX IF NOT EXISTS device_logs_at ON device_logs(at);

-- Deletions, so incremental sync clients can retire rows they hold.
CREATE TABLE IF NOT EXISTS tombstones (
  collection TEXT NOT NULL,
  id         TEXT NOT NULL,
  deleted_at TEXT NOT NULL,
  PRIMARY KEY (collection, id)
);
CREATE INDEX IF NOT EXISTS tombstones_order ON tombstones(collection, deleted_at, id);

CREATE INDEX IF NOT EXISTS devices_updated ON devices(updated_at, id);
CREATE INDEX IF NOT EXISTS alerts_updated ON alerts(updated_at, id);
CREATE INDEX IF NOT EXISTS audit_at ON audit(at, id);
