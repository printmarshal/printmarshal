# PrintMarshal (PM) — build specification

> Instructions for the session that will build PrintMarshal from scratch.
> Read this whole document before writing code. Build in the phase order in
> section 15 — each phase is shippable on its own, and later phases carry more
> risk than earlier ones.

---

## 1. What PrintMarshal is

PrintMarshal is a self-hosted, web-based platform for managing a fleet of network
printers across their whole lifecycle:

**discover → inventory → monitor → configure → secure → update firmware → retire.**

It is **vendor-agnostic**: it manages printers from any manufacturer through open
network standards, and isolates anything manufacturer-specific behind optional
plugins. It manages **devices, not print jobs** — it is not a print-accounting,
quota, or pull-printing product, and it depends on no vendor's own management
software.

The target operator is an IT or print-services team that runs tens to thousands of
printers across one or more sites and needs a single console to see them, keep them
configured to a standard, keep them secure, and keep their firmware current.

## 2. Goals and non-goals

**In scope**

- Discovery and inventory of network printers (and multifunction devices).
- Monitoring: online/offline status, supply/consumable levels, paper-input status,
  page counters, and device errors; alerting on them.
- Configuration: read a device's settings, define reusable configuration profiles,
  and push settings to one device or a group.
- Security: assess a fleet against a security policy, report compliance, and
  remediate; manage device certificates and credentials.
- Firmware: inventory firmware versions and run controlled update campaigns.
- Lifecycle: onboard, group, track, and decommission devices.
- A web UI; self-hosted; container-first; scalable to a Linux host for large fleets.
- Multi-vendor via a plugin model (ship one or two vendors first; add more as
  plugins without touching the core).

**Explicit non-goals**

- Print accounting, quotas, cost allocation, chargeback.
- Pull-printing / secure release / job routing.
- Acting as the primary print server (an *optional* bridge is described in §8.7, but
  it is not the product).
- Any dependency on, or re-use of, a manufacturer's management application.

Holding the line at *managing devices, not print jobs* is what keeps the product
coherent. Resist scope creep toward print management.

## 3. Design principles

1. **Vendor-agnostic core, vendor plugins at the edge.** The core never contains an
   `if vendor == …`. Manufacturer-specific endpoints and quirks live in plugins that
   implement a stable interface (§6).
2. **Standards first.** Everything that can be done over SNMP Printer MIB, IPP, eSCL,
   or mDNS is done that way, so it works on any compliant printer. Vendor interfaces
   are enrichment, never the baseline.
3. **Read-only by default.** Discovery, inventory, and monitoring never change a
   device. Writes (config, security, firmware) are separate, explicit, and gated.
4. **Safe writes.** Every write operation supports a dry run, reports per-setting and
   per-device results, can target one device before a group, and is reversible or
   has documented rollback where the protocol allows.
5. **Never overwrite operator data.** Fields a person edits (display name, location,
   asset tag, notes) are never overwritten by an automated scan.
6. **Least privilege and auditability.** The tool holds fleet-wide credentials;
   encrypt them at rest, scope who can use them, never log secrets, and record every
   write in an audit log.
7. **Idempotent, resumable jobs.** Fleet operations run as jobs that can be retried
   safely and report partial progress.

## 4. Architecture

Five layers. The web UI talks to an API; the API drives a task/job engine and reads
an inventory store and a telemetry store; the task engine and a scheduler call a
device engine; the device engine reaches the fleet through one driver per protocol.
Monitoring, configuration, and security are **task types over one device engine**,
not separate stacks.

