# Intent: renaming from DNS looks the name up, and the sync state is visible

Author: the maintainer (product owner), from testing bulk edit on a 22-printer fleet, 31 October 2026. Status: draft. Issue: none yet.

## Problem
With reverse DNS lookup off (the default), renaming many devices from DNS does nothing: the
bulk edit's "rename from DNS" and the rename dialog's DNS source use the DNS name already
stored on each device, which is empty until a lookup has run, so every device is skipped with
"DNS name is empty" and the page only says "22 unchanged (22 with a note, see the audit trail)".
The per-device "Use DNS name" button does a live lookup first, so one device at a time works,
which makes the bulk path look broken rather than merely uninformed. Separately, "keep these
names in sync with DNS" is applied and stored, but no page shows whether a selected device is
already synced: the rename dialog's checkbox starts unticked every time and the bulk dialog's
sync control starts at "leave", so an operator who checks back concludes the setting did not
stick.

## Proposed outcome
Renaming from DNS behaves the same whether it is one device or fifty: the server looks the name
up when it has none stored, in the preview as well as the apply, so the preview shows the real
names and the apply renames them. When a device still cannot be renamed, the dialogs say why,
on the page, not only in the audit trail. The current sync state is visible: the list marks
devices kept in sync, the bulk dialog and the rename dialog show how many of the selected
devices are already synced, and both say when reverse DNS lookup is off so sync will not keep
running.

## Affected users and systems
Operators and administrators; `POST /api/devices/rename` and `POST /api/devices/bulk`
(`internal/api/server.go`, `internal/api/devices_bulk.go`), `fleet.Service.RefreshHostname`;
`web/src/pages/Devices.tsx` (list, rename dialog, result messages) and
`web/src/components/BulkEditDialog.tsx`; the API reference and the inventory wiki page.

## Constraints
- Read-only toward devices: a DNS lookup is the only network activity added; nothing is sent to
  a printer. Discovery and polling are unchanged.
- Operator-set names are never overwritten by scans; this change only acts on explicit operator
  requests.
- Lookups are bounded: the existing reverse-DNS timeout per device, a small concurrency limit,
  and only for devices with no stored DNS name, so a fifty-device preview stays quick.
- Suspect DNS names (an address disguised as a name) stay skipped.
- No new dependencies; no change to stored data formats.

## Open questions
- Should a stored DNS name older than the refresh interval also be looked up again during a
  rename, or only an empty one? (First version: only empty.)
