# Spec: bulk device edit (from intent.md 2026-10-31)

## Requirements
1. `POST /api/devices/bulk` (operator) takes `{deviceIds:[...], set:{dnsSync?:bool, location?:string, appendNote?:string}, renameFromDns?:bool}`; at least one of `set`'s fields or `renameFromDns` must be present; `deviceIds` is required and capped at 2,000.
2. Group scope applies: a scoped principal naming a device outside its groups gets 403 before any change (the existing `allowedAll` rule). A missing device id is reported in the result, not fatal.
3. For each device, in order: `renameFromDns` sets the display name from the device's DNS name when one is present and not suspect (the same rule as `POST /api/devices/rename` with source `dns`), recording `displayNameSource: dns`; then `set.dnsSync`, `set.location` apply; `set.appendNote` adds a line to the notes (a blank line separates it from existing notes; an empty existing note gets the text alone).
4. The response lists per device `{id, changed:[fields], skipped?:reason}` and totals; an audit entry `device.patch` is written per changed device with before and after for the fields touched, actor from the request, result ok.
5. The devices list's bulk toolbar gains **Edit…**, opening a dialog with: DNS sync (leave / on / off), "Rename from DNS now" (checkbox, with the note that devices without a DNS name are skipped), Location (leave / set to …), Add note (text). Submit shows the per-device outcome summary in the page message and reloads the list.
6. The existing Rename dialog gains a checkbox "Keep these names in sync with DNS" shown when the source is DNS; checked, the rename request is followed by a bulk patch of `dnsSync:true` for the devices that were renamed.
7. The device page shows, beside the DNS name field, "Reverse DNS is off; names are looked up only when you refresh" with a link to Settings when `GET /api/settings` reports `runtime.reverseDns.enabled` false. Administrators see the link; others see the sentence.
8. Nothing here changes reverse-DNS defaults or performs lookups; "rename from DNS" uses the DNS name already stored on the device.

## Design
- **API.** New handler `bulkPatchDevices` in `internal/api/devices_bulk.go` (new file) registered as `POST /api/devices/bulk` (operator). It reuses `store.DevicePatch` and the rename rule by calling a shared helper extracted from `renameDevices` (`dnsRenameTarget(d) (to string, reason string)`). Validation: location trimmed, at most 200 characters; note at most 2,000 characters; dnsSync a bool. Per-device work runs sequentially (operator fields, SQLite-friendly); the result is JSON.
- **Scope.** `authorize` already peeks `deviceIds` in bodies, so the 403 for a foreign id happens before the handler; the handler also calls `allowedAll` as belt and braces.
- **Web.** `Devices.tsx`: `BulkEditDialog` component (new file `web/src/components/BulkEditDialog.tsx`) using the existing dialog and form controls; the rename dialog's checkbox; `DeviceDetail.tsx`: reverse-DNS hint reading settings once. `types.ts`: `BulkPatchResponse`.
- **Docs.** Wiki `Discovery-and-inventory` ("Editing many devices at once"), `API-reference` (the route).

## Flagged concerns
- **Audit volume.** A thousand-device edit writes a thousand audit rows; acceptable (it is a thousand changes) and consistent with per-device audit elsewhere.
- **Notes append vs replace.** Replace in bulk would destroy per-device notes; append only. Reviewer: check the separator rule.
- **Brand.** Dialog uses the existing controls; no new colours.

## Out of scope
Bulk asset tags, bulk web-URL overrides, group-level settings with inheritance (a different model; see OBS-2's second half), and any change to reverse-DNS defaults.
