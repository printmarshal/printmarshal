[Home](Home.md) · [Configuration reference](Configuration-reference.md) · [API reference](API-reference.md)

# Firmware management (spec §8.6, roadmap phase 6)

Firmware is the highest-risk feature in PrintMarshal: a failed or interrupted
update can disable a device. This document describes what the feature does, the
safeguards built into it, what to do when an update goes wrong, and how a driver
or vendor plugin delivers firmware.

## What it does

- **Inventory** — `GET /api/firmware/inventory` groups every device by make,
  model and installed version (the version is already read by every poll through
  the `firmware_read` capability). Each cell carries the latest *registered*
  package version for that make/model and whether the cell is outdated.
  `GET /api/firmware/outdated` lists the individual devices that are older than a
  registered compatible package. Devices that report no version are shown as
  unknown, never as outdated.
- **Packages** — a firmware image uploaded through the API
  (`POST /api/firmware/packages`, multipart: `file` plus `vendor`, `models`,
  `version`, `minVersion`, `notes`). The file is stored under
  `<dataDir>/firmware/<id>/<filename>`, its SHA-256 is recorded, and the digest is
  **re-verified before every delivery**. Vendor and model matching is a
  case-insensitive substring match, using the comparator’s vendor matching rules, and
  `models=4200` matches `Acme LaserPro 4200`. A package that an unfinished
  campaign references cannot be deleted.
- **Campaigns** — a staged rollout of one package to a group (static or rule) or
  an explicit device list, with canary wave, concurrency limit, failure threshold,
  maintenance window, optional schedule and verification timeout. A campaign runs
  as a job (`firmware.campaign`) with per-device results; the detailed state lives
  in the campaign's device rows (`GET /api/firmware/campaigns/{id}/devices`).
- **History** — `GET /api/devices/{id}/firmware` lists every campaign a device
  took part in, with before/after versions and the outcome.

## Campaign lifecycle

```
draft ──start──▶ scheduled ──(time + window)──▶ running ──wave 1 done──▶ paused(canary)
                                                   ▲                          │
                                                   │        confirm           │
                                                   └──────────────────────────┘
running ──wave 2 done──▶ completed | failed
running ──threshold exceeded──▶ paused(threshold) ──resume──▶ running
running/scheduled ──pause──▶ paused(operator) ──resume──▶ running
any unfinished ──cancel──▶ cancelled
```

| Status | Meaning |
| --- | --- |
| `draft` | Defined, nothing planned or delivered. Editable (`PATCH`), deletable. |
| `scheduled` | Started, but waiting for `scheduledAt` or the maintenance window. The job re-checks every minute. |
| `running` | Devices of the current wave are being updated, `concurrency` at a time. |
| `paused` | No new devices are dispatched; devices already updating finish and are verified. `pauseReason` is `canary`, `threshold`, `operator` or `interrupted`. |
| `completed` | Every eligible device reached a terminal state and at least one succeeded (or none failed). |
| `failed` | Every attempted device failed. |
| `cancelled` | Stopped by an operator; pending devices are marked `skipped`. |

Per device the status goes `pending → updating → verifying → succeeded | failed`,
or `skipped` when the device is incompatible, already current, or the campaign
was cancelled before its turn.

**Waves.** At start the campaign is planned: targets are resolved, each device
gets a compatibility verdict, and the eligible devices are split into wave 1 (the
canary) and wave 2 (the rest). `canaryCount` defaults to 3 or 10 % of the
eligible devices, whichever is larger; `0` disables the canary pause and puts
every device in wave 1.

**Per device** the job runs, in order:

1. Compatibility: vendor, model, `minVersion`, and whether the device is already
   at the package version (skipped) or newer (skipped unless `allowDowngrade`).
2. Pre-flight: the device is re-polled and must be reachable and idle (not
   `printing`). Transient failures are retried three times.
3. Delivery: the best `firmware_update` driver for the device is called with the
   verified package and `WriteOptions{DryRun}`.
