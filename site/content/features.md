# Features & Capability Matrix (`site/features/index.html`)

## H1: What works with your printers today.

1. **Discover printers and reconcile identity (`#discovery`)**
   - Active CIDR/IP scans + passive `mDNS / DNS-SD` and `WS-Discovery`.
   - Reconciles by `serial → UUID → lowercased MAC → IP address` (`internal/store/sqlite_devices.go`) with `IdentityConflicts` protection when another printer reuses an IP.
   - Operator fields (`displayName`, `location`, `assetTag`, `notes`) are never overwritten by polls.

2. **Monitor health, supplies, counters, and SNMP traps (`#monitoring`)**
   - RFC 3805 Printer-MIB, RFC 2790 Host-Resources-MIB, IPP attributes, and rate-limited UDP SNMP traps (`1162/udp`).
   - 30-day supply sparklines and spare stock planning (`GET /api/consumables/plan`).

3. **Audit security posture, certificates, and credentials (`#security-compliance`)**
   - 14-item Baseline policy + custom policies across the 84-key canonical settings catalog (43 security-tagged keys), 8 operators (`eq`, `ne`, `gte`, `lte`, `in`, `notIn`, `set`, `unset`), weights `critical = 3`, `high = 2`, `medium = 1`, `low = 0.5`.
   - RSA-2048 CSR generation and enrollment via internal CA, EST, or SCEP; expiry alerts at `≤ 30` days warning and `≤ 7` days critical (`internal/security/certs.go`). Physical installation requires `driver.CertificateInstaller` or the printer web UI.
   - Credential rotation (`internal/security/rotate.go`) for `snmpCommunity` and `adminPassword`.

4. **Preview configuration diffs, verify writes, and detect drift (`#configuration`)**
   - 84-key catalog across 10 areas (`identity`, `network`, `protocols`, `security`, `energy`, `trays`, `panel`, `print`, `scan`, `other`).
   - Dry-run preview before writing, read-back verification (`verified` vs `accepted`), and rollback via saved `prevValues`.
   - Standard writes cover MIB-II `sysName`, `sysLocation`, `sysContact` and writable IPP attributes (with TLS leaf certificate SHA-256 fingerprint pinning).

5. **Stage firmware and inspect the 495-model catalog (`#firmware`)**
   - SHA-256 verified firmware repository and loopback `pm-sim` delivery (`POST /fw-sim`). Physical printer firmware delivery requires `driver.FirmwareDriver`.
   - 15-vendor, 126-family, 495-model enterprise research catalog (`0` physically validated out of the box).

6. **Reports, RBAC, SIEM export, and local CUPS queues (`#platform`)**
   - RBAC (`admin`, `operator`, `viewer`) with local `bcrypt`, OIDC + PKCE, SAML 2.0, LDAP/AD, RADIUS PAP, and SHA-256 hashed API tokens.
   - JSON/CSV reports (`utilisation`, `fleet-health`, `supplies`, `alerts`, `firmware`; `compliance` is a placeholder and live compliance is at `/api/security/compliance`), append-only `audit_log`, and RFC 5424 syslog / JSON-lines / HTTP SIEM export.
   - Optional local CUPS queue add-on (`internal/printprovider/`, `docs/print-provider.md`): off by default (`enabled: false`), reconciles local Linux host CUPS queues (`pm-` prefix) or publishes IPP Everywhere URIs (`ipp-everywhere-list`) for discovered IPP printers; never modifies printers; no job accounting, quotas, or pull-print release.
