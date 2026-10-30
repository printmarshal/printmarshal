# Configuration reference

[Home](Home.md) · [Deployment](Installation-and-deployment.md) · [Identity and access](Identity-and-access.md)

The JSON loader is `internal/config/config.go`. Precedence is built-in defaults → optional JSON file → listed nonempty environment variables. `-config FILE` overrides the `PM_CONFIG` default for the flag. Most file changes require restart. Database-backed runtime settings overlay selected defaults separately (below).

Every leaf field in the current config structs is listed here. `—` in Environment means **no direct environment override** exists; it does not mean invent a `PM_` spelling from the JSON key. Secret-reference fields name an environment variable that you supply, rather than containing the secret. Unset collections are nil/empty, strings empty, booleans false and numbers/durations zero unless a default or effective fallback is shown.

Duration strings use Go duration syntax: `30s`, `5m`, `720h`; `30d` is not supported. JSON numbers mean whole seconds (numeric conversion truncates before multiplying). Environment booleans accept `1`, `true`, `yes` case-insensitively; any other nonempty value means false. Invalid integer/duration environment values are silently ignored. Empty env strings do not clear a configured string. List overrides are comma-split, not shell/JSON lists. JSON loading uses `json.Unmarshal`, so unknown config keys are ignored; this differs from strict API JSON body decoding.


## listen

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `listen` | `string` | `PM_LISTEN` | :8080 | HTTP bind address; the server does not terminate TLS. |

## dataDir

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `dataDir` | `string` | `PM_DATA_DIR` | ./data | Key, initial password file, generated SAML pair and firmware root; SQLite default DB location. |

## database

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `database` | `string` | `PM_DATABASE` | sqlite | Backend selector: sqlite or postgres; empty also selects sqlite. |

## dsn

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `dsn` | `string` | `PM_DSN` | <dataDir>/printmarshal.db (also applied if database=postgres and no DSN; set PM_DSN) | SQLite path or pgx PostgreSQL connection string; not auto-converted between backends. |

## poll

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `poll.snmpPort` | `int` | `PM_SNMP_PORT` | 161 | Destination SNMP agent port. |
| `poll.ippEndpoints` | `[]string` | `PM_IPP_ENDPOINTS` | empty; built-in endpoint list below | Read endpoint templates in order; %s is the device host. Writes derive TLS-first candidates. |
| `poll.interval` | `Duration` | `PM_POLL_INTERVAL` | 5m | Scheduled polling cadence; minimum scheduler interval 30s. |
| `poll.concurrency` | `int` | `PM_POLL_CONCURRENCY` | 32 | Engine limiter capacity and per-job device worker limit, fixed from startup config. |
| `poll.snmpTimeout` | `Duration` | — | 2s | SNMP request timeout. |
| `poll.snmpRetries` | `int` | — | 1 | Retries after an SNMP request timeout. |
| `poll.ippTimeout` | `Duration` | — | 4s | HTTP request timeout for IPP and eSCL. |
| `poll.ippInsecureTls` | `bool` | — | true | Skip normal certificate verification on IPP/eSCL; IPP writes additionally use in-memory first-contact pinning. |
| `poll.ippWriteRequireTLS` | `bool` | `PM_IPP_WRITE_REQUIRE_TLS` | false | IPPWriteRequireTLS refuses to send Set-Printer-Attributes over plaintext; writes then need an IPPS (or HTTPS) endpoint on the device. |

