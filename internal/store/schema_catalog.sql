-- catalogue area tables (applied after schema.sql).

-- Operator-set qualification of a catalogue family. The catalogue itself ships
-- every family as a research candidate; a row here is a person's assertion,
-- with who made it and when, and every change is also written to the audit log.
CREATE TABLE IF NOT EXISTS catalog_qualifications (
  family_id TEXT PRIMARY KEY,
  status TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT '',
  actor TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
