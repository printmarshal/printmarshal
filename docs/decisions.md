# Build decisions (spec §16)

Recorded by the build session that implemented phases 1–2.

| Decision | Choice | Why |
| --- | --- | --- |
| Backend language | **Go** (1.27) | Spec's recommendation: concurrent device I/O, single static binary, mature SNMP (`gosnmp`) and HTTP/TLS libraries. |
| UI framework | **React + TypeScript (Vite)** | Live dashboard, tables and forms; the build output is embedded into the Go binary so there is one artefact to ship. |
| Datastore | **SQLite** (pure-Go `modernc.org/sqlite`) behind a `store.Store` interface | Single-container default. The interface is the seam for PostgreSQL (+ TimescaleDB for telemetry); `config.database` is reserved for it. Telemetry lives in its own narrow table and is pruned on a retention schedule. |
| Job execution | **In-process worker pool** with jobs persisted in the store | Jobs survive as records; on restart anything left running is marked failed ("interrupted by restart") so it can be re-run. Switch to a durable queue when multiple engine instances are needed. |
| Auth model | Local users with bcrypt passwords, cookie or bearer sessions, roles viewer/operator/admin | First admin is bootstrapped from `PM_ADMIN_PASSWORD` or a generated password printed once. SSO can be layered later at the `auth.Service` boundary. |
| First vendor plugins | **None for v1 — standards drivers only** | SNMP + IPP already deliver discovery, identity, full monitoring and firmware version on compliant devices. Plugins attach via `engine.Registry.Register` without touching the core. |
| SLP discovery | **Deferred** | mDNS/DNS-SD and WS-Discovery cover modern printers; SLP can be added as another `discovery` source. |
| eSCL driver | **Deferred to phase 3** | Not needed for monitoring; MFP detection currently uses DNS-SD scanner services and the IPP device-id command set. |

## Other notable choices

- **Capability resolution at identification.** `engine.Identify` runs every `Identifier`
  driver concurrently, merges the identities by driver priority, and records which
  driver serves each capability on the device (`capabilities` map). Polls dispatch per
  capability and fall back to the next driver on error; `servedBy` is recorded per job
  result for traceability.
- **No vendor knowledge in the core.** The SNMP driver stores the raw IANA enterprise
  number from `sysObjectID` and derives make/model from free text; it never maps an
  enterprise number to a vendor.
- **Alert noise rules.** Supply bits in `hrPrinterDetectedErrorState` defer to the real
  percentages; "no paper" is a fleet alert only when all trays of a media size are
  empty; the device's own `prtAlertTable` entries are mirrored with a stable key so they
  raise once and clear when the device clears them.
- **Secrets.** Credentials are AES-256-GCM encrypted under a key from `PM_SECRET_KEY`
  or `data/secret.key`; the `model.Credential` type has no secret fields, so no API
  response or audit entry can carry one by construction.

## Configuration management (docs/configuration.md)

| Decision | Choice | Why |
| --- | --- | --- |
| Config read merge | Every driver that declares `read_config` and identified the device is consulted, profile winner first; first value per key wins, a lower driver that can write the key makes it writable | IPP exposes most settings but cannot set `sysContact`/`sysName`; SNMP can. One merged document, one apply that falls through per key. |
| IPP write path | `Set-Printer-Attributes` (RFC 3380) only, gated on `operations-supported` and `printer-settable-attributes-supported`, one request per setting | The only standard IPP write; per-request results keep per-setting outcomes honest. The recorded HP M283fdw does not support it, so it gets `read_config` only. |
| SNMP write path | SET of the RFC 3418 system group and RFC 3805 `prtGeneralTable` read-write objects only | Everything else in the catalogue has no standard MIB object; vendor subtrees belong in plugins. |
| Apply as a job | `config.apply` job, per-device write record with rollback, audit entry per device, `config.applied` event | Same progress/partial-result model as polls and discovery; revert is just an apply of the stored rollback. |
| Secrets | Secret-typed catalogue keys are redacted in snapshots, write records, job params and audit; profiles refuse them | Spec §11: never persist or return a secret; the un-redacted change lives in memory only until the job runs. |

## Consumables add-on (docs/consumables-feature-spec.md)

