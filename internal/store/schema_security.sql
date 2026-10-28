-- security area tables (applied after schema.sql).

CREATE TABLE IF NOT EXISTS security_policies (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  items TEXT NOT NULL DEFAULT '[]',
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS security_assessments (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  policy_id TEXT NOT NULL,
  job_id TEXT NOT NULL DEFAULT '',
  items TEXT NOT NULL DEFAULT '[]',
  passed INTEGER NOT NULL DEFAULT 0,
  failed INTEGER NOT NULL DEFAULT 0,
  unknown INTEGER NOT NULL DEFAULT 0,
  score REAL NOT NULL DEFAULT 0,
  compliant INTEGER NOT NULL DEFAULT 0,
  at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS security_assessments_device ON security_assessments(device_id, policy_id, at);
CREATE INDEX IF NOT EXISTS security_assessments_policy ON security_assessments(policy_id, at);

CREATE TABLE IF NOT EXISTS ca_profiles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  protocol TEXT NOT NULL,
  url TEXT NOT NULL,
  credential_id TEXT NOT NULL DEFAULT '',
  ca_fingerprint TEXT NOT NULL DEFAULT '',
  validity_days INTEGER NOT NULL DEFAULT 0,
  key_type TEXT NOT NULL DEFAULT '',
  options TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS issued_certificates (
  id TEXT PRIMARY KEY,
  device_id TEXT NOT NULL,
  ca_profile_id TEXT NOT NULL DEFAULT '',
  protocol TEXT NOT NULL DEFAULT '',
  use TEXT NOT NULL DEFAULT 'identity',
  subject TEXT NOT NULL DEFAULT '',
  issuer TEXT NOT NULL DEFAULT '',
  serial TEXT NOT NULL DEFAULT '',
  not_before TEXT NOT NULL,
  not_after TEXT NOT NULL,
  fingerprint TEXT NOT NULL DEFAULT '',
  key_type TEXT NOT NULL DEFAULT '',
  cert_pem TEXT NOT NULL,
  chain_pem TEXT NOT NULL DEFAULT '',
  key_enc BLOB,
  installed INTEGER NOT NULL DEFAULT 0,
  installed_by TEXT NOT NULL DEFAULT '',
  installed_at TEXT,
  job_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS issued_certificates_device ON issued_certificates(device_id, created_at);

CREATE TABLE IF NOT EXISTS device_certificates (
  device_id TEXT NOT NULL,
  source TEXT NOT NULL,
  cert_id TEXT NOT NULL,
  use TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL DEFAULT '',
  issuer TEXT NOT NULL DEFAULT '',
  serial TEXT NOT NULL DEFAULT '',
  not_before TEXT NOT NULL,
  not_after TEXT NOT NULL,
  fingerprint TEXT NOT NULL DEFAULT '',
  self_signed INTEGER NOT NULL DEFAULT 0,
  key_type TEXT NOT NULL DEFAULT '',
  seen_at TEXT NOT NULL,
  PRIMARY KEY (device_id, source, cert_id)
);
CREATE INDEX IF NOT EXISTS device_certificates_expiry ON device_certificates(not_after);
