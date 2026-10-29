# Security Architecture & Compliance Boundaries (`site/security/index.html`)

## H1: Audit printer security. Protect management credentials.

1. **The 14-item Baseline policy and unknown-state scoring (`#baseline`)**
   - Evaluates 14 default rules across the 84-key canonical settings catalog (43 security-tagged keys in `internal/model/settings_catalog.go`).
   - Unreadable settings return `status: "unknown"` and are excluded from the compliance score denominator (`internal/security/policy.go`).

2. **Port probes, TLS inspection, and certificate enrollment (`#probes-certs`)**
   - `security/portprobe` checks TCP ports `21, 23, 80, 443, 515, 631, 9100`.
   - `security/tlsprobe` inspects TLS handshakes and leaf certificates on `443` and `631`.
   - Enrolls certificates via internal CA, EST (`/.well-known/est/simpleenroll`), or SCEP with warning alerts at `≤ 30` days and critical alerts at `≤ 7` days (`internal/security/certs.go`). Physical installation requires `driver.CertificateInstaller` or the printer web UI.

3. **Preview every change and verify writes (`#write-safeguards`)**
   - Dry-run preview supported on profile applies, ad-hoc writes, security remediations, and local CUPS queue syncs.
   - Read-back verification (`internal/configmgmt/service.go`) separates `verified` writes from `accepted` (`verified: false`) writes.
   - IPP write-path TLS leaf fingerprint pinning (`internal/driver/ipp/`) aborts writes on `ErrPinMismatch`.
   - Credential rotation (`internal/security/rotate.go`) stages encrypted `snmpCommunity` / `adminPassword` secrets before writing, deletes on clean refusal, keeps with a warning on ambiguous network errors, and verifies `snmpCommunity` via read poll.
   - Two direct write endpoints without dry-run preview: `POST /api/devices/{id}/certificates/{issuedId}/install` and `DELETE /api/print-provider/queues/{deviceId}`.

4. **Credential encryption, RBAC, and SIEM export (`#platform`)**
   - AES-256-GCM encryption (`PM_SECRET_KEY`, `internal/secrets/secrets.go`) for device credentials and CA/CSR keys, with redaction in API responses and snapshots.
   - Authentication via local `bcrypt`, OIDC + PKCE, SAML 2.0, LDAP/AD, RADIUS PAP, and SHA-256 hashed API tokens (`pm_...`).
   - Append-only `audit_log` with real-time forwarding to RFC 5424 syslog (`udp`/`tcp`/`tls`), JSON-lines files, or HTTP SIEM collectors.
   - Static Go binary (`golang:1.27-alpine`) running on `alpine:3.23` as user `pm` (`UID 10001`).
