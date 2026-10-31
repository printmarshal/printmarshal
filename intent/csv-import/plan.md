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
