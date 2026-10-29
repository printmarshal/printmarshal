# PrintMarshal code review — Harry — 2026-10-29

Verdict: do not enable production device writes or expose SSO to ordinary users until the high-severity findings are addressed.
The architecture has useful safety boundaries, but several fail in error paths or under concurrency; inventory identity and stock accounting also have data-integrity defects.
`GOTOOLCHAIN=local GOPROXY=off go build ./...` and `go vet ./...` passed; `cd web && npm run typecheck` passed.
`go test ./...` failed in `internal/security`: `TestTLSProbeAgainstHTTPTestServer` could not bind a loopback listener (`operation not permitted`). Many other package results were cached; this is not a clean, freshly executed suite.
Live-probe variables `PM_LIVE_HP`, `PM_PROBE_HOST`, and PostgreSQL test DSN were explicitly cleared. No printer was contacted.
The production SPA build was not run because its configured output replaces `internal/web/dist`, contrary to the report-only write restriction. PostgreSQL integration tests were not exercised.
Worst finding: OIDC can authenticate an unrelated identity as an existing local administrator (H-001).
Next: scoped operators can escape their device boundary and submit real writes (H-002); a partially successful poll can crash the entire server (H-004).
Findings below are based on source inspection, with concrete triggering states; exploit requests and device writes were not executed. Total: 20 findings (1 critical, 8 high, 11 medium).

## Findings

### H-001 — OIDC links accounts by mutable username and can take over local administrators
- **Severity:** critical
- **Area:** internal/auth, internal/api, internal/auth/oidc
- **File:** `internal/auth/tokens.go:139`, `internal/auth/oidc/oidc.go:156`, `internal/api/oidc.go:164`, `internal/api/oidc.go:172`
- **What:** `Claims.Username()` prefers `preferred_username`, then email, then subject. `ProvisionExternal` looks up that string in the ordinary local user table and returns any existing account. There is no stored issuer/subject binding, distinction between local and external accounts, or account-linking proof. Without role mapping, the existing role is retained. The domain check also accepts `claims.Email` without requiring `EmailVerified`.
- **Why it matters:** With OIDC configured without role mapping, a valid IdP user whose preferred username is `admin` signs in as the bootstrapped local admin without its password. A provider allowing users to choose or change that claim makes this directly exploitable. Username reassignment can similarly transfer an existing external user's account to a different subject. An allowed-domain filter is insufficient where the provider permits unverified email claims.
- **Fix:** Store external identities by the exact `(issuer, subject)` pair and bind them to a user ID. Never automatically link a local account by display name/email; require authenticated linking. Require verified email when using email domains as an access boundary.
- **Confidence:** high

### H-002 — Group scope is bypassed by nested targets, resource IDs, and editable rules
- **Severity:** high
- **Area:** internal/api/scope.go, configuration and firmware APIs
- **File:** `internal/api/scope.go:46`, `internal/api/scope.go:88`, `internal/api/config.go:205`, `internal/api/config.go:357`, `internal/api/firmware.go:359`, `internal/api/server.go:726`, `internal/store/sqlite_devices.go:567`
- **What:** Scope authorization checks `/api/devices/{id}` and only top-level JSON `deviceId`, `deviceIds`, or `devices`. Profile applies use nested `target.deviceIds`/`target.groupId`; revert and campaign start resolve devices through another resource ID without checking the resolved targets. Group PATCH is also allowed to scoped operators and can change a scoped group to a rule with an empty rule object, which `MatchRule` treats as matching every device. Read paths such as config diffs/write records similarly resolve device IDs outside the middleware's checks.
- **Why it matters:** An operator token limited to group A can POST a profile apply with `{"target":{"deviceIds":["device-in-B"]},"dryRun":false}`. It can also start an existing campaign outside its groups. Alternatively, PATCH its allowed group with `{"kind":"rule","rule":{}}` expands its effective scope to the fleet, after which even direct device routes pass. These paths violate the device boundary promised in `docs/auth.md:71` and `docs/auth.md:85`.
- **Fix:** Authorize the complete resolved device set for every operation, including persisted resources, nested/group targets, reads and queued execution. Prevent scoped principals from broadening the groups that define their authorization. Treat mutable authorization groups separately from operator-managed fleet groups.
- **Confidence:** high

