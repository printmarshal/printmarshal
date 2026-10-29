<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="brand/logo-horizontal-on-dark.svg">
    <img src="brand/logo-horizontal.svg" alt="PrintMarshal" width="440">
  </picture>
</p>

<p align="center"><strong>The command-and-security console for your printer fleet.</strong></p>

<p align="center">
  <a href="LICENSE"><img alt="License: Apache-2.0" src="https://img.shields.io/badge/license-Apache--2.0-2E54E6"></a>
  <img alt="Go 1.27" src="https://img.shields.io/badge/Go-1.27-2E54E6">
  <img alt="React + TypeScript" src="https://img.shields.io/badge/UI-React%20%2B%20TypeScript-2E54E6">
  <img alt="SQLite or PostgreSQL" src="https://img.shields.io/badge/store-SQLite%20%7C%20PostgreSQL-2A313D">
  <a href="wiki/Home.md"><img alt="Wiki" src="https://img.shields.io/badge/docs-wiki-D7A442"></a>
</p>

**PrintMarshal** is a self-hosted, vendor-agnostic platform for managing a fleet of
network printers across their full lifecycle: **discover, inventory, monitor,
configure, secure, update firmware, and retire.**

PrintMarshal manages *devices*, not print jobs. It is **not** a print-accounting,
quota, cost-allocation, or pull-printing tool, and it depends on no manufacturer's
management software. The administrator and developer documentation lives in the
[wiki](wiki/Home.md); the brand assets are in [`brand/`](brand/README.md).

## Status

**All seven phases of the [build spec](docs/printmarshal-build-spec.md) are
implemented**: foundations and read-only monitoring (1–2), configuration read/write
(3–4), security (5), firmware (6) and vendor plugins plus the optional print-provider
add-on (7), together with the enterprise-integration hardening from the research
audit (event bus, durable notifications, authentication and transport controls,
SIEM export, operational endpoints). Everything that touches a printer is read-only
unless an operator explicitly runs a gated, dry-run-by-default, audited write
(configuration apply, security remediation, credential rotation, certificate
install, firmware delivery) through a driver that implements the write-side
capability. Standards drivers (SNMP, IPP, eSCL) have been validated live against
an HP Color LaserJet MFP M283fdw; writes have been exercised only against the
simulator.

What works today:

- **Discovery** — mDNS/DNS-SD and WS-Discovery zero-config, plus active scans of IPs,
  ranges and CIDR blocks with a bounded worker pool and an explicit cap on large scans.
- **Identification** over SNMP (system group, Host Resources MIB, Printer MIB), IPP
  (`Get-Printer-Attributes`) and eSCL (scanner capabilities, which also marks MFPs), merged into one record and de-duplicated by serial → UUID →
  MAC → address. Each device gets a *capability profile* recording which driver serves
  which capability.
- **Monitoring** — scheduled and on-demand polls of status, supplies, trays, counters and
  firmware; time-series telemetry; an SNMP trap listener that reconciles pushed events
  with a poll.
- **Alerts** — raised/cleared/acknowledged with severity; device-reported alerts from
  `prtAlertTable`; supply thresholds; a fleet-level "out of paper" rule that only fires
  when every tray of a size is empty; offline detection; consumable stock alerts.
  Notifications via Slack/Teams/Discord/generic webhooks and SMTP.
