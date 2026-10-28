// Package catalog ships the enterprise printer model catalogue produced by the
// research in research/enterprise-printer-catalogue-2026-10-28.json and
// matches discovered devices to its families.
//
// The catalogue is evidence, not a support manifest. Every record keeps the
// research's own evidence categories: a family is a research candidate until a
// lab validates it, and nothing in here turns an unknown into a "supported".
// The package exposes three separate notions and never merges them:
//
//   - documented: a primary vendor source describes the operation for a model
//     (operation_evidence) or the vendor platform offers a route for it
//     (management_profiles); the source ids are kept with every claim.
//   - observed: PrintMarshal's own drivers answered for the device (its
//     resolved capability profile, see support.go).
//   - unknown: neither. Unknown stays unknown.
//
// The data file is embedded; the research file in research/ is the source of
// truth and is copied here unchanged (see docs/catalogue.md for the update
// procedure).
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed data/catalogue.json
var raw []byte

// Catalogue is the parsed research catalogue.
type Catalogue struct {
	SchemaVersion      string                       `json:"schema_version"`
	Title              string                       `json:"title"`
	ResearchedOn       string                       `json:"researched_on"`
	Purpose            string                       `json:"purpose"`
	CountingRule       string                       `json:"counting_rule"`
	Research           ResearchSummary              `json:"summary"`
	Interpretation     map[string]string            `json:"interpretation"`
	Families           []Family                     `json:"families"`
	ManagementProfiles map[string]ManagementProfile `json:"management_profiles"`
	OperationEvidence  []OperationEvidence          `json:"operation_evidence"`
	HPVariantExamples  []VariantExample             `json:"hp_variant_examples"`
	Sources            map[string]Source            `json:"sources"`
	IdentityConflicts  []IdentityConflict           `json:"documented_identity_conflicts"`

	// indexes built once after parsing
	byID      map[string]*Family
	brandKeys map[string][]modelKey // brand -> normalised model keys of its families
}

// ResearchSummary is the research's own count block.
type ResearchSummary struct {
	FamilyGroups               int            `json:"family_groups"`
	BaseModelIdentifiers       int            `json:"base_model_identifiers"`
	Brands                     int            `json:"brands"`
	ByBrand                    map[string]int `json:"by_brand"`
	HPManagedBaseIdentifiers   int            `json:"hp_managed_base_identifiers"`
	ExactHPVariantExamples     int            `json:"exact_hp_variant_examples"`
	PhysicallyValidatedDevices int            `json:"physically_validated_devices"`
}

// Family is one brand + model-prefix group of base model identifiers.
type Family struct {
	ID                            string   `json:"id"`
	Brand                         string   `json:"brand"`
	ModelPrefix                   string   `json:"model_prefix"`
	ModelIDs                      []string `json:"model_ids"`
	Segment                       string   `json:"segment"`
	ManagementProfile             string   `json:"management_profile"`
	ModelSourceIDs                []string `json:"model_source_ids"`
	ModelEvidence                 string   `json:"model_evidence"`
	ReleaseYear                   *int     `json:"release_year"`
	Lifecycle                     string   `json:"lifecycle"`
	QualificationStatus           string   `json:"qualification_status"`
	Note                          string   `json:"note"`
	ReleaseYearScope              string   `json:"release_year_scope"`
	ManagementSupportEvidenceDate string   `json:"management_support_evidence_date,omitempty"`
}

// ManagementProfile is a vendor/platform context record: the documented routes
// for reading, configuring and updating devices of that platform. It is
// context, not a per-model guarantee (catalogue "interpretation").
type ManagementProfile struct {
	Label             string   `json:"label"`
	ReadPath          string   `json:"read_path"`
	ConfigurationPath string   `json:"configuration_path"`
	FirmwarePath      string   `json:"firmware_path"`
	GeneralAdminAPI   string   `json:"general_admin_api"`
	SourceIDs         []string `json:"source_ids"`
	Scope             string   `json:"scope"`
}

// Evidence is one operation's documented status with its sources.
type Evidence struct {
	Status          string   `json:"status"`
	SourceIDs       []string `json:"source_ids"`
	MinimumFirmware string   `json:"minimum_firmware,omitempty"`
	Licence         string   `json:"licence,omitempty"`
}

// OperationEvidence is the narrower, model-specific evidence block: for the
// listed model ids, which operations a primary source documents.
type OperationEvidence struct {
	ID                      string    `json:"id"`
	ModelIDs                []string  `json:"model_ids"`
	Brand                   string    `json:"brand"`
	Monitoring              *Evidence `json:"monitoring,omitempty"`
	Configuration           *Evidence `json:"configuration,omitempty"`
	Security                *Evidence `json:"security,omitempty"`
	Firmware                *Evidence `json:"firmware,omitempty"`
	AppManagement           *Evidence `json:"app_management,omitempty"`
	GeneralDeviceManagement string    `json:"general_device_management,omitempty"`
	ThirdPartyAdminContract string    `json:"third_party_admin_contract,omitempty"`
	PhysicalValidation      bool      `json:"physical_validation"`
}