| Decision | Choice | Why |
| --- | --- | --- |
| Stock placeholder | **Absorbed**: the phase-2 `stock` table/API/UI is migrated into `consumables` on first open (migration 2) and removed | One feature, not two overlapping ones. Legacy rows become type "other" items with the old match text kept in notes; legacy `stock` alerts are cleared and re-raised by the consumable rules. |
| Installed-supply link | A separate `supply_links` table keyed by (device, supply index), `source` auto or manual | Supplies are stored as JSON on the device and replaced on every poll; a separate table keeps the link stable, lets manual links be operator-owned (`ReplaceAutoLinks` never displaces them) and cascades on device or item deletion. |
| Matching rule | Part number found in the supply description wins; else type + colour + compatible model; an item without compatible models matches only when it is the sole candidate | Printer MIB / IPP expose no part number field, so description matching is the only standard signal; the ambiguity rule means fleets with several model-specific items never link silently to the wrong one. |
| Stock history | Every quantity change goes through `AdjustStock`, which writes the `consumable_stock_events` row atomically; `UpdateConsumable` cannot touch on-hand | History and audit stay complete by construction; the UI's inline edit is a batch of `set` events. |
| Forecast | Rate = observed level drop per day over the last 30 days of `supply.percent` telemetry, excluding rises ≥ the replacement jump; stock depletion is the OnHand-th simulated replacement across linked devices | Uses telemetry polls already record; degrades to "unknown" with a reason until history spans the configured minimum, instead of guessing. |
| Replacement detection | A level rise ≥ `replacementJumpPct` (default 30) between consecutive polls; setting off / prompt / automatic; always logged as a stock event | Spec §3. Prompt is the default so stock never changes without an operator seeing it. |
| Alerts | Source `consumable`, key `consumable:<id>`, no device; out = critical, ≤ critical threshold = critical, ≤ warning = warning | Reuses the existing alert model, stream and notifiers unchanged. |
| Settings | Stored in the `settings` table under `consumables`, admin-writable through the API | Operator defaults belong with the data, not in the config file. |

## Decisions taken during the phase 3–7 build (28 October 2026)

| Decision | Choice | Why |
| --- | --- | --- |
| eSCL driver | Built (`internal/driver/escl`), identify-only | Confirms MFPs and fills serial/UUID/admin URL from `ScannerCapabilities`; scanner status is read on demand. Validated against the HP M283fdw. |
| mDNS implementation | Own DNS-SD browser on `miekg/dns` (group listener with port reuse + QU-bit queries) | The third-party zeroconf library returned nothing on macOS; the direct implementation also records every TXT key for identity hints. |
| Vendor plugins | 13 plugins under `internal/plugins`, shared `vendorkit`, evidence matrix in `internal/plugins/README.md` | Only documented interfaces are implemented; everything else is marked unverified/unsupported rather than guessed. HP enrichment (PML + LEDM) validated live. |
| Model catalogue | Research JSON embedded in `internal/catalog`, per-device support status (documented / observed / unknown) | Keeps evidence separate from observation as the research insists; qualification overrides are operator-set and audited. |
| Runtime settings | Scheduler cadence, discovery targets, alert thresholds and retention editable through `PATCH /api/settings`, stored in the settings table, file/env remain defaults | The design's Settings page shows them editable; the scheduler re-reads them every tick so no restart is needed. |
| Reports | `internal/reports` aggregates over the telemetry/alert/audit tables; CSV export per report | Avoids a second analytics store; the data volume is bounded by telemetry retention. |
| Print provider (§8.7) | Separate optional module, off by default, CUPS adapter behind an interface with a dry backend | Keeps the core free of print serving; only touches the local CUPS, never a printer. |
| PostgreSQL | Implemented last, after every feature's store sub-interface has landed | A Postgres store must satisfy the whole `store.Store`; doing it earlier would have raced the feature agents. |
| Firmware delivery | Standard delivery only to the simulator / explicit allow-list (`PM_FIRMWARE_SIM_TARGETS`); real devices need a plugin | No vendor publishes a universal firmware transport; honesty over coverage. |

## Decisions from the application-integration audit (A19, A20)

| Decision | Choice | Why |
| --- | --- | --- |
| Credential purpose storage | A `purpose` column on `credentials` (`read` default), added by a guarded `ALTER TABLE … ADD COLUMN` in `SQLite.migrate` (migration 3), not a side table | The schema files only ever run `CREATE` statements, and the credentials table may not grow a column there; a side table would need a join on every resolution and could drift from its parent row. The `PRAGMA table_info` guard makes the ALTER idempotent for fresh and existing databases, and the column default makes every pre-existing credential read-only, which is the safe direction. |
| Write-credential resolution | `store.WriteCredentialStore` (`ResolveWriteCreds`, `SetCredentialPurpose`) in its own file, `store.CredsFor` / `fleet.CredsFor(purpose)` as the one entry point for every service, `store.ErrNoWriteCredential` fails the device | One place decides which credentials a write may use; a store that lacks the sub-interface fails closed. Dry runs resolve read credentials because they only read. |
| Rotated credentials | A rotated SNMP community is stored as `read`, a rotated admin password as `both` | The community *is* the polling secret; an admin password authenticates reads and writes on the device's web service. Neither silently becomes a write credential for SNMP SET. |
| Verified vs accepted writes | Per-setting `outcome` (`verified` / `diverged` + `readBack` / `unverifiable` / `written`) in the write record's result JSON, filled from a re-read after every real apply; counted on the record, the event, the job result and the audit entry | The device's "ok" is not evidence that the value is in effect (A20); keeping the verdict in the existing result JSON needs no schema change and reaches every consumer that already reads results. Security remediation derives the same outcome from its re-assessment; firmware uses the same vocabulary for its post-update poll. |
