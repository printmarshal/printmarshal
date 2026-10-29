# PrintMarshal — Get Started (`site/get-started/index.html`)

## Reconciled Prerequisites
- **Source build:** `Go 1.27.3+` (matching `go.mod`) and `Node.js 20+` (for `make web` UI build).
- **Container build:** Docker (`Dockerfile` uses `node:22-alpine` and `golang:1.27-alpine`, running on `alpine:3.20` as UID `65532`).
- **Database:** Embedded pure-Go SQLite (WAL mode) or external PostgreSQL 14+.

## Recommended Evaluation Route: Built-in Printer Simulator (`pm-sim`)
1. Clone and build: `make web && make build`
2. Start `bin/pm-sim` on loopback (`127.0.0.1:10161` SNMP, `127.0.0.1:10631` IPP) with `testdata/fixtures/generic-laser.snmp`.
3. Start `bin/printmarshal` on `127.0.0.1:8080` pointed at those loopback ports.
4. First successful outcome: sign in, discover `127.0.0.1`, run the `Baseline` security policy assessment, and preview + verify a dry-run configuration change.

## Production Deployment Options
- Single Docker container with `/data` volume (`PM_SECRET_KEY`, `PM_ADMIN_PASSWORD`).
- Docker Compose + PostgreSQL 16 (`deploy/docker-compose.yml`).
- Kubernetes Helm chart (`deploy/helm/printmarshal`) and Linux `systemd` service (`deploy/systemd/printmarshal.service`).
- Single active server process per database; always back up `PM_SECRET_KEY` (`secret.key`) alongside SQLite (`POST /api/system/backup`) or PostgreSQL backups.