## discovery

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `discovery.enabled` | `bool` | `PM_DISCOVERY_ENABLED` | false | Enable scheduled discovery; does not block manual discovery jobs. |
| `discovery.interval` | `Duration` | `PM_DISCOVERY_INTERVAL` | 1h | Scheduled discovery cadence; scheduler minimum 5m. |
| `discovery.mdnsWait` | `Duration` | — | 5s | DNS-SD response collection duration. |
| `discovery.wsdWait` | `Duration` | — | 3s | WS-Discovery response collection duration. |
| `discovery.scanWorkers` | `int` | — | 64 | Concurrent active-scan liveness probes. |
| `discovery.maxAddresses` | `int` | — | 4096 | Scan expansion cap unless the request sets allowLarge. |
| `discovery.targets` | `[]string` | `PM_DISCOVERY_TARGETS` | empty | Default scheduled active-scan IPs, ranges or IPv4 CIDRs. |
| `discovery.reverseDns.enabled` | `bool` | `PM_REVERSE_DNS_ENABLED` | false | Run PTR lookups to fill `Device.DNSName` (`dnsName`); off by default and never overwrites `Device.Hostname`. This and the five options below are defaults for the runtime `reverseDns` settings (Settings → Polling & discovery, `PATCH /api/settings`), which apply without a restart. |
| `discovery.reverseDns.server` | `string` | `PM_REVERSE_DNS_SERVER` | empty → system resolver | Custom DNS server (`host` or `host:port`, port 53 added if omitted). |
| `discovery.reverseDns.shortNames` | `bool` | `PM_REVERSE_DNS_SHORT_NAMES` | true | Store only the first DNS label (strip domain suffix). |
| `discovery.reverseDns.timeout` | `Duration` | `PM_REVERSE_DNS_TIMEOUT` | 1s | Context timeout per reverse/forward lookup. |
| `discovery.reverseDns.refreshInterval` | `Duration` | `PM_REVERSE_DNS_REFRESH_INTERVAL` | 24h | Minimum interval between automatic refreshes on successful polls. |
| `discovery.reverseDns.forwardConfirm` | `bool` | `PM_REVERSE_DNS_FORWARD_CONFIRM` | true | Resolve the PTR name back and accept it only when an answer matches the device's address. |

## trap

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `trap.enabled` | `bool` | `PM_TRAP_ENABLED` | true | Start the SNMP trap listener. |
| `trap.listen` | `string` | `PM_TRAP_LISTEN` | 0.0.0.0:1162 | UDP bind address; ports below 1024 need suitable privilege. |
| `trap.allowedSources` | `[]string` | — | empty | AllowedSources are CIDRs (or single addresses) traps are accepted from. Empty = any source, but a trap is only acted on when it comes from the address of a known device. |
| `trap.ratePerMinute` | `int` | — | 0 → effective 60 | RatePerMinute caps traps handled per source address (0 = default 60). |

## alerts

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `alerts.supplyWarningPercent` | `int` | — | 15 | Warning when consumed supply remaining percentage is at/below this, or waste remaining capacity is at/below this. |
| `alerts.supplyCriticalPercent` | `int` | — | 5 | Critical supply remaining/waste free-capacity threshold. |
| `alerts.offlineAfterFailures` | `int` | — | 2 | Consecutive unreachable polls before the offline alert. |

