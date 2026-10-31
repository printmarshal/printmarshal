# Plan: QA round 1 fixes

From `intent/qa-round-1/intent.md` (accepted in PR 56). Status: draft. Spec skipped: seven
small fixes whose correct behaviour the intent states; the owner's answers to the two open
questions are recorded here.

Decisions
- #49: relabel now, track later. The column becomes "Last check" and shows `status.at` as it
  does today; recording when a condition began is a separate change (it needs a stored field).
- #51: rename the widget "Recent jobs" (it already shows the latest jobs of any status), and
  change the dry-run result message to "would write N" wording.

## Files that change
Server (`internal/...`), each with a failing test first
- `internal/configmgmt/service.go` (#51): the dry-run result message becomes
  `"dry run: %d would be written, %d unchanged, %d skipped, %d failed"`; the apply message keeps
  "written". Test `TestDryRunMessageNeverSaysWritten` in `service_test.go`.
- `internal/api/dashboard.go` and `dashboard_test.go` (#53): every alert the dashboard returns
  carries `deviceName` (display name resolved by id, through the same name rule the device list
  uses) so the widget never needs a device it has not loaded. Test
  `TestDashboardAlertsCarryDeviceName`.
- `internal/api/reports.go` and `reports_test.go` (#46): the utilisation summary sends `null`
  (a pointer, `omitempty` off) for a device whose mono or colour counter is unknown, never 0,
  and the fleet totals sum known values only; a `pagesKnown` count says how many devices
  contributed. Test `TestUtilisationUnknownSplitIsNull`. (If the summary already sends `null`
  and only the page prints 0, the test asserts the existing behaviour and the fix is web-only;
  say so in the commit.)
- Status rating of a sleeping printer (companion to #49, from `intent.md`'s "verify the
  sleep-mode severity"): find where the control-panel status line ("Sleep mode is on.", HP
  `gdStatusDisplay` in `internal/plugins/hp/pml.go`) meets the severity rule in
  `internal/fleet` or the SNMP driver. If a sleeping printer is rated critical, add the failing
  test `TestSleepingPrinterIsNotCritical` and fix the rule; if it is not, report what rates
  those rows and stop. This step is bounded to an hour; anything larger becomes its own intent.

Web (`web/src`), no test runner exists today, so the proof is the typecheck plus a screenshot
per fix on the simulated fleet (see Proof)
- `pages/Settings.tsx` (#45): the audit table shows, per entry, the keys whose value differs
  between `before` and `after` as "field: old → new" lines (the arrow is a glyph in the table,
  not prose), falling back to the full JSON when either side is not an object; a "Show full
  entry" toggle per row reveals both complete values in a `<pre>`; the 80-character cut goes.
- `pages/Reports.tsx` (#46): a row whose mono or colour is `null` prints "—"; the fleet detail
  line reads "N mono, M colour, K devices without a split" when some are unknown; the chart
  gets a total-pages series beside mono and colour so a fleet without splits still charts.
- `pages/Consumables.tsx` (#48): the headline counts distinct devices ("Devices with a supply
  ≤ 15%") with the supply count beside it ("6 supplies"); the "Low (≤ 15%)" filter includes
  empty supplies, and "Empty (≤ 5%)" stays a subset.
- `dashboard/widgets.tsx` (#49, #51, #53): attention column header "Last check"; the jobs
  widget title "Recent jobs" (also in `dashboard/catalogue.ts`, id unchanged so stored layouts
  keep working); the alerts widget uses `a.deviceName` from the server and falls back to the
  id's first 8 characters, never the literal "device".
- `pages/Alerts.tsx` (#50): the subtitle reads `trap.enabled` from `/api/settings` (already
  exposed) and says "trap listener on" or "trap listener off"; no text until loaded.
- `wiki/API-reference.md`: `deviceName` on dashboard alerts; `null` counters and `pagesKnown`
  in the utilisation summary. `wiki/Monitoring-and-alerts.md`: the renamed widget and column.

## Order of work
1. Server tests (four, or three if the sleep check finds nothing) committed failing; one commit.
2. Server fixes with `PM_FIX_TASK=1`; `make test` green; one commit.
3. Web fixes; `npm run typecheck && npm run build`; restore `internal/web/dist/index.html`; one
   commit per page is fine, or one for all.
4. Wiki; commit.
5. Verification on the simulated fleet (Proof).
Tier: one medium-tier worker for all of it, the sleep-mode step included (it is reading and a
small rule change); escalate to careful only if the severity rule turns out to be shared by
several drivers.

## Risks
- Renaming "In-progress jobs" changes a visible title users may have hidden by id; the id is
  unchanged, so layouts hold.
- The audit diff view must not evaluate secrets: audit payloads for credentials carry
  redacted values already; the view shows what is stored and nothing more.
- #46's `null` could reach older report consumers (CSV export): check `exportDevicesCSV`-style
  writers print an empty cell, not "null".
- The sleep-mode rule may live in a vendor plugin and affect other vendors; the bounded step
  guards against a sprawling change.
- Not taken: a web test runner (a dependency change for a later decision); tracking condition
  start times (#49, later); showing only running jobs (#51, the owner chose the rename).

## Proof
- Step 1 output with the server tests failing; `make test` after step 2 with every package `ok`.
- `npm run typecheck` and `npm run build` clean after step 3.
- On the simulated fleet instance (`127.0.0.1:8084`, 22 simulated HP devices, admin sign-in):
  a screenshot of the audit trail after one bulk location edit showing "location: old → new"
  and the expanded entry (#45); Reports with a device without a split showing "—" and the
  totals line (#46); the Consumables headline with devices and supplies counts (#48); the
  dashboard with "Last check" and "Recent jobs" (#49, #51); the Alerts subtitle saying
  "trap listener off" on that instance (its trap listener is disabled) (#50); the alerts widget
  with every row named (#53). A dry run of any profile against two simulated devices, with the
  job result line pasted (#51). Nothing in this proof writes to a printer: dry runs read only,
  and the fleet is simulated on loopback.
