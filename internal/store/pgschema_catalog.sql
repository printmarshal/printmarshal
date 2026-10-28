-- catalogue area tables (PostgreSQL; mirrors schema_catalog.sql).

CREATE TABLE IF NOT EXISTS catalog_qualifications (
  family_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  actor TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL
);