### H-003 — Admin API tokens can mint unrestricted human accounts and evade revocation
- **Severity:** high
- **Area:** internal/api authentication authorization
- **File:** `internal/api/scope.go:36`, `internal/api/server.go:131`, `internal/api/server.go:1019`, `internal/auth/auth.go:124`
- **What:** The token-specific authentication-management prohibition matches `/api/auth/*` and mutating `/api/tokens*` requests, but user creation/deletion live under `/api/users`. An admin-role API token can therefore create an ordinary password-bearing admin account. `CreateUser` supplies no scope or token provenance to that account.
- **Why it matters:** A scoped or expiring admin token can create an unscoped administrator, log in with its chosen password, and retain fleet access after token expiry/revocation. This defeats the stated rule that tokens cannot manage authentication (`docs/auth.md:55`) and the isolation intended by using service-account tokens.
- **Fix:** Require a human-session principal for all user and authentication administration, including `/api/users`. Enforce this as a capability on the handler, rather than a partial URL-prefix list.
- **Confidence:** high

### H-004 — A poll with successful telemetry but no status can terminate the server
- **Severity:** high
- **Area:** internal/engine, internal/fleet
- **File:** `internal/engine/engine.go:351`, `internal/engine/engine.go:420`, `internal/store/sqlite_devices.go:318`, `internal/store/postgres_devices.go:281`, `internal/fleet/service.go:422`, `internal/fleet/service.go:440`, `internal/jobs/runner.go:149`
- **What:** Any successful capability makes a poll reachable, but a failed status read leaves `res.Status` nil; the engine only manufactures a status when nothing is reachable. Stores preserve the previous status, which can also be nil for a new device. `runPoll` unconditionally dereferences `upd.Status.State` in a spawned goroutine. The job runner's recovery is in the parent goroutine and cannot catch this panic.
- **Why it matters:** Discover a device whose counters/supplies work while status fails, leaving its initial stored status nil. A subsequent scheduled or requested poll with the same partial success reaches the dereference and kills the process, interrupting every other job. Flaky protocol endpoints make this a realistic device-triggered availability failure.
- **Fix:** Explicitly represent partial/unknown status and guard the result formatting. Recover and report failures at each worker-goroutine boundary so one device cannot terminate the service.
- **Confidence:** high

### H-005 — Configuration writes reacquire their own limiter and can lose verification and audit records
- **Severity:** high
- **Area:** internal/configmgmt
- **File:** `internal/configmgmt/service.go:73`, `internal/configmgmt/service.go:168`, `internal/configmgmt/service.go:534`, `internal/configmgmt/service.go:588`, `internal/configmgmt/service.go:649`, `internal/configmgmt/service.go:662`
- **What:** `applyDevice` acquires `Engine.Limiter` and defers release until its return. After writing, it calls `ReadConfig`, whose `readDevice` tries to acquire another slot on the same limiter. The write record and audit happen only after this nested acquisition.
- **Why it matters:** With concurrency 1, every actual changed-setting write blocks on its own held slot until the two-minute device context expires. At higher concurrency, a full batch can occupy every slot and have the same failure. Settings have already changed, but verification times out; `SaveWriteRecord` then receives the canceled context and can fail before rollback data and audit are saved.
- **Fix:** Release the write's permit before the verification read, or provide an internal read path that runs under an already-held permit. Persist an intent/attempt record before I/O and finalize it using a bounded context that survives device-operation cancellation.
- **Confidence:** high

### H-006 — Address fallback merges replacement printers into the old device and credential scope
- **Severity:** high
- **Area:** internal/store inventory identity, both backends
- **File:** `internal/store/sqlite_devices.go:82`, `internal/store/sqlite_devices.go:133`, `internal/store/postgres_devices.go:77`, `internal/store/postgres_devices.go:127`
- **What:** Identity lookup tries serial/UUID/MAC, then always falls back to address, even when both the existing and incoming records have conflicting nonempty stable identities. The subsequent update overwrites serial, UUID and MAC on the existing device ID and preserves its operator fields, group membership and associated history.
- **Why it matters:** Printer A with serial A is retired or moves, and printer B with serial B takes its IP. Rediscovery finds no serial-B match, finds A by address, and turns A's record into B. Historical telemetry is mixed and B inherits A's groups and device-scoped credentials/write authorization. This can send privileged operations intended for A to B.
- **Fix:** Refuse address-only reconciliation when known stable identifiers conflict. Create a new device or flag an explicit identity conflict; preserve the old record/history. Make identity creation/reconciliation transactional to prevent duplicate records from concurrent discoveries.
- **Confidence:** high

