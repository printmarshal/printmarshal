---
title: "PrintMarshal \u2014 Command your printer fleet"
description: "Self-hosted printer fleet management. Discover devices, monitor faults, assess security, and control supported changes with dry runs and audit records."
---

## Hero

### Bring your printer fleet under command.

The command-and-security console for your printer fleet.

[Get started](/get-started) · [Explore features](/features)

## What it does

### Discovery

Find printers through multicast discovery or bounded active scans. Reconcile identity and keep operator-owned inventory fields intact.

### Monitoring and alerts

Track status, supplies, trays, counters, and firmware. Surface device faults, offline conditions, and low stock, then route notifications to your team.

### Configuration management

Read settings, compare devices, track profile drift, and preview supported changes. Review per-setting results and revert recorded values where possible.

### Security and certificates

Assess policies, inspect TLS, and enroll certificates through SCEP or EST. Device remediation and certificate installation depend on implemented driver capabilities.

### Firmware campaigns

Track versions and rehearse staged rollouts with canary gates. Delivery currently targets the simulator; real printers need an additional driver.

### Identity and access

Use local accounts, OIDC, SAML, LDAP/Active Directory, or RADIUS. Assign viewer, operator, and admin roles, with revocable API tokens for integrations.

## How it works

1. **Deploy on your network.** Run one active server with persistent storage and access to your authorized printer networks.
2. **Discover and establish a baseline.** Add read credentials, identify devices, and review capabilities, status, supplies, and security observations.
3. **Plan changes deliberately.** Select targets, inspect a dry run, and authorize supported writes with separate credentials; review audit records and verification results.

## Built for operators

- **Read-only by default.** Discovery and monitoring read devices; device changes require an explicit authorized workflow.
- **Dry-run first.** Configuration apply, security remediation, credential rotation, certificate enrollment, and firmware start default to preview mode.
- **Audited writes.** Write workflows record the actor, target, and result so operators can inspect what was attempted.
- **Vendor-agnostic.** Standards drivers provide the baseline, while vendor plugins add device-specific observations.
- **Self-hosted.** Run the application and its SQLite or PostgreSQL store in your own environment.

## Standards and integrations

| Area | Implemented connections |
| --- | --- |
| Device information | SNMP, IPP, and eSCL scanner identification |
| Discovery | mDNS/DNS-SD and WS-Discovery, plus active SNMP scans |
| Identity | OIDC, SAML, LDAP/Active Directory, and RADIUS PAP |
| Notifications | Slack, Teams, Discord, generic webhooks, and SMTP |
| Operations | Syslog/SIEM export, printer syslog ingestion, REST API, incremental sync, and authenticated Prometheus metrics |

Available operations depend on the device and its resolved driver capabilities. Integration names describe implemented interfaces, not endorsements.

## Open source

PrintMarshal is licensed under Apache-2.0. Inspect the implementation, deploy it on your infrastructure, or extend its drivers and stores.

[View on GitHub](https://github.com/printmarshal/printmarshal) · [Read the wiki](https://github.com/printmarshal/printmarshal/blob/main/wiki/Home.md)

## Take command of your fleet

Start with discovery and monitoring. Test supported writes in the simulator before using them on authorized hardware.

[Choose your installation](/get-started) · [Read the security model](/security)