## notifiers[]

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `notifiers[].name` | `string` | — | empty | Destination name used for subscription, outbox routing and test path. |
| `notifiers[].type` | `string` | — | empty → generic webhook | webhook \| slack \| teams \| discord \| smtp |
| `notifiers[].url` | `string` | — | empty | Webhook target for webhook/slack/teams/discord. |
| `notifiers[].events` | `[]string` | — | empty → all alert.* events | alert.raised, alert.cleared, job.finished, device.discovered; empty = all alert events |
| `notifiers[].host` | `string` | — | empty | SMTP server hostname. |
| `notifiers[].port` | `int` | — | 0 → 587 | See the port behavior in the linked feature guide. |
| `notifiers[].from` | `string` | — | empty | SMTP envelope sender and From header. |
| `notifiers[].to` | `[]string` | — | empty | SMTP envelope recipients and To header. |
| `notifiers[].username` | `string` | — | empty | SMTP login/mailbox; empty skips AUTH. |
| `notifiers[].passwordEnv` | `string` | — | empty | name of env var holding the SMTP password |
| `notifiers[].startTls` | `bool` | — | false | Explicit SMTP STARTTLS; initial socket is plain SMTP, not implicit TLS on 465. |
| `notifiers[].authMethod` | `string` | — | empty → plain | AuthMethod selects the SASL mechanism: "plain" (default), "login" (Exchange/ older relays) or "xoauth2" (Microsoft 365, Google Workspace). For xoauth2 the bearer token comes from TokenEnv, or is fetched with the OAuth 2.0 client-credentials grant when TokenURL is set. |
| `notifiers[].tokenEnv` | `string` | — | empty | env var holding a ready bearer token |
| `notifiers[].tokenUrl` | `string` | — | empty | e.g. https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token |
| `notifiers[].clientId` | `string` | — | empty | See the clientId behavior in the linked feature guide. |
| `notifiers[].clientSecretEnv` | `string` | — | empty | env var holding the OAuth client secret |
| `notifiers[].scope` | `string` | — | empty | e.g. https://outlook.office365.com/.default |
| `notifiers[].legacy` | `bool` | — | false | Legacy switches the Teams payload back to the retired Office 365 connector MessageCard format; the default is an Adaptive Card for Workflows webhooks. |

## jobs

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `jobs.workers` | `int` | `PM_JOB_WORKERS` | 4 | Number of job workers; nonpositive values select 4 in Runner.New. |

