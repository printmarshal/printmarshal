# Events, notifications and integrations

PrintMarshal has one event contract. Every producer (poller, discovery, job
runner, configuration/security/firmware services, the API) publishes a
`model.Event` once to the **event bus** (`internal/events`). The bus wraps it in
a versioned envelope and fans it out to every subscriber:

| Subscriber | What it does |
|---|---|
| WebSocket hub (`/api/stream`) | pushes the envelope to connected UIs |
| Notifier (`internal/alerts`) | enqueues one **durable outbox** row per subscribed destination |
| (yours) | anything implementing `events.Subscriber` |

The bus invokes subscribers synchronously. Each subscriber applies its own filters; publishing does not guarantee notification or export delivery.

## Envelope

```jsonc
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
event id + destination, preventing duplicate insertion of that envelope). A worker
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

Delivery attempts have a 20 s context budget; webhook HTTP clients have a shorter 10 s timeout. SMTP sessions set a connection
deadline from the same budget, so a stalled mail server cannot pin the worker.
Secrets stay in environment variables (`passwordEnv`); they are never stored in
the outbox or the log.

### SMTP authentication methods

| `authMethod` | When to use | Fields |
| --- | --- | --- |
| `plain` (default) | relays that accept basic auth | `username`, `passwordEnv` |
| `login` | Exchange / older relays that only advertise LOGIN | `username`, `passwordEnv` |
| `xoauth2` | Microsoft 365, Google Workspace (basic auth disabled) | `username` (the mailbox), then either `tokenEnv` (a ready bearer token) or `tokenUrl` + `clientId` + `clientSecretEnv` (+ `scope`) for the OAuth 2.0 client-credentials grant |

Microsoft 365 example (app registration with the `SMTP.SendAsApp` application
permission, and SMTP AUTH enabled on the mailbox):

```json
{ "name": "m365", "type": "smtp", "host": "smtp.office365.com", "port": 587, "startTls": true,
  "from": "printmarshal@contoso.com", "to": ["ops@contoso.com"],
  "username": "printmarshal@contoso.com", "authMethod": "xoauth2",
  "tokenUrl": "https://login.microsoftonline.com/<tenant-id>/oauth2/v2.0/token",
  "clientId": "<app-id>", "clientSecretEnv": "PM_M365_SECRET",
  "scope": "https://outlook.office365.com/.default" }
