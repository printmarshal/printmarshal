# Plan: exported CSV cannot run as a spreadsheet formula

From `intent/csv-formula-safety/intent.md`. Spec skipped: a contained security fix.

## Files that change
- `internal/csvsafe/csvsafe.go` (new): `Cell`, `Undo`, and a `Writer` that applies `Cell` to every
  field, so a new column cannot be forgotten.
- `internal/api/server.go`, `reports.go`, `consumables.go`: exports use `csvsafe.NewWriter`; the
  consumables import applies `Undo`.
- `internal/importcsv/parse.go`: applies `Undo` to values.

## Order of work
1. Failing tests, committed. 2. The package and the swaps.

## Risks
A spreadsheet shows the apostrophe if a person edits the raw file by hand, and scripts that parse
the export see it on values that began with a trigger character. That is rare and is the accepted
cost of the defence. A value already starting with an apostrophe and a trigger gets a second one, so
the round trip is exact.

## Proof
`TestCell`, `TestUndoReversesCell` and `TestCSVExportsNeutraliseFormulas` fail first and pass
without being edited; `make test`.
