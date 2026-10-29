# Development

[Home](Home.md) · [Architecture](Architecture.md) · [Drivers](Vendor-plugins-and-drivers.md) · [API](API-reference.md)

PrintMarshal is a Go module with a React/TypeScript SPA embedded into the server. The module currently requires **Go 1.27.3** (`go.mod`). The Docker build uses Go 1.27 and Node 22; the frontend uses React 18, TypeScript 5.7 and Vite 6 (`web/package.json`). Use the lockfile for reproducible frontend dependencies.

## Build and run

From a development checkout, the Makefile's full artifact flow is `make all`: install locked frontend dependencies/build the SPA, then build server and simulator. The separate steps below show the same order. They change build outputs and are intended for your own development environment; none were executed to prepare this wiki.

```sh
cd web
npm ci --no-audit --no-fund
npm run typecheck
npm run build
cd ..
go build -o bin/printmarshal ./cmd/printmarshal
go build -o bin/pm-sim ./cmd/pm-sim
```

Vite writes into `internal/web/dist` with `emptyOutDir:true`; `internal/web/embed.go` embeds those files. A backend-only build embeds whatever is already there, which may be the placeholder rather than the full UI. `-ldflags '-X main.version=VERSION'` sets the server version; otherwise it is the source default. `make build` also builds the simulator. Docker builds the SPA before embedding it.

For frontend development, `cd web && npm run dev` starts Vite on port 5173 and proxies `/api` (including WebSocket) and `/healthz` to `http://localhost:8080`. Start a separately configured backend on 8080. The development proxy does not list `/readyz` or `/metrics`. See `web/vite.config.ts`.

Start a server with a dedicated data directory and strong bootstrap password; avoid production networks while developing. Discovery and scheduled polls can contact devices already stored in that directory. Read [configuration](Configuration-reference.md) before enabling discovery, traps, syslog, authentication providers or exporters.

## Tests and offline boundaries

| Check | Command / behavior |
| --- | --- |
| Compile | `go build ./...` |
| Static checks | `go vet ./...` (also part of `make test`) |
| Unit/integration suites | `go test ./...`; many use local test servers and temporary databases |
| SPA types | `cd web && npm run typecheck` |
| SPA production build | `cd web && npm run build` |
| Simulator smoke test | `scripts/e2e.sh` or `scripts/e2e.sh 19000` |
| PostgreSQL conformance | `PM_TEST_PG_DSN=... go test ./internal/store/` |

Leave `PM_LIVE_HP` unset to skip the opt-in live HP test (`internal/plugins/hp/live_test.go`). Leave `PM_TEST_PG_DSN` unset to skip external PostgreSQL. Tests using loopback HTTP/LDAP/RADIUS/SAML/CA fixtures still create local network listeners and connections; an absolute no-network environment must not execute them. Debug commands are active probes/recorders, not passive source inspection.

`scripts/e2e.sh` builds both binaries into a temporary directory, starts SNMP/IPP simulator endpoints on loopback at base+1/base+2 and a server at base (default 19000), signs in, discovers `127.0.0.1`, waits for the job and asserts inventory, warning supply alert and CSV. It kills processes/removes temporary data on exit. Its server environment does not explicitly disable every scheduled/discovery default; use it in an isolated environment, not as authorization to probe hardware.

## PostgreSQL conformance

`internal/store/store_conformance_test.go` defines the shared `runStoreTests` contract. `TestSQLite` always runs it; `TestPostgres` uses `PM_TEST_PG_DSN`. The PostgreSQL account needs permission to create schemas in an isolated test database. Each test creates `pm_test_<time>_<sequence>`, sets `search_path`, and drops the schema with `CASCADE` in cleanup. Never use production credentials or a production database.

Example DSN for a separately provisioned local test database:

```sh
PM_TEST_PG_DSN='postgres://postgres:pm@127.0.0.1:55432/pm?sslmode=disable' go test ./internal/store/
```

Do not treat a skipped PostgreSQL suite as backend conformance. The local test DSN's disabled TLS is for loopback testing, not a deployment recommendation.

## Repository map

