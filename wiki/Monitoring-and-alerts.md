[Home](Home.md) · [Configuration reference](Configuration-reference.md) · [API reference](API-reference.md)

# Monitoring and alerts

## Poll scheduling and results

The first scheduled poll is due approximately ten seconds after startup. A five-second scheduler tick submits fleet polling with a default five-minute cadence (minimum 30 seconds). A still-running poll job suppresses a new scheduled poll. Device I/O is limited by startup `poll.concurrency` (32); a per-device guard rejects overlapping polls. Per-device scheduled/job polls use a 90-second context. `POST /api/devices/{id}/poll` is a synchronous operator request; `POST /api/poll` returns a queued job and accepts `deviceIds` or `groupId` (empty means the pollable fleet for an unscoped principal).

The engine tries a winning/fallback driver per `read_status`, `read_supplies`, `read_trays`, `read_counters`, `firmware_read`. The API exposes `servedBy`, `errors`, `reachable` and the updated device. Reachable means at least one read succeeded. Missing capability data preserves previously stored supplies/trays/counters; it is not zero. A partial poll can have no fresh status and says “reachable, status not read” rather than inventing an idle state.

| Observation | Interpretation |
| --- | --- |
| Status | Online flag, state, severity, message, detected errors and printer alert-table entries. |
| Supplies | Slot index, description/type/colorant/class, raw level/capacity/unit and nullable percent. `filled` represents a receptacle whose increasing fill reduces free capacity. |
| Trays | Input/output, index/name, media information, status, raw levels and nullable percent. |
| Counters | Lifetime total; mono/color/duplex only where available. Missing split is not a zero count. |
| Firmware | Observed version string, not an update entitlement or compatibility guarantee. |

## Alert lifecycle, sources and keys

Alerts carry stable source/key, severity, timestamps and acknowledgment identity. Raise reconciles a condition; acknowledgment records operator attention but does not resolve it. Clear closes it; a condition still present can raise again on later evaluation. Events distinguish `alert.raised`, `alert.cleared`, `alert.acked`.

| Source | Key / trigger | Clear behavior |
| --- | --- | --- |
| `offline` | `offline`; consecutive unreachable polls ≥ `offlineAfterFailures` (2). | A reachable poll establishes recovery. |
| `poll` | `state:down`, `error:<error>`, `device:<group>:<code>:<groupIndex>` (description/index fallback), `paper:<size>`. | Only a poll with fresh status reconciles this source. |
| `supply` | `supply:<index>`; known percentage at/below supply threshold, or waste free capacity at/below threshold. | Only a poll with fresh supply readings reconciles this source. |
| `trap` | `trap:` + lowercased description with spaces replaced by `-`, capped at 120 characters. | Link-up clears link-down immediately; otherwise the hold policy below. |
| `security` | `policy:<policyId>:<itemId>` for critical failures; `cert:expiry` for expiry observations. | Reassessment/expiry checks reconcile their own conditions. |
| `consumable` | `consumable:<stockId>`; stock low/critical/out. | Stock reconciliation after changes/replenishment. No device ID on fleet stock alerts. |

Supply defaults are warning 15%, critical 5%. Unknown percentages and class `other` do not trigger supply-threshold alerts. Paper alerts require every eligible non-manual input tray for a media size to be empty; a single empty tray with another stocked tray of that size is not fleet-level out-of-paper. Generic noPaper/inputTrayEmpty reasons are suppressed in favor of that rule. Generic low-toner reasons are likewise suppressed in favor of measured supply levels. Door/cover open, jams, absent toner/supply, offline and service requests have severity mappings in `internal/alerts/rules.go`.

A failed status or supply read does not prove resolution. `alerts.Evaluate` returns observed sources, and reconciliation clears only those sources; unreachable or counters-only polls preserve unobserved faults. Review per-capability errors when the header remains online with stale measurements.

## SNMP traps and the hold

Traps are enabled on UDP `0.0.0.0:1162` by default. Accept sources can be restricted by `trap.allowedSources` (CIDRs or single IPs); handled traffic is capped at 60 per source per minute unless configured otherwise. Known device address must exactly equal source address. Authentication is source-based; the listener accepts any community, not an independent credential proof. Preserve source IP through networking and restrict inbound traffic appropriately.

Informational traps reach the fleet. A `Link up` recovery resolves `trap:link-down` immediately and requests a poll. Other warning/critical traps raise alerts and queue reconciliation polls. `fleet.TrapAlertHold` is **five minutes**, a code variable rather than a config key. After that hold, the next successful fresh status poll clears old trap alerts; current conditions remain represented by the poll-source rules. A printer that stays unreachable can retain its trap alert longer than five minutes.

## Telemetry and retention

Metrics stored per poll include `status.online` (1/0), `status.severity` (0/1/2 when status was read), `supply.percent`, `tray.percent`, `counter.total`, optional `counter.mono` and `counter.color`. Supply labels are `index:description`; tray labels include kind/index/name. Renamed supply descriptions can create a new label series. Query per device with `metric`, `label`, RFC 3339 `from`/`to` and `limit`; default store limit is 5,000 samples ordered ascending. This can truncate a dense series at its oldest samples in the requested range; choose a suitable limit/range.

