# Reports

Reports turn the telemetry, alert and stock history PrintMarshal already keeps
into fleet-level answers over a time range (build spec §8.2 CSV export, §8.3
trends, §8.5 "compliance reports per device, group and policy, over time").
Everything is read-only, computed on request from aggregate SQL plus a little Go
maths, and open to the viewer role. The UI lives at **Reports** in the sidebar
(`/reports`); the API is under `/api/reports/*`.

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

### `GET /api/reports/compliance` — hook

Returns `{available: false, reason}` until the security feature stores
assessments. The intended shape is in `internal/reports/compliance.go`: once a
store method lists `model.Assessment` rows by range, aggregate them per policy
and device (pass/fail counts, score trend, newest result per device) without
touching any security logic.

## Code layout

- `internal/store/store_reports.go` — `ReportStore`: the aggregate read surface
  (daily telemetry aggregates, last-sample-before, offline statistics from
  window functions, alert counts, replacement events, stock consumption).
  Implemented on `*SQLite` in `sqlite_reports.go`; the existing
  `telemetry(device_id, metric, at)` index covers the queries, no new tables.
  It is not yet embedded in `store.Store`; the API reaches it through a type
  assertion (`reports.Store`).
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

`internal/store/sqlite_reports_test.go` (aggregates against seeded telemetry,
including a counter reset and an outage), `internal/reports/maths_test.go`
(deltas, trend/projection, firmware status), `internal/reports/service_test.go`
(every report end to end on SQLite), `internal/api/reports_test.go` (roles,
validation, CSV output, 503 without the report store).
