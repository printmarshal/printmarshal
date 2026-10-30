# Vendor plugins and drivers

The core dispatches capabilities rather than manufacturer names. A driver is compiled into the binary; there is no dynamic shared-library loader. Standards drivers are the baseline, vendor plugins are ordinary drivers that enrich the same profile. Registration is in `cmd/printmarshal/main.go` and `internal/plugins/all.go`.

## Driver contract

A driver serves one or more *capabilities* for devices (spec §6). The two standards
drivers (`internal/driver/snmp`, `internal/driver/ipp`) are the baseline; a vendor
plugin is just another driver that talks to a manufacturer's device web service.

1. Implement `engine.Driver`:

   ```go
   type Driver interface {
       Name() string
       Capabilities() []model.Capability
       Supports(d *model.Device, cap model.Capability) bool   // no network I/O
       Priority(cap model.Capability) int                     // higher wins
   }
   ```

   `engine.ProfileSupports(d, name, cap)` is the standard `Supports` rule: serve the
   capability when the device's profile assigned it to you, or when nobody serves it
   yet and you identified the device.

2. Implement the typed interface for each capability you declare: `engine.Identifier`,
   `StatusReader`, `SupplyReader`, `TrayReader`, `CounterReader`, `FirmwareReader`,
   and on the write side `ConfigReader` / `ConfigWriter` (see
   [configuration management](Configuration-management.md): map your fields onto the catalogue keys,
   honour `WriteOptions.DryRun` with zero write I/O, report every setting, fill
   `Rollback`), `SecurityReader` / `SecurityWriter`, `CertManager`, `FirmwareUpdater`.