```
            ┌───────────────────────────────────────────────┐
            │  Web UI  (dashboard, inventory, policies, jobs)│
            └───────────────────────────────────────────────┘
                                  │
            ┌───────────────────────────────────────────────┐
            │  API  (REST + WebSocket for live updates)      │
            └───────────────────────────────────────────────┘
                                  │
 ┌────────────────────────────────────────────────────────────────────┐
 │ Core services                                                        │
 │  • Task / job engine     • Policy engine (assess / remediate)        │
 │  • Inventory store        • Telemetry store (time series)            │
 │  • Scheduler / poller  ───────────────┐                              │
 └───────────────────────────────────────┼──────────────────────────────┘
                                          ▼
            ┌───────────────────────────────────────────────┐
            │  Device engine — capability model              │
            │  (discover, read, write, firmware, secure)     │
            └───────────────────────────────────────────────┘
                                  │
   ┌───────────────┬───────────────┬───────────────┬───────────────┐
   │  SNMP driver  │  IPP driver   │  eSCL driver  │ vendor plugin │ …
   └───────────────┴───────────────┴───────────────┴───────────────┘
                                  │
            ┌───────────────────────────────────────────────┐
            │  Printer fleet (any manufacturer)              │
            └───────────────────────────────────────────────┘
```

The device engine is the heart: everything above it issues tasks; everything below
it is a protocol driver. Supporting another manufacturer means adding a plugin, not
changing the layers above.

## 5. Recommended technology stack

The stack is a recommendation; confirm it at the start of the build. The choice that
matters most is keeping the **device engine behind a clean internal interface** so it
can be reimplemented or scaled independently later.

**Recommended**

- **Backend (engine + API): Go.** Fleet management is dominated by concurrent network
  I/O to many devices — Go's goroutines fit it well, it compiles to a single static
  binary (tiny containers), and it has mature libraries for SNMP (`gosnmp`), HTTP, and
  TLS. One language for the engine, drivers, scheduler, and API.
- **Frontend: a TypeScript SPA** (React or Svelte). The UI needs live dashboards,
  inventory tables, a policy editor, and job views.
- **Datastore: PostgreSQL** for inventory, groups, credentials, profiles, policies,
  jobs, and audit; **plus a time-series store** for telemetry (toner/counter/status
  history) — the TimescaleDB extension on the same PostgreSQL keeps it to one system.
  For a small single-container deployment, SQLite is an acceptable default behind the
  same data-access layer.
- **Job execution:** an in-process worker pool for small deployments; a durable queue
  (e.g. PostgreSQL-backed) when jobs must survive restarts and scale out.
- **Packaging:** Docker; `docker compose` for small sites; a Linux host or Kubernetes
  for large fleets.

**Acceptable alternative**

- **Python 3.12 + FastAPI + asyncio**, with `pysnmp`, `httpx`, and `APScheduler`.
  Faster to prototype and rich in libraries; async handles the I/O-bound workload.
  Choose this if the team is more productive in Python; it will handle hundreds to
  low-thousands of devices comfortably. If polling later outgrows it, extract only the
  collector into a Go service behind the same API.

Pick one and record the decision; do not mix backend languages without reason.

## 6. Device model and the capability / driver design

This is the single most important design decision, because it is what makes
multi-vendor cheap later.

Model **what a device can do** as *capabilities*, and implement each capability, per
protocol, as a *driver*. The engine asks "which drivers can read supplies on this
device?" — never "is this a model-X printer?".

**Capabilities** (extend as needed):

| Capability | Meaning |
| --- | --- |
| `discover` | Find devices on the network |
| `identify` | Establish make/model/serial/firmware/type for an address |
| `read_status` | Online/offline, error state, display message |
| `read_supplies` | Toner/ink/drum/fuser/waste levels and descriptions |
| `read_trays` | Input/output tray status, media size and type |
| `read_counters` | Page counts (total, mono, colour, duplex, by media) |
| `read_config` | Read the device's current settings |
| `write_config` | Change device settings |
| `read_security` | Read the device's security-relevant settings |
| `write_security` | Change security settings (protocols, access, passwords) |
| `manage_certs` | Install/renew identity and CA certificates |
| `firmware_read` | Read installed firmware version |
| `firmware_update` | Deliver a firmware update |
| `subscribe_alerts` | Receive asynchronous alerts/traps from the device |

**Driver interface** (language-neutral sketch):

