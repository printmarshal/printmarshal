# Events, notifications and the outbox

PrintMarshal has one event contract. Every producer (poller, discovery, job
runner, configuration/security/firmware services, the API) publishes a
`model.Event` once to the **event bus** (`internal/events`). The bus wraps it in
a versioned envelope and fans it out to every subscriber:

| Subscriber | What it does |
|---|---|
| WebSocket hub (`/api/stream`) | pushes the envelope to connected UIs |
| Notifier (`internal/alerts`) | enqueues one **durable outbox** row per subscribed destination |
| (yours) | anything implementing `events.Subscriber` |

Nothing is delivered "only to the UI" or "only to notifiers" any more: if a
feature emits an event, every destination subscribed to that type sees it.

## Envelope

```json
{
  "id": "6f1c…",            // stable per event; used for de-duplication
  "version": 1,
  "type": "alert.raised",
  "at": "2026-10-28T21:14:02Z",
  "origin": { "source": "system|operator|device|scheduler", "actor": "kim", "ip": "10.0.0.7", "deviceId": "…" },
  "data": { … }             // the event payload (alert, device, job, …)
}
```

## Catalogue

`GET /api/events/catalog` returns every event type the product can emit, with a
description and two flags:

- **notifiable** — may be routed to notification destinations (`"*"` in a
  notifier's `events` list means "every notifiable type");
- **security** — included in the security/audit export.

A notifier configured for a type that is not in the catalogue is reported at
start-up (it would never fire). Producers that publish a type outside the
catalogue are logged too, so the catalogue cannot silently drift.

The catalogue itself lives in `internal/events/events.go`. Current types:
device.discovered / updated / deleted, alert.raised / cleared / acked,
job.updated / finished, config.applied, security.assessed / remediated,
firmware.campaign.updated / firmware.device.updated, consumable.updated / low /
replacement, auth.login / failed / denied / logout / session.revoked,
audit.recorded, settings.updated, notification.test / dead, server.stopping.

## Durable outbox

Notification delivery is no longer fire-and-forget. For each event × subscribed
destination the notifier inserts a row into `notification_outbox` (unique on
event id + destination, so a replayed event is never sent twice). A worker
drains due rows:

- attempts are retried with exponential backoff (30 s, 1 m, 2 m … capped at
  30 m) up to 6 attempts;
- every attempt is written to the notification log (`GET /api/notifications`);
- after the last failure the row is marked **dead**, logged at error level and a
  `notification.dead` event is published (which is itself notifiable, but is
  never queued back to the destination that failed);
- rows survive restarts; the worker resumes them on start.

Endpoints:

| Method | Path | Role | Purpose |
|---|---|---|---|
| GET | `/api/notifications/outbox?state=pending|delivered|dead` | operator | inspect the queue |
| POST | `/api/notifications/outbox/{id}/requeue` | admin | retry a dead row |
| POST | `/api/notifications/drain` | admin | run the worker now |
| POST | `/api/notifiers/{name}/test` | admin | send a test message through the outbox |

`/metrics` exposes `printmarshal_notifications_outbox{state="pending"|"dead"}`
so a stuck queue is visible to monitoring.

## SMTP and webhooks

Webhook posts use a 20 s per-attempt deadline. SMTP sessions set a connection
deadline from the same budget, so a stalled mail server cannot pin the worker.
Secrets stay in environment variables (`passwordEnv`); they are never stored in
the outbox or the log.
