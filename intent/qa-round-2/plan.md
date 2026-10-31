# Plan: UI test pass, round 2 fixes

From `intent/qa-round-2/intent.md`. Spec skipped: small fixes whose correct behaviour the intent states.
Each group lands as its own branch and pull request. Bug fixes in Go have a failing test committed first.
The web has no test runner, so proof there is typecheck, build and screenshots.

## Group A: report tabs and failed refresh (#64, #71)
- `web/src/pages/Reports.tsx`: tie loaded data to the tab and query that asked for it, so a tab never renders the previous
  tab's report, and ignore stale responses. Show a loading state and a retry on error.
- `web/src/pages/Dashboard.tsx`: a failed refresh keeps the last data and shows a dismissible banner with Retry. A failed first load shows
  the error with Retry. A later success clears the error.
- An error boundary around the routed page in `web/src/App.tsx`, so a render error leaves navigation in place and offers a reload.

## Group B: public board (#66, #67, #72)
- `internal/api/public.go`: supply alerts map to a category by the part they concern, so toner only means marker supplies. Test first.
- `web/src/pages/PublicStatus.tsx`: online state in words when the board shows it, and no state claim when it does not.
- `internal/api/public.go`: `public.status.changed` records before and after values for the settings, without the slug. Test first,
  including that the slug appears nowhere in the audit entry.

## Group C: settings, activity and report wording (#68, #69, #70)
- `web/src/pages/Settings.tsx`: a section the role may not open explains that and names the role it needs.
- Dashboard customise text only describes links the role can see.
- A shared audit change view used by Settings and device Activity, including Show full entry.
- `internal/reports/compliance.go`: a true reason, and the report links to Security. Test first.

## Group D: decommissioned printers (#65)
- `internal/api`: viewing a decommissioned device's configuration never contacts it and says so. Test first, using a driver that
  fails the test if called. List any other view-triggered reads and apply the same rule.
- Test-instance fixture cleanup happens outside the repository.

## Risks
- Riskiest step: the public board change, since it is anonymous. Mitigation: existing allow-list tests stay, and new tests assert the slug's absence.
- Reports change touches every tab. Mitigation: click through all six tabs in both directions.

## Proof
`make test`, `cd web && npm run typecheck && npm run build`, and screenshots of each visible change against `cmd/pm-sim` on loopback.

## As built, group C
The dashboard customise text needed no change: `web/src/dashboard/EditBar.tsx` already varies by role and never described a link the role lacks.
The compliance report still does not read stored assessments. That is a moderate change, left for later: the report's store interface, aggregation, CSV rows and a results view.

## As built, group B
Supply alerts are categorised from the device's own supply data, because the alert key only carries an index. A part with no usable type or no matching supply is "other".
Beyond the plan: an active offline alert now reads "Not reachable" even when the up/down signal is off, "Needs service" uses the warning tone for a warning-level part, and the unreachable "supply means toner" row in `alertCategory` is gone, with its test row updated.

## Follow-up after verification
Verification found the offline-alert rule above was wrong in both directions: an acknowledged or not-yet-raised alert left a stopped printer reading "Ready", and `state:down` made a reachable printer read "Not reachable" beside "Online". It is replaced.
The board sends one more allow-listed field, `unconfirmed: true`, only when the up/down switch is off and the printer did not answer its last poll or is decommissioned, and such a printer reads "No status". With the switch on, a decommissioned printer has `online: false` and state `unknown`. `state:down` is now category `other`, because the printer answered the poll.
This reveals a little more than the up/down switch implies: a hidden-switch board now shows which printers it can't confirm. Reading "Ready" on a stopped printer was the worse outcome.

## As built, group D
The guards reach beyond `internal/api`, into `fleet`, `configmgmt`, `security` and `firmware`, because explicit device ids could reach apply, revert, remediate, rotate, enroll and install on a decommissioned printer.
A target of only decommissioned printers is refused with 409, and a refused real write is audited. In a longer target, a decommissioned printer gets a skipped result and the rest proceed. A trap from a reused address now reaches the printer in service.
A new setting, `discovery.zeroconf`, lets scheduled discovery scan only its listed targets. The default keeps today's behaviour.
Device status shows "Not contacted" on a decommissioned printer, in place of its last polled status.
Not changed: discovery sweeps still identify by address, certificate expiry alerts still come from stored data, alerts active at decommission stay active, and syslog lines are still stored against retired records.
