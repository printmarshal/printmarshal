# Security: assess → remediate → report (spec §8.5, §11)

The security feature lives in `internal/security` (service and probes),
`internal/security/enroll` (certificate enrollment clients), the
`SecurityStore` part of the store, and `internal/api/security.go`.

## Policy model

A **policy** is a named list of **policy items**. Each item names one canonical
setting key from `internal/model/settings_catalog.go` (the `Security: true` keys
are the ones a `read_security` document carries) and says what value is desired:

| Field | Meaning |
| --- | --- |
| `key` | Catalogue key, e.g. `protocols.telnet.enabled`, `security.tls.minVersion` |
| `operator` | `eq`, `ne`, `gte`, `lte`, `in`, `notIn`, `set`, `unset` |
| `value` | Desired value (a list for `in`/`notIn`; none for `set`/`unset`) |
| `severity` | `critical`, `high`, `medium`, `low` (weights 3 / 2 / 1 / 0.5) |
| `remediate` | `false` = assess-only (the item is never written) |
| `remediation` | Explicit `{key, value}` to write when the item fails; defaults to `key = value` for `eq` |
| `managementPath` | `snmp`, `ipp` or `http` when the remediation could cut the channel the device is managed through |

Operators compare loosely: JSON numbers, booleans and their string forms are
equal (`"true"` = `true`, `1.2` = `"1.2"`); `gte`/`lte` compare numerically when
both sides parse as numbers (so TLS `"1.3" >= "1.2"` works) and lexically
otherwise; strings compare case-insensitively. `set` passes when the setting is
present and non-empty/non-false/non-zero; `unset` is its negation. Secret-typed
settings (`security.adminPassword`, SNMP communities) are reported as
`<set>`/`<unset>` only.

A setting the device does not expose yields `unknown`, never a failure.

## Where the current values come from

For every device an assessment gathers one security document, earlier sources
winning per key:

1. every driver that implements `engine.SecurityReader` (`read_security`);
2. every `engine.ConfigReader` (`read_config`), restricted to the security keys;
3. every `engine.CertManager`'s certificate list → `security.cert.*`;
4. the **TLS probe** (`tlsprobe.go`): a TLS handshake on 443 and 631 offering
   every cipher suite Go knows (printers often support only RSA key exchange),
   then one handshake per protocol version (1.0, 1.1, 1.2, 1.3) to find the
   lowest the device still accepts → `security.tls.minVersion`, and the leaf
   certificate → `security.cert.identityPresent / identitySelfSigned /
   identityExpiresAt / identityDaysLeft`;
5. the **port probe** (`portprobe.go`): TCP connects to 21, 23, 80, 443, 515,
   631, 9100 → `protocols.<name>.enabled` as *observed* values, and one SNMP
   v2c GET of `sysDescr.0` with the configured read communities (falling back to
   `public`) → `protocols.snmp.v1v2cEnabled = true` when it answers. Silence over
   UDP is not reported as "disabled".

Probe-derived settings carry `source: "probe"`, are read-only, and only fill
keys no driver reported. Nothing in the gather step writes to a device.

## Scoring

`score = 100 × Σ weight(pass) / Σ weight(pass + fail)`, rounded to 0.1.
Unknown items are counted but excluded from the denominator; a device whose
every item is unknown scores 0 and is not compliant, because nothing was
verified. `compliant` is `failed == 0 && passed > 0`.

Each failing **critical** item raises an alert (source `security`, key
`policy:<policyId>:<itemId>`) on the device; the alert clears on the next
assessment that passes. Assessments are stored per device/policy/time and
feed `GET /api/compliance` (latest per device plus a fleet summary) and
`GET /api/compliance/history` (per-day assessed / compliant / average score).

## The built-in Baseline

Created on first run when no policy exists: SNMP v1/v2c off (high,
`managementPath: snmp`), SNMPv3 on, Telnet and FTP off (critical), HTTPS on,
HTTP→HTTPS redirect (`managementPath: http`), TLS ≥ 1.2 (`managementPath: http`),
admin password set (critical, assess-only), identity certificate present and
not expiring within 30 days (assess-only; renew by enrolling), raw port 9100
off (medium, **assess-only** because it breaks most print queues), WSD / SLP /
LLMNR off (low). Edit it like any policy.

## Remediation and the management-path rule (spec §11)

