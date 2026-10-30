# PrintMarshal

Self-hosted printer-fleet security, configuration and monitoring console. Go server with an
embedded React SPA. We run the AI-native SDLC loop described in `docs/sdlc.md`: every change
starts from a committed `intent/<slug>/intent.md`, non-trivial ones get `spec.md` and `plan.md`,
PRs are reviewed against `REVIEW.md`, and the owner merges.

## Commands
- Build: `make build` (binaries in `bin/`; run `make web` first when the UI changed)
- UI: `cd web && npm ci && npm run typecheck && npm run build` (`npm ci` once per worktree;
  never symlink `node_modules` between worktrees, it empties the source checkout's copy)
- Test: `make test` (= `go vet ./... && go test ./...`; all packages must print `ok`)
- End to end: `scripts/e2e.sh <port-base>` (starts server + simulator on loopback)
- Eyeball the UI: `PM_LISTEN=127.0.0.1:<port> PM_DATA_DIR=<tmp> PM_ADMIN_PASSWORD=<test> ./bin/printmarshal`
- Site: `cd site && python3 -m http.server <port> --bind 127.0.0.1`

## Verifying your work
Run `make test` and, for UI changes, `cd web && npm run typecheck` before reporting a task
complete, and paste the output. For UI or site changes take a screenshot and look at it. If a test
fails, fix the code, not the test. For a bug fix, write the failing test first and commit it
before the fix (set `PM_FIX_TASK=1` so the hook blocks test edits during the fix).

## Conventions
- Go 1.27, standard library first; the module is `printmarshal`. React + TypeScript in `web/`.
- Every device write is dry-run first, audited, read back and reported as verified / diverged /
  unverifiable. Discovery and polling never write. See the `device-write-safety` skill.
- "Unknown" is never "pass": a setting a device will not reveal is reported as unknown and left
  out of the score denominator.
- Operator-set fields (display name, location, asset tag, notes) are never overwritten by scans.
- Work on a branch in a worktree; push it; the owner opens and merges the PR. Never push to
  `main`, never force-push, never rewrite history. Commits are authored
  `greatkemo <greatkemo@users.noreply.github.com>` and end with a `Co-Authored-By` trailer.
- `internal/web/dist/index.html` is tracked: after `make web`, restore it with
  `git checkout -- internal/web/dist/index.html` before committing unless the UI change is the
  point of the commit.
- Brand: the Steel & Signal palette and the layered-MFP icon with the lowercase wordmark; see
  `brand/README.md` and the `steel-and-signal-brand` skill. Brass is not an identity colour.

## Architecture
`cmd/printmarshal` (entry), `internal/engine` (capability model, driver registry, identify
preflight), `internal/driver/{snmp,ipp,escl}`, `internal/plugins/*` (vendor plugins on
`vendorkit.Base`), `internal/fleet` (discovery runs, polls, alerts), `internal/store`
(SQLite/PostgreSQL behind one interface, numbered migrations), `internal/api` (REST +
WebSocket), `internal/auth`, `internal/configmgmt`, `internal/security`, `internal/firmware`,
`internal/events`, `web/src` (SPA). `docs/decisions.md` records why; `docs/adding-a-driver.md`
explains plugins.

## Things Claude gets wrong
- Sending anything but reads to a real printer. The lab printer at 10.113.1.205 is read-only;
  writes go to `cmd/pm-sim` only. The Bash hook blocks `snmpset`/`ipptool` to non-loopback.
- Publishing product-direction material. Plans, pricing, tiers, naming and company matters
  never enter this repo, the wiki or the site; they live in the owner's private research repo.
  Public `intent/` and `research/` hold engineering material only.
- Leaving concept notes with naming or roadmap asides in `brand/concepts/`.
- Editing generated output by hand: `internal/web/dist/`, `brand/png/`, `brand/icons/png/`
  come from `make web`, `brand/tools/gen.py` and `brand/tools/export_icons.sh`.
- Bumping dependency versions as a side effect; `go.mod`/`go.sum` and `web/package-lock.json`
  change only when the task is the upgrade.
