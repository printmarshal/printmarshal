# Plan: onboard printers from a CSV file

From `spec.md` (accepted in PR 61). Status: draft. Requirement numbers (R1…R16) are the spec's.

## Files that change
Server
- `internal/importcsv/parse.go` (new): `Parse(r io.Reader, maxRows, maxBytes) (Table, error)`
  with BOM stripping, separator detection from the header line (comma or semicolon, whichever
  splits it into more cells), header normalisation (lower-case, strip spaces, underscores,
  hyphens) and the alias table (spec R2, plus the export's column names read from
  `internal/api/export.go`'s header at build time of the table, kept as a literal list with a
  test that compares it to the export). Unknown columns are kept by name for the preview.
- `internal/importcsv/plan.go` (new): `Plan(t Table, lk Lookup, opts Options) PlanResult` as a
  pure function. `Lookup` is an interface: `DeviceByAddress`, `DeviceBySerial`, `GroupByName`,
  `ResolveHostname`, `InScope(deviceID or groupID)`. `Options` carries the replace columns and the
  caller's role. Each `Row` result: number, key, outcome, fields to set, fields held back and
  why, skip reason. Validation per R4 reuses the API's limits: export `bulkMaxLocation`,
  `bulkMaxNote` and an asset-tag limit from `internal/api` into a small `internal/model/limits.go`
  so both packages share them (one move, no behaviour change).
- `internal/importcsv/plan_test.go`, `parse_test.go` (new): table-driven, written first.
- `internal/model/model.go`: `FieldSource{Source string; At time.Time}` and
  `Device.FieldSources map[string]FieldSource` (`json:"fieldSources,omitempty"`);
  `internal/store/store.go`: `DevicePatch.FieldSources map[string]FieldSource` (merge, never
  replace the map), `DeviceFilter.Serials []string`; `internal/store/sqlite.go` migration 9
  (`ALTER TABLE devices ADD COLUMN field_sources TEXT NOT NULL DEFAULT '{}'`, with the existing
  "skip if present" pattern from migration 5) and the same for `postgres.go` in its own
  numbering; read and write of the column in the device scan and patch.
- `internal/api/import.go` (new): `POST /api/discovery/import` (multipart, `file`, `mode`,
  `replace`), size cap by `http.MaxBytesReader` (2 MB), operator for preview, admin for apply;
  a `storeLookup` implementing `importcsv.Lookup` over the store, the fleet's resolver and the
  caller's scope (`scope.go` helpers); the job handler `runImport` registered as job type
  `import` on the runner (beside `discovery` and `poll` in `fleet/service.go`, or from the API
  package at `Routes` time, whichever the runner allows without an import cycle); per row:
  `create` → `s.Fleet.IdentifyAndUpsert(ctx, address, []string{"import"}, Candidate{Address,
  Hostname})` then `PatchDevice` with the row's fields and sources; `update` → `PatchDevice` with
  the allowed fields; `skip` → a result with the reason. Group membership through the existing
  members write. Audit entries per R13. Cancellation via the job context.
- `internal/api/import_test.go` (new): handler tests, written first: preview never calls the
  fleet (a `Server` with `Fleet: nil` previews), operator cannot apply (403), admin apply
  creates and updates with field sources set, replace flag honoured, group out of scope skipped,
  duplicate rows skipped, a 2 MB+1 file refused, the job lists under the runs endpoint.
- `internal/api/server.go`: route registration; `listDiscoveryRuns` returns `discovery` and
  `import` jobs merged by creation time (two `ListJobs` calls, no store change); `patchDevice`
  and `bulkPatchDevices` set `FieldSources[field] = {operator, now}` for each field they change;
  the DNS sync rename in `fleet/rdns.go` sets `displayName → dns`.
- `internal/api/export.go`: no change, but the import's alias test reads its header.
  *Built:* the device export's header is written by `exportDevicesCSV` in `internal/api/server.go`
  (`export.go` is the SIEM export). `importcsv.ExportHeader` keeps it as a literal list;
  `importcsv`'s alias test checks every column is known or listed in `ExportIgnored`, and
  `internal/api/import_test.go` checks the list equals the header the export writes.