`telemetry.retention` defaults to 2160h (90 days); the scheduler deletes old rows every six hours. This is raw-sample pruning, not downsampling. Device syslog has a separate 720h default retention, pruned hourly. Alert/job/audit/outbox histories are not governed by telemetry retention.

## Reports

## Common parameters

| Query | Meaning | Default |
| --- | --- | --- |
| `from`, `to` | RFC 3339 times or `YYYY-MM-DD` dates. `to` is exclusive; a plain date for `to` means the end of that day. | last 30 days |
| `days=N` | Shorthand: `from = to − N×24h` (only when `from` is absent). | — |
| `group` | Group id (static or rule-based); scopes every report to its members. | all devices |
| `device` | One device id (mutually exclusive with `group`). | — |
| `top=N` | Length of top-N lists. | 10 |
| `format=csv` | CSV instead of JSON (`encoding/csv`), same columns as the JSON table. | json |
| `table=<name>` | Which table to export; `GET /api/reports` lists them per report (first is the default). | first table |

Ranges longer than 400 days are rejected (400). Unknown group/device → 404. A
server whose store lacks the report surface answers 503.

Days are UTC calendar days. `range.days` in every response is the number of UTC
days the range touches, so `days=7` spans eight calendar days when it starts
mid-day.

## Reports

### `GET /api/reports/utilisation` — pages per day

Source: `counter.total` (and `counter.mono` / `counter.color` when a device
reports them). For each device and day the volume is the delta between the last
sample of the day and the last sample of the previous observed day; the day
before the range supplies the baseline for day one. A drop means the counter was
reset: that day counts 0, the new value becomes the base and `resets` is
incremented. Without any baseline the first observed day uses the growth within
the day. Fleet totals, top-N devices, per-group totals (every group the devices
belong to), mono/colour split (`hasSplit`), average pages per day and per
device, and `idleDevices` (counter data present, zero pages) are derived from
those per-device-per-day numbers. Tables: `devices`, `days`, `groups`.

### `GET /api/reports/fleet-health` — reachability and severity

Source: `status.online` (1 per successful poll, 0 per failed one) and
`status.severity` (0/1/2). Per day: devices polled, devices reached at least
once, `reachablePct` (successful polls / polls), and devices by their worst
severity that day. Per device: polls, failed polls, availability, offline
minutes (time from an unreachable poll until the next poll, or the range end),
outage count (reachable → unreachable transitions) and mean outage length.
Tables: `devices`, `days`.

### `GET /api/reports/supplies` — levels, rates, projections, stock movement

Source: `supply.percent` (label `index:description`) plus the consumables stock
history. For each installed supply: level at the start of the range (last sample
before it, else the first in it) vs. the last sample, percentage points consumed
(rises are ignored), rate in %/day over the observed span, projected days to
empty and the date. A rise of 20 points or more between consecutive samples
counts as a replacement. Projections need at least half a day of history and
some consumption; otherwise the fields are `null` rather than guessed.
`replacements` lists replacement stock events (status pending / applied /
dismissed…) attributed to devices; `consumption` sums applied stock movements
per consumable (units out / in). Stock movements are not attributable to
devices, so that table stays fleet-wide when a group or device filter is set
(`consumptionScoped: false`). Tables: `supplies`, `replacements`, `consumption`.

### `GET /api/reports/alerts` — volume and response times

Source: the alerts table, by `raisedAt`. Per day by severity and source, totals
by source, per device (count, critical, warning, acknowledged, cleared, mean
time to acknowledge, mean time to clear), and fleet means. Means are over the
alerts that were actually acknowledged / cleared. Consumable stock alerts carry
no device and appear as the "Stock" row. Tables: `devices`, `days`, `sources`.

### `GET /api/reports/firmware` — version distribution

A point-in-time view of the device records (the range only scopes devices):
installed versions per make/model, and each device's status against the newest
registered compatible package — `current`, `outdated`, `unregistered` (no
package applies) or `unknown` (device reports no version). Uses the same
version comparison as the firmware feature (`internal/firmware/fwversion`).
Tables: `devices`, `models`.

### `GET /api/reports/compliance` — current placeholder

Returns `{range, available:false, reason}` unconditionally in this build, even though the security feature already stores assessments. The intended shape is in `internal/reports/compliance.go`: once a
store method lists `model.Assessment` rows by range, aggregate them per policy
and device (pass/fail counts, score trend, newest result per device) without
touching any security logic.

## Code layout

- `internal/store/store_reports.go` — `ReportStore`: the aggregate read surface
  (daily telemetry aggregates, last-sample-before, offline statistics from
  window functions, alert counts, replacement events, stock consumption).
  Implemented on `*SQLite` in `sqlite_reports.go` and `*Postgres` in `postgres_reports.go`; the existing
  `telemetry(device_id, metric, at)` index covers the queries, no new tables.
  It is embedded in the current `store.Store`; the reports service also declares its own read interface (`reports.Store`).