### H-007 — Omitting dryRun performs real configuration and firmware writes
- **Severity:** high
- **Area:** internal/api/config.go, internal/api/firmware.go
- **File:** `internal/api/config.go:200`, `internal/api/config.go:275`, `internal/api/config.go:357`, `internal/api/firmware.go:359`, `README.md:19`, `docs/printmarshal-build-spec.md:315`
- **What:** Profile apply, ad-hoc configuration apply, revert, and firmware start decode `DryRun` into a zero-valued `bool` and forward it unchanged. Firmware start and revert even allow an absent body. Consequently omitted `dryRun` means false. There is also no backend proof that a prior dry run or single-device rehearsal succeeded.
- **Why it matters:** `POST /api/firmware/campaigns/{id}/start` with no body begins real delivery where a writer and write credentials exist. A client submitting only profile targets or setting changes similarly writes immediately. This conflicts with README's dry-run-by-default claim and allows an integration mistake to skip the intended rehearsal.
- **Fix:** Decode optional booleans as pointers and default to dry run. Require explicit real-write confirmation; if the product promises dry-run-first gating, bind that confirmation to the reviewed target set, settings/package, and dry-run result.
- **Confidence:** high

### H-008 — Firmware campaigns lack an atomic start/claim and can deliver the same update concurrently
- **Severity:** high
- **Area:** internal/firmware
- **File:** `internal/firmware/campaign.go:286`, `internal/firmware/campaign.go:314`, `internal/firmware/campaign.go:332`, `internal/firmware/campaign.go:353`, `internal/firmware/run.go:25`, `internal/firmware/run.go:114`, `internal/firmware/service.go:184`
- **What:** `Start` reads a draft, plans/deletes/recreates its rows, changes status and queues a job in separate operations. No lock or conditional database transition claims the campaign. `setActive` simply sets a boolean after execution starts; it does not reject another worker. `ensureJob` also considers a queued campaign inactive. Campaign state updates use separate read and unconditional write operations.
- **Why it matters:** Two overlapping start requests can both observe a draft and enqueue workers. Both can read the same pending rows and deliver firmware to the same device. Pause/resume before the first queued job starts can also enqueue another job. Concurrent state writes can overwrite an operator's pause/cancel. Duplicate firmware delivery is unsafe and breaks the campaign control gates.
- **Fix:** Atomically transition draft to scheduled and reserve a unique active job/lease in a transaction. Claim device rows conditionally before dispatch. Use versioned/conditional status updates for pause/cancel and reject duplicate workers. Keep the design correct across processes, not only within a mutex.
- **Confidence:** high

### H-009 — Credential rotation changes the device before durably saving its only new secret
- **Severity:** high
- **Area:** internal/security/rotate.go
- **File:** `internal/security/rotate.go:85`, `internal/security/rotate.go:114`, `internal/security/rotate.go:133`, `internal/security/rotate.go:149`
- **What:** A fresh random secret is held in memory, applied to the device, and only then encrypted and persisted with `CreateCredential`. A failure after acceptance returns a manual-recovery message, while deliberately omitting the secret from output. There is no durable pending rotation record or recovery mechanism. Rotation also treats driver acceptance as rotated without authenticating with the new secret.
- **Why it matters:** A restart, context timeout, full disk, or database failure between device acceptance and credential insertion permanently loses the generated admin password/read community. The old credential may no longer work and the new one is neither stored nor recoverable. An accepting-but-ignoring device can also leave the highest-priority stored password wrong.
- **Fix:** Encrypt and durably stage the generated secret before changing the device. Track pending/applied/verified rotation state and recover interrupted operations using both old and staged credentials. Verify access with the new secret before promoting it.
- **Confidence:** high

### H-010 — Replacement confirmation can decrement stock twice or lose the decrement
- **Severity:** medium
- **Area:** internal/consumables, stock persistence
- **File:** `internal/consumables/service.go:284`, `internal/store/postgres_consumables.go:271`, `internal/store/sqlite_consumables.go:310`, `internal/store/sqlite_consumables.go:189`
- **What:** Confirmation reads a pending event, unconditionally updates its status, and separately calls `AdjustStock`. These actions do not share a transaction or idempotency key. PostgreSQL's status update has no `status='pending'` condition; serializing stock arithmetic does not serialize the confirmation decision.
- **Why it matters:** Two overlapping confirmations can both read pending, both set confirmed, and both subtract one unit for a single replacement. Conversely, if stock adjustment fails after the status update, the event remains confirmed and a retry is refused, so no unit is removed. Both stores are affected by the service-level split.
- **Fix:** Add a transactional confirmation operation that conditionally claims the pending event and updates stock/history once. Link the applied event uniquely to its originating replacement and roll back the status change if adjustment fails.
- **Confidence:** high

