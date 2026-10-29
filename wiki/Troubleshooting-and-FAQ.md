# Troubleshooting and FAQ

[Home](Home.md) · [Deployment](Installation-and-deployment.md) · [API](API-reference.md)

Start with process logs, `/healthz`, `/readyz`, the device's last-seen/status/capability fields, and job results. A healthy process is not proof that the database, identity provider or printer is healthy. Use a permitted account and an authorized environment for any API or device checks; these procedures were not executed while writing this wiki.

## Startup, storage and upgrades

| Symptom | Likely cause and next step |
| --- | --- |
| Server cannot bind | `listen`/`PM_LISTEN` is already occupied, or a low port needs privileges. Inspect startup error and configured listeners. Traps/syslog have separate UDP listeners. |
| `/healthz` works, `/readyz` is 503 | Readiness cannot count users within its three-second DB budget. Check database connection, permissions and availability; do not diagnose this as a printer failure. |
| UI is a placeholder after a Go build | The existing `internal/web/dist` was embedded. Build the frontend before the backend; see [Development](Development.md). |
| Credentials cannot decrypt after restore | Wrong/missing `secret.key` or `PM_SECRET_KEY`. Restore the original encryption key with the database. Generating a replacement does not recover ciphertext. |
| Firmware file missing after moving data | Current paths are relative to firmware root; legacy absolute paths are used when present, otherwise rebased. Restore firmware files as well as DB/key and check configured `firmware.storageDir`. |
| Database opens with unexpected backend/path | `database` selects backend and `dsn` selects connection/path. A PostgreSQL selection without a DSN still receives the SQLite-style default path; set a real `PM_DSN`. |
| Unexpected setting survives a config edit | Runtime settings in the database overlay file defaults. Inspect `GET /api/settings`; configuration changes alone do not erase that overlay. |
| Jobs failed after restart | Startup marks queued/running generic jobs interrupted. Re-submit after reviewing results. Firmware has its own recovery to paused; it does not automatically resume delivery. |

Back up before upgrading, preserve keys and generated SAML material, and run one active server. See [migration and restore instructions](Installation-and-deployment.md). There is no automatic migration downgrade or SQLite-to-PostgreSQL data transfer command.

## Discovery and inventory

| Symptom | Cause / action |
| --- | --- |
| Zero-config discovery finds nothing | mDNS and WS-Discovery require multicast reachability and suitable network interfaces; container bridge/NAT or another subnet can prevent responses. Host networking is the deployment default for that reason. Use explicit authorized scan targets across routed networks. |
| A reachable IPP-only printer is absent from an active scan | Active target liveness uses SNMP; it does not fall back to IPP. Check read community/SNMPv3 and agent port; multicast discovery and identification are distinct paths. |
| CIDR was skipped | IPv4 expansion exceeded `maxAddresses` (default 4096). Split targets; `allowLarge:true` removes the protection and should be deliberate. Inspect job results for skipped/invalid targets. |
| Hyphenated hostname does not scan | Target parsing treats a hyphen as a range delimiter before hostname handling. Use its IP address or a supported target representation. |
| Same address produced a second device | A nonempty conflicting serial, UUID or MAC blocks address-based reconciliation. Inspect raw identity rather than merging distinct hardware by address. |
| Name/location changed unexpectedly | Reported identity fields and operator overrides are separate. Set display name/operator location rather than relying on a device's reported strings. |
| Decommissioned device missing from lists | Lists exclude decommissioned records by default; request `includeDecommissioned=true`. Reactivate using allowed lifecycle transitions, not an arbitrary string. |

See [Discovery and inventory](Discovery-and-inventory.md) for exact reconciliation and transition rules. No hardware was contacted to validate discovery while preparing these pages.

## Polling, alerts and supplies

| Symptom | Cause / action |
| --- | --- |
| Device is offline after restart | Restart alone does not establish reachability. Inspect persisted failure count/status, scheduled cadence, read credentials, driver profile and job errors; queue a permitted poll. A missing encryption key or changed port/endpoints can break reads. |
| Reachable device still shows an older fault | Any successful read capability can establish reachability; a failed/missing status read preserves the old status and does not reconcile status-source alerts. Check `errors`/`servedBy`, not only `reachable`. |
| Changing runtime poll concurrency has no effect | The startup config constructs workers/limiter. The persisted runtime field does not resize them; restart with `poll.concurrency` or `PM_POLL_CONCURRENCY`. |
| No traps | UDP destination defaults to 1162, not 162. Check trap enabled/listening state, firewall/container mapping, configured printer destination, source allow-list and rate limit. Source address must exactly match an inventoried device; unknown sources are ignored. |
| Trap alert stays after the condition cleared | Trap-source alerts have a five-minute minimum hold and clear on a later successful status poll. Link-up clears the corresponding link-down immediately. |
| Supply warning disagrees with a generic error bit | Percentage-based supply evidence takes priority to reduce false positives. Missing/unknown values are not zero; waste containers invert consumption interpretation. |
| One empty tray did not raise fleet no-paper | Noise rules consider alternative trays of the same media size. Review per-tray state and device alerts rather than assuming a single empty tray stops printing. |
| No forecast / report trend | Forecasts need enough telemetry and elapsed history; retained samples are pruned, not reconstructed. See forecast reasons, range/group filters and [Consumables](Consumables-and-stock.md). |
| Compliance report says unavailable | `/api/reports/compliance` is a placeholder. Use the security compliance summary and assessment history APIs; see [Security](Security.md). |
| Detected replacement did not reduce stock | Default tracking is `prompt`; confirm pending replacements. A supply also needs a link to a stock item. Check stock history before retrying confirmation. |

