# Configuration management (spec §8.4)

PrintMarshal reads a device's settings into a **configuration document** keyed by
the vendor-neutral catalogue (`internal/model/settings_catalog.go`), keeps
**profiles** of desired settings, reports **drift**, and applies profiles or ad-hoc
changes as **jobs** with a dry run, per-setting results, an audit trail and a
rollback. Everything below is standards-only; vendor plugins add keys by
implementing `engine.ConfigReader` / `engine.ConfigWriter`.

## How a read works

`configmgmt.Service.ReadConfig` consults every driver that declares `read_config`
and identified the device, best first (the profile's winner from `engine.Resolve`,
then the others by priority). Documents are merged per key: the first driver to
report a key wins, each setting keeps its `source`, and a key that a lower-priority
driver reports as writable is marked writable even when the winner cannot set it.
Every full read is stored as a snapshot (`config_snapshots`, the newest 50 per
device) so `GET /api/devices/{id}/config` can answer without touching the device,
drift reports can use the last known state, and two devices can be diffed.

Secret-typed catalogue keys (`protocols.snmp.readCommunity`, `security.adminPassword`,
…) are never stored: their value is replaced by `<redacted>` in snapshots, write
records, job parameters and audit entries.

### IPP driver (`internal/driver/ipp/config.go`, priority 40)

Reads are `Get-Printer-Attributes` (RFC 8011 §4.2.5) with the attribute list in
`ipp.ConfigAttrs`. Writes are `Set-Printer-Attributes` (RFC 3380 §4.1, operation
`0x0013`), one request per setting so each outcome is independent.

| Key | Attribute | Writable |
| --- | --- | --- |
| `identity.hostname` | `printer-name` | only if listed in `printer-settable-attributes-supported` |
| `identity.location` | `printer-location` | yes |
| `identity.description` | `printer-info` | yes |
| `identity.organization` | `printer-organization` | yes |
| `identity.organizationalUnit` | `printer-organizational-unit` | yes |
| `print.defaultCopies` | `copies-default` | yes, validated against `copies-supported` |
| `print.defaultDuplex` | `sides-default` (`true` ↔ `two-sided-long-edge`) | yes, validated against `sides-supported` |
| `print.defaultColorMode` | `print-color-mode-default` (`auto` / `color` / `monochrome`) | yes, validated against `print-color-mode-supported` |
| `print.defaultResolution` | `printer-resolution-default` (`600x600dpi`) | yes, validated against `printer-resolution-supported` |
| `print.defaultDocumentFormat` | `document-format-default` | yes, validated against `document-format-supported` |
| `trays.defaultMediaSize` | `media-default` | yes, validated against `media-supported` |
| `trays.defaultTray` | `media-col-default` / `media-source` | no (collection member) |
| `trays.defaultMediaType` | `media-col-default` / `media-type` | no (collection member) |
| `print.jobTimeoutSeconds` | `multiple-operation-time-out` | no |
| `panel.language` | `natural-language-configured` | no |
| `protocols.ipp.enabled` | always `true` when IPP answered | no (observed) |
| `protocols.ipps.enabled` | `ipps:` in `printer-uri-supported` or `tls` in `uri-security-supported` | no (observed) |

"Writable" additionally requires `0x0013` in `operations-supported`; when the device
supports the operation but publishes no `printer-settable-attributes-supported`, the
RFC 3380 candidate list is assumed. A device that answers
`client-error-attributes-not-settable` (`0x0413`), any status ≥ `0x0400`, or echoes
the attribute in the unsupported-attributes group yields a **failed** setting and no
rollback entry. After a successful write the attributes are re-read so `after` is what
the device now holds. No standard IPP attribute carries TLS versions or sleep timers,
so `security.tls.*` and `energy.*` are not produced by this driver.

The recorded HP M283fdw (`testdata/fixtures/hp-m283fdw-full.ipp`) does **not**
support Set-Printer-Attributes; it is identified with `read_config` only.

### SNMP driver (`internal/driver/snmp/config.go`, priority 30)

Only the standard read-write objects are mapped; anything the MIBs cannot express is
simply absent from the document (not "unsupported").

| Key | Object | Writable |
| --- | --- | --- |
| `identity.hostname` | `sysName` (RFC 3418) | yes |
| `identity.location` | `sysLocation` (RFC 3418) | yes |
| `identity.contact` | `sysContact` (RFC 3418) | yes |
| `identity.printerName` | `prtGeneralPrinterName` (RFC 3805) | yes |
| `trays.defaultTray` | `prtInputDefaultIndex`, rendered as the tray's `prtInputName` | yes (by name or index) |
| `print.defaultMarkerIndex` | `prtMarkerDefaultIndex` | yes |
| `protocols.snmp.v1v2cEnabled` / `v3Enabled` | the version that answered | no (observed) |

Writes are one `SetRequest` per setting (`snmp.Session.Set`), read back afterwards.
SNMP SET needs write access: configure the write community (or an SNMPv3 user with
write access) as a credential with the **highest priority** for the device, because
the dialer uses the first credential that answers a read. A read-only community
fails the setting with `notWritable`; nothing else is attempted.