```

Tokens are cached per notifier until a minute before they expire.

### Microsoft Teams

`type: "teams"` posts an **Adaptive Card** in the shape the Teams *Workflows*
("Post to a channel when a webhook request is received") expects, since the
Office 365 connector MessageCard format was retired. Set `"legacy": true` on a
destination that still uses an old connector URL.

## Exact event catalogue

Generated from `internal/events/events.go`. Security export additionally includes all `alert.*` and `notification.dead`.

| Type | Notifiable | Security | Meaning |
| --- | --- | --- | --- |
| `device.discovered` | yes | no | A new device was added to the inventory |
| `device.updated` | no | no | A device record or its polled state changed |
| `device.deleted` | yes | yes | A device was removed |
| `alert.raised` | yes | no | An alert was raised |
| `alert.cleared` | yes | no | An alert was cleared (automatically or by an operator) |
| `alert.acked` | yes | no | An operator acknowledged an alert |
| `job.updated` | no | no | A job's progress changed |
| `job.finished` | yes | no | A job finished (succeeded, partial, failed or cancelled) |
| `config.applied` | yes | yes | A configuration write completed on a device |
| `security.assessed` | yes | yes | A security assessment completed for a device |
| `security.remediated` | yes | yes | A security remediation wrote to a device |
| `firmware.campaign.updated` | yes | yes | A firmware campaign changed state |
| `firmware.device.updated` | yes | yes | A device's firmware update progressed |
| `consumable.updated` | no | no | A stock item changed |
| `consumable.low` | yes | no | A stock item fell to its warning or critical level |
| `consumable.replacement` | yes | no | A supply replacement was detected or confirmed |
| `auth.login` | no | yes | A sign-in succeeded |
| `auth.failed` | yes | yes | A sign-in failed |
| `auth.denied` | no | yes | An authenticated request was refused by authorization |
| `auth.logout` | no | yes | A session ended |
| `auth.session.revoked` | no | yes | Sessions were revoked |
| `auth.token.created` | no | yes | An API token (service account) was created |
| `auth.token.revoked` | no | yes | An API token was revoked |
| `audit.recorded` | no | yes | An audit entry was written |
| `settings.updated` | yes | yes | Runtime settings changed |
| `notification.test` | yes | no | An operator sent a test notification |
| `notification.dead` | yes | no | A notification could not be delivered after all retries |
| `stream.connected` | no | no | (client-side) live stream connected |
| `server.stopping` | no | yes | The server is shutting down |

## SIEM export, ingestion and synchronization

PrintMarshal has three integration surfaces for the systems around it
:

1. a **security event export** that forwards the event bus's security events to
   syslog, a file or an HTTP collector (`internal/siem`);
2. a **printer syslog receiver** that stores the lines printers send and shows
   them per device;
3. an **incremental sync API** so a CMDB / ITSM / asset tool can mirror devices,
   alerts, audit entries, jobs and configuration writes without custom polling
   logic, never missing or repeating a row.

Everything is read-only towards printers: the receiver only listens, nothing
here ever sends anything to a device.

## 1. Security event export

### What is exported

Every event in the [catalogue](#exact-event-catalogue) whose **security** flag is set, plus
every `alert.*` event and `notification.dead` (`GET /api/events/catalog` lists
them; `GET /api/export/status` returns the exact list under `events`):

| Type | Why |
|---|---|
| `auth.login`, `auth.failed`, `auth.denied`, `auth.logout`, `auth.session.revoked` | sign-in and authorization trail |
| `audit.recorded` | every change to PrintMarshal's own records and every device write |
| `device.deleted`, `settings.updated` | inventory and runtime changes |
| `config.applied`, `security.assessed`, `security.remediated` | writes to devices and their assessments |
| `firmware.campaign.updated`, `firmware.device.updated` | firmware delivery |
| `alert.raised`, `alert.cleared`, `alert.acked` | operational alerts |
| `notification.dead` | a notification could not be delivered |
| `server.stopping` | shutdown marker |

The exporter is an `events.Subscriber` like the WebSocket hub and the notifier:
producers publish once and every destination sees the same envelope. Events go
through a bounded in-memory queue (`export.buffer`, default 4096) drained by one
worker, so a slow or dead collector can never block the bus. When the queue is
full the event is **dropped**, counted (`dropped` in the status endpoint) and
logged at error level (at most once every 10 s). The export is at-least-once
only as far as the sinks' own semantics go: UDP syslog is fire-and-forget, TCP /
TLS syslog reconnects and retries the next event, the file sink is durable on
the host, the HTTP sink retries each batch three times with backoff and then
drops the batch (counted as `failed`).

### Configuration

JSON (`export`) with `PM_EXPORT_*` overrides:

```json
{"export": {
  "buffer": 4096,
  "syslog": { "address": "tls://siem.example.com:6514", "format": "cef", "facility": 16, "caFile": "/etc/printmarshal/siem-ca.pem" },
  "file":   { "path": "/var/log/printmarshal/events.jsonl", "maxSizeMb": 50, "keep": 5 },
  "http":   { "url": "https://collector.example.com/ingest/printmarshal", "tokenEnv": "PM_EXPORT_HTTP_TOKEN", "batchSize": 100, "flushInterval": "5s", "timeout": "10s" },
  "ingestListen": "0.0.0.0:1514",
  "ingestRetention": "720h",
  "ingestRatePerMinute": 120
}}
```

| JSON | Environment | Default | Meaning |
|---|---|---|---|
| `export.buffer` | `PM_EXPORT_BUFFER` | `4096` | Queue length before events are dropped |
| `export.syslog.address` | `PM_EXPORT_SYSLOG` | off | `udp://host:514`, `tcp://host:6514` or `tls://host:6514` |
| `export.syslog.format` | `PM_EXPORT_SYSLOG_FORMAT` | `syslog` | `syslog` (RFC 5424 + structured data), `cef` or `json` |
| `export.syslog.facility` | `PM_EXPORT_SYSLOG_FACILITY` | `16` (local0) | Syslog facility 0–23 |
| `export.syslog.tlsInsecure` | `PM_EXPORT_SYSLOG_TLS_INSECURE` | `false` | Skip server certificate verification (lab only) |
| `export.syslog.caFile` | `PM_EXPORT_SYSLOG_CA_FILE` | system roots | PEM bundle that verifies the TLS collector |
| `export.file.path` | `PM_EXPORT_FILE` | off | JSON-lines file, appended |
| `export.file.maxSizeMb` / `keep` | `PM_EXPORT_FILE_MAX_MB` / `PM_EXPORT_FILE_KEEP` | `50` / `5` | Rotate above this size; keep `path.1` … `path.N` |
| `export.http.url` | `PM_EXPORT_HTTP` | off | `POST` target for JSON-lines batches |
| `export.http.tokenEnv` | `PM_EXPORT_HTTP_TOKEN_ENV` | none | Name of the variable holding a bearer token (the token itself never lives in the file) |
| `export.http.batchSize` / `flushInterval` / `timeout` | `PM_EXPORT_HTTP_BATCH` / `PM_EXPORT_HTTP_FLUSH` / – | `100` / `5s` / `10s` | Batch when full or after the interval; per-attempt deadline |
| `export.ingestListen` | `PM_EXPORT_INGEST_LISTEN` | off | UDP address of the printer syslog receiver |
| `export.ingestRetention` | `PM_EXPORT_INGEST_RETENTION` | `720h` (30 days) | Device log lines older than this are pruned hourly |
| `export.ingestRatePerMinute` | `PM_EXPORT_INGEST_RATE` | `120` | Lines accepted per source address per minute |

