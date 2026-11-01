# Plan: personal dashboard layout changes appear in the audit trail

From `intent/layout-audit/intent.md`. Spec skipped: a one-file fix. Issue #52.

## Files that change
`internal/api/dashboard.go`: `putLayout` and `deleteLayout` call `s.audit`.

## Order of work
1. Failing test in `internal/api/dashboard_test.go`, committed.
2. The two audit calls.

## Risks
A person who customises often adds entries to the trail. Saves happen on Done, not on each move, so
this stays small. Recording was chosen over excluding preferences because the default layout is
already recorded and an unexplained gap is what QA reported.

## Proof
`TestPersonalLayoutChangesAreAudited` fails first and passes without being edited; `make test`.
