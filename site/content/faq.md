# Technical FAQ (`site/faq/index.html`)

## H1: Direct answers on protocol scope and boundaries.

1. **Does discovery or polling ever write to my printers?**
   - No. Active CIDR/IP scans, passive `mDNS / DNS-SD` and `WS-Discovery` listeners, and scheduled polling are strictly read-only.

2. **Which printer brands and models work today?**
   - Any printer supporting standard SNMP v1/v2c/v3 (`RFC 1213`, `RFC 2790`, `RFC 3805`) or IPP/IPPS works out of the box for discovery, monitoring, and port/TLS probing. The 15-vendor, 495-model enterprise research catalog ships with `0 physically validated` until an operator records lab qualification.

3. **Why do some Baseline security policy checks show `Unknown`?**
   - Standard SNMP and IPP do not expose every vendor-specific setting across the 84-key canonical settings catalog (43 security-tagged keys). Unreadable keys are marked `unknown` and excluded from the compliance score denominator (`internal/security/policy.go`).

4. **What is the difference between `accepted` and `verified` configuration writes?**
   - `verified` (`verified: true`) means a read-back poll confirmed the new value on the printer. `accepted` (`verified: false`) means the write call succeeded without error, but the key is write-only or could not be confirmed on read-back.

5. **Can PrintMarshal enroll and install TLS certificates on printers?**
   - PrintMarshal generates RSA-2048 CSRs, enrolls via internal CA, EST, or SCEP, and alerts at `≤ 30`d warning / `≤ 7`d critical (`internal/security/certs.go`). Physical installation requires a vendor `driver.CertificateInstaller` or manual upload via the printer web UI.

6. **Does PrintMarshal push firmware updates to physical printers?**
   - Out of the box, HTTP firmware delivery (`POST /fw-sim`) targets loopback `pm-sim` instances only. Physical printer firmware delivery requires implementing `driver.FirmwareDriver`.

7. **Is PrintMarshal a print server, pull-print queue, or print accounting tool?**
   - No. PrintMarshal manages devices, not print jobs; print accounting, quotas, cost allocation, secure release, and pull printing are outside the product (`README.md`, `docs/print-provider.md`, `internal/printprovider/settings.go`).
   - An optional, off-by-default local CUPS queue add-on (`internal/printprovider/`) can publish IPP Everywhere URIs (`ipp-everywhere-list`) or reconcile local Linux host CUPS queues (`cups` mode with `lpadmin`/`lpstat`/`cupsenable`/`cupsaccept` and `pm-` prefix, default `dryRun: true`) for discovered IPP printers without modifying any printer.

8. **How are printer credentials and encryption keys protected?**
   - Encrypted at rest with AES-256-GCM (`PM_SECRET_KEY`, generated via `openssl rand -base64 32` or stored in `<PM_DATA_DIR>/secret.key`). Preserve `PM_SECRET_KEY` across restarts and back it up separately from database snapshots.
