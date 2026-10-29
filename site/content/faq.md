---
title: "PrintMarshal FAQ"
description: "Answers about printer support, device writes, licensing, databases, high availability, authentication, discovery, the simulator, and contributions."
---

## Is PrintMarshal a print-accounting tool?

No. It manages network devices: inventory, status, supplies, configuration, and security observations. It does not provide per-user print accounting, quotas, cost allocation, secure release, or pull printing. An optional add-on provisions local CUPS queues; it does not add those accounting features.

## Which printers can it manage?

The baseline is standards-based: SNMP and IPP for device information, with eSCL for scanner identification. Compiled-in vendor plugins add identity and selected observations for supported families. The actual device profile determines which operations are available; matching a brand or family is not a guarantee that every operation works.

The research catalog contains 126 enterprise printer families. It separates documented routes, observed driver behavior, and unknowns. Catalog coverage is not a list of physically validated models or a manufacturer endorsement. Review the [driver and evidence guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Vendor-plugins-and-drivers.md) for your fleet.

## Will it write to a printer without asking?

Discovery, monitoring, and assessment read devices. Device writes require an explicit authorized workflow, a supporting driver, and write-purpose credentials. Configuration apply, remediation, rotation, enrollment, and firmware start default to dry run; explicit `dryRun:false` enables real execution.

Authorization can cover a campaign rather than an individual confirmation for each device. Installing an already-issued certificate has no dry-run flag. The API does not enforce a previous rehearsal. Review the [security model](/security) before granting operator access.

## Can it update firmware and install certificates today?

Firmware inventory, packages, campaign planning, and simulator delivery are implemented. No real-vendor firmware updater ships, so delivery to physical printers needs an additional driver. There is also no shipped real-vendor security writer or certificate installer.

SCEP and EST enrollment clients can obtain certificates, but issuing a certificate does not install it. Private keys remain encrypted and are not downloadable through the API. Automatic certificate renewal is not implemented.

## What is the license?

PrintMarshal is open source under the Apache License 2.0. Read the [license](https://github.com/printmarshal/printmarshal/blob/main/LICENSE) for its terms and the [repository](https://github.com/printmarshal/printmarshal) for the implementation.

## Which databases are supported?

SQLite is the default local store. PostgreSQL is also implemented. Both backends persist inventory and feature records behind the same store interface.

Back up the encryption key as well as the database. PostgreSQL deployments still need to preserve application files such as firmware images and generated SAML material. There is no supplied automatic SQLite-to-PostgreSQL data-transfer command.

## Can I run an active-active cluster?

No. The supported deployment has one active server. Scheduling, job execution, and firmware lifecycle coordination run in-process. PostgreSQL does not add a distributed worker lease or make multiple active servers safe.

Ordinary queued or running jobs are marked failed after restart. Firmware campaigns have their own paused recovery flow for operator review. See the [architecture guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Architecture.md).

## What sign-in methods are available?

Local accounts, OIDC, SAML, LDAP/Active Directory, and RADIUS PAP. External methods are optional, support role mapping, and bind accounts by issuer and subject. A first external login cannot claim an existing local username.

Viewer, operator, and admin roles control access. API tokens support revocation, optional expiry, and group scope for defined device operations. Scope does not isolate all reports, collections, or live events. RADIUS challenge-response prompts and native RadSec are not implemented. See [identity and access](https://github.com/printmarshal/printmarshal/blob/main/wiki/Identity-and-access.md).

## How does discovery work across networks?

mDNS/DNS-SD and WS-Discovery collect multicast advertisements and responses on reachable networks. They need suitable multicast access; a routed or bridged deployment may not provide it. The supplied deployments use host networking by default.

For routed networks, active scans accept explicit addresses, ranges, and IPv4 CIDRs. Liveness probes use SNMP before identification, so an IPP-only printer that does not answer SNMP can be missed by that path. Use authorized targets and matching read credentials.

## What does the simulator do?

`pm-sim` serves fixture-based SNMP and IPP responses, with optional IPPS and mutable configuration and firmware hooks. It lets developers exercise supported flows without relying on real printer hardware.

It is a test tool, not proof that another device implements the same behavior. Use a separate evaluation environment and the [simulator guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Vendor-plugins-and-drivers.md) for ports, fixtures, and firmware allow-list settings.

## Where can I deploy it?

Use Docker, Docker Compose, Kubernetes manifests, Helm, or a systemd-managed binary. The server includes the web interface and API; there is no separate frontend service to operate in the production package.

Choose a host with access to the printer networks, durable storage, and an HTTPS reverse proxy or ingress for remote browser access. The [getting-started page](/get-started) includes installation commands from the wiki.

## How can I contribute?

Start with the [repository](https://github.com/printmarshal/printmarshal) and [development guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Development.md). Drivers implement capability interfaces, keep vendor-specific behavior outside the core, and use sanitized fixtures. Store extensions must preserve the full persistence contract across the implemented backends.

Contribute concrete protocol evidence, parser fixes, driver implementations, documentation, or reproducible defect reports. Exercise write behavior in the simulator and only test physical devices you are authorized to manage. Keep secrets out of fixtures, logs, and reports.
