-- Durable notification outbox (one event × one destination, retried with backoff).
CREATE TABLE IF NOT EXISTS notification_outbox (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id    TEXT NOT NULL,
  event       TEXT NOT NULL,
  notifier    TEXT NOT NULL,
  subject     TEXT NOT NULL DEFAULT '',
  body        TEXT NOT NULL DEFAULT '',
  payload     TEXT,
  state       TEXT NOT NULL DEFAULT 'pending',
  attempts    INTEGER NOT NULL DEFAULT 0,
  last_error  TEXT NOT NULL DEFAULT '',
  next_at     TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  done_at     TEXT
);
CREATE INDEX IF NOT EXISTS idx_outbox_due ON notification_outbox(state, next_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_outbox_dedupe ON notification_outbox(event_id, notifier);