### H-011 — Concurrent job results race with persistence and event snapshots
- **Severity:** medium
- **Area:** internal/jobs
- **File:** `internal/jobs/runner.go:37`, `internal/jobs/runner.go:47`, `internal/jobs/runner.go:187`, `internal/jobs/runner.go:199`, `internal/jobs/runner.go:208`, `internal/store/postgres_misc.go:109`
- **What:** `Context.Result` protects counter increments with `c.mu`, releases it, then passes the shared `Job` pointer to persistence. `persistNow`, SQL argument construction and `emit` read/copy its fields without `c.mu`. The runner's separate mutex only throttles save timestamps; it does not protect the job data or serialize completion of saves.
- **Why it matters:** Multi-device jobs call `Result` concurrently. One goroutine changes Done/Failed while another reads them for SQL or an event, constituting a Go data race and potentially emitting inconsistent progress. Saves that finish out of order can temporarily regress persisted progress. The shallow `*j` copy is not a synchronized snapshot.
- **Fix:** Take an immutable job snapshot while holding the job mutex and persist/publish that snapshot. Serialize saves or enforce monotonic versions so older snapshots cannot replace newer ones.
- **Confidence:** high

### H-012 — Failed or partial polls clear existing fault alerts as though they were resolved
- **Severity:** medium
- **Area:** internal/alerts, internal/fleet
- **File:** `internal/alerts/rules.go:28`, `internal/alerts/rules.go:34`, `internal/alerts/rules.go:72`, `internal/fleet/service.go:173`, `internal/fleet/service.go:235`
- **What:** An unreachable poll returns only a possible offline alert; a partial poll omits alerts for missing capability results. Reconciliation nevertheless clears every existing alert in all `offline`, `poll`, and `supply` sources that is absent from that result.
- **Why it matters:** A printer with a paper jam and empty toner stops answering, or only its supply endpoint times out. The system emits cleared notifications for unresolved faults. Before the offline threshold is reached it may show no active alert at all. Repeated protocol failures cause false clear/raise cycles and notification noise.
- **Fix:** Carry per-capability observation validity into reconciliation. Preserve alerts for sources that were not successfully observed, and clear a fault only after a successful observation establishes its resolution.
- **Confidence:** high

### H-013 — Trap alerts never reconcile away after the device recovers
- **Severity:** medium
- **Area:** internal/trap, internal/fleet
- **File:** `internal/fleet/service.go:541`, `internal/fleet/service.go:545`, `internal/alerts/rules.go:28`, `internal/trap/listener.go:210`
- **What:** Traps raise alerts with source `trap` and enqueue a poll allegedly to reconcile them. Poll reconciliation only owns `offline`, `poll`, and `supply`, so it never clears `trap` alerts. Informational recovery traps such as link-up are logged and discarded before reaching fleet handling.
- **Why it matters:** A link-down, cold-start or transient printer trap remains active indefinitely after healthy polls and even a link-up trap. Operators must clear it manually, undermining asynchronous alert reconciliation and keeping fleet health misleadingly degraded.
- **Fix:** Define trap lifecycles and reconcile them against successful polls or matching recovery events. Keep historical traps as events; only maintain active alerts for conditions that have a resolution model.
- **Confidence:** high

### H-014 — Dead-notification routing can create an endless feedback loop
- **Severity:** medium
- **Area:** internal/alerts durable outbox
- **File:** `internal/events/events.go:76`, `internal/alerts/notify.go:82`, `internal/alerts/notify.go:113`, `internal/alerts/notify.go:215`
- **What:** `notification.dead` is itself notifiable and is included in wildcard subscriptions. The handler prevents routing a failure back only to the immediately failing notifier. A dead notification about another dead notification is still published with a fresh event ID and routed normally.
- **Why it matters:** Configure two unreachable destinations A and B with `events:["*"]`. Exhausting A's delivery produces a failure for B; exhausting that produces a failure for A, indefinitely. Three or more failing destinations can amplify the traffic. Outbox rows, delivery logs and retry work keep growing after the original event stops.
- **Fix:** Never recursively notify on a delivery failure whose original event is already `notification.dead`, or retain a bounded causal chain and suppress repeated destinations. Expose terminal delivery failures through an independent health/status channel.
- **Confidence:** high

