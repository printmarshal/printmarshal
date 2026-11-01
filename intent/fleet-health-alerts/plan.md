# Plan: a printer with an active alert is not Healthy

From `intent/fleet-health-alerts/intent.md`. Spec skipped: a contained fix whose correct behaviour
the intent states. Issue #47.

## Decisions
- **Warnings count as well as criticals.** A device with an active warning that reads Healthy is
  the same contradiction, and the metric is already documented as "worst current severity".
- **Sources that count: `offline`, `poll`, `supply`, `trap`.** Security findings do not: they have
  their own posture widget, and one failing critical policy item would turn a whole fleet Critical
  and bury real faults. Stock alerts have no device. A new source counts only once it is listed.
- **Acknowledged alerts still count.** Acknowledging records that someone has seen it. The drum is
  still at 5%, so Healthy would still be untrue. Clearing is what restores it, and the poll clears
  an alert when the condition ends.
- **Computed when read, not stored.** A SQL expression over `devices` and `alerts` is correct the
  moment an alert is raised, cleared or a threshold changes, needs no migration and cannot drift.

## Files that change
- `internal/store/sqlite_devices.go`: `effectiveSeveritySQL`, a `deviceSelectCols` for reads
  (`deviceCols` stays for the INSERT), and its use in the filter, the sort and `Dashboard`.
- `internal/store/postgres_devices.go`, `sqlite_logs.go`, `postgres_logs.go`: the same.
- `internal/model/model.go`: `Device.Severity`, the effective value; `HealthAlertSource`.
- `internal/fleet/service.go`: `device.updated` and the poll's result are read back after alerts
  reconcile, so they carry the new severity.
- `internal/api/dashboard.go`: attention uses `Device.Severity` and each row carries a `reason`.
- `internal/api/server.go`: the CSV export's severity column.
- `web/src/types.ts`, `web/src/components/ui.tsx`, `web/src/dashboard/widgets.tsx`: the pill, the
  Reason column and the Fleet health caption.

## Order of work
1. Failing tests, committed: the store conformance suite, the dashboard and export routes, the poll.
2. The store, then the fleet service, the API and the web.
3. `make test`, Go tests against PostgreSQL, web typecheck and build, a screenshot on the simulator.

## Risks
- The riskiest step is the expression in the device read path, used by every device query. A wrong
  expression breaks both backends, so the conformance suite runs against PostgreSQL as well, which
  CI does not do. `alerts_device` (`device_id`, `key`) serves the correlated lookups.
- More devices will read Warning than before, on fleets with many supplies under the warning level.
  That is the intended honest count, and the threshold is already a setting.
- Not changed: the Fleet health report's history. It charts each day's worst polled status, says so
  on the page, and old samples cannot be recomputed, so it can differ from today's dashboard.

## Proof
Failing tests first (`TestSQLite/DeviceSeverityCountsActiveAlerts`,
`TestDashboardAttentionFollowsActiveAlerts`, `TestPollResultCarriesAlertSeverity`), then green
without editing them. A screenshot of the dashboard with a simulated low drum.