3. In `Identify`, return only the capabilities you can actually serve for *that*
   device (probe once, cheaply). The engine merges identities by priority and records
   your name against each capability you win.
   Before the drivers fan out, the engine reads the system group once (`engine.Preflight`:
   whether SNMP answered, the sysObjectID enterprise, sysDescr). A driver that can tell
   from that alone it does not apply implements `engine.Gated` (`Applies(Preflight) bool`)
   and is then skipped without opening a session; `vendorkit.Base` does this for its
   enterprise numbers. When a read that decides "is this a printer" times out, return a
   read failure (the SNMP driver's `ErrReadFailed`), never "not a printer" (issue #19).

4. Register the driver at startup in `cmd/printmarshal/main.go`:

   ```go
   reg.Register(myvendor.New(opts))
   ```

   Nothing else in the core changes. Vendor enterprise OIDs, private REST/XML endpoints
   and model quirks all live inside the plugin.

5. Record real device responses as fixtures under `testdata/fixtures` and unit-test the
   parser against them; add the driver to the contract test so it remains
   interchangeable with the standards drivers.

## Vendor plugins

Built-in vendor plugins live under `internal/plugins/<vendor>/` and share
`internal/plugins/vendorkit` (enterprise-number and badge matching, SNMP
helpers, canonical-key mapping, evidence descriptors). Start a new vendor from
an existing skeleton such as `internal/plugins/oki`; the capability/evidence
matrix and research citations are in `internal/plugins/README.md`.

## Capability and fallback behavior

`Registry.For` selects declaring, supporting drivers and sorts by descending priority, preserving registration order for ties. `ForWithFallback` then adds other declaring drivers listed in the device's `identifyMethods`. `Supports` must not perform I/O. Identification probes run concurrently, merge higher-priority identity first, and record per-capability winners. A declaration is not evidence that an individual device accepted an operation: `Identify` must report its actual subset. See `internal/engine/engine.go` and `profile.go`.

| Standards driver | Actual coverage |
| --- | --- |
| `snmp` | Identify, status, supplies, trays, counters, alert subscription declaration; read/write configuration of the supported standard MIB objects. Supply/tray/counter capability is conditional on returned data. |
| `ipp` | Identify, status, supplies, trays, counters, firmware version, configuration read; configuration write only when printer attributes advertise the relevant operations/settable attributes. |
| `escl` | Identify only in the engine profile. Parses scanner capability/status XML; does not expose scanning jobs or upload documents. |
| `fwsim` | Simulator firmware update only; registered only with `PM_FIRMWARE_SIM_TARGETS`. |

Read interfaces are in `internal/engine/engine.go`; configuration, security, certificates and firmware interfaces are in `internal/engine/write.go`. Return `ErrUnsupported` for absent capabilities and `ErrNotIdentified` for an address that is not a manageable device. Respect context deadlines and keep device I/O within engine limits.

## Existing vendor plugins

All 13 use `vendorkit.Base`. The following is the declaration in each current plugin constructor; discovery can narrow it. Generic `read_config` facts are read-only identity/vendor facts, not a claim to configure that manufacturer's private interface.

| Plugin | Declared capabilities | Source |
| --- | --- | --- |
| `brother` | `identify`, `read_config` | `internal/plugins/brother/plugin.go` |
| `canon` | `identify`, `read_config`, `read_counters` | `internal/plugins/canon/plugin.go` |
| `epson` | `identify`, `read_config` | `internal/plugins/epson/plugin.go` |
| `fujifilm` | `identify`, `read_config` | `internal/plugins/fujifilm/plugin.go` |
| `hp` | `identify`, `read_status`, `read_counters`, `firmware_read`, `read_config`, `read_security` | `internal/plugins/hp/plugin.go` |
| `konicaminolta` | `identify`, `read_config` | `internal/plugins/konicaminolta/plugin.go` |
| `kyocera` | `identify`, `read_config` | `internal/plugins/kyocera/plugin.go` |
| `lexmark` | `identify`, `read_config` | `internal/plugins/lexmark/plugin.go` |
| `oki` | `identify`, `read_config` | `internal/plugins/oki/plugin.go` |
| `ricoh` | `identify`, `read_config`, `read_counters` | `internal/plugins/ricoh/plugin.go` |
| `sharp` | `identify`, `read_config` | `internal/plugins/sharp/plugin.go` |
| `toshiba` | `identify`, `read_config` | `internal/plugins/toshiba/plugin.go` |
| `xerox` | `identify`, `read_config` | `internal/plugins/xerox/plugin.go` |

HP supplies PML identity/counters/firmware and LEDM configuration/security/status enrichment. Samsung enterprise 236 is identity-only through that plugin and receives no HP enrichment. Ricoh and Canon private counters are probe-gated and marked unverified; they are not universal counter contracts. Other plugins primarily establish canonical identity and read-only facts. The descriptors returned by `/api/drivers` include evidence, families, badges, priorities and documented management paths; a path named in a descriptor is vendor context, not an implemented transport.

Badge matching without an enterprise number is permitted only when the device supplies no enterprise identifier. A conflicting enterprise prevents another manufacturer's badge text selecting the plugin. Rebrands are deliberately conservative. See `internal/plugins/vendorkit/base.go`, `descriptor.go` and the per-plugin `Spec`.

No shipped real-vendor plugin implements `ConfigWriter`, `SecurityWriter`, `CertManager` or `FirmwareUpdater`. Standards SNMP/IPP configuration writes remain available independently. Do not infer firmware upload, certificate installation or private configuration writes from a vendor tool's name.

## Adding a safe write capability

Implement the exact typed interface in `engine/write.go`. Dry run may read capabilities and current values but must perform zero device-changing I/O. Return per-setting outcomes and rollback data; never persist secret values in snapshots or profiles. Resolve purpose-bound write credentials through the service/store path, use context deadlines, and let the feature service record audit/write/job results. A transport success alone is not verification; configuration is read back, security is reassessed and firmware is polled. Register only capabilities backed by a concrete protocol contract and tests.

Use `vendorkit.Options` to inject test SNMP/HTTP transports. Fixtures should contain sanitized observations rather than live secrets, names or addresses. Existing tests in each plugin package and `internal/plugins/all_test.go` show matching, unsupported operations and shared contracts.

## Simulator

`cmd/pm-sim` supplies a UDP SNMP fixture agent, IPP endpoint, optional self-signed IPPS, mutable configuration and firmware hooks. It is intended for local tests, not a faithful implementation of every printer.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-fixture` | `testdata/fixtures/generic-laser.snmp` | SNMP fixture |
| `-snmp` | `127.0.0.1:1161` | UDP agent address |
| `-ipp` | `127.0.0.1:6631` | HTTP IPP; empty disables |
| `-ipps` | empty | TLS IPP listen address; empty disables |
| `-admin-password` | empty | When set, writes require HTTP Basic user `admin` |
| `-community` | `public` | Accepted SNMPv2c community |
| `-firmware` | `2.14.7` | Initial IPP firmware string |
| `-firmware-delay` | `0` | Delay before uploaded version takes effect |

An isolated local simulator setup (these commands are documentation, not executed for this wiki):

```sh
go build -o bin/pm-sim ./cmd/pm-sim
./bin/pm-sim -fixture testdata/fixtures/generic-laser.snmp
```

Configure the server's `poll.snmpPort` to 1161 and `poll.ippEndpoints` to `["http://%s:6631/ipp/print"]`; discover `127.0.0.1` with `zeroconf:false`. For firmware set `PM_FIRMWARE_SIM_TARGETS=127.0.0.1:6631`; see `internal/driver/fwsim/driver.go` and [Firmware](Firmware.md) for transport/allow-list behavior. Do not put real devices in this allow-list.

`testdata/fixtures` includes generic monochrome/color devices, vendor SNMP fixtures, HP IPP recordings, LEDM XML and eSCL XML. `snmp.LoadFixture` and fixture transports exercise parsing without hardware. Live HP tests are opt-in through `PM_LIVE_HP`; never set it for offline unit tests.

## Model catalogue and operator qualification

## What the catalogue is — and is not

Every family is a **research candidate**. The research validated **zero physical
devices**. A family being listed means a vendor document or management-tool table names
the models; it does not mean PrintMarshal has implemented or tested any operation on
them. The research's own counting rule, interpretation notes and source-access notes
are carried into the shipped data verbatim.

The data has five parts:

| Part | What it says | What it does *not* say |
| --- | --- | --- |
| `families` | Brand, model prefix, base model identifiers, segment, lifecycle note, the sources that establish identity, and the shipped `qualification_status` (always `research_candidate_unvalidated`). | That any operation works. |
| `management_profiles` | Per vendor platform: the documented read, configuration and firmware routes, and whether a general admin API is verified. | A per-model guarantee. The catalogue's own interpretation: "vendor/platform context; only operation_evidence establishes the narrower documented scope". |
| `operation_evidence` | For named models: a status string and source ids per operation (monitoring, configuration, security, firmware, app management). Some entries document *negative* results (e.g. Sharp SRDM marks MX-C407P cloning and updates unsupported). | Physical validation (`physical_validation` is false everywhere). |
| `sources` | The registry every id above points into: title, URL, access mode (`official_search_index` means indexed extracts, not a full read), check date. | — |
| `hp_variant_examples`, `documented_identity_conflicts` | Exact product-number observations and cases where two sources disagree about one product number (X3A87A is "E87640 du" in one table and "E87640dn" in the guide). | A complete SKU database. |

## The evidence model in PrintMarshal

For each device and each of the five lifecycle operations the research names
(**identify, monitor, configure, secure, update**) PrintMarshal computes two independent
facts and a combined label:

- **Documented** — `documented`, `unsupported` or `unknown`. Model-specific
  `operation_evidence` for the matched base identifier wins; a status containing
  `not_supported` becomes `unsupported`. If there is no model record, a route in the
  family's management profile makes the operation `documented` *at platform level* — the
  API and UI label the basis (`operation_evidence:<id>` vs `management_profile:<id>`) so
  the weaker claim is visible. `secure` is never documented from a profile. `identify` is
  documented by the family record's own identity sources.
- **Observed** — `observed` or `unknown`, from the device's resolved capability profile
  (which driver serves `read_status`, `read_supplies`, `read_config`, `write_config`,
  `read_security`, `firmware_update`, …). No network I/O is done to compute it.
- **Status** — `documented+observed`, `documented`, `observed`, `unsupported` or `unknown`.
  The two sides are shown side by side; "documented" never implies "observed" and vice
  versa.

A device also gets a **tier**: `validated` (an operator recorded lab or production
validation of the family — see below), `documented+observed`, `observed only (standards)`,
`documented only`, `unsupported` (operator-marked) or `unknown`. The tier is a summary of
the rows; it adds no information of its own.

### Matching a device to a family

`catalog.Match(make, model, sysDescr, enterpriseID)`:

1. **Brand** from the make string or badge (Savin, Lanier, Gestetner → Ricoh; Develop →
   Konica Minolta; UTAX/Triumph-Adler → Kyocera; Fuji Xerox → Fujifilm …), else from the
   IANA enterprise number of `sysObjectID`, else from a brand word in the model text.
   The brand ↔ enterprise map lives in `internal/catalog/brands.go` — it is catalogue data,
   the engine still stores the raw number and interprets nothing.
2. **Model** by tolerant tokenisation of the model string and `sysDescr`: identifiers are
   compared upper-cased without spaces or hyphens (`IM C3010` = `IMC3010`), adjacent
   words are joined (`BP-70M90`), long alpha prefixes are stripped (`e-STUDIO6529A`),
   a letter-only bundle suffix is ignored (`E87770z`, `CX943adse`, `C3730i`), explicit
   ranges (`M282-M285`) and trailing-`x` wildcards (`M28x`) in `model_ids` are honoured,
   and a generic identity shorter than the base id (`E877dn` → E877 40/50/60/70) matches
   at family level.
3. **Confidence**: `exact` (base identifier found), `family` (generic/wildcard identity,
   or several families tied — the candidates are listed), `brand-only` (brand known, no
   family contains the model) or `none` (brand unknown). An unknown make with an unknown
   enterprise number is `none`, never a guess.

The raw observations are returned verbatim with every match; the research insists a
reported name is an observation (licensed speed changes can change it), not a key.

Worked examples (all covered by `internal/catalog/catalog_test.go`):

| Make · model | Result |
| --- | --- |
| HP · HP Color LaserJet MFP M283fdw | `brand-only` — the Pro M283 is not an Enterprise/Managed model; nothing in the catalogue claims it. |
| HP · Color LaserJet Managed MFP E87770z | `exact` → HP01 (E87770), evidence HP_E877, profile HP_FS. |
| Savin · SAVIN IM C3010 | `exact` → RI02 (Ricoh). |
| KONICA MINOLTA · bizhub C300i | `exact` → KM01. |
| SHARP · MX-C407P | `exact` → SH05 with configure/update `unsupported` from SHARP_A4_LIMIT. |
| Acme · Acme LaserPrint 4200dn | `none`. |

## API

All read endpoints need the viewer role; the override needs operator.

| Method · path | Returns |
| --- | --- |
| `GET /api/catalog/summary` | Catalogue counts (families, identifiers, brands, profiles, evidence, sources, conflicts, variants, physically validated), fleet totals (matched/unmatched and by confidence), override counts, brand and segment lists. |
| `GET /api/catalog/families?brand=&q=&segment=` | Family rows with the profile label, documented state per operation (family level, never observed), model-record count, fleet device count and effective qualification. |
| `GET /api/catalog/families/{id}` | Family, management profile, model-specific evidence, brand-level context, per-operation summary, every referenced source, identity conflicts, product-number examples, fleet devices in the family, qualification and override. |
| `GET /api/catalog/profiles` | The vendor management profiles. |
| `GET /api/catalog/match?make=&model=&sysDescr=&enterpriseId=` | A match for arbitrary observations (for testing identification strings). |
| `GET /api/devices/{id}/support` | `{match, support}` for one device: confidence, family, evidence, and the documented/observed/unknown rows plus tier. |
| `PUT /api/catalog/families/{id}/qualification` | Body `{status, note}`; `status` ∈ `research_candidate`, `lab_validated`, `production_validated`, `unsupported`. Recorded with actor and time; audited as `catalog.qualification`. |

## Qualification workflow

The catalogue cannot be edited from the UI — it is research output. What an operator
*can* record is what their own lab or production established for a family, following the
research's acceptance progression (identify → monitor → configure → secure → update, each
with an evidence record of exact identity, firmware, enabled protocols, credentials used,
observed objects, results and limitations):

1. Open **Catalogue**, pick the family (or follow the family link from a device's
   **Support & evidence** card).
2. Check the per-operation evidence and the fleet devices listed under the family.
   The device card shows which operations drivers actually observed on each device.
3. In **Qualification**, set `lab_validated` once the acceptance evidence exists for a
   representative device and firmware, `production_validated` once it has run in a
   customer fleet, or `unsupported` when testing established that it should not be
   managed. Write the note: which device, firmware and what was tested.
4. The change is attributed to the signed-in user, time-stamped, written to the audit
   trail, and raises the family's devices to the `validated` (or `unsupported`) tier. It
   never alters the documented or observed rows — those stay what the sources and the
   drivers say.

Setting `research_candidate` again returns the family to the shipped state.

## Updating the catalogue from a new research JSON

1. Copy the new research file over `internal/catalog/data/catalogue.json` (keep the
   research file itself untouched in `research/`). The JSON schema is the one documented
   in the file's `schema_version`, `counting_rule` and `interpretation` blocks; the Go
   structs in `internal/catalog/catalog.go` mirror it field by field.
2. Run `go test ./internal/catalog/`. `Parse` validates on load and rejects the file if
   a family's management profile is missing, an `operation_evidence` model id belongs to
   no family of that brand (ids containing `_` are treated as pointers into a source
   table, not models), or any referenced source id is absent from the registry. The test
   also pins the counts and asserts every family still ships as
   `research_candidate_unvalidated` and that no evidence claims physical validation —
   change those assertions only when the research genuinely changes them.
3. Add brand aliases or enterprise numbers to `internal/catalog/brands.go` if a new brand
   appears, and add a match test for a representative model string.
4. Operator overrides are keyed by family id and survive the update; a family whose id
   disappears keeps a dangling override that the UI no longer shows.

## Known limitations

- Enterprise numbers are listed only where confidence is high; an unlisted number
  resolves to no brand. RISO has none listed.
- A model string that contains a serial or firmware token could, in rare cases, match a
  purely numeric base identifier of the same brand; the matched token is always shown so
  this can be spotted.
- Management-profile routes make `monitor`/`configure`/`update` "documented" at platform
  level for every family of that vendor. The basis is labelled so the weaker claim is
  visible; treat it as a research lead, not as model evidence.

## Known discrepancies

Older phase decisions in `docs/decisions.md` defer eSCL, PostgreSQL, vendor plugins and SSO; those later implementations are now present. The catalogue's brand aliases and documented management routes are broader than driver transports; matching a family does not enable a writer. Main constructs vendor plugins with their own default transport options, so file overrides applied to the standards drivers are not automatically propagated into vendor transports (`internal/plugins/all.go`, `cmd/printmarshal/main.go`).

Sources: `internal/engine/`, `internal/driver/`, `internal/plugins/`, `internal/catalog/`, `cmd/pm-sim/`, `docs/adding-a-driver.md`, `docs/catalogue.md`. See [Development](Development.md) for builds and store extension.