`POST /api/policies/{id}/remediate` runs as a job. **It is a dry run unless the
body says `"dryRun": false`.** Per device it re-assesses, then plans one change
per failing item that has `remediate: true`:

- The device's **management paths** are the distinct drivers in its capability
  profile: `snmp`, `ipp`, or `http` for any vendor plugin.
- A change whose `managementPath` is one of those paths removes that path for
  the rest of the batch. **If it is the last remaining path the change is
  skipped** with status `skipped` and a message naming the rule. So a device
  managed only over SNMP never has SNMP disabled, and a batch that would turn
  off both SNMP and IPP on a device managed through both keeps the second one.
- Changes go through `engine.SecurityWriter` drivers first, then
  `engine.ConfigWriter`, each setting offered to the next writer when one skips
  it. Per-item results are `written`, `would_write`, `unchanged`, `skipped` or
  `failed` with the driver name.
- Every real write is audited (`security.remediate`, actor, device, before /
  after per key; secrets redacted) and followed by a fresh assessment.

**Write credentials and verified outcomes (audit A19 / A20).** A real
remediation — like rotation and certificate installation — is handed only
credentials whose purpose is `write` or `both` (see "Credentials and purposes"
in `docs/configuration.md`); when none is bound to the device, its groups or
globally, every planned item is `failed` with "no write credential bound to
this device or its groups …", nothing is written, and the audit entry says
`refused`. The dry run keeps working with read credentials. After a real
write, the re-assessment is the read-back: each written item gets an explicit
`outcome` — `verified` when the policy item now passes, `diverged` (with
`readBack`) when the device accepted the change but still fails the item,
`unverifiable` when the setting is no longer readable, `written` when the
re-assessment itself failed — and the result counts `verified` / `diverged`
next to `written`. A remediation ticket should close on `verified`, not on
`written`.

`internal/security/service_test.go` and `purpose_test.go` have the safety
tests the spec asks for: dry run performs no write, the last management path
survives, secrets never appear in results, audit or logs, a device without a
write credential is never written to, and a device that accepts a write but
keeps the old value is reported as diverged.

## Certificates

`GET /api/devices/{id}/certificates` returns what drivers report
(`installed`), what the TLS probe saw (`observed`), and what PrintMarshal
issued (`issued`); add `?live=true` to probe now instead of reading the stored
observations. Observations are stored so `CheckExpiry` needs no network I/O; it
raises `cert:expiry` (warning ≤ 30 days, critical ≤ 7 days or expired) and runs
at the start of every assess job, and `cmd/printmarshal/main.go` also runs
`Security.CheckExpiry(ctx)` at startup and then every 24 hours.

### Enrollment

A **CA profile** (`POST /api/ca-profiles`, admin) names a protocol, a URL, an
optional credential, an optional `caFingerprint` (SHA-256 of the enrollment
server's TLS leaf or chain certificate, used instead of system roots — the
normal way to reach a private CA), a `keyType` (`rsa2048` default, `rsa3072`,
`ecdsa-p256`) and protocol `options`.

`POST /api/devices/{id}/certificates/enroll {caProfileId, use, commonName,
sans, dryRun}` runs the `security.enroll` job: generate the key and PKCS#10
CSR in Go, obtain the certificate, store it (certificate and chain in clear,
private key AES-256-GCM encrypted like every credential), and install it
through a driver implementing `engine.CertManager`. A dry run generates the
CSR, reports the subject/SANs and which driver would install, and contacts no
CA.

