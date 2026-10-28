# Device model catalogue

PrintMarshal ships a **catalogue of enterprise printer families** and matches every
discovered device to one of them. The catalogue tells you what a primary vendor
source *documents* for a family; the device's capability profile tells you what
PrintMarshal's drivers have *observed*; everything else is *unknown*. The feature
exists to keep those three apart — it never turns an unknown into a "no", and it
never calls anything "supported".

- Code: `internal/catalog` (data, loader, matcher, support status), `internal/api/catalog.go`
  (endpoints), `internal/store/*_catalog.*` (operator qualification overrides),
  `web/src/pages/Catalog.tsx`, `web/src/components/catalog/*`.
- Data: `internal/catalog/data/catalogue.json`, an unchanged copy of
  `research/enterprise-printer-catalogue-2026-10-28.json` (126 families, 495 base model
  identifiers, 15 brands, 15 vendor management profiles, 9 operation-evidence records,
  155 sources). The narrative behind it is
  `research/enterprise-printer-expansion-and-integration-evidence-2026-10-28.md`.

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