A sink whose address cannot be parsed (or whose file cannot be opened) is
logged at start and skipped; it never prevents the server from starting.

### Formats

**JSON lines** (file and HTTP sinks, and `format: "json"` over syslog) are the
event envelope exactly as the live stream carries it, one per line:

```json
{"id":"6f1c…","version":1,"type":"auth.failed","at":"2026-10-28T21:14:02.1Z","origin":{"source":"operator","actor":"kim","ip":"10.0.0.7"},"data":{"user":"kim","detail":"invalid credentials","result":"denied","ip":"10.0.0.7","summary":"kim: invalid credentials"}}
```

The HTTP sink posts batches as `Content-Type: application/x-ndjson`,
`User-Agent: printmarshal-export`, with `Authorization: Bearer <token>` when
`tokenEnv` is set. Any 2xx is success; anything else is retried (500 ms, 1 s,
2 s) and then dropped.

**Syslog** (`format: "syslog"`) is RFC 5424:

```
<132>1 2026-10-28T21:14:02.1Z pm-host printmarshal 4123 auth.failed [printmarshal@32473 id="6f1c…" type="auth.failed" source="operator" actor="kim" ip="10.0.0.7"] auth.failed kim: invalid credentials {"user":"kim",…}
```

- `PRI` = facility × 8 + severity. Severity: `alert.raised` critical → 2,
  other `alert.raised` / `auth.failed` / `auth.denied` → 4 (warning),
  `notification.dead` → 3 (error), `config.applied` / `security.remediated` /
  `device.deleted` / `settings.updated` / `auth.session.revoked` /
  `server.stopping` → 5 (notice), everything else → 6 (informational).