### H-015 — Key loading can overwrite an existing encryption key after a read error
- **Severity:** medium
- **Area:** internal/secrets
- **File:** `internal/secrets/secrets.go:47`, `internal/secrets/secrets.go:54`, `internal/secrets/secrets.go:61`
- **What:** Every `os.ReadFile` error falls through to generating and writing a new key, not just file-not-found. Creation uses `os.WriteFile`, which truncates an existing file and does not reserve creation exclusively.
- **Why it matters:** A writeable but unreadable existing key, or a transient read failure followed by a successful write, replaces the sole key for all stored device credentials and certificate private keys. Concurrent initial starts can similarly generate different keys and overwrite the same file. Existing ciphertext then becomes unreadable, or records become encrypted under inconsistent keys.
- **Fix:** Generate only on `os.IsNotExist`; fail startup for other read errors. Create with exclusive creation and restrictive permissions, handle a concurrent creator by reloading, and durably write/sync the key before accepting encrypted writes.
- **Confidence:** high

### H-016 — IPP writes ignore configured authentication credentials
- **Severity:** medium
- **Area:** internal/driver/ipp
- **File:** `internal/driver/ipp/config.go:394`, `internal/driver/ipp/config.go:448`, `internal/driver/ipp/config.go:479`, `internal/driver/ipp/driver.go:54`
- **What:** `WriteConfig` accepts `model.Creds` but never uses it. `setAttribute` hardcodes requesting-user-name to `printmarshal` and passes only URL/body to the transport. The HTTP transport adds no authentication header and has no challenge-response path.
- **Why it matters:** An IPP printer requiring administrative HTTP authentication rejects every write even when correct purpose-bound admin credentials are configured. Reads may succeed anonymously, so the printer advertises writable settings and the dry run succeeds before the real apply fails. On unauthenticated devices the configured credential acts only as an application-side permission marker.
- **Fix:** Carry the selected credentials into an authentication-capable transport, implement the supported device authentication schemes, and report an explicit unsupported-auth result where necessary. Dry-run validation should distinguish advertised write capability from authenticated write readiness.
- **Confidence:** high

### H-017 — The IPP write path inherits insecure read transport defaults
- **Severity:** medium
- **Area:** internal/driver/ipp transport safety
- **File:** `internal/driver/ipp/driver.go:40`, `internal/driver/ipp/driver.go:43`, `internal/driver/ipp/driver.go:78`, `internal/driver/ipp/config.go:395`, `internal/driver/ipp/config.go:448`, `internal/config/config.go:213`
- **What:** Production defaults disable TLS certificate verification and prefer plain HTTP endpoints. Configuration writes discover an endpoint using that same transport and send `Set-Printer-Attributes` to it. The transport comment still justifies insecure TLS as “reads only,” although it now performs writes. There is no separate verified/pinned write transport policy.
- **Why it matters:** A network attacker can impersonate an IPPS printer or interfere with an HTTP session, advertise write support, receive desired settings, and forge successful read-back results. Enabling writes does not establish that PrintMarshal is talking to the intended device. Even turning off insecure TLS does not require HTTPS because endpoint selection still starts with HTTP.
- **Fix:** Require an explicitly trusted HTTPS/IPPS channel for writes, with CA validation or a device-bound certificate pin. Separate exploratory read behavior from write transport policy and make plaintext writes an explicit per-device opt-in if supported.
- **Confidence:** high

### H-018 — Restoring into another data directory leaves firmware packages pointing at the old files
- **Severity:** medium
- **Area:** firmware package persistence, backup/restore
- **File:** `internal/firmware/packages.go:60`, `internal/firmware/packages.go:64`, `internal/firmware/packages.go:133`, `internal/store/sqlite_firmware.go:39`, `deploy/scripts/restore.sh:110`, `deploy/scripts/restore.sh:117`
- **What:** Firmware metadata persists `StoragePath` including the original data directory. Restore copies the database unchanged and places firmware files under the new `DATA_DIR`; it does not rebase those paths. Verification and delivery use the stored path directly.
- **Why it matters:** Back up a bare-host installation in `/var/lib/printmarshal`, then restore into a container at `/data`. Packages remain listed, but campaign start fails because it opens `/var/lib/printmarshal/firmware/...`. Relative data directories also depend on the process working directory. A stale old directory can make the restored instance consult the wrong copy.
- **Fix:** Persist a relative package identifier/path and resolve it against the current configured firmware root. Migrate existing rows and validate package containment/integrity after restore. Document and verify cross-directory restore behavior.
- **Confidence:** high

