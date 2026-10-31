# Intent: QA round 1 fixes

Author: the maintainer (product owner), from a read-only review of the web UI, 31 October 2026. Status: draft. Issues: #45, #46, #48, #49, #50, #51, #53.

## Problem
- #45 The audit trail truncates before/after values to 80 characters of JSON. Keys serialise
  alphabetically, so asset tag and display name fill the space and the field that changed
  (location, notes) is never visible; there is no way to expand the entry.
- #46 The utilisation report prints 0 mono / 0 colour for devices that do not report a
  mono/colour split, as soon as any device in the fleet does. Unknown becomes zero, and the
  chart has no series for known totals.
- #48 Fleet supplies headlines "Devices with a supply ≤ 15%" but counts supply rows, so six
  supplies on two devices reads as six devices; the "Low (≤ 15%)" filter excludes empty supplies.
- #49 "Since" in Devices needing attention shows the time of the last status update, which is
  refreshed on every poll, not when the condition began.
- #50 The Alerts subtitle always says "trap listener on", whatever the configured state.
- #51 The "In-progress jobs" widget lists the latest jobs of any status. Dry-run results read
  "dry run: 1 written, …", which reads as a completed write.
- #53 The Recent alerts widget labels an alert "device" when the device is not in the list the
  widget has loaded.

## Proposed outcome
- #45 Each audit entry shows the fields that differ between before and after, and the full
  values are reachable (expand or tooltip).
- #46 A device without a split shows its mono/colour as unknown ("—"), not zero; fleet totals
  sum only known values and the total-pages series is available.
- #48 The headline counts devices (or is labelled as supplies), and "Low (≤ 15%)" includes empty.
- #49 "Since" is the time the current condition began; if that is not known, the column is
  labelled for what it shows.
- #50 The subtitle reflects the effective trap-listener state reported by the server.
- #51 The widget shows running jobs only (or is renamed "Recent jobs"); dry-run results say
  "would write N" and never "written".
- #53 Alerts always show the device's display name, resolved by id.

## Affected users and systems
Operators and administrators. `web/src/pages/Settings.tsx` (audit table), `web/src/pages/Reports.tsx`
and the utilisation summary it consumes, `web/src/pages/Consumables.tsx`,
`web/src/dashboard/widgets.tsx` (attention, jobs, recent alerts), `internal/api/dashboard.go`,
`web/src/pages/Alerts.tsx`, `internal/configmgmt/service.go` (dry-run result message), and
where status condition start time is set in `internal/fleet`.

## Constraints
- Read-only toward devices: nothing here sends anything to a printer.
- Failing test first for each bug, committed before its fix.
- No change to stored data formats; audit payloads already stored are rendered differently, not rewritten.
- Unknown stays unknown: a value a device does not report is never shown as zero.

## Open questions
- #51: show only running jobs, or rename the widget to "Recent jobs"?
- #49: if condition start is not tracked today, add it (in memory or derived from alerts) or relabel the column?
