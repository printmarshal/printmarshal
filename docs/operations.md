# PrintMarshal operations guide

How to install, configure, run, back up, upgrade and troubleshoot a PrintMarshal
server. It complements the [build spec](printmarshal-build-spec.md) (§12
"Deployment and runtime") and the feature docs ([configuration management](configuration.md),
[firmware](firmware.md)).

Contents

1. [What runs where](#what-runs-where)
2. [Install options](#install-options) — Docker, Compose, Kubernetes, Helm, systemd
3. [First run and the admin account](#first-run-and-the-admin-account)
4. [Configuration reference](#configuration-reference)
5. [Networking requirements](#networking-requirements)
6. [The data directory and the secret key](#the-data-directory-and-the-secret-key)
7. [Backups and restore](#backups-and-restore)
8. [Upgrades](#upgrades)
9. [Logs](#logs)
10. [Health and monitoring](#health-and-monitoring)
11. [Troubleshooting](#troubleshooting)
12. [Security hardening checklist](#security-hardening-checklist)
13. [Sizing](#sizing)

---

## What runs where

PrintMarshal is **one static binary** (`printmarshal`) that contains the REST/WebSocket
API, the embedded React UI, the device engine (SNMP, IPP, eSCL drivers and vendor
plugins), the scheduler, the job runner and the SNMP trap listener. State lives in a
single SQLite database inside the **data directory**. There is no separate database
server, cache or message queue to run.

The only hard constraint comes from the network (spec §12): **zero-config discovery
uses multicast and printers push SNMP traps to the server**, so the process must sit
on the same L2 network as the printers, or at least be reachable from them on a real
address. That is why every container/Kubernetes recipe below defaults to *host
networking*. Polling and active scans (explicit IPs/CIDRs) work from anywhere with IP
reachability.

```
          browsers / API clients                      printers (SNMP 161, IPP 631/443/80, eSCL)
                  │ TCP 8080 (or via reverse proxy)            ▲ polls, scans, reads
                  ▼                                            │
   ┌────────────────────────────────────────────────────────────────────┐
   │ printmarshal                                                       │
   │  API + UI ─ engine/drivers ─ scheduler ─ jobs ─ trap listener      │
   │                       │                               ▲ UDP 162/1162 traps
   │                       ▼                               │
   │            <data dir>/printmarshal.db  secret.key  firmware/      │
   └────────────────────────────────────────────────────────────────────┘
                  ▲ mDNS 224.0.0.251:5353, WS-Discovery 239.255.255.250:3702 (multicast)
```

## Install options

| Option | Best for | Files |
| --- | --- | --- |
| `docker run` / Docker Compose | a site with one server and up to a few hundred printers | [`Dockerfile`](../Dockerfile), [`deploy/docker-compose.yml`](../deploy/docker-compose.yml) |
| Kubernetes manifests | clusters without Helm, GitOps with kustomize | [`deploy/kubernetes/`](../deploy/kubernetes/) |
| Helm chart | clusters with Helm | [`deploy/helm/printmarshal/`](../deploy/helm/printmarshal/) |
| systemd | a dedicated Linux host, the largest fleets, simplest networking | [`deploy/systemd/`](../deploy/systemd/) |

All of them run the same binary with the same configuration; pick by how you run
everything else. The binary is also fine for evaluation on macOS/Windows (`make build`,
README "Quick start"), but discovery and traps are only exercised properly on a Linux
host with L2 access to the printer subnet (spec §13).

### Docker

The image is built in three stages (Node builds the SPA, Go embeds it into static
binaries, Alpine runs them), runs as the unprivileged user `pm` (uid 10001), declares
`/data` as a volume and carries a `HEALTHCHECK` on `/healthz`. It also ships `pm-sim`
(the printer simulator), the recorded fixtures under `/opt/printmarshal/fixtures`, the
`sqlite3` CLI and the `pm-backup` / `pm-restore` scripts.

```bash
make docker                       # printmarshal:<git describe> and printmarshal:latest
# or: docker build --build-arg VERSION=v0.1.0 -t printmarshal:v0.1.0 .

docker run -d --name printmarshal --restart unless-stopped \
  --network host \
  -e PM_ADMIN_PASSWORD='change-me' \
  -v pm-data:/data \
  printmarshal:latest

docker logs -f printmarshal       # "http listening addr=:8080"
```

Build arguments: `VERSION` (stamped into `printmarshal -version`, `/healthz` and the
`org.opencontainers.image.version` label), `VCS_REF`, `BUILD_DATE`, and the base
images `NODE_IMAGE`, `GO_IMAGE`, `RUNTIME_IMAGE`. Multi-arch builds work with
`docker buildx build --platform linux/amd64,linux/arm64` (the Go stage cross-compiles).

Notes:

- `--network host` is what makes discovery and traps work; see
  [Networking](#networking-requirements). Without it, publish `-p 8080:8080` and
  use active scans only.
- A **bind mount** instead of the named volume must be writable by uid 10001:
  `mkdir -p /srv/printmarshal && chown 10001:10001 /srv/printmarshal`, then
  `-v /srv/printmarshal:/data`.
- The process cannot bind UDP 162 (not root, no capabilities). It listens on **1162**
  by default; point the printers' trap destination at port 1162, or redirect on the
  host (`iptables -t nat -A PREROUTING -p udp --dport 162 -j REDIRECT --to-ports 1162`),
  or lower the privileged-port floor on the host (`sysctl net.ipv4.ip_unprivileged_port_start=162`,
  which host networking honours) and set `PM_TRAP_LISTEN=0.0.0.0:162`.
- The health check probes `http://127.0.0.1:<port of PM_LISTEN>/healthz`; set
  `PM_HEALTH_URL` if `PM_LISTEN` binds a specific non-loopback address.
- Want a distroless runtime? Change `RUNTIME_IMAGE` to `gcr.io/distroless/static-debian12:nonroot`
  and drop the `RUN apk …`, `HEALTHCHECK` (no shell/wget there) and the `pm-backup`/`pm-restore`
  copies; take backups from the host instead.

### Docker Compose (small sites)

```bash
cp deploy/printmarshal.example.json deploy/printmarshal.json      # optional, for notifiers etc.
printf 'PM_ADMIN_PASSWORD=%s\nPM_SECRET_KEY=%s\n' "$(openssl rand -base64 18)" "$(openssl rand -base64 32)" > deploy/printmarshal.env
chmod 600 deploy/printmarshal.env
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml logs -f
```

[`deploy/docker-compose.yml`](../deploy/docker-compose.yml) runs the single container
with `network_mode: host`, a named volume `pm-data` for `/data`, JSON logs and
`restart: unless-stopped`. The optional `deploy/printmarshal.env` (do **not** commit it)
is read automatically for secrets; uncomment the `volumes` line to mount a JSON config.
A commented `printer-sim` service shows how to run the simulator next to it.

Why host networking: mDNS and WS-Discovery are multicast and Docker's bridge does not
forward multicast; a published UDP port (`-p 162:1162/udp`) does deliver traps but
NATs the source address, so the listener cannot tell which printer sent the trap and
drops it. With `network_mode: host` the container sees the host's interfaces and
VLANs exactly as the host does. The costs: 8080/tcp and 1162/udp are bound directly on
the host, and Docker's network isolation does not apply to this container.

### Kubernetes (plain manifests)

[`deploy/kubernetes/`](../deploy/kubernetes/) contains a Namespace, a ConfigMap with
`printmarshal.json`, a Secret for `PM_ADMIN_PASSWORD` / `PM_SECRET_KEY`, a 5 Gi
ReadWriteOnce PersistentVolumeClaim, a ServiceAccount (no token), the Deployment
(1 replica, `Recreate`, `hostNetwork: true`, non-root, read-only root filesystem, all
capabilities dropped, startup/readiness/liveness probes on `/healthz`, 100m/128Mi
requests), a ClusterIP Service, an optional LoadBalancer Service for traps on 162, an
optional Ingress, and a `kustomization.yaml`.

```bash
# 1. Secrets out of band (never commit real values):
kubectl create namespace printmarshal
kubectl -n printmarshal create secret generic printmarshal-secrets \
  --from-literal=PM_ADMIN_PASSWORD="$(openssl rand -base64 18)" \
  --from-literal=PM_SECRET_KEY="$(openssl rand -base64 32)"
# 2. Remove 20-secret.yaml from kustomization.yaml (or edit its placeholders), set the
#    image in kustomization.yaml `images:`, edit 60-ingress.yaml or drop it, then:
kubectl apply -k deploy/kubernetes
kubectl -n printmarshal rollout status deploy/printmarshal
kubectl -n printmarshal logs deploy/printmarshal | grep -i "initial admin"   # if no PM_ADMIN_PASSWORD
```

**`hostNetwork: true` — the trade-off.** The pod uses the node's network namespace,
so multicast discovery and traps work on every VLAN the node is attached to. In
return: the ports 8080/tcp and 1162/udp are bound on the node (so exactly one replica,
`strategy: Recreate`), NetworkPolicies do not apply, `dnsPolicy` must be
`ClusterFirstWithHostNet`, the namespace needs the `baseline` (not `restricted`) Pod
Security Standard, and you must pin the pod with a `nodeSelector`/affinity to a node
that is really on the printer network. Alternatives, from least to most involved:

1. **No host networking.** Keep the pod on the CNI, set `discovery.enabled=false`,
   list the printer subnets in `discovery.targets` (active scans) and let polling do
   the rest. Traps can still arrive through the `printmarshal-trap` Service
   (`type: LoadBalancer`, **`externalTrafficPolicy: Local`** — without it kube-proxy
   SNATs the packet and the trap is dropped as "unknown device").
2. **Multus macvlan/ipvlan.** Attach a second interface with a real address on the
   printer VLAN (`NetworkAttachmentDefinition` + the `k8s.v1.cni.cncf.io/networks` pod
   annotation). Multicast and traps work, no node ports, NetworkPolicies still apply to
   the primary interface. The node's uplink must allow the VLAN.
3. **DaemonSet** is *not* a good fit: state is a single SQLite database on one RWO
   volume, so one pod per node would be N independent servers. Use a Deployment pinned
   to one node; if the printers are spread over several isolated L2 segments, run one
   release per segment or rely on active scans.

Trap port 162 on Kubernetes: the container is unprivileged and cannot bind 162.
Namespaced sysctls cannot be set on hostNetwork pods, so either keep 1162 and point the
printers at `<node IP>:1162`, set `net.ipv4.ip_unprivileged_port_start=162` **on the
node** (then `trap.listen: "0.0.0.0:162"`), or use the trap Service.

Storage: SQLite needs a POSIX block filesystem (RWO). NFS/SMB-backed volumes cause
`database is locked` errors and can corrupt the database under WAL.

### Helm chart

[`deploy/helm/printmarshal/`](../deploy/helm/printmarshal/) renders the same objects
from `values.yaml`:

| Value | Purpose |
| --- | --- |
| `image.repository`, `image.tag`, `image.pullPolicy`, `imagePullSecrets` | which image (tag defaults to `appVersion`) |
| `hostNetwork` (default `true`), `dnsPolicy`, `nodeSelector`, `affinity`, `tolerations` | networking and placement, see above |
| `ports.http`, `ports.trap` | container ports; must agree with `config.listen` / `config.trap.listen` |
| `config.*` | the whole `printmarshal.json` (rendered with `toPrettyJson`); a change rolls the pod via a checksum annotation |
| `logging.format`, `logging.level` | `PM_LOG_FORMAT`, `PM_LOG_LEVEL` |
| `extraEnv`, `extraArgs`, `extraVolumes`, `extraVolumeMounts` | e.g. the SMTP password referenced by a notifier's `passwordEnv` |
| `secrets.existingSecret` / `secrets.adminPassword` / `secrets.secretKey` / `secrets.generateSecretKey` | see below |
| `persistence.enabled`, `existingClaim`, `storageClass`, `accessModes`, `size`, `annotations` | the `/data` PVC (kept on uninstall by default) |
| `service.type`, `service.port`, `service.trap.*` | ClusterIP for the UI; optional trap LoadBalancer with `externalTrafficPolicy: Local` |
| `ingress.*` | standard Ingress block (`className`, `annotations`, `hosts`, `tls`) |
| `resources`, `startupProbe`, `readinessProbe`, `livenessProbe`, `podSecurityContext`, `securityContext` | runtime; defaults match the plain manifests |

Secret handling: with `secrets.existingSecret` the chart creates nothing and reads the
keys `PM_ADMIN_PASSWORD` / `PM_SECRET_KEY` from your Secret. Otherwise it creates one:
`secrets.secretKey` if given, else (with `generateSecretKey: true`, the default) a
random 32-byte key generated **once** — on upgrades the template uses `lookup` to keep
the existing value, and the Secret carries `helm.sh/resource-policy: keep` so
`helm uninstall` does not delete it. Never rotate this key casually: every stored
device credential is sealed with it. With `generateSecretKey: false` and no key the
server writes `/data/secret.key` instead (then that file is part of the backup).

```bash
helm install printmarshal deploy/helm/printmarshal -n printmarshal --create-namespace \
  --set image.repository=registry.example.com/printmarshal --set image.tag=v0.1.0 \
  --set secrets.adminPassword="$(openssl rand -base64 18)" \
  --set 'nodeSelector.printmarshal\.io/printer-network=true'
helm upgrade printmarshal deploy/helm/printmarshal -n printmarshal --reuse-values --set image.tag=v0.2.0
make helm-lint          # helm lint + helm template (set HELM=... to run it from a container)
```

`NOTES.txt` prints how to reach the UI, where the generated admin password is, and the
backup reminder.

### systemd (bare Linux host)

[`deploy/systemd/printmarshal.service`](../deploy/systemd/printmarshal.service) and
[`printmarshal.env.example`](../deploy/systemd/printmarshal.env.example):

```bash
make web build                                   # or download a release binary
sudo install -m 0755 bin/printmarshal /usr/local/bin/printmarshal
sudo useradd --system --home-dir /var/lib/printmarshal --shell /usr/sbin/nologin printmarshal
sudo install -d -m 0750 -o root -g printmarshal /etc/printmarshal
sudo install -m 0640 -g printmarshal deploy/systemd/printmarshal.env.example /etc/printmarshal/printmarshal.env
sudo install -m 0644 deploy/systemd/printmarshal.service /etc/systemd/system/
sudoedit /etc/printmarshal/printmarshal.env      # PM_ADMIN_PASSWORD, PM_TRAP_LISTEN, PM_LOG_FORMAT, ...
sudo systemctl daemon-reload && sudo systemctl enable --now printmarshal
journalctl -u printmarshal -f
```

The unit runs as the `printmarshal` user, lets systemd create `/var/lib/printmarshal`
(`StateDirectory`, mode 0700) and `/etc/printmarshal`, grants only
`CAP_NET_BIND_SERVICE` (so `PM_TRAP_LISTEN=0.0.0.0:162` works without root), and
applies the usual hardening (`ProtectSystem=strict`, `NoNewPrivileges`,
`PrivateTmp/Devices`, `RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK`,
`SystemCallFilter=@system-service`, `MemoryDenyWriteExecute`, …). Check the score with
`systemd-analyze security printmarshal`. Stopping sends SIGTERM; the server finishes
HTTP requests (10 s) and marks running jobs as interrupted so they resume on restart.

## First run and the admin account

On the very first start with an empty database the server creates the user **`admin`**
(role admin):

- with the password from `PM_ADMIN_PASSWORD` if set (log: `created initial admin user
  from PM_ADMIN_PASSWORD`), or
- with a generated 24-hex-character password written to
  **`<data dir>/initial-admin-password`** (mode 0600, never to the log; the log line
  names the file). Read it, sign in, change the password, then delete the file.

`PM_ADMIN_PASSWORD` is ignored afterwards (it never resets an existing password) and
can be removed from the environment. Sign in, change the password under your profile,
create further users (viewer / operator / admin) under **Users**, and then add device
credentials (SNMP communities / v3 users) under **Credentials** — they are encrypted
with the secret key and write-only through the API.

Lost admin password: there is no CLI reset yet. Create a second admin while you still
have one; otherwise restore a backup from before the loss.

## Configuration reference

Precedence: built-in defaults → JSON file (`-config FILE` or `PM_CONFIG=FILE`) →
`PM_*` environment variables (only the ones listed; unlisted fields are file-only).
Durations use Go syntax (`30s`, `5m`, `720h`); in JSON a bare number means seconds.
Booleans from the environment accept `1`/`true`/`yes` (case-insensitive); anything else
is false. Comma-separated lists for `PM_IPP_ENDPOINTS` and `PM_DISCOVERY_TARGETS`.
Source of truth: [`internal/config/config.go`](../internal/config/config.go);
example file: [`deploy/printmarshal.example.json`](../deploy/printmarshal.example.json).

### Server and datastore

| JSON | Env | Default | Meaning |
| --- | --- | --- | --- |
| `listen` | `PM_LISTEN` | `:8080` | HTTP listen address for UI and API |
| `dataDir` | `PM_DATA_DIR` | `./data` | Directory for the database, `secret.key` and `firmware/` |
| `database` | `PM_DATABASE` | `sqlite` | Datastore: `sqlite` (single file in the data directory) or `postgres` (see [PostgreSQL](#postgresql)) |
| `dsn` | `PM_DSN` | `<dataDir>/printmarshal.db` | SQLite file path, or the PostgreSQL connection URL when `database` is `postgres` |

### PostgreSQL

`database: postgres` keeps every feature of the SQLite mode but stores the state in a
PostgreSQL server (14 or newer; tested against 16), which is the scale-out option of
the spec (§5): several hundred thousand telemetry rows a day, a database the DBAs
already back up, and a server that is not tied to one node's disk. The data directory
is still needed for `secret.key` (unless `PM_SECRET_KEY` is set), the generated
admin password and uploaded firmware images.

```bash
createdb printmarshal                                  # or let the DBA hand you an empty database
PM_DATABASE=postgres \
PM_DSN='postgres://printmarshal:secret@db.example.internal:5432/printmarshal?sslmode=verify-full&pool_max_conns=16' \
PM_ADMIN_PASSWORD=change-me ./bin/printmarshal
```

- **DSN.** A `postgres://user:password@host:port/dbname?…` URL or a libpq-style
  `host=… dbname=…` string (anything [pgx](https://github.com/jackc/pgx) accepts;
  `PGHOST`/`PGPASSWORD`/`PGSSLMODE` and `~/.pgpass` are honoured). Keep the password
  out of the JSON file: put the DSN in `PM_DSN` from the platform's secret store.
- **TLS.** `sslmode=verify-full` with `sslrootcert=/path/ca.pem` in production;
  `require` encrypts without verifying the server; `disable` only on a loopback
  socket or a private network you trust.
- **Pooling.** The server opens a connection pool per process: `pool_max_conns`
  (default 4, use 8–16 for the larger tiers in [Sizing](#sizing)), `pool_min_conns`,
  `pool_max_conn_lifetime`, `pool_max_conn_idle_time` in the DSN. One pool per
  PrintMarshal process, so size the server's `max_connections` for
  `instances × pool_max_conns` plus your other clients. The connection parameters are
  also where `search_path=` or `application_name=` go.
- **Privileges.** The role needs `CREATE` on the database's schema (it creates its
  tables on first start) and ordinary DML afterwards; no superuser.
- **Migrations** run automatically at start under an advisory lock, so two instances
  starting at once do not race; `schema_migrations` records the versions, which are
  numbered the same as the SQLite ones. Columns have native types (`TIMESTAMPTZ`,
  `BOOLEAN`, `JSONB`, `BYTEA`), one table per SQLite table.
- **Backup.** Use the database's own tooling instead of `pm-backup`:
  `pg_dump -Fc -d "$PM_DSN" -f printmarshal-$(date -u +%Y%m%dT%H%M%SZ).dump` (consistent
  while the server runs) plus `secret.key` / `PM_SECRET_KEY` and `firmware/`. Restore
  with `pg_restore -d <empty database> file.dump`, then start the server with the same
  secret key. Point-in-time recovery, replication and HA are the database's job.
- **Moving from SQLite** is not automated yet: start a fresh PostgreSQL instance,
  recreate users, credentials and groups, and run a discovery; telemetry history stays
  in the old file.
- **Multiple instances** against one database are safe for the API and the UI, but
  the scheduler, trap listener and job runner still assume a single process: run one
  active instance, use the database for durability and a quick restart elsewhere.

### Polling (`poll`)

| JSON | Env | Default | Meaning |
| --- | --- | --- | --- |
| `poll.interval` | `PM_POLL_INTERVAL` | `5m` | Scheduled poll cadence for every active device |
| `poll.concurrency` | `PM_POLL_CONCURRENCY` | `32` | Max simultaneous device reads (engine-wide limiter, also bounds config-apply jobs) |
| `poll.snmpPort` | `PM_SNMP_PORT` | `161` | SNMP agent port on devices (simulators use 1161) |
| `poll.ippEndpoints` | `PM_IPP_ENDPOINTS` | `http://%s:631/ipp/print`, `…:631/ipp`, `…:631/ipp/printer`, `…:631/`, `https://%s:443/ipp/print`, `http://%s:80/ipp/print` | IPP URL templates tried in order; `%s` is the host |
| `poll.snmpTimeout` | — | `2s` | Per-request SNMP timeout |
| `poll.snmpRetries` | — | `1` | SNMP retries after a timeout |
| `poll.ippTimeout` | — | `4s` | HTTP timeout for IPP and eSCL requests |
| `poll.ippInsecureTls` | — | `true` | Accept printers' self-signed certificates for IPPS/eSCL over HTTPS |

### Discovery (`discovery`)

| JSON | Env | Default | Meaning |
| --- | --- | --- | --- |
| `discovery.enabled` | `PM_DISCOVERY_ENABLED` | `false` | Run discovery on a schedule (manual runs from the UI always work) |
| `discovery.interval` | `PM_DISCOVERY_INTERVAL` | `1h` | Scheduled discovery cadence |
| `discovery.targets` | `PM_DISCOVERY_TARGETS` | none | Default active-scan targets: IPs, ranges (`a.b.c.1-50`) and CIDRs |
| `discovery.mdnsWait` | — | `5s` | How long to collect mDNS/DNS-SD answers |
| `discovery.wsdWait` | — | `3s` | How long to collect WS-Discovery answers |
| `discovery.scanWorkers` | — | `64` | Parallel probes during an active scan |
| `discovery.maxAddresses` | — | `4096` | Largest scan accepted without the explicit "allow large" flag |

### Trap listener (`trap`)

| JSON | Env | Default | Meaning |
| --- | --- | --- | --- |
| `trap.enabled` | `PM_TRAP_ENABLED` | `true` | Listen for SNMP traps/informs and reconcile them with a poll |
| `trap.listen` | `PM_TRAP_LISTEN` | `0.0.0.0:1162` | UDP listen address; 162 needs root or `CAP_NET_BIND_SERVICE` |
| `trap.allowedSources[]` | — | any | CIDRs / addresses traps are accepted from; traps are acted on only when they come from a known device's address in any case |
| `trap.ratePerMinute` | — | `60` | Traps handled per source address per minute; excess is dropped (one log line per source per minute) |

### Alerts (`alerts`), jobs, auth, telemetry

| JSON | Env | Default | Meaning |
| --- | --- | --- | --- |
| `alerts.supplyWarningPercent` | — | `15` | Supply level at/below which a *warning* alert is raised |
| `alerts.supplyCriticalPercent` | — | `5` | Supply level at/below which a *critical* alert is raised |
| `alerts.offlineAfterFailures` | — | `2` | Consecutive failed polls before a device is marked offline |
| `jobs.workers` | `PM_JOB_WORKERS` | `4` | Concurrent jobs (discovery runs, bulk polls, config applies, firmware campaigns) |
| `auth.sessionTtl` | — | `720h` (30 d) | Session cookie lifetime |
| `auth.secureCookies` | `PM_SECURE_COOKIES` | `false` | Mark the session cookie `Secure`; **set `true` behind TLS** (the server warns when listening off-loopback without it) |
| `auth.allowedOrigins[]` | — | same-origin only | Extra `Origin` host patterns accepted on the WebSocket stream (`console.example.com`, `*.corp.example`) |
| `auth.loginFailuresPerMinute` | — | `10` | Failed sign-ins per source address before further attempts get `429` for a minute |
| `auth.oidc.*` | `PM_OIDC_ISSUER`, `PM_OIDC_CLIENT_ID`, `PM_OIDC_CLIENT_SECRET_ENV`, `PM_OIDC_REDIRECT_URL` | off | OpenID Connect single sign-on; see `docs/auth.md` for role mapping and provider notes |
| `telemetry.retention` | — | `2160h` (90 d) | Telemetry samples older than this are pruned by the scheduler |

### Exports and SIEM (`export`)

| JSON | Env | Default | Meaning |
| --- | --- | --- | --- |
| `export.syslog.address` | `PM_EXPORT_SYSLOG` | off | `udp://`, `tcp://` or `tls://host:port` collector for security events |
| `export.syslog.format` | `PM_EXPORT_SYSLOG_FORMAT` | `syslog` | `syslog` (RFC 5424 + structured data), `cef` or `json` |
| `export.file.path` | `PM_EXPORT_FILE` | off | JSON-lines file with size rotation (`PM_EXPORT_FILE_MAX_MB`, `PM_EXPORT_FILE_KEEP`) |
| `export.http.url` | `PM_EXPORT_HTTP` | off | HTTP collector for JSON-lines batches; bearer token from the variable named by `PM_EXPORT_HTTP_TOKEN_ENV` |
| `export.ingestListen` | `PM_EXPORT_INGEST_LISTEN` | off | UDP syslog receiver for printer logs, e.g. `0.0.0.0:1514` |
| `export.ingestRetention` | `PM_EXPORT_INGEST_RETENTION` | `720h` | Device log retention |

Every field, the formats, the CEF mapping and the sync API are in [export.md](export.md).

### Notifiers (`notifiers[]`, file only)

| Field | Applies to | Meaning |
| --- | --- | --- |
| `name` | all | Identifier shown in the audit log |
| `type` | all | `webhook`, `slack`, `teams`, `discord` or `smtp` |
| `url` | webhook types | Destination URL (incoming-webhook URL for Slack/Teams/Discord) |
| `events` | all | Subscribed event types from the catalogue (`GET /api/events/catalog`, `docs/events.md`), or `*` for every notifiable type; empty = all alert events. Unknown types are reported at start-up |
| `legacy` | teams | `true` posts the retired Office 365 connector MessageCard instead of a Workflows Adaptive Card |
| `host`, `port`, `startTls` | smtp | SMTP server and whether to upgrade with STARTTLS |
| `from`, `to[]` | smtp | Sender and recipients |
| `username`, `passwordEnv` | smtp | Login; the password is read from the **environment variable named** in `passwordEnv`, never from the file |
| `authMethod` | smtp | `plain` (default), `login`, or `xoauth2` (Microsoft 365 / Google Workspace) |
| `tokenEnv`, or `tokenUrl` + `clientId` + `clientSecretEnv` + `scope` | smtp (xoauth2) | A ready bearer token from the environment, or the OAuth 2.0 client-credentials grant (token cached until it expires) |

### Process-level environment variables (not in the JSON file)

| Variable | Default | Meaning |
| --- | --- | --- |
| `PM_CONFIG` | none | Path of the JSON config (same as `-config`) |
| `PM_ADMIN_PASSWORD` | generated | Initial admin password, first start only |
| `PM_SECRET_KEY` | generated into `<dataDir>/secret.key` | Base64 of 32 random bytes; encrypts stored credentials |
| `PM_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `PM_LOG_FORMAT` | `text` | `text` or `json` |
| `PM_FIRMWARE_SIM_TARGETS` | none | Comma-separated simulator `host[:port]` list that enables the simulator-only firmware delivery driver ([firmware.md](firmware.md)); leave unset in production |
| `PM_HEALTH_URL` | derived from `PM_LISTEN` | Container image only: URL probed by the Docker `HEALTHCHECK` |

Flags: `-config FILE`, `-version`.

## Networking requirements

| Direction | Protocol / port | Purpose | Notes |
| --- | --- | --- | --- |
| Inbound | TCP **8080** (`listen`) | UI, REST API, WebSocket `/api/stream` | Put a TLS reverse proxy or Ingress in front; keep WebSockets enabled |
| Inbound | UDP **162** or **1162** (`trap.listen`) | SNMP traps/informs from printers | Source address must be preserved (no NAT) — it identifies the device |
| Outbound | UDP **161** (`poll.snmpPort`) | SNMP v1/v2c/v3 polls, active-scan probes, SNMP SET for config writes | |
| Outbound | TCP **631**, **443**, **80** | IPP / IPPS (`Get-Printer-Attributes`, `Set-Printer-Attributes`) | per `poll.ippEndpoints` |
| Outbound | TCP **80**, **8080**, **443** | eSCL scanner capabilities/status on MFPs | `http(s)://<host>[:8080]/eSCL` |
| Multicast | UDP **5353** ↔ 224.0.0.251 | mDNS / DNS-SD discovery (`_ipp._tcp`, `_printer._tcp`, …) | Listens on `0.0.0.0:5353` with port reuse (coexists with avahi/Bonjour) and joins the group on every multicast-capable interface |
| Multicast | UDP **3702** → 239.255.255.250, replies unicast to an ephemeral port | WS-Discovery Probe/ProbeMatch | |
| Outbound | TCP 443 (webhooks), TCP 25/465/587 (SMTP) | Notifiers | Only if configured |

Firewall on the server: allow 8080/tcp from operator networks and 162/1162/udp from
the printer subnets; everything else is outbound. Multicast is link-local: discovery
only sees printers on L2 segments the server has an interface in. For other segments,
use active scans (`discovery.targets`) or a per-segment deployment.

Reverse proxy: terminate TLS there, forward to `127.0.0.1:8080`, pass `Upgrade` /
`Connection` headers for `/api/stream`, and set `PM_SECURE_COOKIES=true`. The server
does not read `X-Forwarded-*` headers, so audit entries record the proxy's address
unless the proxy is transparent.

## The data directory and the secret key

```
<dataDir>/                     default ./data, /data in containers, /var/lib/printmarshal with systemd
├── printmarshal.db            SQLite, WAL mode (printmarshal.db-wal / -shm appear while running)
├── secret.key                 base64 32-byte key, mode 0600 — only when PM_SECRET_KEY is not set
└── firmware/<package id>/…    uploaded firmware images (verified by SHA-256 on every use)
```

**The secret key encrypts every stored device credential** (SNMP communities, SNMPv3
users, HTTP logins). It is loaded in this order: `PM_SECRET_KEY` (base64 of 32 bytes)
→ `<dataDir>/secret.key` → generated and written to that file. Consequences:

- A database restored without its key still opens, but every credential decrypts to
  garbage and polls fail with authentication errors. **Back up the key with the
  database**, every time.
- Changing the key is equivalent to deleting all credentials; re-enter them afterwards.
- Prefer the platform's secret store (`PM_SECRET_KEY` from a Kubernetes Secret, a
  compose `env_file` with mode 0600, the systemd `EnvironmentFile`) over the file, and
  back that secret up as you back up any other secret.

## Backups and restore

What to back up: `printmarshal.db` (consistently), `secret.key` **or** the
`PM_SECRET_KEY` secret, `firmware/`, and your JSON config. (With
`database: postgres` the database part is a `pg_dump`, see [PostgreSQL](#postgresql);
the rest of this section still applies to the data directory.) The database is in WAL mode
and the server keeps one connection open, so copying the file while it runs is unsafe;
use the SQLite online-backup API.

[`deploy/scripts/backup.sh`](../deploy/scripts/backup.sh) (also installed in the image
as `pm-backup`) does exactly that:

1. `sqlite3 <db> ".backup <snapshot>"` — a transactionally consistent copy while the
   server runs — followed by `PRAGMA integrity_check`. Without the `sqlite3` CLI it
   refuses unless `--cold` is given, in which case **the server must be stopped** and
   the db + `-wal` + `-shm` files are copied verbatim.
2. Adds `secret.key` (warns loudly if it is absent because the key comes from
   `PM_SECRET_KEY`), `firmware/`, the config passed with `-c`, and a `MANIFEST`.
3. Writes `printmarshal-backup-<UTC stamp>.tar.gz` with mode 0600; `--keep N` prunes
   older archives.

```bash
# systemd host, nightly (cron or a systemd timer), keep two weeks:
sudo -u printmarshal deploy/scripts/backup.sh -o /var/backups/printmarshal -c /etc/printmarshal/printmarshal.json --keep 14

# Docker:
docker exec printmarshal pm-backup -o /data/backups --keep 7
docker cp printmarshal:/data/backups/. ./backups/

# Kubernetes:
kubectl -n printmarshal exec deploy/printmarshal -- pm-backup -o /data/backups --keep 7
kubectl -n printmarshal cp "$(kubectl -n printmarshal get pod -l app.kubernetes.io/name=printmarshal -o name | cut -d/ -f2):/data/backups" ./backups
kubectl -n printmarshal get secret printmarshal-secrets -o yaml > backups/printmarshal-secrets.yaml   # the key!
```

The archive contains the encryption key: store it like a password store (encrypt at
rest with `age`/`gpg`, restrict access, keep it off shared drives).

Restore with [`deploy/scripts/restore.sh`](../deploy/scripts/restore.sh) (`pm-restore`
in the image) **with the server stopped**:

```bash
sudo systemctl stop printmarshal
sudo -u printmarshal deploy/scripts/restore.sh -f /var/backups/printmarshal/printmarshal-backup-20260101-020000Z.tar.gz
sudo systemctl start printmarshal
# Docker (the server container must be stopped; the volume is shared):
#   docker stop printmarshal
#   docker run --rm --entrypoint /usr/local/bin/pm-restore -v pm-data:/data -v "$PWD/backups:/b" \
#     printmarshal:latest -f /b/<archive> --yes
#   docker start printmarshal
```

It checks the archived database, moves the current contents of the data directory to
`<dataDir>/pre-restore-<stamp>/` (the directory itself stays, it is usually a mount),
removes any stale `-wal`/`-shm` files, copies the database, `secret.key` (unless
`--keep-key`) and `firmware/` into place, and tells you if the archive has no key so
you remember to provide the original `PM_SECRET_KEY`. Delete the `pre-restore-*`
directory once you are satisfied.

Test a restore now and then: restore into a scratch directory
(`restore.sh -f … -d /tmp/pm-test --yes`), start a second instance against it with
`PM_DATA_DIR=/tmp/pm-test PM_LISTEN=127.0.0.1:18080 PM_TRAP_ENABLED=false`, log in, and
check that a credential still works (poll a device).

## Upgrades

1. **Back up first** (above). Schema migrations run automatically at start and there
   is no downgrade path: rolling back means restoring the backup.
2. Read the release notes for new or renamed settings; the
   [configuration reference](#configuration-reference) always matches the shipped
   `config.go`.
3. Replace the binary/image and restart:
   - Docker: `make docker` (or pull), `docker stop printmarshal && docker rm printmarshal`,
     run again with the same `-v pm-data:/data`. Compose: `docker compose … up -d --build`.
   - Kubernetes: change the image tag (`kustomization.yaml` `images:` or
     `helm upgrade --set image.tag=…`). `Recreate` stops the old pod before the new one
     claims the volume and host ports.
   - systemd: `install` the new binary, `systemctl restart printmarshal`.
4. Watch the log for `printmarshal starting version=…` and `http listening`, then
   `/healthz` reports the new version. Jobs that were running are marked *interrupted*
   and can be resumed from the Jobs page.

Downtime is the restart itself (seconds); polls scheduled during it run at the next
tick, and traps sent meanwhile are lost (printers do not retry plain traps).

## Logs

Logs go to **stdout** (captured by Docker, the kubelet, or journald) using Go `slog`.

- `PM_LOG_FORMAT=text` (default): `time=2026-10-28T10:00:00.000Z level=INFO msg="http listening" addr=:8080`
- `PM_LOG_FORMAT=json`: `{"time":"2026-10-28T10:00:00.000Z","level":"INFO","msg":"http listening","addr":":8080"}`

`PM_LOG_LEVEL=debug` adds per-request driver detail (SNMP/IPP exchanges, discovery
answers); keep `info` in production. Lines worth alerting on:

| Message | Level | Meaning |
| --- | --- | --- |
| `printmarshal starting` | INFO | Version, listen address, DSN — first line after a (re)start |
| `created initial admin user …` | WARN | First run; the generated password is in this line (once) |
| `http listening` | INFO | Ready to serve |
| `trap listener ready` | INFO | The UDP trap socket is bound (`addr=`) |
| `trap listener stopped` | ERROR | Could not bind `trap.listen` (port in use / privileged) or the socket failed |
| `marked jobs interrupted by previous shutdown` | WARN | Unclean stop; resume the jobs if wanted |
| `fatal: …` on stderr | — | Start-up failure (bad config, unwritable data dir, key not base64, unsupported datastore) |
| `shutting down` | INFO | SIGTERM/SIGINT received |

The HTTP layer does not emit an access log; put the reverse proxy's access log in front
if you need one. Every state-changing action (users, credentials, config writes,
firmware deliveries, …) is recorded in the **audit log** inside the database and
visible in the UI.

## Health and monitoring

- `GET /healthz` → `200 {"ok":true,"version":"…"}` without authentication: used by the
  Docker `HEALTHCHECK`, the Kubernetes probes and any external monitor. It answers as
  soon as the HTTP server is up (the database is already open and migrated by then).
- `GET /readyz` → `200` only when the database answers; use it as the Kubernetes
  readiness probe and keep `/healthz` for liveness.
- `GET /metrics` (viewer role: scrape with an API token, `Authorization: Bearer pmt_…`)
  serves Prometheus text: `printmarshal_devices_total`, `_devices_offline`,
  `_devices_by_severity{severity}`, `_devices_by_state{state}`, `_alerts_active`,
  `_alerts_critical`, `_supplies_low`, `_stock_low`, `_jobs_running`,
  `_notifications_outbox{state="pending"|"dead"}`, `_http_requests_total{class}`,
  `_http_inflight`, `_http_latency_ms_total`. Alert on `dead` outbox rows and on
  `_devices_offline` growing.
- Security events can be forwarded to a SIEM (syslog / file / HTTP sinks, `export.*`);
  see `docs/export.md`.
- Watch disk usage of the data directory (telemetry grows with device count × poll
  rate until `telemetry.retention` kicks in) and the container's memory.

## Troubleshooting

**Discovery finds nothing.**
Almost always networking, not the printers. Check in this order:

1. Is the server on the printers' L2 segment? mDNS/WS-Discovery are link-local. In a
   container this means `--network host` / `hostNetwork: true` (or macvlan); on a VM,
   bridged (not NAT) networking; on Kubernetes also the `nodeSelector` to the right node.
2. Does the host firewall allow UDP 5353 and 3702 in and out, and IGMP? Enterprise
   switches may have *IGMP snooping* / *multicast filtering* / *client isolation* on the
   printer VLAN; ask the network team.
3. Run an **active scan** of the printer CIDR instead (Discovery → targets). If that
   finds the printers, the devices are fine and multicast is blocked. If it does not,
   check that UDP 161 reaches them and that the SNMP community matches (see below).
4. Zero-config run with `PM_LOG_LEVEL=debug`: the log shows the interfaces joined and
   each answer received.

**A device shows "not identified".**
The address answered the probe but no driver recognised a printer: it is probably not
a printer (a switch, a NAS with SNMP, a print server box, or a PC), or SNMP answered
only the system group with the Host Resources/Printer MIB disabled. Check the record's
capability profile; if IPP is expected, make sure 631/443 are reachable and that the
device uses one of the `poll.ippEndpoints` paths (some vendors use a different path —
add it).

**SNMP timeouts / "device offline" while the printer is up.**
The community or SNMPv3 credentials do not match (printers silently drop requests
with a wrong community, which looks exactly like a timeout), SNMP is disabled on the
device, SNMP is restricted to a management-station IP, or a firewall blocks UDP 161.
Add the correct credential under **Credentials** with a priority that puts it first
for that device (the dialer uses the first credential that answers). Only raise
`poll.snmpTimeout` for genuinely slow WAN links; raising it fleet-wide multiplies the
cost of every offline device.

**Traps never arrive.**
Printers must be configured with the server's address *and* port (1162 unless you
arranged 162), the path must not NAT the source address (a Docker published port or a
Service without `externalTrafficPolicy: Local` does), the firewall must allow the UDP
port in, and the device must already be known to PrintMarshal (traps from unknown
addresses are ignored). `trap listener stopped` in the log means the bind failed.

**`database is locked` / `SQLITE_BUSY` / corruption.**
The data directory is on NFS/SMB, or two server processes share one database. Use
local/block storage and one instance per data directory (`Recreate` strategy, no
second container on the same volume).

**Container restarts in a loop.** Check `docker logs` / `kubectl logs --previous` for
the `fatal:` line: usually a `/data` not writable by uid 10001 (bind mount ownership),
`PM_SECRET_KEY` not valid base64, or a port already in use with host networking.

**UI loads but every API call is 401 after moving behind HTTPS.** The cookie was set
`Secure` (`PM_SECURE_COOKIES=true`) but the browser still reaches the server over plain
HTTP, or the proxy strips the `Cookie` header on the WebSocket upgrade.

**Discovery/polls hit the wrong port with the simulator.** The simulator listens on
1161/6631; set `PM_SNMP_PORT=1161` and `PM_IPP_ENDPOINTS='http://%s:6631/ipp/print'`
(README "Quick start").

## Security hardening checklist

- [ ] **TLS in front**: terminate HTTPS at a reverse proxy (nginx, Caddy, Traefik) or
      Ingress, forward to `127.0.0.1:8080` / the ClusterIP, keep WebSocket upgrade on.
- [ ] **`PM_SECURE_COOKIES=true`** once TLS is in place (cookies are already `HttpOnly`
      and `SameSite=Strict`).
- [ ] **Secrets from the platform**: `PM_SECRET_KEY` and `PM_ADMIN_PASSWORD` from a
      Kubernetes Secret, a 0600 `env_file`, or the systemd `EnvironmentFile`; never in
      the image, the ConfigMap, or a committed compose file. SMTP passwords only via
      `passwordEnv`.
- [ ] **Change the initial admin password** at first login; remove `PM_ADMIN_PASSWORD`
      from the environment afterwards.
- [ ] **Least privilege for users**: viewers for dashboards, operators for day-to-day
      work, admins only for user/credential management and deletions.
- [ ] **Least privilege for the process**: non-root (the image, the unit and the
      manifests already are), read-only root filesystem, all capabilities dropped
      except `CAP_NET_BIND_SERVICE` when you need port 162, seccomp `RuntimeDefault`.
- [ ] **Firewall**: 8080 only from operator networks (or only from the proxy),
      162/1162 only from printer subnets.
- [ ] **SNMPv3** with authPriv where printers support it; treat communities as
      passwords and give each site its own.
- [ ] **Data directory permissions** 0700, `secret.key` 0600, backups encrypted at rest
      and access-controlled (they contain the key).
- [ ] **Keep writes gated**: configuration and firmware writes are dry-runnable and
      audited; restrict operators, review the audit log, and keep
      `PM_FIRMWARE_SIM_TARGETS` unset in production.
- [ ] **Pin and verify images**: deploy by digest or immutable tag; rebuild on base
      image CVEs (`apk` packages: `ca-certificates`, `tzdata`, `sqlite`).
- [ ] **Session lifetime**: lower `auth.sessionTtl` (default 30 days) for shared
      workstations.

## Sizing

Work is dominated by polling: every `poll.interval` each active device gets one SNMP
pass (system group, Host Resources and Printer MIB walks — typically 30–150 requests)
and one IPP `Get-Printer-Attributes`, bounded by `poll.concurrency` simultaneous
devices. A healthy LAN printer takes roughly 0.5–2 s; an **offline** device costs up to
`snmpTimeout × (snmpRetries + 1)` per request before the driver gives up, and the IPP
endpoints another `ippTimeout` each, so budget ~10 s per unreachable device. CPU use is
small (protocol parsing); memory is a ~60 MB baseline plus on the order of 100 KB per
device; disk grows with telemetry (roughly 1–2 KB per device per poll until
`telemetry.retention`, 90 days by default).

Rule of thumb — one poll cycle must finish well inside the interval:

```
cycle ≈ ceil(devices / concurrency) × per-device time   (use 10 s if many devices may be offline)
```

| Fleet | Suggested settings | CPU / memory | Disk (90 d telemetry) | Deployment |
| --- | --- | --- | --- | --- |
| ≤ 100 devices | `interval 5m`, `concurrency 32`, `jobs.workers 4` | 1 vCPU, 256 MB | 1–2 GB | Docker/Compose on any host |
| 100–500 | `interval 5m`, `concurrency 64` | 1–2 vCPU, 512 MB | 5–10 GB SSD | Compose, Kubernetes, or systemd |
| 500–2 000 | `interval 10m`, `concurrency 96–128`, `snmpTimeout 1s` | 2 vCPU, 1 GB | 20–40 GB SSD | dedicated Linux host or a pinned Kubernetes node |
| 2 000–5 000 | `interval 15m`, `concurrency 128–192`, `jobs.workers 8`, `telemetry.retention 720h` | 4 vCPU, 2–4 GB | 40–80 GB SSD | dedicated host per region/site; one instance per L2 segment if multicast discovery is needed |

Guidance:

- Worked example: 1 500 devices, 5 % offline, concurrency 96 → 16 batches × ~2 s for
  the healthy ones plus 75 offline devices × 10 s / 96 ≈ 40 s ⇒ ~70 s per cycle; a
  10-minute interval leaves ample headroom for on-demand polls and jobs.
- Raise `poll.concurrency` before shortening timeouts; 128 concurrent SNMP sessions
  are negligible for the server but check that the network path (firewalls doing
  connection tracking on UDP) can take it.
- Discovery scans are separate: `scanWorkers` (64) parallel probes, so a /16 active
  scan (65 k addresses, which needs the "allow large" flag) takes a few minutes at 1 s
  timeout. Scan at most the subnets printers live in.
- Faster than `interval 1m` rarely helps: traps deliver urgent events immediately
  (configure them), and supplies change slowly.
- SQLite on a local SSD comfortably handles this range on one instance. For the
  largest tiers, or when the state must live on a managed database rather than one
  node's disk, switch to [PostgreSQL](#postgresql); the process itself still runs as
  one active instance (scheduler, traps and jobs are in-process), so split by site
  rather than by cluster. Measure the real database growth after a week
  (`du -sh <dataDir>` or `pg_database_size`) and adjust `telemetry.retention`.
- The resource defaults in the manifests (requests 100m/128Mi, limit 512Mi) match the
  first two rows; raise the memory limit to 1–2 GiB for the larger tiers.
