# Intent: edit many printers at once (DNS sync, names, location, notes)

Author: the maintainer, from testing the work fleet on 31 October 2026. Status: draft. Issue: none yet (open one when accepted).

## Problem
A printer's display name can be set from its DNS name and kept in sync as the DNS name changes, but only one printer at a time, from its own page. There is no way to turn DNS sync on for many printers, or for a group, and nothing under Configuration covers it because configuration profiles hold settings written to printers, not how PrintMarshal presents them. With a fleet of a thousand printers that is a thousand clicks. The same applies to the other operator fields a fleet shares, such as the location of a site's printers. The devices list already has multi-select with poll, rename (from DNS, hostname, sysName or a pattern, with a dry run), add to group and export, so the mechanism exists; the fields do not.

Related: reverse DNS is an existing setting, off by default (Settings, Polling and discovery, Reverse DNS), so until it is enabled nothing picks up DNS names during discovery or polling and the per-device refresh is the only lookup. That is why names "had to be refreshed"; the setting is not obvious from the device page.

## Proposed outcome
From the devices list, select any set of printers (a group's printers via the group filter and "Select all") and edit them in one go: turn DNS sync on or off, rename them from DNS now, set a location, append a note. Each field can be left unchanged. The change is audited per device and respects group scope. The rename dialog gains "keep these names in sync with DNS" so one dialog does both. The device page says when reverse DNS is off, with a link to the setting, so an administrator understands why no DNS name is present.

## Affected users and systems
Operators and administrators; `web/src/pages/Devices.tsx` (bulk toolbar, rename dialog), `web/src/pages/DeviceDetail.tsx` (reverse-DNS hint); `internal/api/server.go` (a bulk patch route beside `renameDevices`), `store.DevicePatch`; wiki pages for discovery/inventory and the API reference.

## Constraints
- Operator fields stay operator-owned: a bulk edit is an explicit operator action, audited per device, never a scan.
- Group-scoped principals edit only devices in their scope (the existing `allowedAll` rule).
- Asset tags are per device and are not bulk-edited; notes are appended, never replaced, in bulk.
- No change to reverse-DNS defaults in this intent; whether to default it on is a separate decision.

## Open questions
- Should a bulk edit also be offered from a group's page ("edit all members")? The list with a group filter covers it; a group-page button would be a shortcut.
- Should reverse DNS default to on with its conservative timeout, given it is now the way names are kept? Owner decision; some sites have no DNS and some do not want the lookups.
