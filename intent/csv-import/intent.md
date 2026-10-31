# Intent: onboard printers from a CSV file

Author: the maintainer (product owner), 31 October 2026, from onboarding a 22-printer fleet. Status: draft. Issue: none yet (open one when accepted).

## Problem
The only way a printer enters the inventory today is a network scan: Discovery takes a range or
a list of addresses, probes them, and identifies what answers. Many fleets already exist as a
list somewhere else: a spreadsheet kept by facilities, an export from a print server or a
print-management system, an asset register. An administrator holding such a list has to type the
addresses into a scan, wait for identification, and then set every operator field (display name,
location, asset tag, notes, group) one device at a time, or in bulk after the fact. Nothing in
that list carries over, and a printer that is switched off during the scan is not created at
all, so the list and the inventory never match on the first day.

## Proposed outcome
An administrator can import printers from a CSV file on the Discovery page. The minimum is one
column, the address; every operator field the console knows is an optional column (display name,
location, asset tag, notes, web address, DNS sync, lifecycle state, group), and a hostname or a
serial number may stand in for the address when the file has one and the console can resolve or
match it. The import shows a preview before it changes anything: each row with what will be
created, what will be updated, what is skipped and why (bad address, duplicate row, unknown
group, a field over its limit), and the count of each. On apply, new devices are created with the
file's fields and queued for identification exactly as discovered devices are; existing devices
matched by address or serial get only the fields the file supplies, and a field an operator has
already set is not overwritten unless the administrator chooses "replace" for that column.
Every imported value records that it came from an import, so a later scan never overwrites it
(the existing rule for operator-set fields) and the device page can say where a value came from.
The same path is meant to serve later onboarding sources, a print server's queues or a
print-management system's export, which produce the same shape of candidate list.

## Affected users and systems
Administrators (operators may preview, only administrators apply, since an import can create
devices and change many records at once); `web/src/pages/Discovery.tsx` (an "Import CSV" action
beside the scan, the preview table, the apply step); a new `POST /api/discovery/import`
(preview and apply, multipart or JSON) in `internal/api`; `internal/discovery` for the candidate
list and identification queue; `internal/store` for matching by address and serial and for the
field-source record; `internal/model` if a field-source value is added to operator fields; the
audit trail (one entry per import with counts, one per device changed); the API reference and
the discovery wiki page.

## Constraints
- Read-only toward devices: an import writes to the inventory, never to a printer.
  Identification of new rows uses the existing read-only probe.
- Operator-set fields are never overwritten by scans; the import respects the same rule unless
  the administrator opts in per column, and the audit entry records that choice.
- Unknown stays unknown: an empty cell leaves the field untouched; it never writes an empty
  string over a value.
- Bounded: a file of at most 5,000 rows and a few megabytes; the preview is computed without
  contacting any device; apply runs as a job with progress and can be reviewed afterwards.
- Group scope: a scoped administrator can only import into groups they may see and only touch
  devices in their scope; rows outside it are skipped with that reason.
- No new dependencies; CSV parsing with the standard library; UTF-8 with or without a BOM,
  comma or semicolon separators detected from the header.
- Nothing from the file reaches logs beyond counts and row numbers; addresses and names appear
  in the audit entry only as the per-device before/after the audit already records.

## Open questions
- Should a row without an address but with a serial create a device that waits for a later scan
  to find its address, or be skipped until the scan has run?
- Is "replace operator fields" one switch for the whole import, or per column?
- Should the import also accept the console's own CSV export format, so an export from one
  instance imports into another?
- Where does the field-source record live: a per-field map on the device, or an audit lookup?
  (The group-inheritance work needs the same answer.)