// VariantExample is an exact product-number observation (HP bundles).
type VariantExample struct {
	BaseModel          string   `json:"base_model"`
	Bundle             string   `json:"bundle"`
	ProductNumber      string   `json:"product_number"`
	SourceIDs          []string `json:"source_ids"`
	PhysicalValidation bool     `json:"physical_validation"`
}

// Source is one entry of the source registry.
type Source struct {
	Title               string `json:"title"`
	URL                 string `json:"url"`
	Access              string `json:"access"`
	CheckedOn           string `json:"checked_on"`
	FirstPassAccessNote string `json:"first_pass_access_note,omitempty"`
}

// IdentityConflict records two sources that disagree about one product number.
type IdentityConflict struct {
	Brand         string                `json:"brand"`
	ProductNumber string                `json:"product_number"`
	Observations  []ConflictObservation `json:"observations"`
	Resolution    string                `json:"resolution"`
}

// ConflictObservation is one side of an identity conflict.
type ConflictObservation struct {
	Model    string `json:"model"`
	Bundle   string `json:"bundle"`
	SourceID string `json:"source_id"`
}

var (
	loadOnce sync.Once
	loaded   *Catalogue
	loadErr  error
)

// Load parses the embedded catalogue once and returns it. The result is shared
// and must be treated as read-only.
func Load() (*Catalogue, error) {
	loadOnce.Do(func() { loaded, loadErr = Parse(raw) })
	return loaded, loadErr
}

// MustLoad is Load for callers that cannot continue without the catalogue.
func MustLoad() *Catalogue {
	c, err := Load()
	if err != nil {
		panic(err)
	}
	return c
}

// Parse decodes a catalogue JSON document and builds the lookup indexes.
func Parse(b []byte) (*Catalogue, error) {
	var c Catalogue
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("catalogue: %w", err)
	}
	c.index()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Catalogue) index() {
	c.byID = make(map[string]*Family, len(c.Families))
	c.brandKeys = map[string][]modelKey{}
	for i := range c.Families {
		f := &c.Families[i]
		c.byID[f.ID] = f
		for _, id := range f.ModelIDs {
			c.brandKeys[f.Brand] = append(c.brandKeys[f.Brand], newModelKey(f.ID, id))
		}
	}
}

