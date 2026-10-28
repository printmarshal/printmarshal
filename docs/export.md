# Exports, SIEM and incremental sync

PrintMarshal has three integration surfaces for the systems around it
(application-integration audit findings A12 and A15):

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

Every event in the [catalogue](events.md) whose **security** flag is set, plus
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
"export": {
  "buffer": 4096,
  "syslog": { "address": "tls://siem.example.com:6514", "format": "cef", "facility": 16, "caFile": "/etc/printmarshal/siem-ca.pem" },
  "file":   { "path": "/var/log/printmarshal/events.jsonl", "maxSizeMb": 50, "keep": 5 },
  "http":   { "url": "https://collector.example.com/ingest/printmarshal", "tokenEnv": "PM_EXPORT_HTTP_TOKEN", "batchSize": 100, "flushInterval": "5s", "timeout": "10s" },
  "ingestListen": "0.0.0.0:1514",
  "ingestRetention": "720h",
  "ingestRatePerMinute": 120
}
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
  verbatim with facility `user` and severity `notice`, so no line is lost.
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
{ "collection": "devices", "items": [ { …device… }, { "id": "…", "deleted": true, "updatedAt": "…" } ], "cursor": "MjAyNi0xMC0yOFQy…", "hasMore": true }
```

### Cursor semantics

- Rows are ordered by **(updatedAt, id) ascending** and the page contains only
  rows strictly after `since`. `cursor` is the position of the **last item in
  the page** (or `since` itself when the page is empty). It is opaque — base64
  of the stored timestamp and the id — so store it as a string and pass it
  back; do not build cursors yourself.
- Because the order is total and the comparison strict, a client that keeps
  the last cursor it received **never misses a row and never sees a row
  twice** for a given change. A row that changes again after it was synced
  gets a later `updatedAt` and reappears, with its new state, in a later page.
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
