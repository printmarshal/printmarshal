# Plan: bulk device edit (from spec.md 2026-10-31)

## Files that change
- `internal/api/devices_bulk.go` (new): handler, validation, per-device loop, result type; `internal/api/server.go`: route; the DNS rename rule extracted from `renameDevices` into `dnsRenameTarget` and reused.
- `internal/api/devices_bulk_test.go` (new): scope 403, validation 400, dnsSync + location + append note applied and audited, rename-from-DNS skips suspect/empty names, missing id reported.
- `web/src/components/BulkEditDialog.tsx` (new), `web/src/pages/Devices.tsx` (toolbar button, rename checkbox), `web/src/pages/DeviceDetail.tsx` (hint), `web/src/types.ts`.
- `wiki/Discovery-and-inventory.md`, `wiki/API-reference.md`.

## Order of work
1. Extract `dnsRenameTarget`; keep `renameDevices` behaviour (existing tests stay green).
2. Handler + tests (failing first for the scope and append-note rules).
3. Dialog, toolbar button, rename checkbox; typecheck; screenshot of the dialog and of the result message against the simulator or the local fleet copy.
4. Device-page hint; screenshot with reverse DNS off.
5. Docs; plan updated if anything moved.

## Risks
- Partial application when one device fails mid-loop: the result reports per device, and each device's change is independent, so a retry is safe. Riskiest step: the note-append rule (data loss if wrong); covered by a test with an existing note and with an empty note.
- Options not taken: a background job for bulk edits (operator fields are cheap; a job adds a result page for little gain); group-level inherited settings (a different model).

## Proof
`make test` green with the new tests; `npm run typecheck`; screenshots of the dialog, the outcome message, and the device-page hint; an audit-log entry per changed device visible in Settings, Audit.