## Safety rules (spec §3 principle 4, §11)

- **Dry run first.** `dryRun: true` reads the device, reports `would_write` /
  `unchanged` / `skipped` / `failed` per setting and performs **no write I/O**
  (asserted by tests against a fake transport and a recording fixture).
- **Per-setting, per-device results.** Every apply is a `config.apply` job; each
  device gets a job result and a persisted write record (`config_writes`) with the
  outcome of every setting, its `before`/`after`, and the driver that handled it.
- **One device before a group.** The API accepts a single device or a group target;
  the UI's discipline is to apply to one device, check, then apply to the group.
- **Rollback.** A real write stores the previous values; `POST /api/config/writes/{id}/revert`
  re-applies them (dry-runnable). Dry runs and secret keys are not revertible.
- **Audit.** Every apply writes an `audit` entry: actor, action `config.apply`,
  target device, before/after per written key, result `ok` / `partial` / `failed` /
  `dry-run`. Profile CRUD, apply submission and reverts are audited by the API too.
- **No secrets at rest.** Secret-typed values are redacted everywhere they would be
  persisted; profiles refuse secret keys outright.
- **Rate limiting.** Device I/O for reads and writes goes through the engine's
  concurrency limiter; the apply job additionally bounds itself to `poll.concurrency`.
- **Roles.** Reads need `viewer`; profiles, applies and reverts need `operator`.
- **Events.** `job.updated` from the runner, plus `config.applied` per device
  (`deviceId`, `jobId`, `writeId`, counts).

## Driver fallback on write

The service hands all changes to the best `write_config` driver. Settings it reports
as `skipped` are offered to the next driver that identified the device, so a profile
can carry `identity.contact` (SNMP only) next to `print.defaultDuplex` (IPP only).
Results are merged per key; a key no driver serves is `skipped` with a reason.

## API

| Method | Path | Role | Body / query | Returns |
| --- | --- | --- | --- | --- |
| GET | `/api/config/catalog` | viewer | | `{areas, settings[]}` |
| GET | `/api/profiles` | viewer | | `{profiles, total}` |
| POST | `/api/profiles` | operator | `{name, description?, settings:[{key,value}]}` | 201 profile |
| GET | `/api/profiles/{id}` | viewer | | `{profile, drift[], writes[]}` |
| PATCH | `/api/profiles/{id}` | operator | `{name?, description?, settings?}` | profile |
| DELETE | `/api/profiles/{id}` | operator | | `{ok}` |
| POST | `/api/profiles/{id}/apply` | operator | `{target:{deviceIds?,groupId?}, dryRun}` | 202 `{job}` |
| POST | `/api/profiles/{id}/drift` | viewer | `{target, refresh?}` | `{reports[], devices, drifted}` |
| GET | `/api/devices/{id}/config` | viewer | `?refresh=true` reads the device, else latest snapshot | `{document, snapshotId, at, fresh}` |
| GET | `/api/devices/{id}/config/snapshots` | viewer | `?limit=` | `{snapshots, total}` |
| POST | `/api/devices/{id}/config` | operator | `{changes:[{key,value}], dryRun}` | 202 `{job}` |
| GET | `/api/config/diff` | viewer | `?a=<deviceId>&b=<deviceId>&refresh=` | `{a, b, items[], same}` |
| GET | `/api/config/writes` | viewer | `?device=&profile=&job=&limit=` | `{writes, total}` |
| GET | `/api/config/writes/{id}` | viewer | | `{write, reversible}` |
| POST | `/api/config/writes/{id}/revert` | operator | `{dryRun}` | 202 `{job}` |

Jobs are read through the existing `GET /api/jobs/{id}`; each per-device result
carries `data.writeId` and `data.results[]` (the per-setting outcomes).

Setting values are coerced to the catalogue type (`model.CoerceValue`): numbers may
arrive as JSON floats, booleans as `"true"`, lists as arrays or comma-separated
strings; enum values are validated against the catalogue options.

## Trying it against the simulator

```bash
./bin/pm-sim -fixture testdata/fixtures/generic-laser.snmp -snmp 127.0.0.1:1161 -ipp 127.0.0.1:6631
PM_SNMP_PORT=1161 PM_IPP_ENDPOINTS='http://%s:6631/ipp/print' PM_ADMIN_PASSWORD=change-me ./bin/printmarshal
```

`cmd/pm-sim/config_hooks.go` makes the simulator answer Set-Printer-Attributes for
`printer-location`, `printer-info`, `printer-organization(-unit)` and the `*-default`
attributes, and SNMP SET for `sysName`, `sysLocation`, `sysContact`,
`prtGeneralPrinterName`, `prtInputDefaultIndex` and `prtMarkerDefaultIndex`; later
reads over either protocol reflect the change. Real printers are read-only in
development: never point a write at hardware you have not been authorised to change.