```
interface Driver {
    // Which capabilities this driver can serve, and for which devices.
    capabilities() -> Set<Capability>
    supports(device Profile, cap Capability) -> bool

    // Each capability is a typed method; drivers implement the subset they declare.
    readStatus(ctx, device, creds) -> Status
    readSupplies(ctx, device, creds) -> []Supply
    readConfig(ctx, device, creds, selector) -> ConfigDocument
    writeConfig(ctx, device, creds, changes, opts{dryRun}) -> WriteResult
    // …one method per capability the driver declares…
}
```

- **Capability resolution happens at identification.** When a device is first seen,
  probe which protocols and services it answers (SNMP `sysObjectID`/`sysDescr`,
  IPP `get-printer-attributes`, mDNS TXT records, any vendor service-discovery
  endpoint), and store the resolved **capability profile** on the device record. The
  engine then dispatches each requested capability to the best driver that supports
  it for that device.
- **Standards drivers are the baseline.** Build the SNMP, IPP, and eSCL drivers to
  work on any compliant printer. These alone should deliver discovery, identification,
  full monitoring, and much of configuration for most devices.
- **Vendor plugins are enrichment.** A plugin is a driver that targets a
  manufacturer's own device interface (typically a REST/JSON or XML-over-HTTPS service
  the device exposes). It fills gaps the standards don't cover — deep configuration,
  security settings, certificate enrollment, firmware delivery. Plugins register
  against the same driver interface and are discovered dynamically; the core has no
  compile-time knowledge of any vendor.
- **Driver registry**, keyed by capability, chooses among available drivers for a
  device (preferring the richest/most reliable for that capability), and records which
  driver served each result for traceability.

Adding a manufacturer = writing a plugin that declares its capabilities and
implements them against that vendor's interface. No core change.

## 7. Protocols (all open standards)

Build these as the standard drivers. Cite the relevant RFC/spec in the code.

- **SNMP v1 / v2c / v3** — the workhorse for monitoring and basic identity on nearly
  every network printer.
  - **Printer MIB (RFC 3805)** `1.3.6.1.2.1.43`: supplies (`prtMarkerSupplies`),
    input/output trays, media, console messages, and the alert table
    (`prtAlertTable`) for human-readable error descriptions.
  - **Host Resources MIB (RFC 2790)** `1.3.6.1.2.1.25`: device status and run state.
  - **System group** `1.3.6.1.2.1.1`: `sysDescr`, `sysName`, `sysObjectID`,
    `sysUpTime` for identity and naming.
  - **Interfaces** `1.3.6.1.2.1.2`: MAC address.
  - Manufacturer **enterprise OID subtrees** are read only inside vendor plugins —
    never hard-code a vendor's enterprise number in the core.
  - Use **SNMPv3** (user + auth + privacy) wherever the device supports it for secure
    polling; fall back to v1/v2c communities otherwise.
- **IPP / IPP Everywhere** (RFC 8010/8011, PWG 5100.x) — `get-printer-attributes` for
  status, supplies, capabilities, and identity; configuration where the device exposes
  it. IPP Everywhere devices are fully manageable without any vendor code.
- **eSCL (AirScan)** — status and capabilities for scanners / MFPs that support
  driverless scanning.
- **mDNS / DNS-SD** — zero-configuration discovery and identity via service types such
  as `_ipp._tcp`, `_ipps._tcp`, `_print`, `_pdl-datastream._tcp`, `_scanner._tcp`, and
  their TXT records (which carry model, capabilities, and admin URL).
- **WS-Discovery** (multicast) — zero-config discovery of devices that advertise it.
- **SLP** — optional; include only if you need print-server/appliance discovery that
  isn't covered by mDNS or WS-Discovery.
- **Vendor device web services** — described generically: many printers expose a
  private HTTP(S) API (JSON or XML) for configuration, security, certificates, and
  firmware beyond what SNMP/IPP cover. Implement these **only in vendor plugins**,
  behind the capability interface.

## 8. Feature areas

### 8.1 Discovery

Find devices and turn each responding address into an identified device record.

- **Zero-config (preferred):** listen for and query mDNS / DNS-SD and WS-Discovery;
  this finds most modern printers with no configuration.
