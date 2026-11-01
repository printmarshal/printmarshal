# Plan: group-scoped users can bulk edit the devices in their groups

From `intent/bulk-edit-scope/intent.md`. Spec skipped: a one-line fix against the bulk edit spec's R2.

## Files that change
`internal/api/scope.go`: `devicePathID` treats `bulk` like `rename`, as a route that names its devices
in the body. These are the only two such routes under `/api/devices/`.

## Order of work
1. Failing test in `internal/api/devices_bulk_test.go`, committed. 2. The fix.

## Risks
This route now gets past the path check, so the body check is what protects it. The test proves the
body check refuses a foreign device and changes nothing, including the in-scope device named with it.

## Proof
`TestScopedBulkEdit` fails first and passes without being edited; `make test`.
