# Spec: onboard printers from a CSV file

From `intent.md` (accepted in PR 60). Status: draft.

## Decisions proposed for the intent's open questions
1. **A row with a serial but no address updates a matching device and is otherwise skipped**
   ("no address; scan first"). Creating a device that has no address would make a record the
   poller cannot reach and the list cannot explain; the row is reported so the administrator can
   add the address and import again.
2. **"Replace existing values" is per column**, a checkbox in the preview's column header,
   default off. One switch for the whole file would make "fill in locations but keep the names
   people chose" impossible, which is the common case.
3. **The console's own export format is accepted.** The export's column names are header
   aliases, so an export from one instance imports into another; columns the import does not
   know are ignored and listed in the preview as "ignored columns".
4. **Field sources live on the device as a small map**, `fieldSources` (field → source and
   time), stored in a new JSON column and written by the import ("import"), by operator edits
   ("operator"), and by DNS sync ("dns"); group inheritance adds "group:<id>" later. This is the
   smallest record that lets the device page say where a value came from, and it is the seed the
   provenance work generalises rather than a second mechanism.

## Requirements

File and columns
- R1. The import accepts a UTF-8 CSV with an optional byte-order mark, comma or semicolon
  separated (detected from the header line), at most 5,000 data rows and 2 MB. Larger files are
  refused before parsing with the limit in the message.
- R2. The header names columns; matching is case-insensitive and ignores spaces, underscores
  and hyphens. Known columns: `address`, `hostname`, `serial`, `displayName`, `location`,
  `assetTag`, `notes`, `webUrl`, `dnsSync`, `state`, `group`, plus the export's names for the
  same fields as aliases. Unknown columns are ignored and named in the preview. A file with no
  known key column (`address`, `hostname` or `serial`) is refused.
- R3. Each row needs an address, or a hostname the console can resolve to one (one forward
  lookup per row, in the preview, bounded by the reverse-DNS timeout), or a serial that matches
  an existing device. Otherwise the row is skipped with "no address" (decision 1).
- R4. Values are validated as the API validates them today: location and asset tag up to their
  limits, notes up to the bulk-edit limit, `webUrl` by `model.ValidWebURL`, `dnsSync` as
  true/false/yes/no/1/0, `state` as a lifecycle state name, `group` as the exact name of an
  existing group. An invalid cell skips the row with the cell named ("row 12: webUrl is not a
  URL"); it never partially applies a row.
- R5. Duplicate keys within the file (same address, or same serial) skip every copy after the
  first, with "duplicate of row N".

Preview
- R6. `POST /api/discovery/import` with `mode=preview` returns, without changing anything and
  without contacting any device: per row its number, the key used, the outcome (`create`,
  `update`, `skip`), the fields that would be set, the fields that would be left because an
  operator set them (unless the column is marked replace), and the skip reason; plus totals
  per outcome, the ignored columns, and the detected separator. Operators and administrators
  may preview.
- R7. Matching for `update` is by address first, then by serial; a row whose address and serial
  both match but on different devices is skipped with "address and serial belong to different
  devices".
- R8. The preview is deterministic for the same file and inventory, so the page can show it and
  the apply can recompute it; no server-side preview state is kept.

Apply
- R9. `mode=apply` is administrator-only and takes the same file plus the per-column replace
  flags. It submits a job of type `import` and returns the job; the job recomputes the plan
  (R8), then for each row: `create` upserts a device from the row's address and hostname through
  the existing `IdentifyAndUpsert` path with the row's operator fields applied, and the device is
  queued for identification exactly as a discovered one; `update` patches only the supplied
  fields, respecting the replace flags; `skip` records the reason. One job result per row.
- R10. An empty cell leaves the field untouched (never writes an empty string). A supplied value
  on a field an operator set is left alone unless that column is marked replace; the preview
  showed this in advance (R6).
- R11. Every applied field records its source as "import" with the time (decision 4); an
  operator edit later records "operator", and the device page shows "from import, 31 Oct" beside
  such fields.
- R12. Group scope: rows whose device or target group is outside the caller's scope are skipped
  with "outside your groups"; the preview says so too. A `group` cell adds the device to that
  group's membership; it never removes memberships.
- R13. Audit: one `devices.import` entry with the counts, the replace flags and the file's row
  count (never its content), and the existing per-device `device.patch` entries for updates
  and `device.created` for creations.
- R14. The job is cancellable like other jobs; rows already applied stay applied and the result
  list says where it stopped.

Page
- R15. Discovery gets "Import CSV…" beside "Run discovery", for operators and administrators.
  The dialog: choose a file, the preview table (row, key, outcome, fields, reason) with the
  per-column replace checkboxes in its header and the totals above it, a link to download a
  header-only template, and "Import N printers" (administrators only; operators see the preview
  and a note that an administrator applies it). After apply, the job appears in the runs list
  like a discovery run and its results read like the preview with final outcomes.
- R16. The template and the preview use the console's words (sentence case, "Display name",
  "Asset tag"); error rows carry the exact reason from R4 and R5.

## Design
- **Packages.** `internal/importcsv` (new, standard library only): `Parse(r io.Reader) (Table,
  error)` with header aliasing and separator detection; `Plan(table, lookup) (Plan, error)` as a
  pure function over an interface that answers "device by address", "device by serial", "group
  by name", "resolve hostname"; the plan is what preview returns and apply executes.
  `internal/api/import.go` for the route and the job handler registration; `internal/fleet` gains
  nothing new beyond calling `IdentifyAndUpsert` and `PatchDevice`; `internal/store` gains
  `FieldSources` on `Device` and `DevicePatch`, a migration adding `field_sources TEXT NOT NULL
  DEFAULT '{}'` to `devices` (the next migration number), and `ListDevices` filters by serials
  if not present.
- **Routes.** `POST /api/discovery/import` (multipart: `file`, `mode`, `replace` as a
  comma-separated column list), operator for preview, administrator for apply; the job type
  `import` runs through `jobs.Runner` with one `Result` per row.
- **Model.** `Device.FieldSources map[string]FieldSource` with `Source string` and `At time.Time`,
  `omitempty`; set by the import, by `patchDevice`/bulk ("operator") and by the DNS sync rename
  ("dns"). Read-only elsewhere.
- **UI.** `web/src/pages/Discovery.tsx` plus a new `web/src/components/ImportDialog.tsx`; the
  device page shows the source beside operator fields when present.
- **Wiki.** `Discovery-and-inventory.md` (a "Import from a CSV file" section with the template
  and the rules), `API-reference.md` (the route, modes, the plan shape), a line in
  `Configuration-reference.md` if a limit becomes configurable (it is not, in this version).

## Flagged concerns
- **Identification of created rows contacts devices** with the existing read-only probe; that
  is discovery's behaviour, unchanged, and the preview never does it.
- **The field-source map is the first provenance record.** It must stay small and generic
  (source string plus time) so the provenance intent can extend it without a second migration of
  shape; the review should hold the line against adding import-specific fields to it.
- **Replace flags and group scope interact:** a scoped administrator may replace fields only on
  devices in scope; the plan computes scope first (R12), then replace.
- **Large previews in the browser:** 5,000 rows render as a virtualised or paged table; the
  first version pages at 200 rows.

## Out of scope
Creating groups from the file; a tags column (no tags on devices yet); a credentials column;
scheduled or repeated imports that sync with the file; importing supplies or counters; the
print-server and print-management producers (they later call the same `Plan`).
