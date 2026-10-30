![PrintMarshal](https://raw.githubusercontent.com/printmarshal/printmarshal/main/brand/logo-horizontal.svg)

# PrintMarshal administrator and developer wiki

PrintMarshal is a self-hosted manager for network printers and multifunction devices: discover, inventory, monitor, configure, assess security, and orchestrate firmware campaigns. It manages devices rather than users' print jobs. Print accounting, quotas, cost allocation, secure release and pull printing are outside the core. An optional [print-provider add-on](Print-provider.md) manages queues on the server's local CUPS installation.

This guide describes the repository as read on 2026-10-29. It is for operators running the service, administrators configuring access and integrations, and developers adding drivers or persistence. Source paths identify implementation points; “Known discrepancies” notes distinguish executable behavior from older specifications or feature documentation. Examples are instructions for an authorized environment, not a record of commands executed while writing this wiki.

## Principles and actual boundaries

- **Read-only by default:** discovery and polling read devices. Stock, groups and profiles change PrintMarshal records. Device configuration/security/firmware writes require explicit operator/admin actions and suitable driver capability.
- **Dry run first:** apply, revert, remediation, rotation, enrollment, firmware start and print-provider sync default to dry run. Explicit `dryRun:false` releases the API gate; the backend does not require a previous successful rehearsal. The already-issued certificate install and queue DELETE endpoints have no preview flag.
- **Audited writes:** services and API handlers record actor, action, target and result. Review job results and verification outcomes; accepted is not the same as verified. A database/audit failure is an operational failure to investigate, not a guarantee that hardware stayed unchanged.
- **Vendor-agnostic core:** capabilities select drivers. Standards provide the baseline; vendor plugins enrich observed information and report unsupported operations honestly.
- **Preserve operator data:** automated reads do not overwrite display name, operator location, asset tag or notes. Reconciliation checks stable identity conflicts before reusing an address-matched record.
- **Least privilege:** viewer/operator/admin roles, optional API-token group scope, purpose-bound read/write credentials, encrypted credential/private-key storage, and hashed sessions/tokens.

## Feature map

| Need | Read |
| --- | --- |
| Understand one process and its boundaries | [Architecture](Architecture.md) |
| Install on a host or orchestrator; back up and upgrade | [Installation and deployment](Installation-and-deployment.md) |
| Find an exact JSON key, environment variable or default | [Configuration reference](Configuration-reference.md) |
| Find printers, reconcile records and organize a fleet | [Discovery and inventory](Discovery-and-inventory.md) |
| Poll devices, understand alerts, traps, telemetry and reports | [Monitoring and alerts](Monitoring-and-alerts.md) |
| Track spare supplies and stock movements | [Consumables and stock](Consumables-and-stock.md) |
| Preview/apply profiles, inspect drift and revert | [Configuration management](Configuration-management.md) |
| Assess/remediate policy, stage rotations and enroll certificates | [Security](Security.md) |
| Register packages and run staged campaigns | [Firmware](Firmware.md) |
| Configure local login, tokens, OIDC, SAML, LDAP/AD or RADIUS | [Identity and access](Identity-and-access.md) |
| Configure notifications, SIEM, syslog, sync and metrics | [Events, notifications and integrations](Events-notifications-and-integrations.md) |
| Integrate through exact routes and body schemas | [API reference](API-reference.md) |
| Add or qualify device support; use fixtures/simulator | [Vendor plugins and drivers](Vendor-plugins-and-drivers.md) |
| Provision optional local CUPS queues | [Print provider](Print-provider.md) |
| Build, test or extend the store | [Development](Development.md) |
| Diagnose concrete symptoms | [Troubleshooting and FAQ](Troubleshooting-and-FAQ.md) |

## First operational workflow

1. Deploy one active process with durable storage and the encryption key preserved. Terminate HTTPS at a proxy and enable secure cookies.
2. Sign in with the initial local administrator; change the initial password and remove its generated password file. Configure external authentication without replacing local recovery access.
3. Add read credentials, run a bounded discovery in an authorized subnet, and inspect identity/capability results. Use static or rule groups for organization.
4. Establish healthy polls, notification delivery, telemetry retention and backups. Choose operator/viewer roles appropriate to daily work.
5. Test writes against the simulator or an explicitly authorized lab device. Bind separate write credentials and inspect outcomes before expanding targets.

## Known discrepancies and feature limits

The original build specification is a design target, not an inventory of completed transports. No shipped real-vendor plugin supplies certificate installation, security writing or firmware delivery; standards SNMP/IPP implement a limited configuration write set, and firmware delivery is simulator-only unless a new driver is added. The report endpoint `/api/reports/compliance` remains a placeholder even though security assessment summary/history APIs work. Jobs retain history but ordinary interrupted jobs do not auto-resume. Group scope is not a tenant-isolation boundary; see its exact limits under [Identity and access](Identity-and-access.md).

Sources: `cmd/printmarshal/main.go`, `internal/engine/`, `internal/api/`, `internal/plugins/`, `internal/reports/compliance.go`, `docs/printmarshal-build-spec.md` and the feature guides in `docs/`.
