# PrintMarshal — Security Assessment & Platform Architecture (`site/security/index.html`)

## Part I: Printer Security Assessment (What You Investigate on Devices)
- **Policy evaluation & unknown-evidence rule:** 14-item built-in `Baseline` policy and 17-item security catalog. Unreadable settings evaluate to `unknown` and are excluded from `score = 100 × passedWeight / (passedWeight + failedWeight)`.
- **Management-path lockout guard:** Flags rules that could sever `snmp` or `http` connectivity and refuses to disable the last management path during remediation.
- **Certificate inventory & CA enrollment:** TLS handshake probe on `443` and `631`, expiry alerts at `30 / 14 / 7` days, and enrollment via `SCEP`, `EST`, and `ACME DNS-01` (manual DNS TXT challenge). Private keys are encrypted at rest and never downloadable via API. Installing an issued certificate requires a `CertManager` driver (none shipped for real vendors).
- **Staged credential rotation:** 4-stage workflow (`staged → device_written → verified → committed` or `rolled_back`). Writing to a printer requires a `SecurityWriter` driver (none shipped for real vendors).

## Part II: Platform Security Architecture (How PrintMarshal Protects Itself)
- **Credential vault & purpose binding:** `AES-256-GCM` encryption at rest (`PM_SECRET_KEY`) with `read`, `write`, and `both` credential purposes and write-only secret APIs.
- **Authentication, RBAC & CSRF:** Local `bcrypt`, `OIDC` (Authorization Code + PKCE), `LDAP`, SHA-256 hashed `pmk_…` API keys, `admin / operator / viewer` roles, and `Sec-Fetch-Site` + `Origin` CSRF validation on `pm_session` cookies.
- **Write gates & read-back verification:** Configuration, remediation, and rotation endpoints default to `dryRun: true` (except certificate install `POST /api/devices/{id}/certificates/{certId}/install` and print-provider queue delete `DELETE /api/print/providers/{id}/queue/{jobId}`, which execute immediately).
- **IPP TLS TOFU pinning & SIEM export:** Pins IPPS leaf SHA-256 fingerprints on first connection (`tls_pin_mismatch` alert on change) and exports audit records over RFC 5424 Syslog or CEF.