| Protocol | Client | Credential | Limits |
| --- | --- | --- | --- |
| **SCEP** (RFC 8894) | `enroll/scep.go`: GetCACert, GetCACaps, PKIOperation (PKCSReq, GetCertInitial polling while the CA answers PENDING), SCEP attributes, PKCS#7 via `smallstep/pkcs7` | `password` = challenge password, sent as the PKCS#9 `challengePassword` CSR attribute | RSA keys only (the protocol encrypts to RSA); AES-256-CBC when the CA advertises `AES`, else the library's DES-CBC (no 3DES encrypt in the library); renewal (`RenewalReq`) not implemented |
| **EST** (RFC 7030) | `enroll/est.go`: `/cacerts`, `/simpleenroll`, 202 + Retry-After polling, optional `options.label` | HTTP Basic (`username`/`password`) | TLS client-certificate auth (§3.3.2) and `/simplereenroll` not implemented; pin the server with `caFingerprint` for private CAs |
| **ACME** (RFC 8555) | `enroll/acme.go` on `golang.org/x/crypto/acme`: account, order, **dns-01** only, finalize, chain download | `pem` = account key (otherwise a fresh account per enrollment, which the job log says) ; `options.email` | Printers cannot answer http-01 / tls-alpn-01 (they own ports 80/443), so only DNS names can be ordered and IP SANs are refused. DNS is pluggable (`enroll.DNSProvider`); the built-in **manual** provider reports the `_acme-challenge` TXT record to create and, with `options.dnsWait` (e.g. `5m`), polls DNS until it appears. Without a wait the job fails with the record in its result (`dnsRecord`) |

**No standards driver can install a certificate**: SNMP and IPP have no such
operation. When no `CertManager` driver serves the device, the job still
succeeds in issuing, says so, and keeps the certificate retrievable under
`issued` so an operator can install it by hand through the device's web UI.
`POST /api/devices/{id}/certificates/{issuedId}/install` re-tries installation
later (for example once a vendor plugin is added). The private key is only
ever handed to a driver, never returned by the API.

## Credential rotation

`POST /api/security/rotate {kind: snmpCommunity | adminPassword, target,
dryRun}` (job `security.rotate`, dry run by default). Per device a new random
24-character secret is generated and **staged first**: it is encrypted and
stored as a device-scoped credential (audited as `staged`) with a priority
above the previous one, and only then written as `protocols.snmp.readCommunity`
or `security.adminPassword` through the security/config writers. A crash or
timeout between the device accepting the secret and it being stored can
therefore never lose the only copy. When the device cleanly refuses the write
the staged credential is deleted again; when the outcome is ambiguous
(timeout, reset, EOF) it is kept and the result says so, because the device may
hold the new secret; when the device accepts it, `ResolveCreds` picks the new
credential first (purpose `read` for a community, `both` for an admin
password) and a rotated community is verified by a read with it. The write
itself needs a write credential bound to the device; without one the device is
`failed` and nothing is staged. The secret never appears in job results,
events, the audit entry (`security.rotate` records kind, driver and credential
id) or logs; dry run reports which devices would change and stores nothing.

## Transport, sessions and trust (application hardening)

These controls came out of the enterprise integration audit (research folder,
findings A03–A06, A13, A18) and apply to PrintMarshal itself rather than to the
printers it manages.

| Control | Behaviour | Setting |
| --- | --- | --- |
| Session cookie | `HttpOnly`, `SameSite=Strict`; `Secure` when `auth.secureCookies` is on. The server warns at start-up when it listens on a non-loopback address without secure cookies. | `PM_SECURE_COOKIES=true` behind TLS |
| Tokens in URLs | Never accepted. Only the cookie or an `Authorization: Bearer` header carries a session. | — |
| Live stream origins | `/api/stream` accepts same-origin WebSocket upgrades only; additional hosts must be listed. | `auth.allowedOrigins` |
| Live stream lifetime | Every open stream re-validates its session once a minute and closes when the session was revoked or expired. | — |
| Password change | Ends every other session of the account and records `auth.session.revoked`. | — |
| Sign-in throttling | After 10 failed sign-ins from one address within a minute, further attempts get `429` for a minute; failures are recorded as `auth.failed`. | `auth.loginFailuresPerMinute` |
| Authorization denials | A `403` from the role check is recorded as `auth.denied` with user, method and path. | — |
| Audit writes | A failed audit write is logged at error level with the full action; every audit entry is also published as `audit.recorded` on the event bus for exporters. | — |
| Initial admin password | When `PM_ADMIN_PASSWORD` is unset the generated password is written to `<dataDir>/initial-admin-password` (mode 0600) instead of the log. Delete the file after the first login. | `PM_ADMIN_PASSWORD` |
| Trap trust | Traps are acted on only when they come from the address of a known device; `trap.allowedSources` (CIDRs) narrows the accepted sources further and `trap.ratePerMinute` (default 60) caps traps per source. Drops are logged at most once per source per minute. | `trap.allowedSources`, `trap.ratePerMinute` |
| Readiness and metrics | `GET /readyz` checks the database; `GET /metrics` (viewer) serves Prometheus text with device, alert, job, outbox and HTTP counters. | — |