4. Audit: every real delivery attempt — accepted, rejected or errored — writes an
   audit entry (`firmware.update`, actor = the operator who started the campaign).
5. Verification: the device is polled until the firmware it reports equals the
   package version, up to `verifyTimeoutSecs` (default 15 minutes). The verdict
   uses the same words as configuration writes (audit A20) in the row message
   and the job result's `data.outcome`: `verified` (device reports the package
   version → `succeeded`), `diverged` (it still reports another version at the
   deadline → `failed`, with that version), `unverifiable` (it reported no
   version at all → `failed`).

A real delivery is handed only write-purpose credentials (audit A19,
"Credentials and purposes" in `docs/configuration.md`); a device with no write
credential bound is `failed` before the driver is called, with an audit entry
of result `refused`. Dry runs validate with read credentials.

Live updates are published as `firmware.campaign.updated` and
`firmware.device.updated` events on `/api/stream`.

## Safeguards

- **Two explicit gates.** Creating a campaign never starts it.
  `POST /api/firmware/campaigns/{id}/start` is the operator's confirmation for
  wave 1; after the canary wave the campaign pauses and
  `POST …/confirm` is a second, separate confirmation that releases wave 2.
  `resume` cannot bypass the canary gate.
- **Dry run.** `start` with `{"dryRun": true}` walks the whole campaign —
  planning, pre-flight, driver validation, the same waves and gates — but every
  driver is called with `DryRun: true` and must not send anything. Dry runs never
  write `firmware.update` audit entries and never change a device.
- **Failure threshold.** `failureThresholdPercent` pauses the campaign as soon as
  the failed share of attempted devices exceeds it; the operator reviews and
  resumes or cancels. The check runs after every batch.
- **Maintenance window.** `window` (`days`, `start`, `end`, `tz`) gates every
  batch; a window whose end is before its start spans midnight and belongs to
  the day it opens. Outside the window the campaign shows `scheduled` and polls
  every minute.
- **Concurrency.** At most `concurrency` devices (default 2) update at once, so
  a bad package cannot take the whole fleet down in one step.
- **Integrity.** The package digest is checked at start and again immediately
  before each delivery; the standard driver checks it a third time.
- **Never interrupt a write.** Pause and cancel stop dispatching new devices;
  a device that is updating or verifying always finishes.
- **Restart recovery.** If the server restarts while a campaign runs, the
  campaign is paused with reason `interrupted` and devices that were updating or
  verifying are marked `failed` ("verify the device manually"); the operator
  resumes or cancels. Nothing restarts on its own.
- **Audit.** Package registration/deletion, every campaign action (including
  refused ones) and every real delivery are in the audit log.
- **Roles.** Reads are open to viewers; packages are admin-only; campaigns and
  their gates are operator actions. Viewers never write.
- **Decommissioned devices** are never targeted.

## Rollback and recovery guidance

Each campaign carries `recoveryNotes` in the API. The generic guidance:

1. Do not power-cycle a device whose panel or status LED shows an update, flash
   or reboot in progress. Many devices take 10–30 minutes to come back.
2. Re-poll the device once it answers and compare the reported version with the
   package version. A device still on the old version usually rejected the image
   and is safe to retry.
3. Retry once with the same package, only when the device is idle and reachable.
   Repeated failures mean an incompatible package, a bad download or a
   device-side problem — stop and investigate.
4. To roll back, create a campaign with the previous package and
   `allowDowngrade: true`, if the vendor permits downgrades. Not every vendor does.
5. If the device never comes back or shows a recovery screen, use the vendor's
   documented recovery procedure (typically a USB or service-mode flash) or
   contact vendor support. Keep the device powered and on the network meanwhile.

PrintMarshal only claims vendor-specific knowledge where it has been exercised:
for the simulator the note says how to reset it. For any other vendor the note
states explicitly that there is no verified procedure and points at the vendor's
service mode and support — it never guesses.

## How firmware is delivered