// Validate checks the catalogue's internal consistency: every family's
// management profile exists, every operation_evidence model id belongs to a
// family of that brand or is a prefix of one (or is an explicit non-model
// pointer such as "exact_products_in_NHP03_only"), and every source id
// referenced anywhere exists in the registry.
func (c *Catalogue) Validate() error {
	var errs []string
	seen := map[string]bool{}
	for _, f := range c.Families {
		if f.ID == "" {
			errs = append(errs, "family without id")
			continue
		}
		if seen[f.ID] {
			errs = append(errs, "duplicate family id "+f.ID)
		}
		seen[f.ID] = true
		if _, ok := c.ManagementProfiles[f.ManagementProfile]; !ok {
			errs = append(errs, fmt.Sprintf("family %s: management profile %q not found", f.ID, f.ManagementProfile))
		}
		if len(f.ModelIDs) == 0 {
			errs = append(errs, "family "+f.ID+" has no model ids")
		}
		for _, sid := range f.ModelSourceIDs {
			if _, ok := c.Sources[sid]; !ok {
				errs = append(errs, fmt.Sprintf("family %s: source %q not found", f.ID, sid))
			}
		}
	}
	for id, p := range c.ManagementProfiles {
		for _, sid := range p.SourceIDs {
			if _, ok := c.Sources[sid]; !ok {
				errs = append(errs, fmt.Sprintf("profile %s: source %q not found", id, sid))
			}
		}
	}
	for _, e := range c.OperationEvidence {
		for _, mid := range e.ModelIDs {
			if isPointerModelID(mid) {
				continue
			}
			if len(c.FamiliesForModelID(e.Brand, mid)) == 0 {
				errs = append(errs, fmt.Sprintf("evidence %s: model id %q is not in any %s family", e.ID, mid, e.Brand))
			}
		}
		for _, ev := range e.evidenceBlocks() {
			for _, sid := range ev.SourceIDs {
				if _, ok := c.Sources[sid]; !ok {
					errs = append(errs, fmt.Sprintf("evidence %s: source %q not found", e.ID, sid))
				}
			}
		}
	}
	for _, v := range c.HPVariantExamples {
		for _, sid := range v.SourceIDs {
			if _, ok := c.Sources[sid]; !ok {
				errs = append(errs, fmt.Sprintf("variant %s: source %q not found", v.ProductNumber, sid))
			}
		}
	}
	for _, k := range c.IdentityConflicts {
		for _, o := range k.Observations {
			if _, ok := c.Sources[o.SourceID]; !ok {
				errs = append(errs, fmt.Sprintf("conflict %s: source %q not found", k.ProductNumber, o.SourceID))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("catalogue is inconsistent: %s", strings.Join(errs, "; "))
	}
	return nil
}

// isPointerModelID reports whether an operation_evidence model id is a pointer
// to a source table rather than a model identifier (e.g.
// "exact_products_in_NHP03_only"). Such entries carry brand-level evidence.
func isPointerModelID(id string) bool { return strings.Contains(id, "_") }

// evidenceBlocks lists the non-nil per-operation evidence of an entry, keyed by
// the catalogue's operation names.
func (e *OperationEvidence) evidenceBlocks() map[string]*Evidence {
	out := map[string]*Evidence{}
	if e.Monitoring != nil {
		out["monitoring"] = e.Monitoring
	}
	if e.Configuration != nil {
		out["configuration"] = e.Configuration
	}
	if e.Security != nil {
		out["security"] = e.Security
	}
	if e.Firmware != nil {
		out["firmware"] = e.Firmware
	}
	if e.AppManagement != nil {
		out["app_management"] = e.AppManagement
	}
	return out
}

// IsModelSpecific reports whether the entry names real model ids (as opposed
// to pointing at a vendor table).
func (e *OperationEvidence) IsModelSpecific() bool {
	for _, id := range e.ModelIDs {
		if !isPointerModelID(id) {
			return true
		}
	}
	return false
}

// Summary is what the UI header and the summary endpoint show.
type Summary struct {
	Title                string         `json:"title"`
	ResearchedOn         string         `json:"researchedOn"`
	SchemaVersion        string         `json:"schemaVersion"`
	Purpose              string         `json:"purpose"`
	Families             int            `json:"families"`
	BaseIdentifiers      int            `json:"baseIdentifiers"`
	Brands               int            `json:"brands"`
	ByBrand              map[string]int `json:"byBrand"`
	Profiles             int            `json:"profiles"`
	EvidenceEntries      int            `json:"evidenceEntries"`
	Sources              int            `json:"sources"`
	IdentityConflicts    int            `json:"identityConflicts"`
	VariantExamples      int            `json:"variantExamples"`
	PhysicallyValidated  int            `json:"physicallyValidated"`
	QualificationByState map[string]int `json:"qualificationByState"`
}

// Summary computes counts from the parsed data (not from the research's own
// summary block, so a hand-edited file cannot drift from what is shipped).
func (c *Catalogue) Summary() Summary {
	s := Summary{
		Title: c.Title, ResearchedOn: c.ResearchedOn, SchemaVersion: c.SchemaVersion, Purpose: c.Purpose,
		Families: len(c.Families), ByBrand: map[string]int{}, Profiles: len(c.ManagementProfiles),
		EvidenceEntries: len(c.OperationEvidence), Sources: len(c.Sources), IdentityConflicts: len(c.IdentityConflicts),
		VariantExamples: len(c.HPVariantExamples), PhysicallyValidated: c.Research.PhysicallyValidatedDevices,
		QualificationByState: map[string]int{},
	}
	for _, f := range c.Families {
		s.BaseIdentifiers += len(f.ModelIDs)
		s.ByBrand[f.Brand] += len(f.ModelIDs)
		s.QualificationByState[f.QualificationStatus]++
	}
	s.Brands = len(s.ByBrand)
	return s
}

// Filter selects families.
type Filter struct {
	Brand   string // exact brand name (case-insensitive)
	Q       string // free text over id, brand, model prefix, model ids, segment, profile label
	Segment string // exact segment (case-insensitive)
}

// FamilyList returns families matching the filter, in catalogue order.
func (c *Catalogue) FamilyList(f Filter) []Family {
	q := strings.ToLower(strings.TrimSpace(f.Q))
	out := []Family{}
	for _, fam := range c.Families {
		if f.Brand != "" && !strings.EqualFold(fam.Brand, f.Brand) {
			continue
		}
		if f.Segment != "" && !strings.EqualFold(fam.Segment, f.Segment) {
			continue
		}
		if q != "" {
			hay := strings.ToLower(strings.Join(append([]string{fam.ID, fam.Brand, fam.ModelPrefix, fam.Segment, fam.ManagementProfile, c.ManagementProfiles[fam.ManagementProfile].Label}, fam.ModelIDs...), " "))
			if !strings.Contains(hay, q) && !strings.Contains(strings.ReplaceAll(hay, " ", ""), strings.ReplaceAll(q, " ", "")) {
				continue
			}
		}
		out = append(out, fam)
	}
	return out
}

// Family returns one family by id.
func (c *Catalogue) Family(id string) (*Family, bool) {
	f, ok := c.byID[id]
	return f, ok
}

// Profile returns one management profile by id.
func (c *Catalogue) Profile(id string) (*ManagementProfile, bool) {
	p, ok := c.ManagementProfiles[id]
	if !ok {
		return nil, false
	}
	return &p, true
}

// Brands lists the brands in the catalogue, sorted.
func (c *Catalogue) Brands() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range c.Families {
		if !seen[f.Brand] {
			seen[f.Brand] = true
			out = append(out, f.Brand)
		}
	}
	sort.Strings(out)
	return out
}

// Segments lists the distinct segments, sorted.
func (c *Catalogue) Segments() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range c.Families {
		if !seen[f.Segment] {
			seen[f.Segment] = true
			out = append(out, f.Segment)
		}
	}
	sort.Strings(out)
	return out
}

