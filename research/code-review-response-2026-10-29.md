# Response to Harry's code review — 2026-10-29

Review: `code-review-harry-2026-10-29.md` (20 findings: 1 critical, 8 high, 11 medium).
Each finding was re-read against the cited code before anything was changed.
Verdicts: 20 agreed (two of them partially), 0 rejected. Fixes landed on `main`
in the commits noted below, each with a regression test unless stated.

| # | Verdict | What was done | Where |
|---|---|---|---|
| H-001 | Agree, critical | Accounts bind by `(issuer, subject)` stored on the user row (SQLite migration 4, Postgres `ALTER … IF NOT EXISTS`, partial unique index). `ProvisionExternal` resolves by that pair only; a first sign-in whose username already belongs to a local or differently-bound account is refused with a clear message and `auth.failed`. `email_verified: false` fails the domain boundary (an absent claim is accepted, because Entra ID omits it). Test: an IdP user with `preferred_username=admin`, a renamed subject, and an unverified email are all refused; a fresh subject gets its own account. | `internal/auth/tokens.go`, `internal/api/oidc.go`, `internal/store/{sqlite,postgres}_misc.go`, `docs/auth.md` |
| H-002 | Agree, high | `authorize` now resolves `groupId`/`groupIds` and `target{deviceIds,groupId}` in bodies, and device-owned resources named in the path (`/api/alerts/{id}`, `/api/config/writes/{id}`, `/api/firmware/campaigns/{id}`, `/api/consumables/replacements/{id}`) to their devices before the scope check. Scoped principals get 403 on every mutating `/api/groups`, `/api/users`, `/api/tokens` request (the empty-rule widening you described is therefore impossible). A missing resource stays a 404 from the handler. | `internal/api/scope.go`, `TestScopeBoundaryClosed` |
| H-003 | Agree, high | API tokens are refused on mutating `/api/users` as well as `/api/auth/*` and `/api/tokens`. | `internal/api/scope.go` |
| H-004 | Agree, high | The job's per-device goroutine no longer dereferences a nil status (a reachable poll without a status read reports "reachable, status not read") and recovers from panics, reporting them as device errors. The engine still leaves `Status` nil for a partial poll on purpose; see H-012 for how that is now interpreted. | `internal/fleet/service.go` |
| H-005 | Agree, high | The write permit is released before the verification read (which takes its own), and the write record plus audit entry are persisted on a 30 s context detached from the device context. | `internal/configmgmt/service.go` |
| H-006 | Agree, high | Address-only reconciliation is refused when the address-matched record and the incoming identity carry different non-empty serial, UUID or MAC; the replacement becomes a new device and the old record keeps its history. Both stores. Not done: making UpsertDevice transactional across concurrent discoveries of the same new device (both backends would still need a unique constraint on serial to be airtight). | `internal/store/{sqlite,postgres}_devices.go` |
| H-007 | Agree, high | `dryRun` is decoded as `*bool`; nil means dry run on profile apply, device config, revert, campaign start and print-provider sync. Security remediation, enrolment and rotation already behaved this way. The UI always sends the flag explicitly. Not done: binding a real write to a prior dry-run result; the gate remains the explicit `false`. | `internal/api/{config,firmware,printprovider}.go`, docs |
| H-008 | Agree, high (partial) | `Start/Confirm/Pause/Resume/Cancel` are serialised by a service mutex; `Start` refuses a campaign that already has a job; the job id is chosen and written on the campaign before the job is queued, and a worker exits at start (and on every loop iteration) when it is not the campaign's recorded job. This closes the duplicate-worker and pause-then-resume cases within one process. Not done: a database-level lease for multi-process deployments, which the packaging does not support (one replica, `Recreate`). | `internal/firmware/{campaign,run,service}.go` |
| H-009 | Agree, high | The new secret is encrypted and stored (audited as `staged`) before the device is told. On a clean refusal/skip the staged credential is deleted; on an ambiguous failure (timeout, deadline, reset, EOF) it is kept and the message says so; on acceptance an SNMP community is verified by a read with the new credential and the message records the outcome. Bookkeeping runs on a detached context. | `internal/security/rotate.go` |
| H-010 | Agree, medium | `SetStockEventStatus(id, from, to)` is conditional (`WHERE status=from`), so a second confirmation loses; a failed stock update returns the event to pending so it can be retried. Both stores; store test added. Not done: one transaction across both tables (the conditional claim makes the race benign). | `internal/consumables/service.go`, stores |
| H-011 | Agree, medium | `jobs.Context` persists and publishes snapshots taken under its mutex; saves of one job are serialised by a second mutex; the throttle is separate. `job.finished` carries a snapshot too. | `internal/jobs/runner.go` |
| H-012 | Agree, medium | `alerts.Evaluate` returns the sources it observed (`offline` always; `poll` only when status was read; `supply` only when supplies were read) and reconciliation clears only those. Test covers unreachable, counters-only and status-only polls. | `internal/alerts/rules.go`, `internal/fleet/service.go` |
| H-013 | Agree, medium | Informational traps now reach the fleet; a link-up trap clears link-down at once; other trap alerts are cleared by the next successful status poll once they are older than `fleet.TrapAlertHold` (5 min), so a trap is never lost before a poll has had a chance to see the condition. | `internal/fleet/service.go`, `internal/trap/listener.go` |
| H-014 | Agree, medium | A `notification.dead` row that itself dies is logged but never republished as another `notification.dead`. | `internal/alerts/notify.go` |
| H-015 | Agree, medium | The key is generated only on `ErrNotExist`; any other read error stops startup; creation is `O_EXCL` with fsync and a concurrent creator's key is read back. | `internal/secrets/secrets.go` |
| H-016 | Agree, medium | Writes go through an `AuthTransport`: on 401 the configured admin credential answers a Basic or Digest (RFC 7616, MD5/SHA-256, `-sess`, `qop=auth`/`auth-int`) challenge exactly once, never to a different host; the admin user is the `requesting-user-name`; no credential gives a per-setting "requires authentication but no admin credential is bound" failure. httptest coverage for every scheme. | `internal/driver/ipp/{auth,driver,config}.go` |
| H-017 | Agree, medium (partial) | Writes try IPPS/HTTPS endpoints before plaintext, pin the device certificate's SHA-256 on first TLS contact and refuse a changed certificate or a downgrade to plaintext afterwards; `poll.ippWriteRequireTLS` (`PM_IPP_WRITE_REQUIRE_TLS`) refuses plaintext writes outright. Not done: persisting the pin (it lives for the process lifetime, documented) and CA validation (`ippInsecureTls` still defaults to true because printers ship self-signed certificates). | `internal/driver/ipp/`, `docs/security.md` |
| H-018 | Agree, medium | Package `StoragePath` is stored relative to the firmware root and resolved at use; legacy absolute paths that no longer exist are rebased by id and file name. Drivers receive the resolved path. Test covers both. | `internal/firmware/packages.go`, `run.go` |
| H-019 | Agree, medium | The detail page remounts per device id, ignores stale responses with a generation counter, clears errors on a successful load, and actions use the loaded record's id and are disabled until it matches the route. Typecheck only; not exercised in a browser yet. | `web/src/pages/DeviceDetail.tsx` |
| H-020 | Agree, medium | SQLite timestamps use a fixed-width layout (`2006-01-02T15:04:05.000000000Z07:00`); migration 5 rewrites every timestamp column once (older rows keep millisecond precision). Test: trimmed legacy values are migrated, a midnight range includes the first fractional second, new writes round-trip at nanosecond precision. | `internal/store/sqlite.go` |

## Things we did not change

- The `go test` failure you saw in `internal/security` (`operation not permitted` binding a loopback listener) is the sandbox you ran in, not the code; the suite passes here.
- Job lists and results remain fleet-wide for scoped principals, as `docs/auth.md` states.
- A14 (managed secret rotation) and A17 (vendor PKI delivery) from the earlier audit remain out of scope.

## Race detector

You listed the race detector as not run. `go test -race` over jobs, fleet,
firmware, alerts, configmgmt, consumables, security and api found one real
race besides H-011: `jobs.Runner.Submit` returned the same `*model.Job` the
worker was about to mark running, and HTTP handlers serialised it. Submit now
hands callers a copy taken before the job is queued. The packages are
race-clean after that.

## Verification

`go build ./... && go vet ./... && go test ./...` green on every package; `cd web && npm run typecheck` green. New or extended tests: `TestOIDCSignIn`, `TestScopeBoundaryClosed`, `TestPartialPollKeepsFaults`, `TestEvaluate`, `TestFixedWidthTimestampMigration`, `TestPackagesIntegrityAndDeleteRefusal`, the store stock-event test, and the IPP `auth_test.go` suite. The simulator end-to-end script is run before the final commit of the day.