## Sign-in, sessions and scope

| Symptom | Cause / action |
| --- | --- |
| External sign-in refused with “already exists” | The username belongs to an account not bound to that issuer/subject. PrintMarshal refuses automatic linking to avoid account takeover. Choose a distinct mapped username or plan an administrator-controlled account/data migration; there is no account-linking endpoint. Do not delete a local administrator as a quick workaround. |
| LDAP/RADIUS credentials fail for an existing local username | Local accounts take precedence, even when their password fails; it does not fall through to another provider. Existing external accounts are authenticated only by their bound provider. |
| Role is higher than an external mapping suggests | `defaultRole` is a floor; mapping only raises it. Use `viewer` as the default for least privilege. Inspect actual provider group/attribute strings and case rules. |
| OIDC redirects fail or SAML assertion rejected | Check exact base URL/callback URLs, issuer/metadata, clock and claim/attribute configuration. SAML needs stable subject identity and tracked request state; unsolicited responses are disabled. |
| SAML works until restart / IdP certificate change | Keep the generated SP key/certificate and correct persistent data volume. IdP metadata is cached in-process; restart after verified metadata rollover. |
| Cookie login works over HTTP but fails after secure-cookie changes | `auth.cookieSecure:true` requires HTTPS in the browser. Terminate TLS at the proxy and configure external base URLs consistently. The server itself serves HTTP. |
| WebSocket closes after revocation | Streams recheck authentication every minute. Cross-origin streams also need explicit allowed origins. Query-string tokens are not accepted. |
| Login gets 429 / directory unavailable | Login is limited by direct peer address; proxy clients share that address. Directory outages can end in 503. The code does not trust forwarded headers to identify peers. |
| Token can read collections beyond its group | Scope is device-operation filtering, not tenancy. Reports, sync, metrics and other fleet collections have stated exceptions. Use [the exact scope list](Identity-and-access.md) before distributing a token. |
| API token cannot change its password or manage users/tokens | Protected auth routes and mutation controls explicitly refuse token principals, even with admin role. Use a human session. |

There is no user role-edit/admin password-reset endpoint; the supported password change is the authenticated user's own password. Provider keys, secrets, callback URLs and exact defaults are in [Configuration reference](Configuration-reference.md).

## Writes, security and firmware

| Symptom | Cause / action |
| --- | --- |
| Apply succeeded but printer unchanged | Omitted `dryRun` means preview. For real writes inspect per-setting outcome: verified, diverged, unverifiable or written. `accepted`/job success is not a universal read-back guarantee. |
| “No write credential” | Existing credentials migrated as read-only. Bind a credential with `purpose:write` or `both`; a read community does not authorize SET. Never change purpose merely to suppress an authentication error. |
| Profile cannot store a password | Secret settings are refused in profiles and redacted from snapshots/results. Submit secret changes through the supported transient apply flow; restart requires resubmission. |
| IPP write refused after certificate replacement | TLS first-contact pins are in memory per driver/device. Check that the certificate change is expected; process restart resets pins. `ippWriteRequireTLS:true` prevents plaintext fallback. |
| Rotation failure left a new credential | Rotation stages encrypted material before writing. Ambiguous errors keep it to avoid losing the possibly active secret. Clean refusal unstages it; inspect audit/results before removing any copy. |
| Security field remains unknown | Missing observations are unknown, not false/compliant. Scores exclude unknown checks. No real-vendor security writer is shipped. |
| Certificate enrollment worked but install unsupported | CA enrollment and printer installation are distinct. No shipped real-vendor `CertManager` exists. The API returns the certificate/CSR, not the generated private key. |
| ACME request asks for DNS action | Only DNS-01 is implemented, with manual challenge publication/wait. No DNS provider plugin or automatic renewal exists; IP-only SAN requests cannot use this path. |
| Campaign cannot deliver to a real printer | Only allow-listed simulator firmware transport is shipped. Package registration and research compatibility do not create a vendor update transport. |
| Canary campaign is paused | Confirm canary results before releasing later waves. Also inspect failure gate, maintenance window and interrupted-restart reason. Paused campaigns occupy a job worker while waiting. |
| Firmware restart shows failed devices | Interrupted updating/verifying rows are marked failed; pending rows remain. Resume does not retry those failed rows automatically. |
| Two replicas behave unpredictably | Lifecycle serialization/job claim is process-local, not a distributed lease. Supported packaging runs one replica. |

## Notifications and integrations

Inspect `/api/notifications`, outbox state and `/api/export/status` independently. Notifications use a durable outbox; SIEM uses a bounded in-memory queue. SMTP needs the correct STARTTLS/auth method; selecting port 465 does not enable implicit TLS. A removed notifier name makes pending rows dead. Requeue is an admin action and can produce duplicate external delivery after an ambiguous failure.

Printer syslog ingestion is off until `export.ingestListen` is set. It is UDP-only, accepts known device source addresses, rate limits and stores receipt time. It does not raise alerts from log text. A NAT/proxy source may not match the inventoried printer. Metrics outbox counts cap at 10,000 and metrics are authenticated; `/readyz` remains public.

## Known discrepancies

Older documentation describes broader firmware/certificate transports, resume guarantees, immutable sync semantics or configuration behavior than the current code supplies. Each feature page records the specific implementation limits. Troubleshooting should start from the current job/result record and source-backed API, not a roadmap claim.

Sources: `cmd/printmarshal/main.go`, `internal/api/`, `internal/auth/`, `internal/fleet/`, `internal/discovery/targets.go`, `internal/trap/listener.go`, `internal/alerts/`, `internal/configmgmt/`, `internal/security/`, `internal/firmware/`, and the linked feature documentation.