- **Active scan:** operator-defined targets — single IPs, ranges, and CIDR blocks —
  probed by SNMP (and optionally ICMP as a liveness pre-check). Thread the scan with a
  bounded worker pool; cap very large ranges with an explicit opt-in and log anything
  skipped (never silently truncate).
- **Identification:** for each candidate, read identity over SNMP (`sysDescr`,
  `sysObjectID`, Printer MIB), IPP (`get-printer-attributes`), and mDNS TXT, and
  resolve its capability profile (§6). Record make, model, serial, MAC, firmware,
  device type, and the methods that found and identified it.
- **De-duplication:** key devices by a stable identity (serial number or device
  UUID where available, else MAC, else address) so a device found by two methods, or
  whose address changes, is one record.
- Discovery is **read-only** and safe to run on a schedule.

### 8.2 Inventory and lifecycle

- A **device record** per printer (see §9), with automatic fields (identity,
  firmware, capabilities, status) kept current by scans, and **operator fields**
  (display name, location, asset tag, notes) that scans never overwrite.
- **Groups**: static (hand-picked) and dynamic/rule-based (by model, location,
  subnet, capability). Groups are the target of policies, profiles, and jobs.
- **Lifecycle states**: e.g. discovered → onboarded → active → needs-attention →
  decommissioned, with the transitions auditable.
- **Search, sort, filter** across the fleet; **CSV export** of device and consumable
  lists.

### 8.3 Monitoring and alerting

The highest-value, lowest-risk area — build it first.

- **Scheduled polling** at a configurable cadence, plus **on-demand poll** of a single
  device with live feedback. Poll:
  - status (online/offline, error state, console/display message),
  - supplies (each toner/ink/drum/fuser/waste unit: level, capacity, description),
  - trays (per-tray OK/low/empty, media size and type),
  - counters (total, mono, colour, duplex; store as time series for trends).
- **Asynchronous alerts:** a listener for SNMP traps/informs so devices can push
  events; reconcile trap events with polled state.
- **Alert model:** raise, acknowledge, and clear alerts with severity (OK / warning /
  critical), source, and a human-readable description. Pull device-reported error
  descriptions from the Printer MIB `prtAlertTable`. Reduce noise with sensible rules
  (e.g. raise a "no paper" fleet alert only when all trays of a size are empty, while
  still showing per-tray state).
- **Consumable stock alerts:** optional stock tracking per consumable type with
  warning/critical thresholds.
- **Notifications:** webhook (Slack / Teams / Discord / generic) and SMTP email, per
  configurable event set.

### 8.4 Configuration management

- **Read config:** pull a device's current settings over the best available
  capability (IPP, SNMP, or a vendor plugin) into a normalized **configuration
  document** grouped by area (network, protocols, security, trays/media, energy/sleep,
  control panel, etc.).
- **Configuration profiles:** a named, reusable set of desired settings. A profile
  targets a device or a group.
- **Apply (write):** push a profile or ad-hoc changes with a **dry run** first, then
  for real, returning **per-setting and per-device results**. Start on one device,
  then a group. Writes go through the capability interface (IPP set / SNMP set /
  vendor plugin) and are audited.
- **Baseline / drift:** diff a device or group against a profile and report settings
  that have drifted; optionally re-apply.

### 8.5 Security

Model security as a first-class task type: **assess → remediate → report**.

- **Security policy:** a named set of **policy items**. Each policy item is a desired
  security setting (e.g. SNMPv3 required, legacy protocols disabled, admin password
  set, TLS enforced, specific services off, an identity certificate present and valid)
  with a way to **assess** it (read the current value) and **remediate** it (write the
  desired value).
- **Assessment:** evaluate a device or group against a policy; produce per-item
  pass/fail and an overall **compliance result**, with optional risk weighting.
- **Remediation:** apply the fixes for failing items, with the same safeguards as any
  write (dry run, per-item results, one device before a group, audit). Remediation can
  lock you out if done wrong — always preserve a working management path and confirm
  before fleet-wide changes (see §11).
- **Certificate management:** install and renew device identity and CA certificates;
  support standard enrollment protocols (**SCEP**, **EST**, **ACME**) so the tool can
  obtain certificates from a CA on the device's behalf. Track expiry and alert ahead
  of it.