There is no universal firmware transport across printer vendors, so delivery is
a capability (`firmware_update`) served by drivers, exactly like every other
capability (spec §6). The engine asks "which driver can deliver firmware to this
device?" and never "which vendor is it?".

```go
type FirmwareUpdater interface {
    UpdateFirmware(ctx, d *model.Device, creds model.Creds, pkg *model.FirmwarePackage,
        opts model.WriteOptions) (*model.FirmwareResult, error)
}
```

A driver that declares `firmware_update` must:

- check compatibility itself (`internal/firmware/fwversion.Check` is the shared
  helper) and report an incompatible package as `Accepted: false` with a reason;
- treat `opts.DryRun` as "validate and report only" — no device I/O that changes
  state; a read-only reachability probe is fine;
- verify the package digest before sending (`pkg.SHA256`, file at
  `pkg.StoragePath`);
- return `Accepted: true` only when the device acknowledged the image. The
  service verifies the installed version afterwards by polling; a driver may set
  `Verified`, but the poll is what decides.

**Standard driver: `internal/driver/fwsim`.** It serves `firmware_update` only for
devices that identified through the simulator (identity method `sim`) or whose
address is on an explicit allow-list, and uploads the image as a multipart POST
(field `firmware`, plus `version` and `sha256`) to `http://<addr>:<port>/firmware`,
expecting `{"accepted":true,"version":"…"}`. It is never offered to an arbitrary
printer. Register it from `cmd/printmarshal/main.go` when `PM_FIRMWARE_SIM_TARGETS`
(comma-separated `host` or `host:port`) is set:

```go
if v := os.Getenv("PM_FIRMWARE_SIM_TARGETS"); v != "" {
    reg.Register(fwsim.New(strings.Split(v, ","), fwsim.Options{}))
}
```

**Simulator.** `pm-sim` mounts `POST /firmware` and `GET /firmware/status` on its
HTTP port. The `-firmware-delay` flag delays the moment the new version takes
effect, so verification by polling can be exercised; the IPP
`printer-firmware-string-version` attribute reflects the new version once it is
applied.

**Vendor plugins** implement `engine.FirmwareUpdater` against the manufacturer's
own update interface and register through `internal/plugins`. Nothing in the
firmware service changes for a new vendor.

## API

| Method | Path | Role | Notes |
| --- | --- | --- | --- |
| GET | `/api/firmware/inventory` | viewer | `?make=&model=&group=&q=` |
| GET | `/api/firmware/outdated` | viewer | |
| GET / POST | `/api/firmware/packages` | viewer / admin | POST is multipart: `file`, `vendor`, `models`, `version`, `minVersion`, `notes` |
| GET / DELETE | `/api/firmware/packages/{id}` | viewer / admin | DELETE → 409 while an unfinished campaign references it |
| GET / POST | `/api/firmware/campaigns` | viewer / operator | `?status=&package=&active=true` |
| GET / PATCH / DELETE | `/api/firmware/campaigns/{id}` | viewer / operator | PATCH and DELETE on drafts (DELETE also on finished) |
| POST | `/api/firmware/campaigns/{id}/start` | operator | body `{"dryRun": bool}`; a missing body or field is a dry run, only `false` delivers; gate for wave 1 |
| POST | `/api/firmware/campaigns/{id}/confirm` | operator | gate for wave 2 |
| POST | `/api/firmware/campaigns/{id}/pause` · `/resume` · `/cancel` | operator | |
| GET | `/api/firmware/campaigns/{id}/devices` | viewer | per-device state with name/address/model |
| GET | `/api/devices/{id}/firmware` | viewer | update history |

Campaign body fields: `name`, `description`, `packageId`, `groupId` or
`deviceIds`, `canaryCount` (omit for the default, `0` for none), `concurrency`,
`failureThresholdPercent` (0 = never), `window` (`{days,start,end,tz}`, an empty
object clears it), `scheduledAt` / `clearScheduledAt`, `verifyTimeoutSecs`,
`allowDowngrade`. Errors: 400 for invalid input, 409 for an action the campaign's
state does not allow, 404 for unknown ids.