## auth

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `auth.sessionTtl` | `Duration` | — | 720h (30 days) | Session expiry/cookie lifetime, evaluated when sessions are issued. |
| `auth.secureCookies` | `bool` | `PM_SECURE_COOKIES` | false | Set Secure on session/flow cookies; SAML flow also switches to SameSite=None. |
| `auth.allowedOrigins` | `[]string` | — | empty | AllowedOrigins are extra Origin host patterns accepted on the WebSocket stream (e.g. "console.example.com", "*.corp.example"). Same-origin is always accepted; nothing else is unless listed here. |
| `auth.loginFailuresPerMinute` | `int` | — | 0 → effective 10 | LoginFailuresPerMinute per source IP before sign-in is rate limited (0 = default 10). |
| `auth.oidc.issuer` | `string` | `PM_OIDC_ISSUER` | empty | OIDC discovery issuer; requires clientId to advertise the provider. |
| `auth.oidc.clientId` | `string` | `PM_OIDC_CLIENT_ID` | empty | OIDC client/audience identifier. |
| `auth.oidc.clientSecretEnv` | `string` | `PM_OIDC_CLIENT_SECRET_ENV` | empty | Name of environment variable containing client secret; empty for a public client. |
| `auth.oidc.redirectUrl` | `string` | `PM_OIDC_REDIRECT_URL` | empty | Exact registered callback URL. |
| `auth.oidc.displayName` | `string` | — | empty → issuer host | login button label; defaults to the issuer host |
| `auth.oidc.scopes` | `[]string` | — | empty → profile, email (openid always included) | Scopes requested besides "openid" (default: profile, email). |
| `auth.oidc.roleClaim` | `string` | — | empty | RoleClaim names the ID-token claim (dot path allowed, e.g. "realm_access.roles") whose values are looked up in RoleMap; the highest mapped role wins. |
| `auth.oidc.roleMap` | `map[string]string` | — | empty | claim value -> viewer \| operator \| admin |
| `auth.oidc.defaultRole` | `string` | — | empty → viewer | DefaultRole is used when no RoleMap entry matches (default viewer). |
| `auth.oidc.allowedDomains` | `[]string` | — | empty | AllowedDomains restricts sign-in to these email domains (empty = any). |
| `auth.ldap.url` | `string` | `PM_LDAP_URL` | empty | ldap:// or ldaps:// server; nonempty enables password-provider LDAP. |
| `auth.ldap.startTls` | `bool` | `PM_LDAP_START_TLS` | false | Upgrade ldap:// before binding (ignored for ldaps://). |
| `auth.ldap.caFile` | `string` | `PM_LDAP_CA_FILE` | empty | PEM root bundle; system roots if empty. |
| `auth.ldap.insecureSkipVerify` | `bool` | — | false | skip server certificate verification (lab use only) |
| `auth.ldap.bindDN` | `string` | `PM_LDAP_BIND_DN` | empty | BindDN / BindPasswordEnv are the optional read-only service account used to find the user's entry. Without them PrintMarshal binds directly as the user, using UserDNTemplate to build the bind name. |
| `auth.ldap.bindPasswordEnv` | `string` | `PM_LDAP_BIND_PASSWORD_ENV` | empty | env var holding the service-account password (never the password itself) |
| `auth.ldap.userDNTemplate` | `string` | `PM_LDAP_USER_DN_TEMPLATE` | empty | UserDNTemplate is used when BindDN is empty: "uid=%s,ou=people,dc=example,dc=com" or an AD UPN such as "%s@corp.example.com". |
| `auth.ldap.baseDN` | `string` | `PM_LDAP_BASE_DN` | empty | search base for the user entry |
| `auth.ldap.userFilter` | `string` | `PM_LDAP_USER_FILTER` | empty → (\|(sAMAccountName=%s)(uid=%s)(userPrincipalName=%s)(mail=%s)) | UserFilter finds the entry; every %s is replaced by the RFC 4515-escaped login name. Default (\|(sAMAccountName=%s)(uid=%s)(userPrincipalName=%s)(mail=%s)). |
| `auth.ldap.usernameAttr` | `string` | — | empty → matched login attribute, sAMAccountName, uid, typed direct-bind name, DN | UsernameAttr names the attribute that becomes the local username (default: the attribute that matched, then sAMAccountName, uid, the DN). |
| `auth.ldap.subjectAttr` | `string` | — | empty → objectGUID, entryUUID, DN | SubjectAttr is the stable identifier the account is bound to (default objectGUID, then entryUUID, then the DN). |
| `auth.ldap.groupAttr` | `string` | — | empty → memberOf | GroupAttr lists the user's groups (default memberOf). |
| `auth.ldap.roleMap` | `map[string]string` | — | empty | RoleMap maps a group DN or CN to viewer \| operator \| admin; the highest mapped role wins and is re-applied on every sign-in. |
| `auth.ldap.defaultRole` | `string` | — | empty → viewer | when no RoleMap entry matches (default viewer) |
| `auth.ldap.allowedGroups` | `[]string` | — | empty | AllowedGroups, when set, refuses users in none of these groups (DN or CN). |
| `auth.ldap.timeout` | `Duration` | — | 0 → 10s | per connection (default 10s) |
| `auth.ldap.displayName` | `string` | — | empty → LDAP server host | shown on the login page; defaults to the server host |
| `auth.radius.servers` | `[]string` | `PM_RADIUS_SERVERS` | empty | Ordered RADIUS UDP host:port list; nonempty entry enables provider. |
| `auth.radius.secretEnv` | `string` | `PM_RADIUS_SECRET_ENV` | empty | Name of environment variable holding required RADIUS shared secret. |
| `auth.radius.timeout` | `Duration` | — | 0 → 3s | per attempt (default 3s) |
| `auth.radius.retries` | `int` | — | 0 → 2 retransmissions (negative → none) | retransmissions per server (default 2) |
| `auth.radius.nasIdentifier` | `string` | `PM_RADIUS_NAS_IDENTIFIER` | empty → printmarshal | NAS-Identifier attribute (default "printmarshal") |
| `auth.radius.roleAttribute` | `string` | — | empty → Filter-Id | RoleAttribute is the reply attribute whose values are looked up in RoleMap: "Filter-Id" (default) or "Class". |
| `auth.radius.roleMap` | `map[string]string` | — | empty | Role mapping values → viewer/operator/admin. |
| `auth.radius.defaultRole` | `string` | — | empty → viewer | See the defaultRole behavior in the linked feature guide. |
| `auth.radius.displayName` | `string` | — | empty → first configured server host | See the displayName behavior in the linked feature guide. |
| `auth.saml.idpMetadataUrl` | `string` | `PM_SAML_IDP_METADATA_URL` | empty | HTTPS IdP metadata, loaded lazily; loopback HTTP allowed. |
| `auth.saml.idpMetadataFile` | `string` | `PM_SAML_IDP_METADATA_FILE` | empty | Local IdP metadata file; takes precedence over URL. |
| `auth.saml.baseUrl` | `string` | `PM_SAML_BASE_URL` | empty | Public origin; may instead supply both entityId and acsUrl. |
| `auth.saml.entityId` | `string` | `PM_SAML_ENTITY_ID` | empty → <baseUrl>/api/auth/saml/metadata | See the entityId behavior in the linked feature guide. |
| `auth.saml.acsUrl` | `string` | `PM_SAML_ACS_URL` | empty → <baseUrl>/api/auth/saml/acs | See the acsUrl behavior in the linked feature guide. |
| `auth.saml.spCertFile` | `string` | `PM_SAML_SP_CERT_FILE` | empty → <dataDir>/saml-sp.crt | SPCertFile / SPKeyFile hold the SP signing pair (PEM). When absent a self-signed pair is generated under the data directory on first start. |
| `auth.saml.spKeyFile` | `string` | `PM_SAML_SP_KEY_FILE` | empty → <dataDir>/saml-sp.key | See the spKeyFile behavior in the linked feature guide. |
| `auth.saml.signRequests` | `bool` | — | false | sign AuthnRequests (RSA-SHA256) |
| `auth.saml.usernameAttr` | `string` | — | empty → NameID | UsernameAttr names the assertion attribute used as the local username (default: the NameID). |
| `auth.saml.subjectAttr` | `string` | — | empty → stable NameID; transient/email formats need explicit attribute | SubjectAttr is the stable identifier the account is bound to. Default: the NameID when its format is persistent or unspecified; a transient NameID requires an attribute here (Entra ID: objectidentifier). |
| `auth.saml.roleAttr` | `string` | — | empty | RoleAttr is the attribute carrying groups / roles looked up in RoleMap. |
| `auth.saml.roleMap` | `map[string]string` | — | empty | Role mapping values → viewer/operator/admin. |
| `auth.saml.defaultRole` | `string` | — | empty → viewer | See the defaultRole behavior in the linked feature guide. |
| `auth.saml.allowedGroups` | `[]string` | — | empty | refuse users with none of these RoleAttr values |
| `auth.saml.displayName` | `string` | — | empty → metadata URL host, else SAML single sign-on | See the displayName behavior in the linked feature guide. |