- **Consumables** ([spec](docs/consumables-feature-spec.md)) — a stock inventory of
  spare toner / ink / drums / fusers / kits, each linked to the devices that use it
  (matched by part number, else type + colour + compatible model; manual links are
  never overridden), with operator-owned on-hand quantities and thresholds, inline
  edit with batch save, a complete audited stock history, low/out-of-stock alerts, a
  fleet-wide installed-supply view, replacement detection with off / prompt /
  automatic stock decrement, consumption forecasting ("days remaining", "reorder
  soon", or an honest "unknown" until there is enough history), and CSV import/export.
  Stock management only: cost and reorder fields are optional references.
- **Configuration** ([docs](docs/configuration.md)) — normalised config documents read
  over IPP (`Get-Printer-Attributes`) and SNMP (system group, Printer MIB) and merged
  per key, snapshots, device-vs-device diff, named profiles, drift reports, and gated
  writes (IPP `Set-Printer-Attributes`, SNMP SET) that run as jobs with a dry run,
  per-setting results, rollback/revert and an audit entry for every write.
- **Firmware** ([docs](docs/firmware.md)) — fleet-wide version inventory with
  "outdated" detection against registered packages, uploaded packages verified by
  SHA-256 on every use, and staged update campaigns run as jobs: canary wave with an
  explicit confirmation gate before the remainder, concurrency limit, failure-threshold
  auto-pause, maintenance windows, dry runs that never deliver, post-update
  verification by re-polling the device, an audit entry per delivery, restart
  recovery and honest rollback/recovery notes. Delivery is a driver capability
  (`firmware_update`): the standard driver targets the simulator or an explicit
  allow-list only; real devices need a vendor plugin.
- **Device model catalogue** ([docs](docs/catalogue.md)) — a shipped research catalogue
  of 126 enterprise printer families (495 base model identifiers, 15 brands) with each
  vendor's documented management routes, per-model operation evidence and its source
  registry; every discovered device is matched to a family (exact / family / brand-only /
  none) and shown with what is *documented*, what PrintMarshal's drivers have *observed*,
  and what is *unknown* — kept separate, never collapsed into "supported". Operators can
  record lab/production validation per family (audited).
- **Reports** ([docs](docs/reports.md)) — utilisation (pages per day per device,
  group and fleet, reset-aware), fleet health (reachability, outages, worst
  severity per day), supplies (consumption rate and days-to-empty projections,
  replacements, stock movement), alerts (volume by day/source/device, mean time
  to acknowledge and clear) and firmware version distribution, over any date
  range, scoped to a group or device, as JSON or CSV; compliance reporting is a
  documented hook awaiting the security feature.
- **Inventory** — operator fields (display name, location, asset tag, notes) that scans
  never overwrite, lifecycle states with validated transitions, static and rule-based
  groups, search/sort/filter, CSV export of devices and supplies.
- **Security** ([docs](docs/security.md)) — policies of desired security settings with
  eight operators and severity-weighted compliance scores, assessed from drivers plus
  vendor-neutral TLS and port probes; dry-run-by-default remediation that never removes
  the last management path and audits every write; SCEP / EST / ACME (dns-01)
  certificate enrollment with expiry alerts; SNMP community and admin password
  rotation that never exposes the secret.
- **Print provider (optional, off by default)** ([docs](docs/print-provider.md)) — a
  CUPS / IPP Everywhere bridge for a Linux host that should also serve print queues
  for discovered printers: plan, dry run, sync as an audited job, per-device removal.
  It only configures CUPS on the PrintMarshal host and never changes a printer; it is
  a convenience, not print management.
- **Events and notifications** ([docs](docs/events.md)) — one event catalogue and
  bus feeding the live stream and a durable, retried notification outbox (dead-letter
  rows are visible and requeueable); Slack / Discord / generic webhooks, Teams
  Workflows (Adaptive Cards) and SMTP with PLAIN, LOGIN or XOAUTH2 (Microsoft 365,
  Google Workspace) authentication.
- **Identity and access** ([docs](docs/auth.md)) — local accounts, optional OpenID
  Connect single sign-on (authorization code + PKCE, discovery, RS256/ES256 ID-token
  verification with the standard library, role mapping from IdP claims, domain
  allow-list) for Entra ID / Okta / Keycloak and the like, plus the legacy enterprise
  methods: SAML 2.0 (SP metadata, redirect/POST bindings, signature, audience,
  validity and replay checks, Entra ID / Okta / ADFS / Keycloak notes), LDAP /
  Active Directory (service-account search or direct bind, StartTLS / LDAPS,
  groups → roles) and RADIUS PAP (RFC 2865/2869, stdlib only) behind the same login
  form. Every external account is bound by issuer + subject, never by username.
  API tokens are first-class service accounts (admin-issued, secret shown once,
  expiring, revocable, unable to touch authentication or users), and group scopes
  confine a token to the devices in named groups across every device route, nested
  target and device-owned resource. Every sign-in, denial and token change is a
  security event in the audit log.
- **Exports and SIEM** ([docs](docs/export.md)) — every security event, alert
  and dead notification forwarded from the event bus to RFC 5424 syslog (UDP /
  TCP / TLS, with structured data, CEF or JSON payloads), a rotating JSON-lines
  file and/or an HTTP collector (batched, bearer token from the environment,
  retries), through a bounded queue that never blocks the bus; a UDP syslog
  receiver that keeps what printers send per device; and an incremental sync API
  (`GET /api/sync/{collection}?since=`) with an opaque `(updatedAt, id)` cursor
  and tombstones so CMDB / ITSM tools mirror devices, alerts, audit, jobs and
  configuration writes without missing or repeating a row.
- **Platform** — REST + WebSocket API, role-based users (viewer / operator / admin),
  encrypted credential store whose secrets are write-only through the API, an audit log
  that also flows to the event bus, sign-in throttling, session revocation on password
  change, same-origin streams that re-check their session, trap source trust and rate
  limits, `/readyz` and Prometheus `/metrics`, resumable jobs with per-device results,
  a React web UI, a printer simulator for development/CI, Docker / Kubernetes / Helm /
  systemd packaging.
  encrypted credential store whose secrets are write-only through the API, an audit log,
  resumable jobs with per-device results, a React web UI, a printer simulator for
  development/CI, Docker packaging.
- **Purpose-bound credentials and verified writes** — every credential is `read`
  (default), `write` or `both`; configuration, remediation, certificate and firmware
  writes use only write credentials and are refused on a device that has none bound,
  and every real write is read back and reported as `verified` / `diverged` /
  `unverifiable`, not merely accepted (see `docs/configuration.md`).

## Quick start

Requirements: Go 1.27.3+ (matching `go.mod`), Node 20+ (only to build the UI; the Dockerfile uses `golang:1.27-alpine` and `node:22-alpine`), or Docker.

```bash
make web        # build the SPA into internal/web/dist (once, or after UI changes)
make build      # bin/printmarshal and bin/pm-sim
PM_ADMIN_PASSWORD=change-me ./bin/printmarshal
```

Open <http://localhost:8080>, sign in as `admin`, go to **Discovery** and start a run.
Without `PM_ADMIN_PASSWORD` the first start generates a password and writes it to
`<data dir>/initial-admin-password` (mode 0600); change it after signing in and delete the file.

To try it without a printer, run the simulator and point the server at its ports:

```bash
./bin/pm-sim -fixture testdata/fixtures/generic-laser.snmp -snmp 127.0.0.1:1161 -ipp 127.0.0.1:6631
PM_SNMP_PORT=1161 PM_IPP_ENDPOINTS='http://%s:6631/ipp/print' PM_ADMIN_PASSWORD=change-me ./bin/printmarshal
# then discover target 127.0.0.1 with zero-config off
```

## Deploying

Everything below is covered in depth in the **[operations guide](docs/operations.md)**
(install options, full configuration reference, networking, backups, upgrades,
troubleshooting, hardening and sizing).

```bash
make docker                                   # builds printmarshal:<version> (multi-stage, non-root, /healthz check)
docker run -d --name printmarshal --network host \
  -e PM_ADMIN_PASSWORD=change-me -v pm-data:/data printmarshal:latest
```

- **Docker Compose** (small sites): `docker compose -f deploy/docker-compose.yml up -d --build`.
- **Kubernetes**: plain manifests in [`deploy/kubernetes/`](deploy/kubernetes/)
  (`kubectl apply -k deploy/kubernetes`) or the Helm chart in
  [`deploy/helm/printmarshal/`](deploy/helm/printmarshal/).
- **Bare Linux host**: [`deploy/systemd/`](deploy/systemd/) unit and environment file.
- **Backups**: [`deploy/scripts/backup.sh`](deploy/scripts/backup.sh) / `restore.sh`
  (the database *and* `secret.key` together, or stored credentials are lost).

The container and the Kubernetes pod use **host networking** because multicast
discovery (mDNS, WS-Discovery) and SNMP traps do not cross a bridge or overlay
network (spec §12); the manifests explain the trade-off and the alternatives.

## Configuration

Defaults are sensible for a small site. Override with a JSON file (`-config` or
`PM_CONFIG`, see [`deploy/printmarshal.example.json`](deploy/printmarshal.example.json))
and/or environment variables:

| Variable | Meaning | Default |
| --- | --- | --- |
| `PM_LISTEN` | HTTP listen address | `:8080` |
| `PM_DATA_DIR` | SQLite database and encryption key | `./data` |
| `PM_DATABASE` / `PM_DSN` | `sqlite` (file in the data dir) or `postgres` with a `postgres://…` DSN — see [operations guide](docs/operations.md#postgresql) | `sqlite` |
| `PM_ADMIN_PASSWORD` | Initial admin password (first start only) | generated |
| `PM_SECRET_KEY` | Base64 32-byte key for credentials at rest | generated into `data/secret.key` |
| `PM_POLL_INTERVAL` | Scheduled poll cadence | `5m` |
| `PM_POLL_CONCURRENCY` | Max simultaneous device reads | `32` |
| `PM_DISCOVERY_ENABLED` / `PM_DISCOVERY_INTERVAL` / `PM_DISCOVERY_TARGETS` | Scheduled discovery | off / `1h` / none |
| `PM_TRAP_ENABLED` / `PM_TRAP_LISTEN` | SNMP trap listener (`trap.allowedSources`, `trap.ratePerMinute` in the file) | on / `0.0.0.0:1162` |
| `PM_SECURE_COOKIES` | Mark the session cookie `Secure` (set it behind TLS) | `false` (warns on non-loopback listen) |
| `PM_SNMP_PORT`, `PM_IPP_ENDPOINTS` | Device ports (for simulators) | `161`, standard IPP paths |
| `PM_LOG_LEVEL`, `PM_LOG_FORMAT` | `debug|info|warn|error`, `text|json` | `info`, `text` |

Notifiers (webhooks, SMTP) are configured in the JSON file; SMTP passwords are read
from the environment variable named in `passwordEnv`, never from the file. The
complete list of settings (every JSON field, its `PM_*` override and default) is in
the [operations guide](docs/operations.md#configuration-reference).

## Development

```bash
make test                       # go vet + unit, contract, integration and safety tests
cd web && npm run dev           # UI dev server on :5173 proxying /api to :8080
```

Layout:

```
cmd/printmarshal      server entry point          cmd/pm-sim        SNMP+IPP printer simulator
internal/engine       capability model, driver registry, identify/poll
internal/driver/snmp  SNMP driver (RFC 3418/2863/2790/3805)   internal/driver/ipp  IPP driver (RFC 8010/8011, PWG 5100.13)
internal/discovery    mDNS, WS-Discovery, active scan          internal/fleet       orchestration: discovery runs, polls, alerts
internal/store        Store interface + SQLite / PostgreSQL     internal/jobs        worker pool, resumable jobs
internal/alerts       rules + durable notifier                  internal/trap        SNMP trap listener
internal/events       event catalogue, envelope and bus         internal/catalog     device model catalogue
internal/configmgmt   configuration (phases 3–4)                internal/security    policies, remediation, PKI (phase 5)
internal/firmware     packages and campaigns (phase 6)          internal/plugins     vendor plugins (phase 7)
internal/reports      report aggregates                         internal/printprovider  optional CUPS bridge
internal/api          REST + WebSocket                          internal/auth        users, sessions, roles
web/                  React/TypeScript SPA (built into internal/web/dist and embedded)
testdata/fixtures     recorded-style SNMP images used by tests and the simulator
```

See [`docs/decisions.md`](docs/decisions.md) for the stack decisions and
[`docs/adding-a-driver.md`](docs/adding-a-driver.md) for how vendor plugins attach.

## Principles

- **Vendor-agnostic core, vendor plugins at the edge.** The core speaks open
  standards to any printer; manufacturer-specific behaviour lives in optional
  plugins.
- **Standards first** — SNMP (Printer MIB, Host Resources MIB), IPP / IPP Everywhere,
  eSCL, mDNS / DNS-SD.
- **Read-only by default.** Configuration, security, and firmware *writes* are
  explicit, gated, dry-runnable, auditable, and reversible where possible.
- **Never overwrite operator data.** Fields a person sets (name, location, asset tag,
  notes) are never overwritten by an automated scan.
- **Secrets encrypted; least-privilege access; full audit trail.**

Only discover and manage printers you own or are authorised to manage.

## License

Apache License 2.0; see [LICENSE](LICENSE).