- **Credential management:** store and supply the credentials a device needs
  (SNMP communities, SNMPv3 users, admin/EWS passwords, certificate material),
  encrypted and access-scoped.
- **Reporting:** compliance reports per device, group, and policy, over time.

### 8.6 Firmware management

Highest operational risk — build last, with safeguards.

- **Inventory** installed firmware versions across the fleet.
- **Updates:** deliver a firmware package to a device via the capability interface
  (vendor plugin or a standard mechanism where one exists), with
  **version/compatibility checks** before delivery.
- **Campaigns:** schedule staged rollouts across groups, with maintenance windows,
  concurrency limits, and clear status.
- **Safeguards:** confirmation gates, staged rollout (canary a few devices first),
  and documented rollback/recovery guidance. A failed or interrupted update can
  disable a device — treat this feature with the most care.

### 8.7 Optional print-provider add-on

A separate, optional module, **off by default**, for sites that want PrintMarshal to
also act as a print server. On Linux the natural form is a **CUPS / IPP Everywhere
bridge** that creates and serves queues to discovered printers. Keep it entirely out
of the core; it is a convenience for the "run it as a print/web server too" case, not
part of fleet management. Build it only after the core lifecycle features land.

## 9. Data model

A sketch, not a final schema.

- **Device** — stable id; identity (addresses, hostname, MAC, serial, UUID); make,
  model, vendor, device type; firmware; resolved capability profile; discovery and
  identification methods; group membership; **operator fields** (display name,
  location, asset tag, notes); lifecycle state; timestamps.
- **Group** — static or rule-based; the target for profiles, policies, and jobs.
- **Credential** — type (SNMP community, SNMPv3 user, admin/EWS password, certificate
  material); scope (device or group); **encrypted**; referenced by id, never inlined.
- **Configuration profile** — named desired settings and the capability that applies
  each.
- **Security policy** — named set of policy items (desired security settings) with
  assess + remediate definitions; plus per-device compliance results.
- **Job / task** — a unit of work (discovery run, poll, config apply, security
  assess/remediate, firmware campaign) with target, parameters, status, schedule, and
  per-device results.
- **Telemetry sample** — time-series status, supply levels, and counters per device.
  This is the fastest-growing table; size the store for it and down-sample old data.
- **Alert** — raised condition (device or consumable), severity, source, description,
  acknowledgement; plus a notification-delivery log.
- **Audit log** — actor, action, target device(s), before/after, and result, for
  every write operation.

## 10. API surface

Resource-oriented REST plus a WebSocket channel for live dashboard/job updates.
Illustrative:

```
GET    /devices            ?group=&model=&status=&q=
GET    /devices/{id}
PATCH  /devices/{id}                 # operator fields only
POST   /devices/{id}/poll            # on-demand read
GET    /groups  /groups/{id}
POST   /discovery/runs               # start a discovery job
GET    /credentials  POST /credentials       # secrets write-only via API
GET    /profiles  POST /profiles
POST   /profiles/{id}/apply          # body: target + {dryRun}
GET    /policies  POST /policies
POST   /policies/{id}/assess         # body: target
POST   /policies/{id}/remediate      # body: target + {dryRun}
GET    /firmware/campaigns  POST /firmware/campaigns
GET    /jobs  /jobs/{id}             # status + per-device results
GET    /alerts  POST /alerts/{id}/ack
GET    /telemetry?device=&metric=&from=&to=
WS     /stream                       # device status, job progress, new alerts
```

All write endpoints are audited; all that touch devices accept `dryRun` where
meaningful and return per-device results.

## 11. Security and safety requirements

- **Secrets at rest:** encrypt all device credentials and certificate private keys.
  Never return secret values from the API; never write them to logs.
- **Access control:** role-based (at least viewer / operator / admin). Writes to
  devices and to credentials require operator/admin.
- **Safe-write discipline:** dry run, per-setting/per-device results, single-device
  test before group, and preservation of a working management path on every security
  remediation (never disable the last channel you manage the device through).