## telemetry

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `telemetry.retention` | `Duration` | — | 2160h (90 days) | samples older than this are down-sampled/pruned |

## export

| JSON key | Type | Environment | Default / effective fallback | Meaning |
| --- | --- | --- | --- | --- |
| `export.buffer` | `int` | `PM_EXPORT_BUFFER` | 4096 | Buffer is the number of events queued for the export worker before new ones are dropped (and counted) so the event bus never blocks. |
| `export.syslog.address` | `string` | `PM_EXPORT_SYSLOG` | empty | Address is udp://host:514, tcp://host:6514 or tls://host:6514. |
| `export.syslog.format` | `string` | `PM_EXPORT_SYSLOG_FORMAT` | syslog | Format is "syslog" (RFC 5424 with structured data), "cef" or "json". |
| `export.syslog.facility` | `int` | `PM_EXPORT_SYSLOG_FACILITY` | 16 | Facility is the syslog facility number (default 16 = local0). |
| `export.syslog.tlsInsecure` | `bool` | `PM_EXPORT_SYSLOG_TLS_INSECURE` | false | TLSInsecure skips server certificate verification for tls://. |
| `export.syslog.caFile` | `string` | `PM_EXPORT_SYSLOG_CA_FILE` | empty | CAFile is a PEM bundle that verifies the tls:// server. |
| `export.file.path` | `string` | `PM_EXPORT_FILE` | empty | JSON lines, appended |
| `export.file.maxSizeMb` | `int` | `PM_EXPORT_FILE_MAX_MB` | 50 | rotate above this size (default 50) |
| `export.file.keep` | `int` | `PM_EXPORT_FILE_KEEP` | 5 | rotated files to keep (default 5) |
| `export.http.url` | `string` | `PM_EXPORT_HTTP` | empty | POST target for JSON-lines batches |
| `export.http.tokenEnv` | `string` | `PM_EXPORT_HTTP_TOKEN_ENV` | empty | env var holding a bearer token |
| `export.http.batchSize` | `int` | `PM_EXPORT_HTTP_BATCH` | 100 | events per POST (default 100) |
| `export.http.flushInterval` | `Duration` | `PM_EXPORT_HTTP_FLUSH` | 5s | send a partial batch after this (default 5s) |
| `export.http.timeout` | `Duration` | — | 10s | per attempt (default 10s) |
| `export.ingestListen` | `string` | `PM_EXPORT_INGEST_LISTEN` | empty | IngestListen is the UDP address the printer syslog receiver binds (e.g. "0.0.0.0:1514"); empty = off. |
| `export.ingestRetention` | `Duration` | `PM_EXPORT_INGEST_RETENTION` | 720h (30 days) | IngestRetention prunes device log lines older than this (default 30 days). |
| `export.ingestRatePerMinute` | `int` | `PM_EXPORT_INGEST_RATE` | 120 | IngestRatePerMinute caps lines handled per source address (default 120). |


