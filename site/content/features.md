---
title: "PrintMarshal features"
description: "Discovery, monitoring, stock, configuration, security assessment, firmware planning, and identity controls for a self-hosted printer fleet."
---

## Discovery and inventory

Find candidate devices through mDNS/DNS-SD, WS-Discovery, or active scans of addresses, ranges, and IPv4 CIDRs. Active scanning uses SNMP liveness probes and caps large expansions unless an operator explicitly allows them. Multicast discovery needs suitable network access.

Identification combines standards and vendor observations into a device record with a capability profile. Reconciliation checks serial, UUID, MAC, then address; conflicting stable identity prevents an address-only merge. Organize devices with static or rule groups, lifecycle states, asset tags, and notes. Automated scans preserve operator-owned fields. [Discovery guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Discovery-and-inventory.md).

## Monitoring and alerts

Schedule polls or request a fresh read of status, supplies, trays, counters, and firmware. Device capabilities determine what can be read. Partial failures remain visible: a successful counter read does not prove that status or supplies were refreshed.

Alerts cover unreachable devices, measured supply thresholds, device-reported faults, paper availability, and spare-stock levels. SNMP traps can trigger alerts and follow-up polls. Noise rules account for alternative trays and measured supply levels; acknowledgments and clear events keep the alert history explicit. [Monitoring guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Monitoring-and-alerts.md).

## Consumables and stock

Track spare toner, ink, drums, fusers, maintenance kits, waste containers, and staples. Link stock items to installed supplies through automatic matching or a manual choice. Record deliveries and adjustments with stock history, and exchange inventory through CSV.

Replacement detection can log a change, request confirmation, or decrement stock automatically; confirmation is the default. Forecasts use observed consumption and return an unknown result until enough history exists. Cost and reorder fields are references, not purchasing or billing features. [Stock guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Consumables-and-stock.md).

## Configuration management

Read a normalized settings document assembled from applicable drivers. Keep snapshots, compare devices, define desired profiles, and inspect drift. The interface distinguishes writable settings from observations that a driver can only read.

Apply and revert run as jobs and default to dry run. Real changes require write-purpose credentials and an implemented write capability. Standards SNMP and IPP support a limited setting set; results distinguish accepted changes from verified, diverged, or unverifiable outcomes. Revert uses recorded prior values where available. [Configuration guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Configuration-management.md).

## Security and certificates

Assess desired security settings against driver observations, TLS handshakes, and port probes. Missing evidence stays unknown. Policies produce severity-weighted scores and critical-failure alerts; assessments and history are available separately from the placeholder compliance report.

Enroll certificates through SCEP or EST, retain encrypted private keys, and monitor observed certificate expiry. ACME is also implemented for DNS-01 with manual DNS publication. Enrollment is separate from installation: no shipped real-vendor certificate installer or security writer exists, and automatic certificate renewal is not implemented. [Security model](/security).

## Firmware campaigns

Inventory installed versions and compare them with compatible registered packages. Campaigns provide canary waves, confirmation gates, maintenance windows, concurrency limits, failure-threshold pauses, dry runs, and post-delivery version checks. Package hashes are checked when images are used.

The shipped delivery driver targets the simulator or its explicit allow-list; real printers need an additional vendor driver. Interrupted campaigns recover to a reviewable paused state, with interrupted device attempts marked failed. This is a single-active-process workflow, not distributed firmware execution. [Firmware guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Firmware.md).

## Identity and access

Use local accounts or configure OIDC, SAML, LDAP/Active Directory, and RADIUS PAP. External identities bind by issuer and subject rather than matching an existing username. Viewer, operator, and admin roles separate observation, daily operations, and administration.

Issue revocable API tokens with a role, optional expiry, and optional group scope. Scope checks cover device-targeted routes and related resources, but several collections, reports, metrics, jobs, and live events remain fleet-wide. Group scope is not a tenant-isolation boundary. [Identity guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Identity-and-access.md).

## Events and integrations

Route notifications to Slack, Teams, Discord, generic webhooks, or SMTP. Enqueued notifications survive restarts, retry delivery, and expose dead letters for inspection and requeue. The event bus also feeds the live interface and security-event export.

Export eligible security events, alerts, and dead notifications to syslog, JSON-lines files, or an HTTP collector. Receive printer syslog, synchronize current records through cursor-based APIs, and scrape authenticated Prometheus metrics. SIEM export uses a bounded in-memory queue and can drop under overload; it does not share the notification outbox's durability. [Integration guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Events-notifications-and-integrations.md).

## Reports and evidence

Review utilization, fleet health, supply consumption, alert activity, and firmware distribution over selected ranges. Filter reports by group or device and export tables as CSV. The dedicated compliance report remains a placeholder; security assessment summaries and history are implemented separately.

The research catalog keeps documented management routes, observed driver capabilities, and unknowns distinct. Operators can record lab or production qualification without turning research coverage into an automatic support claim. A catalog match does not enable a device writer. [Driver and catalog guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Vendor-plugins-and-drivers.md).

## Optional print provider

The optional add-on manages local CUPS queues on a Linux host. Preview a queue plan, run an audited sync job, or remove a managed queue. It is disabled by default and configures the host's CUPS service rather than changing printer settings.

This is a queue-provisioning convenience. Print accounting, quotas, cost allocation, secure release, and pull printing remain outside the core product. [Print-provider guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Print-provider.md).

## Deployment and development

Run the embedded web interface, API, engine, scheduler, and workers in one server process. Choose SQLite for the default local store or PostgreSQL for external persistence. Docker, Compose, Kubernetes, Helm, and systemd packaging are included; one active server is the supported model.

Extend the capability interfaces with compiled-in drivers or implement the full store contract for a new backend. The simulator and sanitized protocol fixtures let developers exercise supported flows without relying on printer hardware. [Get started](/get-started) or [read the development guide](https://github.com/printmarshal/printmarshal/blob/main/wiki/Development.md).
