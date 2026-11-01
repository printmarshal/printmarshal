# Plan: usability follow-ups from the UI test pass

From `intent/usability-followups/intent.md`. Spec skipped: small fixes whose correct behaviour the
intent states. Issue #54. The web has no test runner, so proof there is typecheck, build and
screenshots; the Go changes have failing tests first.

## Decisions
- **Sorting happens in the API, in Go.** Numeric addresses, version order and lowest-supply order are
  awkward to write twice in SQL. `GET /api/devices` and the CSV export take `sort` and `dir`. The
  list is sorted before `limit` and `offset` are applied. The store's own sort still serves the
  dashboard's attention list.
- **An empty "Any" tray is not out of paper while another tray has paper.** "Any" is not a size, so
  those trays join the group with no known size, and that group raises only when every eligible tray
  is empty. Old `paper:Any` alerts clear at the next poll.
- **`-1` for the default tray means none is set.** RFC 3805 uses -1 when the printer has no default
  input as a whole. Only the display changes.
- **Phones start on cards.** Below 900 px the device list opens as cards unless the person has chosen
  a view. On the dashboard's tables, secondary columns hide below 640 px.

## Files that change
- `internal/api/device_sort.go` (new), `internal/api/server.go`: sort, direction, paging.
- `internal/alerts/rules.go`: the paper rule.
- `internal/reports/reports.go`: `Range.Decommissioned`.
- `web/src/pages/Devices.tsx`: sortable headers, direction, default view, supply labels, DNS lookup.
- `web/src/components/ui.tsx`: `supplyTypeLabel`, `LongMessage`.
- `web/src/components/config/shared.tsx`: the default tray's value in words.
- `web/src/pages/DeviceDetail.tsx`, `web/src/components/consumables.tsx`: supply labels.
- `web/src/pages/Login.tsx`: required fields, wrong-password wording.
- `web/src/pages/Groups.tsx`: saved counts, "not saved yet".
- `web/src/pages/Credentials.tsx`, `Settings.tsx`: a read-only note for roles that cannot change them.
- `web/src/pages/Reports.tsx`, `web/src/types-reports.ts`: the decommissioned count.
- `web/src/pages/PublicStatus.tsx`: says which number the toner figure is.
- `web/src/pages/Discovery.tsx`, `JobDetail.tsx`: long messages collapse.
- `web/src/pages/settings/PublicStatus.tsx`: Regenerate's effect stated beside the button.
- `web/src/dashboard/widgets.tsx`, `web/src/styles.css`: narrow tables, sort buttons, dark theme.

## Order of work
1. Failing Go tests, committed: sort, paper rule, report scope.
2. The Go changes.
3. The web changes.
4. `make test`, web typecheck and build, end to end, screenshots on the simulator.

## Risks
- The riskiest step is the bulk DNS lookup: it runs one request per selected device. It runs on a
  person's click, at most four at a time, and only looks names up in DNS.
- The API now reads the whole list before paging. Fleets are small enough, and the scope and rule
  filters already had to work on the whole list to page correctly.
- Hiding an empty "Any" tray's alert could hide a real empty bypass tray. It stays visible per tray
  on the device page, which is the spec's rule for individual trays.

## Reviewed and left alone
- Wall mode and empty location labels on the public board: QA saw these working as designed.
- The online-state label on the public board: fixed since by #77.
- A successful DNS rename could not be tested in QA because no names resolved, and still has only
  the simulator's coverage.

## Proof
`TestSortDevices`, `TestDeviceListSortRoute`, `TestPaperRuleTreatsAnyAsNoSize` and
`TestReportRangeCountsDecommissioned` fail first and pass without being edited. Screenshots of the
device table, phone width, rename dialog, sign-in and the dark theme.
