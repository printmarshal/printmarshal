# Print provider (optional add-on)

The print provider is the optional module described in the build spec §8.7: a
**CUPS / IPP Everywhere bridge** that lets the PrintMarshal host also act as a print
server by creating and serving local print queues for the printers it has
discovered.

It is **off by default**, lives entirely outside the core (`internal/printprovider`,
one API route file, one Settings section), and is a convenience for the "run it as a
print/web server too" case. It is not fleet management and it is not print
management: no accounting, no job routing, no pull printing (spec §2 non-goals).

**It never changes a printer.** Everything it does is `lpadmin` / `cupsenable` /
`cupsaccept` / `lpstat` against the local CUPS instance.

## Requirements

- A **Linux** host running CUPS with the command-line tools on `PATH`: `lpadmin`,
  `lpstat`, `cupsenable`, `cupsaccept`. The PrintMarshal process needs permission to
  administer CUPS (typically membership of the `lpadmin` group, or running as a user
  CUPS's `SystemGroup` allows).
- Printers that speak **IPP** — the device's capability profile must have at least
  one capability served by the `ipp` driver. Devices found via DNS-SD (`_ipp._tcp`
  / `_ipps._tcp`) are the usual IPP Everywhere candidates; devices that only answered
  SNMP are skipped and shown as such in the plan.

On any other host (macOS, Windows, CI, a Linux host without CUPS) the module runs the
**dry backend**: it records the commands it would run and never executes them. The
Settings section says which backend is active and why.

## Settings

Stored in the settings table under the key `printprovider` (not in the config file).

| Field | Default | Meaning |
|---|---|---|
| `enabled` | `false` | Nothing — not even `lpstat` — runs on the host until this is on. |
| `mode` | `cups` | `cups` creates and maintains queues on this host; `ipp-everywhere-list` only publishes the IPP Everywhere URIs in the plan for clients to add themselves and never changes the host. |
| `queuePrefix` | `pm-` | Prepended to every queue name and the only way the module recognises queues it owns. Required (1–32 chars of `A-Z a-z 0-9 _ -`). Only prefixed queues are ever removed. |
| `driverless` | `true` | `lpadmin -m everywhere` (IPP Everywhere). Off creates a raw queue (`-m raw`) and clients supply the driver. |
| `defaultOptions` | empty | `lpadmin -o key=value` pairs, space separated, applied to every queue. |
| `autoCreateForGroupId` | empty | Restrict queue creation to one group; empty means every IPP-capable device. |

Changing settings is admin-only and audited (`printprovider.settings.update`).

## How queues are named

`<queuePrefix>` + the device's name, sanitised:

1. The name is the operator's display name, else sysName, else hostname, else model,
   else `printer-<first 8 chars of the id>`.
2. Sanitised to lower-case letters, digits, `_` and `-`; any other run of characters
   becomes a single `-`; leading/trailing `-` removed; at most 63 characters.
3. Collisions get `-2`, `-3`, … — the device PrintMarshal has known longest keeps the
   base name.

Examples: "Front Desk HP" → `pm-front-desk-hp`; "Café / #3 (Lobby)" → `pm-caf-3-lobby`.

The queue's device URI is `ipp://<address>:631/ipp/print` (IPv6 addresses are
bracketed), its description is the device name plus model, and its location is the
device's operator location field.

## Plan and sync

- **Plan** (`GET /api/print-provider/plan`) lists every device in scope with the queue
  it maps to and an action: `create`, `update` (the queue exists but its URI,
  description or location differ), `unchanged`, `remove` (a prefixed queue with no
  matching device), `skip` (no IPP, decommissioned) or `list` (list mode). While the
  module is disabled the plan still derives names and URIs so you can see what
  enabling would do, but existing queues are **not** inspected (`inventoryRead: false`).
- **Preview (dry run)** runs the sync job without touching the host; each per-device
  result carries the exact commands.
- **Sync queues** (`POST /api/print-provider/sync {"dryRun": false}`) runs the job for
  real: one result per device and one audit entry per queue change
  (`printprovider.queue.create|update|remove`, actor = the user who started the job).
  Unmanaged queues (without the prefix) are never touched.
- **Remove** (`DELETE /api/print-provider/queues/{deviceId}`) deletes the queue serving
  one device.

Queue commands, exactly as the dry backend records them:

```
lpadmin -p <name> -E -v ipp://<addr>:631/ipp/print -m everywhere -D '<description>' -L '<location>' [-o k=v ...]
cupsenable <name>
cupsaccept <name>
lpadmin -x <name>          # remove
```

## API

| Method | Path | Role | Purpose |
|---|---|---|---|
| GET | `/api/print-provider` | viewer | settings, backend availability, queue inventory (empty while disabled) |
| PUT | `/api/print-provider` | admin | replace settings (validated, audited) |
| GET | `/api/print-provider/plan` | viewer | the plan |
| POST | `/api/print-provider/sync` | operator | body `{"dryRun": bool}` (omitted = dry run) → 202 `{job}` (`printprovider.sync`) |
| DELETE | `/api/print-provider/queues/{deviceId}` | operator | remove that device's queue |

Sync and remove answer 409 while the module is disabled or in list mode, and 503 when
the add-on is not wired into the binary.

## Limits

- Linux + CUPS only for real queues; everything else is a preview.
- Queues are created on the PrintMarshal host alone. Sharing them to clients
  (`cupsctl --share-printers`, Bonjour/DNS-SD browsing, Samba) is CUPS configuration
  outside this module.
- `ipps://` is not inferred; CUPS negotiates TLS with the printer itself when
  available.
- The planner compares URI, description and location. Options (`-o`) are re-applied
  on every create/update but a drift in options alone does not mark a queue as
  `update`.
- Queues PrintMarshal did not create (no prefix) are listed for information and never
  modified or removed.
- No printer is ever written to, and nothing here is print accounting, quotas or job
  routing.