## Top-level sections and process variables

The four scalar keys are `listen`, `dataDir`, `database`, `dsn`. Object sections are `poll`, `discovery`, `trap`, `alerts`, `jobs`, `auth`, `telemetry`, `export`; `notifiers` is an array. OIDC/LDAP/RADIUS/SAML are nested under `auth`, not top-level sections. Consumables and the print provider are database settings, not JSON-file sections.

| Variable / flag | Default | Purpose/source |
| --- | --- | --- |
| `PM_CONFIG`, `-config FILE` | no file | Config file path (`cmd/printmarshal/main.go`). |
| `-version` | false | Print version and exit; does not start networking. |
| `PM_ADMIN_PASSWORD` | generated if no users | Initial local admin only. Normal generated-password destination is `<dataDir>/initial-admin-password`, mode 0600. |
| `PM_SECRET_KEY` | load/create `<dataDir>/secret.key` | Base64 of exactly 32 bytes; AES-GCM master key. Read errors other than missing file stop startup; exclusive creation protects concurrent creators. |
| `PM_LOG_LEVEL` | info | `debug`, `info`, `warn`, `error`; unrecognized values leave info. |
| `PM_LOG_FORMAT` | text | `json` selects JSON; other values select text. |
| `PM_FIRMWARE_SIM_TARGETS` | unset | Explicit simulator host/host:port allowlist enabling the firmware simulator driver. |
| `PM_HEALTH_URL` | derived from PM_LISTEN | Docker HEALTHCHECK URL only. |
| `PM_VERSION` | dev build arg / latest image label in Compose | Packaging interpolation in `deploy/docker-compose.yml`, not server config. |
| `PM_BACKUP_DIR` | current directory | Backup helper output directory; not server config. |

`PM_TEST_PG_DSN`, `PM_LIVE_HP`, `PM_LIVE_COMMUNITY`, and `PM_PROBE_HOST` are test-only variables, not deployment configuration. Do not set live-test variables for an offline test run.


## Endpoint defaults and transport choices

When `poll.ippEndpoints` is empty the read candidates are:

```text
http://%s:631/ipp/print
http://%s:631/ipp
http://%s:631/ipp/printer
http://%s:631/
https://%s:443/ipp/print
http://%s:80/ipp/print
```