- *Built, step 1:* no asset-tag limit existed in `internal/api`; `internal/model/limits.go` holds
  `MaxLocation` and `MaxNote` (moved from the bulk edit, same values) plus `MaxAssetTag` and
  `MaxDisplayName` (200 each, applied by the import only). `Lookup.InScope` is two methods,
  `DeviceInScope` and `GroupInScope`. A state the lifecycle cannot reach from the device's current
  state (a new device is "discovered") is held back with "cannot move from X to Y" rather than
  skipping the row; a scoped caller's create row needs a `group` cell in its scope.
- *Built, step 2:* `DeviceFilter.Serials` is not added: the import's lookup loads the inventory
  once per plan (devices including decommissioned, and groups) and matches in memory.
  `DevicePatch.Empty()` replaces the bulk edit's `patch != (DevicePatch{})`, which no longer
  compiles once the patch holds a map. A field with a recorded source counts as set by someone
  for the held-back rule (narrowed by review fix 3, below), so DNS sync turned off or a state left
  at "discovered" on purpose by an operator is kept too. Migration 9 on both stores (SQLite `TEXT`, PostgreSQL `JSONB`, default `'{}'`).
- *Built, step 3:* the parsed file waits in memory (`Server.imports`, under a random key in the
  job's params) until the job takes it, so no file content is persisted; a restart fails the job
  as usual. The job type is registered in `Handler()`, which runs after the scheduler has started,
  so `jobs.Runner` now guards its handler map; the runner also counts a result of status `failed`
  as a failure, so an import with failed rows ends `partial`. `fleet.LookupHostAddress` is new
  (the forward lookup; the resolver is unexported). `peekBody` in `scope.go` truncated any body
  over 1 MB for a scoped principal; it now puts the whole body back. `Server.importIdentify` is the
  test seam that keeps tests from probing. Field sources are also written by `renameDevices` and
  `setDeviceState`, the other two operator write paths, beyond the plan's list. There is no
  per-job cancel API yet; the job stops when the runner's context ends (shutdown), writing one
  result that says where it stopped (`outcome: cancelled`) rather than one per remaining row.
- *Built, review fix 1:* a printer that does not answer is still created: on an identification
  error the row makes an address-only record through `UpsertDevice`, with the file's fields and
  sources, and reports `created`. `fleet.Service.PollDevice` identifies a record with no make,
  model, serial or capability profile before polling it, through the read-only `Engine.Identify`
  with the record's own credentials; a later discovery merges by address in `findByIdentity`.
  If the printer turns out to be recorded already under another address (matched by serial),
  that record takes the address and the address-only one stays for an administrator to remove.
- *Built, review fix 2:* the job identifies the create rows first (eight at a time, each bounded by
  60 seconds), then applies every row in file order on one goroutine, so results keep row order
  and the row writes are sequential. `Job.Done` therefore stays at 0 during identification. A stop
  ends new identifications; a printer already identified still gets its row applied, the job ends
  `failed` with "stopped at row N" as its error, and the audit entry says `cancelled`.
- *Built, review fix 3:* hold-back goes by source, each with its own reason: `import` is not held
  back, so an import may overwrite its own earlier value; `operator` says "set by an operator";
  `dns` says "kept in sync with DNS"; a non-empty value with no source says "already has a value",
  since it may be an old operator edit or seeded from `sysLocation`.
- *Built, review fix 4:* a `displayName` value equal to the device's current `Name()` is no change,
  so re-importing an export does not freeze a derived name. Create rows are unaffected.
- *Built, review fix 5:* `authorize` refuses a group-scoped principal's JSON body over the 1 MiB
  peek with 413 and an `auth.denied` entry, since the probe cannot have seen all of it.
- *Built, not in the spec's wording:* skip reasons carry no "row N:" prefix (R4's example), since
  the preview has a row column and job results carry the row in `data`. A non-IP address that
  matches a recorded device is accepted, since a device discovered by name is recorded under it.
  Joining a group writes a `group.members` audit entry beside R13's. R14: there is no per-job
  cancel API yet, so only a shutdown stops the job; where it stopped is a result with `outcome:
  cancelled`, an outcome beyond the plan JSON's create, update and skip.

