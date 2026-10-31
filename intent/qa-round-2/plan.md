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