## Campaign definition reference

| JSON key | Default / behavior |
| --- | --- |
| `name` | Required, trimmed; `description` is optional. |
| `packageId` | Required existing package. |
| `groupId`, `deviceIds` | A target is required; group takes precedence if both are supplied. Decommissioned devices are excluded. |
| `canaryCount` | Omit for automatic max(3, ceil(eligible/10)); `0` puts every eligible device in wave 1. |
| `concurrency` | `2` when nonpositive/omitted. |
| `failureThresholdPercent` | `0` disables; otherwise 0–100. Pauses when failed percentage is strictly greater than this value, after a batch. |
| `window` | None; `{days,start,end,tz}`. `{}` clears it. Days accept weekday names/prefixes `mon`…`sun`; both times must be present together. |
| `scheduledAt` | Optional RFC 3339 timestamp; `clearScheduledAt:true` clears it. |
| `verifyTimeoutSecs` | `0` selects service default 900 seconds; negative rejected. |
| `allowDowngrade` | `false`. |

A window with `end <= start` crosses midnight; the weekday belongs to the opening day. The start minute is inclusive, the end minute exclusive. An absent timezone uses `time.Local`, so set `tz` explicitly for reproducible schedules. `scheduledAt` is normalized to UTC. Firmware verification interval is 20 seconds, transient preflight retries wait 2 seconds, and control/window checks wait up to one minute (`internal/firmware/service.go`).

```json
{"name":"Lab rollout","packageId":"PACKAGE_ID","deviceIds":["DEVICE_ID"],"canaryCount":1,"concurrency":2,"window":{"days":["sat"],"start":"22:00","end":"02:00","tz":"UTC"},"failureThresholdPercent":20,"verifyTimeoutSecs":900,"allowDowngrade":false}
```

The create request registers a draft only. Start a dry-run campaign using `POST /api/firmware/campaigns/{id}/start` with `{}` or `{"dryRun":true}` (an absent body is accepted too). Dry runs retain the selected mode for that campaign; a completed dry run cannot be started again as a real rollout. Create a separate draft for real delivery. Start it with explicit `{"dryRun":false}`, inspect canary results, then call `POST .../confirm` when it pauses for canary review.

## Job claim and restart handling

Lifecycle API methods serialize through the firmware service mutex. A job ID is persisted on the campaign before submission, and workers check that ID on entry and every control loop; superseded workers exit. Start refuses a non-draft or a campaign already associated with a job. This is a single-process claim, not a database lease for multiple active servers.

At startup, scheduled/running campaigns become paused with `pauseReason:"interrupted"`; updating/verifying rows become failed with a manual-verification message. Previously paused campaigns remain paused. Resume continues remaining pending rows; it does not automatically retry failed devices. Review those devices and create a new campaign if a retry is appropriate. Pause/cancel gates stop new dispatch within a running process, but cannot promise uninterrupted delivery through process termination or power loss.

Package paths are now stored relative to the firmware root and resolved at use. A legacy absolute path is used if it still exists; otherwise it is rebased using package ID and filename. Keep the actual image files as well as database metadata in backups.

## Known discrepancies

No shipped real-vendor plugin implements `firmware_update`; the implemented delivery transport is the explicitly enabled simulator driver. The existence of campaign APIs or vendor firmware documentation does not imply unattended delivery support for a physical model. Job claims are not multi-process leases. The generic promise “never interrupt a write” applies to pause/cancel dispatch behavior, not server shutdown. Package matching is implemented in `internal/firmware/fwversion/version.go`; do not infer compatibility from a filename alone.

Sources: `internal/api/firmware.go`, `internal/firmware/{campaign,run,service,packages,window}.go`, `internal/firmware/fwversion/version.go`, `internal/driver/fwsim/driver.go`, `internal/model/firmware.go`.