The write driver derives TLS candidates before plaintext forms. `ippWriteRequireTLS` is a distinct switch; `ippInsecureTls:false` verifies certificates but does not by itself forbid plaintext. eSCL has its own built-in endpoint list; `ippEndpoints` does not configure eSCL URLs. Vendor transports are constructed separately by `plugins.RegisterAll`; these poll transport settings must not be assumed to configure every vendor-specific probe. See `cmd/printmarshal/main.go`, `internal/driver/{ipp,escl}` and `internal/plugins/all.go`.


## Runtime settings through API/UI

`GET /api/settings` returns effective `runtime`, `defaults`, a safe notifier summary, poll/discovery/alert/retention summaries and version. `PATCH /api/settings` (admin) overlays fields on the current runtime record and validates the result; `DELETE /api/settings` saves an empty runtime override record. The file/env config is not rewritten. The record key is `runtime`.

| Runtime JSON key | File default | Validation |
| --- | --- | --- |
| `pollInterval` | `poll.interval` | At least 30s. |
| `pollConcurrency` | `poll.concurrency` | 1–512; see implementation limitation below. |
| `discoveryEnabled` | `discovery.enabled` | Boolean. |
| `discoveryInterval` | `discovery.interval` | At least 5m. |
| `discoveryTargets` | `discovery.targets` | No empty/whitespace-only target. |
| `supplyWarningPercent` | `alerts.supplyWarningPercent` | Critical < warning ≤ 100. |
| `supplyCriticalPercent` | `alerts.supplyCriticalPercent` | Critical ≥ 0 and below warning. |
| `offlineAfterFailures` | `alerts.offlineAfterFailures` | At least 1. |
| `telemetryRetention` | `telemetry.retention` | At least 24h. |
| `names.source` | `"advertised"` | `"advertised"` (default), `"dns"` or `"display"`. |

The scheduler re-reads intervals/flags/targets every five-second tick. Alert evaluation loads effective thresholds per poll. Pruning runs every six hours. Settings changes do not rebuild the engine or resize its existing channel.

```json
{"pollInterval":"5m","pollConcurrency":32,"discoveryEnabled":false,"discoveryInterval":"1h","discoveryTargets":[],"supplyWarningPercent":15,"supplyCriticalPercent":5,"offlineAfterFailures":2,"telemetryRetention":"2160h","names":{"source":"advertised"}}
```

Consumable settings use `GET/PUT /api/consumables/settings`, stored under `consumables`; see [Consumables and stock](Consumables-and-stock.md). Print-provider settings use `GET/PUT /api/print-provider`, stored under `printprovider`; see [Print provider](Print-provider.md). Neither has direct env overrides.


## Known discrepancies

- The config comment mentions downsampling, but retention currently deletes old telemetry; there is no downsampling worker.
- `pollConcurrency` is persisted and displayed as runtime-adjustable, but `internal/fleet/service.go` job limits and `Engine.Limiter` still use startup `Cfg.Poll.Concurrency`. Change file/env and restart to change actual concurrency.
- Runtime numeric overrides are applied only when positive. A saved critical threshold of 0 passes validation but falls back to the file value when read. `discoveryEnabled` is always assigned from the saved record, so reset's zero record forces it false even if the file default was true. Other zero values mostly mean “inherit.”
- `config.Default()` leaves external-provider fields zero-valued; defaults such as LDAP 10s, RADIUS retries 2, role viewer and SAML URL derivation are applied by the provider clients/handlers, as distinguished in the tables.
- PostgreSQL requires a suitable DSN: if it is omitted the generic loader still derives a SQLite-looking filename. There is no automatic database migration from SQLite to PostgreSQL.

Sources: `internal/config/config.go`, `internal/fleet/runtime.go`, `internal/fleet/service.go`, `cmd/printmarshal/main.go`, `internal/alerts/{notify,smtpauth}.go`, `internal/auth/{oidc,ldap,radius,saml}/` and the packaging files.