Web
- `web/src/components/ImportDialog.tsx` (new): file input, "Download template" (a header-only
  CSV built client-side from the known columns), preview table paged at 200 rows with the
  per-column replace checkboxes in the header, totals, ignored-columns note, "Import N printers"
  for admins and the operator note (R15, R16).
- `web/src/pages/Discovery.tsx`: the "Import CSV…" button, the dialog, import jobs shown in the
  runs list with their type, results rendered with the row number and outcome.
- `web/src/pages/DeviceDetail.tsx`: "from import, 31 Oct" / "from DNS" beside operator fields
  when `fieldSources` has them (R11).
- `web/src/types.ts`: `FieldSource`, `fieldSources` on `Device`; `types-import.ts` (new) for the
  plan shape.

Docs
- `wiki/Discovery-and-inventory.md` ("Import from a CSV file": the template, the key rule,
  replace, what the preview shows, the limits), `wiki/API-reference.md` (the route, both modes,
  the plan JSON, the job type and results, `fieldSources` on devices).

## Order of work
1. **Parser and planner with tests first** (careful-tier worker): `internal/importcsv` tests
   written and failing, then the package; `go test ./internal/importcsv` green; commit each.
2. **Model, store, migration** (same worker): field sources end to end with a store test that
   round-trips the map on SQLite; `make test` green; commit.
3. **API, job and handler tests** (same worker, tests first, then `PM_FIX_TASK=1` for the fix
   step): commit tests, commit implementation.
4. **Web** (medium-tier worker, in parallel with step 3 once step 1 fixes the plan JSON shape):
   dialog, Discovery page, device page; typecheck and build; restore `internal/web/dist/index.html`;
   commit.
5. **Wiki** (light-tier worker, from the spec and the final JSON shape); commit.
6. **Verification** (verifier agent): the proof below on loopback against the simulated fleet.
Steps 3 and 4 run in separate worktrees; the API contract is the plan JSON from step 1 plus the
route shape in the spec.

## Risks
- **Migration on both stores.** SQLite and PostgreSQL migrations must both add the column and
  both scans must read it; the PostgreSQL path has no test on this machine, so the worker
  compares the two migration lists by eye and the review checks the diff for both files.
- **Riskiest step: field sources on existing write paths.** `patchDevice`, bulk and DNS sync all
  start writing sources; a missed path leaves a field without a source, which the device page
  then shows as nothing (harmless) but the provenance work later has to backfill. The test for
  R11 covers all three paths.
- **Hostname resolution in the preview** adds up to one lookup per row; capped by the reverse
  DNS timeout and done concurrently (8 at a time, as the rename lookup does); the preview for
  5,000 hostname-only rows could take minutes, which the spec accepts and the page shows a
  progress line for.
- **Runs list merge** changes the Discovery page's data shape slightly (jobs of two types); the
  page already keys on job id.
- **Not taken:** a server-side preview token (the plan is recomputed on apply, R8); creating
  groups from the file; a separate import page (it belongs with Discovery).

## Proof
- Step 1: `go test ./internal/importcsv -v` with the table tests failing, then passing; the
  alias test against the export header.
- Steps 2–3: `make test` with every package `ok`; the failing-then-passing handler tests.
- Step 4: `npm run typecheck` and `npm run build` clean.
- Step 6, on loopback: a copy of the simulated fleet's data on a free port (the 22 simulators on
  127.0.0.2–23 answer SNMP on 1161, so created rows identify as the real models); a CSV with
  (a) 10 existing addresses with new locations and asset tags, (b) 2 of them with display names
  already set by an operator and the display-name column not marked replace, (c) 3 new
  addresses from the alias range not yet in that copy, (d) one duplicate row, one bad URL, one
  unknown group, one hostname-only row that does not resolve; preview as operator (no device
  contact: the server log shows no SNMP activity during the preview), apply as admin, then: the
  job's results read as predicted, the device page shows "from import" beside the imported
  fields and keeps the two operator names, the three new devices identify as HP models within a
  minute, the audit trail has the `devices.import` entry with counts and no cell values, and the
  runs list shows the import beside discovery runs. Screenshots of the preview, the results and
  one device page.