- `internal/reports` — report maths and shapes (`Utilisation`, `FleetHealth`,
  `Supplies`, `Alerts`, `Firmware`, `Compliance`), each with `Tables()` for CSV.
  Group scope uses the same static/rule membership as the rest of the app
  (`store.MatchRule`).
- `internal/api/reports.go` — routes (`reportRoutes`), parameter parsing, CSV.
- `web/src/pages/Reports.tsx`, `web/src/types-reports.ts` — the page: range
  control (7d / 30d / 90d / custom), group filter, one tab per report with KPI
  tiles, hand-built SVG charts (categorical `--cat1..5` colours, dashed second
  series, `<title>` hover values, `role="img"` labels that state the numbers),
  tables and "Export CSV" links.

## Tests

`internal/store/store_reports_test.go` (aggregates against seeded telemetry,
including a counter reset and an outage), `internal/reports/maths_test.go`
(deltas, trend/projection, firmware status), `internal/reports/service_test.go`
(every report end to end on SQLite), `internal/api/reports_test.go` (roles,
validation, CSV output, 503 without the report store).


## Known discrepancies

Security assessment APIs exist (`/api/compliance` and `/api/compliance/history`) and both stores implement the report aggregate surface, but `internal/reports/compliance.go` is still a placeholder: `/api/reports/compliance` answers `{available: false}`. The supplies report's fixed 20-point replacement heuristic and half-day projection minimum differ from consumables forecasting's configurable 30-point/default-three-day logic; these are separate calculations. Reports are read-only, and their group/device query filters are not an automatic authenticated group-scope boundary.

Sources: `internal/fleet/service.go`, `internal/engine/engine.go`, `internal/alerts/rules.go`, `internal/trap/listener.go`, `internal/api/{server,reports,security}.go`, `internal/reports/`, `internal/store/{sqlite,postgres}_reports.go`.


## Customising the dashboard

The dashboard is a catalogue of widgets (six number cards, security posture, fleet health, recent alerts, devices needing attention, supplies running low, in-progress jobs, recent activity, consumables low on stock, upcoming replacements). **Customise** opens an edit mode where each widget has buttons to move it up or down, hide it, or collapse it to its title; hidden widgets wait in a tray at the bottom with a **Show** button. **Done** saves the layout for your sign-in; **Use the default** removes it. An administrator also sees **Save as default for everyone**, which sets the instance default that applies to anyone without a personal layout, and **Clear default**. Layouts are presentation only: hiding a widget never changes what a role or a group-scoped principal can read, and a stored layout survives widgets being added or renamed in later releases (unknown ids are dropped, new widgets appear after the listed ones). The security-posture widget shows the compliance score, compliant, non-compliant and not-assessed devices, and the number of items reported as *unknown*, which are never counted as passing; it covers only the devices the signed-in principal may see.

## Public status board

The public status board is a page people can open without signing in, so staff and students can see whether a printer is up and has paper and toner before they walk to it. It is off until an administrator turns it on.

**Turning it on.** In **Settings → Public status**, switch **Enabled** on, set the board title (default "Printer status", up to 60 characters) and add printers with the picker (search and group filter, up and down buttons to set the order; at most 200). Each printer can carry a public label of up to 80 characters, such as "Library ground floor, colour", which is shown instead of its display name; the label lives in the board's configuration and does not change the device. The dashboard's edit bar links here as **Public board…**. Saving with no printers is allowed, with a warning that the board will be empty. **Preview** opens the saved board in a new tab, and a one-line summary says what the public will see.

**What it shows.** Each printer always shows its label. Six switches add more:

| Switch | Shows |
| --- | --- |
| Location | The printer's location text. |
| Up/down | Whether the last poll reached the printer; a printer that was not reached gets a "not reachable" pill. |
| State | The printer's own state word, such as sleeping or printing (off by default, so a sleeping printer never reads as down). |
| Paper | One pill per input tray: ok, low, empty or unknown. |
| Toner | A gauge per supply in its own colour, with the percentage when the printer reports it. |
| Alerts | Active alerts as a fixed category, such as "paper jam" or "cover open"; the alert text is never shown. |

**Address.** The board is at `/status`. Switch on **Unlisted** to move it to `/status/<slug>` with a random 22-character slug; **Regenerate** issues a new slug and retires the old address, and `/status` then says the board is not available. An unlisted address hides the board, it does not protect it: anyone who has the link can open it.

**Signage screens.** The console refuses to be framed. To show the board inside a signage system's page, list that system's origins (one per line, `https://host[:port]`) under framing origins; only the board's paths can then be framed by those origins. For a wall screen, open the board with `?wall=1`: the type grows to read from about three metres, printers are laid out as tiles, and when more than twelve are listed the board pages through them twelve at a time, ten seconds a page.

**What readers see.** The page refreshes every 30 seconds and shows "Updated N s ago"; when the data is older than three poll intervals it says the board may be out of date. A board that is off reads "This board is not available."; one with no printers reads "No printers are listed yet."; after three failed refreshes it reads "Status unavailable. Trying again." instead of showing old data.

The board never reads a printer itself. It shows the console's last poll, so it is as fresh as the poll schedule above, and opening it adds no traffic to the printers.
