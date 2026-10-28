-- API tokens (PostgreSQL; mirrors schema_tokens.sql). The secret is stored hashed.

CREATE TABLE IF NOT EXISTS api_tokens (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  secret_hash TEXT NOT NULL UNIQUE,
  role TEXT NOT NULL,
  scope JSONB NOT NULL DEFAULT '[]',
  expires_at TIMESTAMPTZ,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  last_used_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ
);