- `MSGID` is the event type; `APP-NAME` is `printmarshal`; `PROCID` the pid.
- The structured-data element `printmarshal@32473` carries the envelope:
  `id`, `type`, `source`, and when present `actor`, `ip`, `requestId`,
  `deviceId`. (32473 is the enterprise number IANA reserves for documentation,
  RFC 5612; PrintMarshal has no PEN of its own, and a reserved number cannot
  collide with a real vendor's.)
- `MSG` is a one-line summary followed by the event `data` as JSON.

Transport: UDP sends one datagram per event (truncated at 8 KiB); TCP and TLS
use octet-counting framing (`<length> <message>`, RFC 6587 / RFC 5425) with a
64 KiB cap, a 5 s dial / write deadline and automatic reconnection (at most
one dial every 2 s while the collector is down). TLS requires TLS 1.2+ and
verifies the server against the system roots or `caFile`.

**CEF** (`format: "cef"`) wraps an ArcSight Common Event Format record in the
same RFC 5424 header with a NILVALUE structured-data field, which is what
Microsoft Sentinel's and ArcSight's syslog/CEF connectors expect:

```
<134>1 2026-10-28T21:14:03Z pm-host printmarshal 4123 audit.recorded - CEF:0|PrintMarshal|PrintMarshal|1.2.0|audit.recorded|An audit entry was written|1|rt=1793222043000 externalId=6f1c… cat=audit.recorded suser=kim src=10.0.0.7 cs1Label=source cs1=operator act=device.delete outcome=ok msg=audit.recorded device.delete dev-1 by kim: ok cs6Label=target cs6=dev-1 flexString1Label=data flexString1={"action":"device.delete",…}
```

| CEF field | Value |
|---|---|
| Device Vendor / Product / Version | `PrintMarshal` / `PrintMarshal` / server version |
| Signature ID | event type |
| Name | the catalogue description of the type |
| Severity | 9 for syslog severity ≤ 2, 7 for error, 5 for warning, 3 for notice, 1 otherwise |
| `rt` | event time, milliseconds since the epoch |
| `externalId` | envelope id (stable; de-duplicate on it) |
| `cat` | event type |
| `suser` / `src` | origin actor / IP (for `auth.*` events the `user` field is used when the origin has no actor) |
| `cs1` `source` | origin source: `system`, `operator`, `device`, `scheduler` |
| `cs2` `deviceId` | the device the event is about, when any |
| `cs3` `requestId` | API request correlation id, when any |
| alerts: `dvchost`, `cs4` `alertSeverity`, `cs5` `alertSource`, `msg` | device id, severity, source, description |
| devices: `dvc`, `dvchost`, `msg` | address, name, summary |
| jobs: `cs4` `jobType`, `outcome`, `msg` | type, status, summary |
| audit / auth / settings: `act`, `outcome`, `cs6` `target`, `msg` | action, result, target, summary |
| `flexString1` `data` | the event payload as JSON (truncated at 4000 characters) |

Header fields escape `|` and `\`; extension values escape `=`, `\` and
newlines, per the CEF specification.

### Status

`GET /api/export/status` (viewer) reports the exporter, each sink and the
receiver:

```json
{
  "enabled": true,
  "sinks": [
    { "name": "syslog", "type": "syslog", "target": "tls://siem.example.com:6514", "format": "cef", "ok": true, "sent": 1532, "failed": 0, "lastAt": "2026-10-28T21:14:03Z" },
    { "name": "http", "type": "http", "target": "https://collector.example.com/ingest/printmarshal", "format": "json", "ok": false, "sent": 1400, "failed": 100, "lastError": "collector returned 503", "pending": 32 }
  ],
  "queued": 0, "queueSize": 4096, "received": 1532, "dropped": 0,
  "events": ["device.deleted", "alert.raised", "…"],
  "ingest": { "enabled": true, "listen": "0.0.0.0:1514", "listening": true, "received": 812, "stored": 790, "unknownSource": 22, "dropped": 0, "lastAt": "2026-10-28T21:10:40Z", "retention": "720h0m0s" },
  "syncCollections": ["devices", "alerts", "audit", "jobs", "config-writes"]
}
```

`ok` is true until a sink's last write fails; `sent` / `failed` count events
(for the HTTP sink, events in delivered / dropped batches). The same data is
shown under **Settings → Exports & SIEM**.

## 2. Printer syslog receiver

Most printers can send syslog to a server (the configuration catalogue's
`security.syslog.server` setting). Set `export.ingestListen` (for example
`0.0.0.0:1514`; port 514 needs root or `CAP_NET_BIND_SERVICE`) and point each
printer at the PrintMarshal host.

- UDP only, RFC 5424 and BSD (RFC 3164) formats; anything unparsable is kept
  verbatim with facility `user` and severity `notice`, subject to source attribution, rate limiting and database availability.
- A line is attributed to the device whose **address equals the source
  address** — the same trust model as the SNMP trap listener. Lines from an
  unknown address are counted (`unknownSource`) and discarded.
- Per-source rate limit (`ingestRatePerMinute`, sliding minute); drops are
  counted and logged at most once per source per minute.
- Stored with the **time of receipt** (printer clocks are not trusted),
  facility, severity and the message (app name prefixed, at most 2048
  characters, control characters stripped), in `device_logs`; pruned hourly
  beyond `ingestRetention`.
- Lines are evidence only: nothing is raised, polled or remediated from them.

`GET /api/devices/{id}/logs?limit=200` (viewer) returns the newest lines first:

```json
[{"id":17,"deviceId":"…","at":"2026-10-28T21:10:40Z","facility":4,"severity":2,"message":"lpd: Toner cartridge missing","severityName":"critical","facilityName":"auth"}]
```

Try it without a printer (the source address must match a device's address;
`127.0.0.1` works against a simulator device):

```bash
PM_EXPORT_INGEST_LISTEN=127.0.0.1:1514 ./bin/printmarshal
echo '<34>1 2026-10-28T21:14:02Z printer-7 lpd - - - Toner cartridge missing' | nc -u -w1 127.0.0.1 1514
```

## 3. Incremental sync API

```
GET /api/sync/{collection}?since=<cursor>&limit=<1..1000>
```

Collections: `devices`, `alerts`, `jobs`, `config-writes` (viewer) and `audit`
(operator, the same gate as `GET /api/audit`).

Response:

```json
{ "collection": "devices", "items": [], "cursor": "", "hasMore": false }
```

### Cursor semantics

- Rows are ordered by **(updatedAt, id) ascending** and the page contains only
  rows strictly after `since`. `cursor` is the position of the **last item in
  the page** (or `since` itself when the page is empty). It is opaque — base64
  of the stored timestamp and the id — so store it as a string and pass it
  back; do not build cursors yourself.
- This walks current records, not an immutable change log or a transactionally frozen snapshot. Intermediate changes can be coalesced. Upsert by id; a changed record may reappear at a later cursor. Persist a cursor only after processing its page.
- `hasMore` says whether another page exists right now. Walk until it is false,
  then poll with the last cursor on a schedule; an empty page means nothing
  changed.
- "Updated" means: `updatedAt` for devices and alerts; the entry time for
  audit; the latest of finished / started / created for jobs (so a job
  resurfaces when it starts and when it finishes); the write time for
  configuration writes.
- **Deletions.** Devices removed through `DELETE /api/devices/{id}` leave a
  tombstone and appear once in `devices` as `{ "id", "deleted": true,
  "updatedAt" }` at the time of deletion. Alerts, jobs and audit rows are never
  deleted by the API; a device deletion cascades to its alerts, which simply
  stop appearing (treat an alert whose device is gone as closed).
- Every response carries a weak `ETag` over the page; send it back as
  `If-None-Match` when re-polling the same cursor to get `304 Not Modified`.
- Limits: default 200 rows, maximum 1000.

### Example walk

```bash
PM=https://printmarshal.example.com
PM_TOKEN=$(curl -s -X POST $PM/api/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"sync-bot","password":"…"}' | jq -r .token)

cursor=""
while :; do
  page=$(curl -s -H "Authorization: Bearer $PM_TOKEN" "$PM/api/sync/devices?limit=500&since=$cursor")
  echo "$page" | jq -c '.items[]'            # upsert live rows, retire rows with .deleted == true
  cursor=$(echo "$page" | jq -r .cursor)
  [ "$(echo "$page" | jq .hasMore)" = true ] || break
done
echo "$cursor" > devices.cursor               # next run starts from here
```

Other collections work the same way:

```bash
curl -s -H "Authorization: Bearer $PM_TOKEN" "$PM/api/sync/alerts?limit=200"
curl -s -H "Authorization: Bearer $PM_TOKEN" "$PM/api/sync/audit?limit=200"          # operator role
curl -s -H "Authorization: Bearer $PM_TOKEN" "$PM/api/sync/jobs?since=$CURSOR"
curl -s -H "Authorization: Bearer $PM_TOKEN" "$PM/api/sync/config-writes?since=$CURSOR"
curl -s -H "Authorization: Bearer $PM_TOKEN" -H 'If-None-Match: W/"…"' "$PM/api/sync/devices?since=$CURSOR" -o /dev/null -w '%{http_code}\n'
```

Use a dedicated viewer (or operator, for `audit`) account for integrations so
its activity is attributable in the audit trail; the other list endpoints
(`GET /api/devices`, `/api/alerts`, …) remain the right tool for interactive
filtering and are not a sync contract.


## Notifier configuration and delivery boundaries

An empty `events` list subscribes to `alert.*`. `events:["*"]` selects catalogued notifiable types. Configure destinations in the file's `notifiers` array; there is no runtime destination CRUD API. Changing configuration requires restart. Keep destination names stable: pending rows resolve their name against the current configuration; removal makes them dead, and reusing a name sends old pending rows to that destination's current address.

| Type | Payload or transport |
| --- | --- |
| `webhook` or empty | JSON `{event,subject,body,data,at}` |
| `slack` | JSON `text` |
| `discord` | JSON `content` |
| `teams` | Workflow Adaptive Card; `legacy:true` selects MessageCard |
| `smtp` | Plain-text MIME; TCP port defaults to 587, optional STARTTLS; no implicit TLS listener on 465 |

For example, add this destination to a complete configuration (replace the URL before use):

```json
{"notifiers":[{"name":"operations","type":"webhook","url":"https://hooks.example.com/printers","events":["alert.raised","alert.cleared","notification.dead"]}]}
```

The outbox is durable after enqueue; it is not in the same transaction as the originating state change. Delivery is at least once: a receiver can accept a message before the sender fails to mark it delivered. Webhook JSON has no envelope-id field; use receiver-side application logic if deduplication is needed. Failed delivery of `notification.dead` is logged without generating another dead event, preventing two failing destinations from recursively notifying each other. The originating failed destination is also excluded.

## Prometheus and probes

`GET /healthz` is public process liveness; `GET /readyz` is public and checks `CountUsers` with a three-second deadline (503 on failure). Neither confirms printer connectivity or successful external authentication. `GET /metrics` requires viewer authentication; protect the scrape token and use a fleet-wide principal because aggregates are not group-filtered. Source: `internal/api/metrics.go`.

| Series | Labels / interpretation |
| --- | --- |
| `printmarshal_info` | `version`; constant 1 |
| `printmarshal_up` | API serving, not database or printer health |
| `printmarshal_devices_total`, `printmarshal_devices_offline` | inventory and reachability |
| `printmarshal_devices_by_severity`, `printmarshal_devices_by_state` | `severity`, `state` |
| `printmarshal_alerts_active`, `printmarshal_alerts_critical` | current alert totals |
| `printmarshal_supplies_low`, `printmarshal_stock_low` | device supplies and spare stock |
| `printmarshal_jobs_running` | running jobs |
| `printmarshal_notifications_outbox` | `state=pending/dead`; each query capped at 10,000 rows |
| `printmarshal_store_error` | emitted as 1 when the dashboard store read fails |
| `printmarshal_http_requests_total` | counter by `class=1xx` through `5xx` |
| `printmarshal_http_inflight` | in-flight requests |
| `printmarshal_http_latency_ms_total` | cumulative milliseconds, not a histogram |

## Known discrepancies

`docs/events.md` describes the catalogue as complete, but `internal/security/rotate.go` and `certs.go` emit `security.rotated`, `security.certificate.issued` and `security.certificate.installed` without catalogue entries. The bus still broadcasts them and logs unknown types. Wildcard notification and security export do not include these types as security events; corresponding `audit.recorded` events provide the exported audit trail. `stream.connected` is explicitly client-side. The notifier formatter drops device payloads except `device.discovered`, so a `device.deleted` subscription with a device payload does not produce a human notification despite its catalogue flag.

The older webhook deadline description says 20 seconds; the HTTP client imposes 10 seconds. The sync guide's absolute no-misses/no-repeats wording exceeds a mutable-record cursor's guarantee. SIEM's bounded queue is in memory and drops on overload; it is separate from the durable notification outbox. Group-scoped tokens do not filter the sync collections or fleet-wide metrics. See [Identity and access](Identity-and-access.md) before exposing integrations.

Sources: `internal/events/events.go`, `internal/alerts/notify.go`, `internal/alerts/smtpauth.go`, `internal/siem/`, `internal/api/events.go`, `internal/api/export.go`, `internal/api/sync.go`, `internal/api/ws.go`, `docs/events.md`, `docs/export.md`. See [configuration](Configuration-reference.md) for every destination/export key and [API reference](API-reference.md) for roles and bodies.