| Path | Responsibility |
| --- | --- |
| `cmd/printmarshal` | composition, bootstrap, scheduler and process lifecycle |
| `cmd/pm-sim` | fixture printer, IPP configuration/firmware hooks |
| `cmd/pm-debug` | protocol inspection and recording utilities |
| `internal/config`, `internal/model` | process configuration, API/domain types and capability constants |
| `internal/engine`, `internal/driver`, `internal/plugins` | capability registry/dispatch, standards and vendor transports |
| `internal/discovery`, `internal/trap`, `internal/fleet` | discovery, inbound traps, inventory/poll orchestration and runtime settings |
| `internal/jobs`, `internal/events`, `internal/alerts` | workers, event contract, evaluation and notification outbox |
| `internal/configmgmt`, `internal/security`, `internal/firmware` | device-changing workflows and verification |
| `internal/consumables`, `internal/catalog`, `internal/reports` | stock, evidence catalogue, analytics |
| `internal/auth`, `internal/secrets` | local/external identity, tokens, encrypted material |
| `internal/store` | SQLite/PostgreSQL, migrations, feature interfaces and conformance tests |
| `internal/siem`, `internal/printprovider` | exports/ingest and optional local CUPS adapter |
| `internal/api`, `internal/web` | HTTP/WebSocket handlers and embedded SPA |
| `web/src` | React pages, components and frontend API/types |
| `testdata/fixtures` | sanitized protocol recordings and simulator data |
| `deploy`, `scripts` | packaging, backups, deployment templates, e2e script |
| `docs`, `research` | product documentation, design specification and recorded evidence/reviews |

## Extending the store

`store.Store` is the full persistence contract, including embedded `ConfigStore`, `SecurityStore`, `FirmwareStore`, `OutboxStore`, `CatalogStore`, `ReportStore`, `TokenStore`, `LogStore` and `WriteCredentialStore`. A new backend must satisfy all of them, not only inventory. Keep a feature interface in `store_<feature>.go`, domain shapes in `internal/model`, and parallel implementations in `sqlite_<feature>.go` and `postgres_<feature>.go`.

Schema fragments are embedded and applied during open. Add idempotent forward migrations for existing databases; adding a field only to a `CREATE TABLE IF NOT EXISTS` statement does not alter an installed table. SQLite versions 1–5 and PostgreSQL versions 1–4 are described in [deployment](Installation-and-deployment.md). PostgreSQL uses advisory locking and native timestamps/JSONB/booleans/bytes; preserve semantics rather than copying SQLite syntax. Use query parameters, transactions for coupled updates, and normalized UTC cursors. Keep identity conflict rules, stock movement atomicity, outbox uniqueness and secret encryption consistent across both stores.

The SQLite implementation has one database connection. Close rows before another query/transaction needs the connection; never keep a streaming result open across nested store calls. Extend shared conformance cases for actual persistence behavior (ordering, conflicts, restart, transaction outcomes) and keep dialect-specific migration tests separate. Source examples: `store_conformance_test.go`, `store_credentials.go`, `sqlite_timestamps_test.go`.

## Contribution boundaries

The concrete conventions in `docs/adding-a-driver.md`, `docs/decisions.md` and the feature tests are: keep vendor knowledge in plugins; declare only real capabilities; sanitize fixtures; keep secrets out of persisted snapshots and API output; make device writes dry-run by default, purpose-bound, audited and verified; implement store behavior for both backends. Update the event catalogue and route/config documentation alongside new producers or options. There is no runtime plugin upload/install API.

The original specification remains useful for intended product boundaries, but implement against current interfaces and tests. A catalogue entry or vendor tool compatibility claim is not a wire-level transport contract. There is no claim here that historical live-device evidence establishes broad vendor support.

## Known discrepancies

`docs/decisions.md` contains superseded phase-1 statements (PostgreSQL reserved, no vendor plugins, eSCL deferred, SSO later, bootstrap password logged). Follow current startup and implementations. PostgreSQL records the same migration version numbers as SQLite (currently 1–5) even where a version is a no-op for it, so later migrations number the same on both backends; `TestPostgres/MigrateIsIdempotent` asserts that count.

This documentation task ran source/link/schema inspections only, not builds, tests, simulators or network probes. The commands above describe development workflows rather than test results for this checkout.
