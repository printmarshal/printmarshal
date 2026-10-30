# Get Started & Simulator Guide (`site/get-started/index.html`)

## H1: Evaluate locally with the built-in simulator.

1. **Run PrintMarshal against `pm-sim` on loopback (`#simulator`)**
   - Build with Go 1.27+ and Node 22+ (`make web && make build`).
   - Start `pm-sim` on `127.0.0.1:19261` (SNMP) and `127.0.0.1:19231` (IPP).
   - Persist `/tmp/pm-eval/secret.key` once (`[ -f /tmp/pm-eval/secret.key ] || (umask 077 && openssl rand -base64 32 > /tmp/pm-eval/secret.key)`) and start `printmarshal` on `127.0.0.1:19280` with `PM_DISCOVERY_ENABLED=false` and `PM_TRAP_ENABLED=false`.
   - With both processes bound to `127.0.0.1` and zero-config multicast unchecked during discovery, discovery and polling traffic stays on loopback.

2. **First-Success Checklist (`#first-five`)**
   - Discover `127.0.0.1` (zero-config unchecked) → Poll supplies & counters → Run Baseline assessment → Preview configuration diff in dry-run → Apply, verify read-back, and test one-click revert.

3. **Docker & Single-Binary Deployment (`#deploy`)**
   - `Dockerfile`: `node:22-alpine` → `golang:1.27-alpine` (`CGO_ENABLED=0`) → `alpine:3.23` running as `pm` (`UID 10001`).
   - **Docker key handling:** the example uses a named volume (`-v pm-data:/data`) and no `PM_SECRET_KEY`, so the application creates `/data/secret.key` itself (0600, owned by `pm`) and reuses it on every restart or recreation; no host `chown`/`sudo`. Bootstrap password via `docker exec printmarshal cat /data/initial-admin-password`. Key backup via `docker run --rm --entrypoint cat -v pm-data:/data:ro ...` under `umask 077`. Host notes cover bind mounts (privileged chown to 10001), supplying the same `PM_SECRET_KEY` on every start, and host networking.
   - **Preserve and reuse `PM_SECRET_KEY` (`openssl rand -base64 32`):** binary examples create `secret.key` only if it does not already exist so repeating the command against existing data never replaces the encryption key.
   - Online backups use `deploy/scripts/backup.sh` (`/usr/local/bin/pm-backup`), which runs `sqlite3 "VACUUM INTO ..."` or `pg_dump -Fc`.

4. **Environment Variables & Ports (`#env-reference`)**
   - Matches `internal/config/config.go`: `PM_LISTEN`, `PM_DATA_DIR`, `PM_DATABASE`, `PM_DSN`, `PM_SECRET_KEY`, `PM_ADMIN_PASSWORD`, `PM_SECURE_COOKIES`, `PM_DISCOVERY_ENABLED`, `PM_DISCOVERY_INTERVAL`, `PM_DISCOVERY_TARGETS`, `PM_TRAP_ENABLED`, `PM_TRAP_LISTEN`, `PM_SNMP_PORT`, `PM_IPP_ENDPOINTS`.
