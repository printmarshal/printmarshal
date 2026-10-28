CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);

CREATE TABLE IF NOT EXISTS devices (
  id TEXT PRIMARY KEY,
  address TEXT NOT NULL,
  hostname TEXT NOT NULL DEFAULT '',
  mac TEXT NOT NULL DEFAULT '',
  serial TEXT NOT NULL DEFAULT '',
  uuid TEXT NOT NULL DEFAULT '',
  make TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  device_type TEXT NOT NULL DEFAULT '',
  firmware TEXT NOT NULL DEFAULT '',
  sys_descr TEXT NOT NULL DEFAULT '',
  sys_name TEXT NOT NULL DEFAULT '',
  enterprise_id INTEGER NOT NULL DEFAULT 0,
  admin_url TEXT NOT NULL DEFAULT '',
  capabilities TEXT NOT NULL DEFAULT '{}',
  discovery_methods TEXT NOT NULL DEFAULT '[]',
  identify_methods TEXT NOT NULL DEFAULT '[]',
  display_name TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  asset_tag TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'discovered',
  status TEXT,
  supplies TEXT NOT NULL DEFAULT '[]',
  trays TEXT NOT NULL DEFAULT '[]',
  counters TEXT,
  severity TEXT NOT NULL DEFAULT 'ok',
  consecutive_failures INTEGER NOT NULL DEFAULT 0,
  first_seen TEXT NOT NULL,
  last_seen TEXT NOT NULL,
  last_polled TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS devices_serial ON devices(serial);
CREATE INDEX IF NOT EXISTS devices_uuid ON devices(uuid);
CREATE INDEX IF NOT EXISTS devices_mac ON devices(mac);
CREATE INDEX IF NOT EXISTS devices_address ON devices(address);
CREATE INDEX IF NOT EXISTS devices_state ON devices(state);

CREATE TABLE IF NOT EXISTS groups (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT 'static',
  rule TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS group_members (
  group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  PRIMARY KEY (group_id, device_id)
);

CREATE TABLE IF NOT EXISTS credentials (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  scope TEXT NOT NULL DEFAULT '',
  username TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0,
  secret BLOB NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '{}',
  params TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL,
  total INTEGER NOT NULL DEFAULT 0,
  done INTEGER NOT NULL DEFAULT 0,
  failed INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT
);
CREATE INDEX IF NOT EXISTS jobs_created ON jobs(created_at);

CREATE TABLE IF NOT EXISTS job_results (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  device_id TEXT NOT NULL DEFAULT '',
  address TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT '',
  data TEXT,
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS job_results_job ON job_results(job_id);

CREATE TABLE IF NOT EXISTS telemetry (
  device_id TEXT NOT NULL,
  metric TEXT NOT NULL,
  label TEXT NOT NULL DEFAULT '',
  value REAL NOT NULL,
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS telemetry_lookup ON telemetry(device_id, metric, at);
CREATE INDEX IF NOT EXISTS telemetry_at ON telemetry(at);

CREATE TABLE IF NOT EXISTS alerts (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  severity TEXT NOT NULL,
  source TEXT NOT NULL,
  key TEXT NOT NULL,
  description TEXT NOT NULL,
  raised_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  cleared_at TEXT,
  acked_at TEXT,
  acked_by TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS alerts_device ON alerts(device_id, key);
CREATE INDEX IF NOT EXISTS alerts_active ON alerts(cleared_at);

CREATE TABLE IF NOT EXISTS notification_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  notifier TEXT NOT NULL,
  event TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '',
  ok INTEGER NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor TEXT NOT NULL,
  action TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '',
  before TEXT,
  after TEXT,
  result TEXT NOT NULL DEFAULT '',
  at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  token_hash TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS consumables (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL DEFAULT 'other',
  color TEXT NOT NULL DEFAULT '',
  part_number TEXT NOT NULL DEFAULT '',
  compatible_models TEXT NOT NULL DEFAULT '[]',
  unit TEXT NOT NULL DEFAULT 'each',
  on_hand INTEGER NOT NULL DEFAULT 0,
  warning_at INTEGER NOT NULL DEFAULT 0,
  critical_at INTEGER NOT NULL DEFAULT 0,
  location TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  reorder_url TEXT NOT NULL DEFAULT '',
  unit_cost REAL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS consumables_part ON consumables(part_number);
CREATE INDEX IF NOT EXISTS consumables_type ON consumables(type, color);

CREATE TABLE IF NOT EXISTS consumable_stock_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  consumable_id TEXT NOT NULL REFERENCES consumables(id) ON DELETE CASCADE,
  actor TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  delta INTEGER NOT NULL DEFAULT 0,
  resulting INTEGER NOT NULL DEFAULT 0,
  reason TEXT NOT NULL DEFAULT '',
  device_id TEXT NOT NULL DEFAULT '',
  supply_index INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'applied',
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS stock_events_item ON consumable_stock_events(consumable_id, at);
CREATE INDEX IF NOT EXISTS stock_events_status ON consumable_stock_events(status);

CREATE TABLE IF NOT EXISTS supply_links (
  device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  supply_index INTEGER NOT NULL,
  consumable_id TEXT NOT NULL REFERENCES consumables(id) ON DELETE CASCADE,
  source TEXT NOT NULL DEFAULT 'auto',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (device_id, supply_index)
);
CREATE INDEX IF NOT EXISTS supply_links_item ON supply_links(consumable_id);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
