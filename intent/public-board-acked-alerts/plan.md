# Plan: the public board keeps a fault until it clears

From `intent/public-board-acked-alerts/intent.md`. No new spec: this amends R4 of
`intent/public-status-dashboard/spec.md`.

## Files that change
- `internal/api/public.go`: `buildPublicBoard` stops skipping alerts with `AckedAt` set.
- `internal/api/public_test.go`: the new test, and `TestPublicBoardNeverCarriesIdentity`, which asserted
  the old R4. It keeps checking that an acknowledged alert's text never appears.
- `intent/public-status-dashboard/spec.md` R4, `wiki/API-reference.md`, `wiki/Monitoring-and-alerts.md`
  and the Alerts hint in `web/src/pages/settings/PublicStatus.tsx`: say the same thing.

## Order of work
1. The tests, committed failing. 2. The change. 3. The spec, wiki and hint.

## Risks
An alert that never cleared would now stay on the board for good, because acknowledging no longer hides
it. Every source that reaches the board clears by itself: `offline`, `poll` and `supply` through
`reconcileAlerts` on each poll, and `trap` through `reconcileTrapAlerts` once `TrapAlertHold` has passed.
`security` and `consumable` alerts never reach the board.

A decommissioned printer is not polled, so its alerts do not clear by themselves. An acknowledged one
used to leave the board and now stays until an operator clears it, as an unacknowledged one already did.
Clear on the Alerts page remains the way to remove it.

Option not taken: a "being looked at" note beside the fault, which would tell the public about staff
activity.

## Proof
`TestPublicBoardKeepsAcknowledgedAlerts` fails first; `make test`; `cd web && npm run typecheck`;
`scripts/e2e.sh`; the board on a local instance with a simulated printer, seen before and after clearing.
