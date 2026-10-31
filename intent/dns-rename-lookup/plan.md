# Plan: renaming from DNS looks the name up, and the sync state is visible

Spec skipped: a bug fix whose requirements are the intent's proposed outcome; this plan is the
design.

## Files that change
1. `internal/api/server.go`: a helper `ensureDNSNames(ctx, devices []*model.Device)` that, for
   every device whose trimmed `DNSName` is empty, calls `s.Fleet.RefreshHostname` with a
   concurrency limit of 8 (a buffered-channel semaphore, no new dependency) and replaces the
   device with the refreshed record; errors keep the original record. `renameDevices` calls it
   on the loaded devices when `source` is `dns`, or `pattern` containing `{dnsName}`, before the
   per-device switch, for dry runs and applies alike.
2. `internal/api/devices_bulk.go`: when `renameFromDns` is set, the same helper runs before the
   loop. Because `RefreshHostname` itself renames a device that already has `dnsSync`, record
   each device's name before the refresh and report `displayName` as changed when the refreshed
   record's name differs, instead of "unchanged".
3. `internal/api/server_test.go` / `devices_bulk_test.go`: a `fakeResolver` for the api package
   (LookupAddr returns a name for a given address, LookupHost the reverse), set on
   `s.Fleet.Resolver`. Tests, written first and committed failing:
   - `TestRenamePreviewLooksUpEmptyDNSName`: device with empty DNS name; `POST /api/devices/rename`
     `{source:"dns", dryRun:true}` returns the looked-up name, not skipped.
   - `TestBulkRenameLooksUpEmptyDNSName`: `POST /api/devices/bulk` `{renameFromDns:true}` renames
     the device and reports `displayName` changed; the stored device has `displayNameSource: dns`.
   - `TestBulkRenameCountsSyncRename`: device with `dnsSync:true` and an empty DNS name is reported
     changed (not "unchanged") after the refresh renamed it.
   - `TestRenameSkipsSuspectAfterLookup`: the resolver returns `10-0-0-9.example.net` for 10.0.0.9;
     the device stays skipped with "DNS name is suspect".
4. `web/src/pages/Devices.tsx`: the bulk result message aggregates skip reasons ("Edited 0
   devices; 22 skipped: DNS name is empty"); `RenameDialog` receives the selected devices, starts
   "keep in sync" ticked when every selected device already has `dnsSync`, and shows "already on
   for N of M"; the list shows a small muted pill "DNS sync" under the name of a device with
   `dnsSync` (same place as the "DNS name changed" pill).
5. `web/src/components/BulkEditDialog.tsx`: takes the selected devices, shows "Currently on for N
   of M" beside the DNS sync control, and the same reverse-DNS-off hint the device page shows
   (read `runtime.reverseDns.enabled` from `/api/settings` as `DeviceDetail.tsx` does).
6. `wiki/API-reference.md` (rename and bulk: lookup on empty DNS name; `displayName` reported
   changed when sync renamed it) and `wiki/Discovery-and-inventory.md` (one sentence on the
   indicator and the dialogs' hints).

## Order of work
1. Tests (step 3) on the branch; `go test ./internal/api -run 'Rename|Bulk'` shows the four new
   tests failing; commit "Test: renaming from DNS must look up an empty DNS name".
2. Server (steps 1 and 2) with `PM_FIX_TASK=1` set so test files stay untouched; `make test`
   green; commit.
3. Web (steps 4 and 5); `cd web && npm run typecheck && npm run build`; restore
   `internal/web/dist/index.html`; commit.
4. Wiki (step 6); commit.

## Risks
- A fleet of devices without DNS records makes every rename wait for lookups: bounded by the
  per-lookup timeout (1 s default) and the concurrency limit, so fifty devices cost about seven
  seconds at worst; the preview shows the result once, not per keystroke.
- `RefreshHostname` writes `dnsCheckedAt` and may rename synced devices as a side effect of a
  preview; that is the existing per-device button's behaviour and is what sync promises.
- Riskiest step: the counting change in bulk (step 2); covered by `TestBulkRenameCountsSyncRename`.
- Not taken: looking up stale names too (open question in the intent); a server-side skip-reason
  summary (the UI aggregates the per-device reasons it already receives).

## Proof
- `make test` output with the four tests passing; the first commit's `go test` output showing them
  failing.
- `cd web && npm run typecheck` output.
- A run of the branch binary on `127.0.0.1:8083` against a copy of the test data dir
  (`cp -R ~/printmarshal-test-data`), signed in as `admin`: bulk rename of five devices from DNS
  through the UI, then the list showing the new names and the "DNS sync" pills; a screenshot.

## Departures
- Hint wording: the device page's hint says "press Refresh", which does not fit a dialog, so
  both dialogs say "Reverse DNS is off, so DNS sync will not keep these names up to date on its
  own." with the same Settings link (or the administrator line). The rename dialog shows it
  too, as the intent asks of both dialogs.
- `selected` in `Devices.tsx` is memoised on the selection, so an open rename preview, which
  may now run lookups, does not re-run on every list refresh.
- Rename-route counting: `renameDevices` keeps each device's name from before the lookup. When
  the lookup itself renamed a device kept in sync, the row reports `from` (the earlier name) →
  `to`, not skipped, and is counted in `updated`, in a preview as well as an apply; the apply
  does not patch it again. Test: `TestRenameCountsSyncRename`, committed failing first.
- Rename dialog checkbox: it starts ticked when every selected device is already synced. On
  apply with source DNS, ticked when it started unticked posts `set.dnsSync: true` for all
  selected devices (including ones skipped as unchanged); unticked when it started ticked posts
  `set.dnsSync: false` for all of them; left as it started, nothing is sent.