// FamiliesForModelID returns the families of a brand that contain the model id
// exactly or for which it is a prefix of a model id (the catalogue uses
// generic identities such as "E877" for the E87740..E87770 group).
func (c *Catalogue) FamiliesForModelID(brand, modelID string) []*Family {
	key := normalise(modelID)
	var out []*Family
	seen := map[string]bool{}
	for _, mk := range c.brandKeys[brand] {
		if mk.key == key || (len(key) >= 4 && strings.HasPrefix(mk.key, key)) {
			if !seen[mk.familyID] {
				seen[mk.familyID] = true
				out = append(out, c.byID[mk.familyID])
			}
		}
	}
	return out
}

// EvidenceForModel returns the model-specific operation_evidence entries that
// name modelID (exactly, or as a generic prefix such as "E877" for "E87770").
func (c *Catalogue) EvidenceForModel(brand, modelID string) []OperationEvidence {
	key := normalise(modelID)
	var out []OperationEvidence
	for _, e := range c.OperationEvidence {
		if !strings.EqualFold(e.Brand, brand) {
			continue
		}
		for _, mid := range e.ModelIDs {
			if isPointerModelID(mid) {
				continue
			}
			k := normalise(mid)
			if k == key || (len(k) >= 4 && strings.HasPrefix(key, k)) {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// EvidenceForFamily returns every model-specific evidence entry that names
// any model id of the family.
func (c *Catalogue) EvidenceForFamily(f *Family) []OperationEvidence {
	var out []OperationEvidence
	seen := map[string]bool{}
	for _, id := range f.ModelIDs {
		for _, e := range c.EvidenceForModel(f.Brand, id) {
			if !seen[e.ID] {
				seen[e.ID] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// BrandEvidence returns the evidence entries of a brand that point at a vendor
// table instead of naming models (brand-level context, never a model claim).
func (c *Catalogue) BrandEvidence(brand string) []OperationEvidence {
	var out []OperationEvidence
	for _, e := range c.OperationEvidence {
		if strings.EqualFold(e.Brand, brand) && !e.IsModelSpecific() {
			out = append(out, e)
		}
	}
	return out
}

// SourcesFor resolves the given source ids against the registry, skipping
// unknown ids (Validate guarantees there are none in shipped data).
func (c *Catalogue) SourcesFor(ids ...string) map[string]Source {
	out := map[string]Source{}
	for _, id := range ids {
		if s, ok := c.Sources[id]; ok {
			out[id] = s
		}
	}
	return out
}

// VariantsFor lists the product-number examples whose base model is in the family.
func (c *Catalogue) VariantsFor(f *Family) []VariantExample {
	var out []VariantExample
	for _, v := range c.HPVariantExamples {
		for _, id := range f.ModelIDs {
			if normalise(v.BaseModel) == normalise(id) {
				out = append(out, v)
				break
			}
		}
	}
	return out
}

// ConflictsFor lists the documented identity conflicts touching the family.
func (c *Catalogue) ConflictsFor(f *Family) []IdentityConflict {
	var out []IdentityConflict
	for _, k := range c.IdentityConflicts {
		if !strings.EqualFold(k.Brand, f.Brand) {
			continue
		}
		for _, o := range k.Observations {
			if families := c.FamiliesForModelID(f.Brand, o.Model); len(families) > 0 && families[0].ID == f.ID {
				out = append(out, k)
				break
			}
		}
	}
	return out
}