TLS termination is expected at a reverse proxy or ingress (see
`docs/operations.md`); the proxy must forward the `Host` header unchanged so
the same-origin check on the stream keeps working. Client addresses used for
throttling are the TCP peer, so a proxy should apply its own sign-in rate limit.

### Device writes over IPP

These controls came out of the external code review (findings H-016, H-017) and
apply to the IPP driver's write path (`Set-Printer-Attributes`, `internal/driver/ipp`).
Reads are unchanged.

What a write now guarantees:

- **TLS preferred.** The write and the read that precedes it try the TLS form of
  every configured endpoint (`ipps` on 631, `https` on 443) before any plaintext
  form, and every request of one write goes to the endpoint that answered the read.
  With `poll.ippWriteRequireTLS` (`PM_IPP_WRITE_REQUIRE_TLS=true`) plaintext is
  refused outright and the write fails with a message naming the option.
- **Certificate pinning, trust on first use.** On the first successful TLS contact
  of a write the SHA-256 fingerprint of the leaf certificate the device presented
  is pinned for that device. A later write that sees a different certificate is
  refused before anything is sent, with an error naming both fingerprints; a later
  write that would fall back to plaintext while a pin exists is refused as a
  downgrade. Reads keep working either way.
- **Credentials go only to the challenger.** The bound admin credential is sent
  only in answer to a `401` challenge, once, to the host that was asked (Basic, or
  Digest with MD5 / SHA-256 and `qop=auth`); redirects are never followed on the
  write path, so a credential cannot be bounced to another host. A device that
  challenges while no admin credential is bound fails the setting with a clear
  reason and nothing is written.

What it does not guarantee:

- `poll.ippInsecureTls` (default `true`) still means **no CA validation**: the
  device's certificate is not checked against any trust store, on reads or writes.
  Pinning only detects a *change* after the first contact; the first contact
  itself is trusted, so an attacker already in the path before PrintMarshal ever
  wrote to the device is not detected.
- The pin lives **in memory for the process lifetime** (the device model has no
  field for it and persisting one would need a schema change). A restart re-pins
  on the next write; a planned certificate change on a printer therefore needs a
  PrintMarshal restart, and an unplanned one is only noticed between restarts.
- Plaintext writes are still possible when the device offers no TLS endpoint and
  `ippWriteRequireTLS` is off; the credential then travels in Basic (base64) or,
  when the device offers it, Digest form.

## API summary

| Method and path | Role | Result |
| --- | --- | --- |
| `GET /api/policies`, `GET /api/policies/{id}` | viewer | policies |
| `POST /api/policies`, `PATCH /api/policies/{id}`, `DELETE /api/policies/{id}` | operator | validated policy |
| `POST /api/policies/{id}/assess` `{deviceIds or groupId}` | operator | 202 job `security.assess` |
| `POST /api/policies/{id}/remediate` `{deviceIds or groupId, dryRun}` | operator | 202 job `security.remediate` (dry run unless `dryRun:false`) |
| `GET /api/compliance?policy=&group=&device=` | viewer | latest assessments + summary |
| `GET /api/compliance/history?policy=&from=&to=` | viewer | per-day points |
| `GET /api/devices/{id}/security` | viewer | latest assessments, stored certificates, security alerts |
| `GET /api/devices/{id}/certificates[?live=true]` | viewer | installed / observed / issued |
| `POST /api/devices/{id}/certificates/enroll` | operator | 202 job `security.enroll` |
| `POST /api/devices/{id}/certificates/{issuedId}/install` | operator | 202 job `security.install` |
| `GET /api/ca-profiles` | viewer | profiles (no secrets; credentials are referenced by id) |
| `POST /api/ca-profiles`, `DELETE /api/ca-profiles/{id}` | admin | profile |
| `POST /api/security/rotate` | operator | 202 job `security.rotate` |
| `GET /api/security/catalog` | viewer | security keys, operators, severities, management paths |

Jobs report per-device results under `GET /api/jobs/{id}`; live updates arrive
on the WebSocket stream as `job.updated`, `security.assessed`,
`security.remediated`, `security.rotated`, `security.certificate.issued` /
`.installed` and the usual `alert.raised` / `alert.cleared`.