### H-019 — Device navigation can display one printer while actions target another
- **Severity:** medium
- **Area:** web/src/pages/DeviceDetail.tsx
- **File:** `web/src/pages/DeviceDetail.tsx:23`, `web/src/pages/DeviceDetail.tsx:37`, `web/src/pages/DeviceDetail.tsx:47`, `web/src/pages/DeviceDetail.tsx:53`, `web/src/pages/DeviceDetail.tsx:61`, `web/src/App.tsx:51`
- **What:** The detail component keeps `device` across route-parameter changes, starts unguarded asynchronous loads, and neither clears old data nor cancels/ignores old responses. Header and identity render from `device`, whereas polling and lifecycle changes use the current route `id`. The error state is also never cleared on a subsequent successful load.
- **Why it matters:** Navigate from A to B using the global device search while B is slow. The page can still show A's name/address while “Poll now” or “Change state” targets B. If A's older request resolves last, that mismatch persists. A transient error can leave the page permanently displaying its error banner even after later loads succeed.
- **Fix:** Reset all device-specific state when `id` changes and render a loading state until data for that ID arrives. Abort old requests or use a request-generation check; make action targets derive from the verified displayed record. Clear errors on successful reload.
- **Confidence:** high

### H-020 — SQLite text timestamps do not sort chronologically at fractional-second boundaries
- **Severity:** medium
- **Area:** internal/store SQLite time comparisons
- **File:** `internal/store/sqlite.go:166`, `internal/store/sqlite.go:168`, `internal/store/sqlite_reports.go:29`, `internal/store/sqlite_misc.go:256`, `internal/store/sqlite_outbox.go:37`
- **What:** SQLite timestamps use `time.RFC3339Nano`, which omits zero/trailing fractional digits, but range predicates and ordering compare those strings lexically. The UTC suffix `Z` sorts after `.`, so timestamps with equal whole seconds but different fractional encodings are ordered incorrectly.
- **Why it matters:** `2026-10-29T00:00:00.1Z` is later than `2026-10-29T00:00:00Z`, but compares smaller as text. A report beginning at midnight excludes samples in its first fractional second; an exclusive midnight end can include samples just after the boundary. Outbox due checks and latest/earliest selection have the same boundary defect. PostgreSQL's native timestamps avoid this particular issue.
- **Fix:** Use integer epoch timestamps or a fixed-width UTC representation with a fixed number of fractional digits. Migrate existing SQLite values before relying on lexical comparisons.
- **Confidence:** high

## Things that looked good

Most routes consistently use the shared role middleware. Session and API-token secrets are randomly generated and stored as hashes; cookies are HttpOnly and SameSite, and WebSockets recheck authentication. Credentials/private keys use authenticated encryption, and the write-purpose split generally fails closed when no write credential exists. Firmware images are hashed and rechecked before delivery. Driver capability/evidence declarations are unusually candid about unsupported vendor writes. SQL filters generally use bound parameters and allowlisted sort choices. Deployment defaults use a non-root user and Kubernetes disables service-account token mounting.

## Not reviewed

- No real printer traffic, simulator process, exploit request, migration exercise, backup/restore execution, or deployment was performed. The identified failure scenarios are source-derived, not live reproductions.
- PostgreSQL runtime/conformance behavior: no test DSN was supplied. Both store implementations were inspected in relevant paths, but not every query or historical schema upgrade was audited.
- Production frontend build and browser/accessibility interaction testing. The configured Vite build writes into the repository and was intentionally skipped. Frontend source/state handling received a narrower review than backend safety.
- Exhaustive OIDC/JWKS, ACME/SCEP/EST cryptographic interoperability, every vendor OID/XML parser, CUPS behavior, and SIEM transport edge cases.
- Sustained-load, goroutine/resource profiling, race-detector execution, power-loss durability tests, and full dependency vulnerability analysis. Passing cached tests do not validate these properties.
- All original specification acceptance criteria end to end. The review checked the central safety/authorization claims and selected operational behavior; broad feature-completeness claims remain unverified.