- **Rate limiting / throttling** of device I/O so scans and campaigns don't overwhelm
  devices or the network.
- **Idempotent jobs** with retries and partial-progress reporting.
- **Audit everything** that changes device state.
- **Authorization boundary:** PrintMarshal is for devices the operator owns or is
  authorized to manage. Discovery, configuration, firmware, and security writes act on
  real hardware; require authorization before fleet-wide action.

## 12. Deployment and runtime

**Discovery constrains how the collector runs.** It relies on broadcast and multicast
(mDNS, WS-Discovery, SNMP broadcast, SLP) and sometimes raw ICMP. These do **not**
work through ordinary container bridge networking.

- The **collector/engine** needs host networking (or a macvlan) and access to the same
  L2 network as the printers to do discovery. The **API, UI, and database** containerize
  normally.
- Ship a `docker compose` for small sites (engine with host networking + Postgres +
  UI). Scale to a dedicated **Linux host** or Kubernetes for large fleets; size by
  device count and polling cadence.
- Configuration via a file plus environment overrides; secrets via the platform's
  secret mechanism, not the image.

## 13. Development and test environment

- Develop the collector on a **Linux environment with real L2 access to a printer
  subnet** so discovery actually works. On a Mac, a **Lima VM** with shared/bridged
  networking is a good fit; plain container bridge networking will make discovery look
  broken when it isn't.
- Test against **at least one real network printer** that supports standard IPP, SNMP,
  and (for MFPs) eSCL. Many printers expose standard read endpoints without
  authentication, so monitoring can be validated end to end early. For breadth, add a
  printer **simulator** (e.g. an SNMP agent seeded with Printer MIB values, and an IPP
  server) so CI can run without hardware.
- Record real device responses as **fixtures** and unit-test the parsers and drivers
  against them, so behaviour is pinned without a live device.

## 14. Testing strategy

- **Unit:** protocol parsers and each driver, against recorded fixtures.
- **Contract:** every driver against the capability interface, so plugins are
  interchangeable.
- **Integration:** against a real printer and/or the simulator — discovery,
  identification, polling, config read, and a guarded config write.
- **Safety:** explicit tests that dry-run writes change nothing, that operator fields
  survive a scan, that secrets never appear in API output or logs, and that a
  security remediation never removes the last management path.

## 15. Build roadmap (phased by risk)

Each phase is shippable. Order is by blast radius: read-only first, writes later,
firmware last.

1. **Foundations + monitoring (read-only).** Project skeleton, data layer, the SNMP
   and IPP drivers, mDNS/WS-Discovery discovery, scheduled + on-demand polling of
   status/supplies/trays/counters, the dashboard, and the trap/alert listener with
   webhook/email notifications. Immediate value, near-zero risk.
2. **Inventory and lifecycle.** Device records, operator fields, groups (static +
   rule-based), states, search/filter, CSV export, and telemetry history/trends.
3. **Configuration read.** Normalized configuration documents; device-vs-profile and
   device-vs-device diffs; drift reporting.
4. **Configuration write.** Profiles applied to a device, then groups, with dry run,
   per-setting results, and audit. Start with low-risk settings.
5. **Security.** Policy model (assess → remediate → report), compliance scoring,
   credential management, and certificate enrollment (SCEP/EST/ACME).
6. **Firmware.** Version inventory, controlled updates, staged campaigns, and
   safeguards. Highest risk — last.
7. **Breadth and extras.** Additional vendor plugins; the optional print-provider
   module (§8.7).

## 16. Open decisions for the build session to confirm

- Backend language (Go recommended; Python/FastAPI acceptable) and UI framework.
- Datastore: Postgres + time-series for production; SQLite for the single-container
  default — confirm the data-access abstraction that supports both.
- Job execution: in-process pool vs durable queue, and when to switch.
- Auth model and how the first admin is bootstrapped.
- Which one or two vendors (and which device interfaces) to target as the first
  plugins, versus relying purely on the standard drivers for v1.
- Whether to include SLP discovery in v1 or defer it.
