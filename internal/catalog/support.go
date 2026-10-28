package catalog

import (
	"strings"
	"time"

	"printmarshal/internal/model"
)

// Operations are the five lifecycle operations the research's acceptance
// progression names (identify, monitor, configure, secure, update), in order.
var Operations = []string{"identify", "monitor", "configure", "secure", "update"}

// Documented states of an operation for a model.
const (
	Documented  = "documented"  // a primary source describes the operation for this model/platform
	Unsupported = "unsupported" // a primary source says the checked tool path does not support it
	Unknown     = "unknown"     // nothing documented either way
	Observed    = "observed"    // PrintMarshal's own drivers serve it for this device
)

// Qualification statuses an operator may assert for a family. They are
// recorded in the store with actor, note and time, and audited. The catalogue
// itself ships every family as research_candidate_unvalidated.
const (
	QualResearchCandidate   = "research_candidate"
	QualLabValidated        = "lab_validated"
	QualProductionValidated = "production_validated"
	QualUnsupported         = "unsupported"
)

// QualificationStatuses lists the values accepted by the API, in order.
var QualificationStatuses = []string{QualResearchCandidate, QualLabValidated, QualProductionValidated, QualUnsupported}

// ValidQualification reports whether s is an accepted status.
func ValidQualification(s string) bool {
	for _, q := range QualificationStatuses {
		if q == s {
			return true
		}
	}
	return false
}

// Qualification is an operator's recorded qualification of a family.
type Qualification struct {
	FamilyID  string    `json:"familyId"`
	Status    string    `json:"status"`
	Note      string    `json:"note"`
	Actor     string    `json:"actor"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Tiers summarise a device's standing, strongest first.
const (
	TierValidated          = "validated"                 // operator recorded lab/production validation of the family
	TierDocumentedObserved = "documented+observed"       // at least one operation is both documented and observed
	TierObservedOnly       = "observed only (standards)" // drivers answer but nothing is documented for the model
	TierDocumentedOnly     = "documented only"           // documented but no driver has observed it
	TierUnsupported        = "unsupported"               // operator marked the family unsupported
	TierUnknown            = "unknown"
)

// OpStatus is one operation's standing for one device: what is documented,
// what is observed, kept apart.
type OpStatus struct {
	Operation string `json:"operation"`
	// Documented is "documented", "unsupported" or "unknown".
	Documented string `json:"documented"`
	// DocumentedBasis says where the documentation claim comes from:
	// "operation_evidence:<id>" (model-specific) or "management_profile:<id>"
	// (vendor-platform route; context, not a per-model guarantee), or "" if unknown.
	DocumentedBasis string `json:"documentedBasis,omitempty"`
	// DocumentedDetail is the source's own status text or route description, verbatim.
	DocumentedDetail string   `json:"documentedDetail,omitempty"`
	SourceIDs        []string `json:"sourceIds"`
	// Observed is "observed" or "unknown".
	Observed string `json:"observed"`
	// ObservedBy lists "capability·driver" pairs from the device's resolved profile.
	ObservedBy []string `json:"observedBy"`
	// Status combines the two without merging them: documented+observed |
	// documented | observed | unsupported | unknown.
	Status string `json:"status"`
}

// SupportStatus is the per-device result.
type SupportStatus struct {
	Operations []OpStatus `json:"operations"`
	// CatalogueQualification is the family's status as shipped (verbatim).
	CatalogueQualification string `json:"catalogueQualification,omitempty"`
	// Qualification is the effective status: the operator override if any, else research_candidate.
	Qualification string         `json:"qualification"`
	Override      *Qualification `json:"override,omitempty"`
	Tier          string         `json:"tier"`
	// PhysicalValidation is true only if a model-specific evidence entry says so (none do today).
	PhysicalValidation bool `json:"physicalValidation"`
}

// capability -> operation mapping for the observed side.
var opCaps = map[string][]model.Capability{
	"identify":  {model.CapIdentify},
	"monitor":   {model.CapReadStatus, model.CapReadSupplies, model.CapReadTrays, model.CapReadCounters, model.CapFirmwareRead, model.CapSubscribeAlerts},
	"configure": {model.CapReadConfig, model.CapWriteConfig},
	"secure":    {model.CapReadSecurity, model.CapWriteSecurity, model.CapManageCerts},
	"update":    {model.CapFirmwareUpdate},
}

// DocumentedOp computes the documented side of one operation for a family and
// (optionally) one of its base model ids. With modelID == "" the family's
// strongest model-specific evidence is used, which is what the catalogue table
// shows; with a model id only that model's evidence counts, which is what a
// device's support card shows. Model-specific operation_evidence wins over the
// management profile; a profile route alone is documented at platform level.
func (c *Catalogue) DocumentedOp(f *Family, modelID, op string) OpStatus {
	st := OpStatus{Operation: op, Documented: Unknown, Observed: Unknown, SourceIDs: []string{}, ObservedBy: []string{}}
	if f == nil {
		st.Status = Unknown
		return st
	}
	var evs []OperationEvidence
	if modelID == "" {
		evs = c.EvidenceForFamily(f)
	} else {
		evs = c.EvidenceForModel(f.Brand, modelID)
	}
	pick := func(e OperationEvidence, ev *Evidence) {
		if ev == nil {
			return
		}
		doc := Documented
		if strings.Contains(strings.ToLower(ev.Status), "not_supported") {
			doc = Unsupported
		}
		// Prefer documented over unsupported over unknown; keep the first of equal rank.
		if rank(doc) > rank(st.Documented) {
			st.Documented, st.DocumentedBasis, st.DocumentedDetail, st.SourceIDs = doc, "operation_evidence:"+e.ID, ev.Status, append([]string{}, ev.SourceIDs...)
		}
	}
	for _, e := range evs {
		switch op {
		case "monitor":
			pick(e, e.Monitoring)
		case "configure":
			pick(e, e.Configuration)
		case "secure":
			pick(e, e.Security)
		case "update":
			pick(e, e.Firmware)
		}
	}
	if st.Documented == Unknown {
		p, ok := c.Profile(f.ManagementProfile)
		if ok {
			var path string
			switch op {
			case "identify":
				// Model identity is what the family's own sources document.
				st.Documented, st.DocumentedBasis, st.DocumentedDetail, st.SourceIDs = Documented, "family:"+f.ID, f.ModelEvidence, append([]string{}, f.ModelSourceIDs...)
			case "monitor":
				path = p.ReadPath
			case "configure":
				path = p.ConfigurationPath
			case "update":
				path = p.FirmwarePath
			}
			if path != "" {
				st.Documented, st.DocumentedBasis, st.DocumentedDetail, st.SourceIDs = Documented, "management_profile:"+f.ManagementProfile, path, append([]string{}, p.SourceIDs...)
			}
		}
	}
	st.Status = st.Documented
	return st
}

func rank(doc string) int {
	switch doc {
	case Documented:
		return 2
	case Unsupported:
		return 1
	}
	return 0
}

// ObservedOp computes the observed side of one operation from the device's
// resolved capability profile. It never touches the network.
func ObservedOp(d *model.Device, op string) (string, []string) {
	by := []string{}
	if d != nil && d.Capabilities != nil {
		for _, cap := range opCaps[op] {
			if drv, ok := d.Capabilities[cap]; ok && drv != "" {
				by = append(by, string(cap)+"·"+drv)
			}
		}
	}
	if op == "identify" && len(by) == 0 && d != nil {
		for _, m := range d.IdentifyMethods {
			by = append(by, "identify·"+m)
		}
	}
	if len(by) > 0 {
		return Observed, by
	}
	return Unknown, by
}

// Support computes the support status of a device given its match and the
// operator's qualification override for the matched family (nil if none).
func (c *Catalogue) Support(d *model.Device, m *Match, override *Qualification) SupportStatus {
	var f *Family
	modelID := ""
	if m != nil && m.Family != nil {
		f = m.Family
		modelID = m.ModelID
	}
	s := SupportStatus{Operations: make([]OpStatus, 0, len(Operations)), Qualification: QualResearchCandidate}
	if f != nil {
		s.CatalogueQualification = f.QualificationStatus
	}
	if override != nil && override.Status != "" {
		s.Qualification = override.Status
		s.Override = override
	}
	anyBoth, anyObs, anyDoc := false, false, false
	for _, op := range Operations {
		st := c.DocumentedOp(f, modelID, op)
		st.Observed, st.ObservedBy = ObservedOp(d, op)
		switch {
		case st.Documented == Documented && st.Observed == Observed:
			st.Status = "documented+observed"
			anyBoth = true
		case st.Observed == Observed:
			st.Status = Observed
			anyObs = true
		case st.Documented != Unknown:
			st.Status = st.Documented
			anyDoc = anyDoc || st.Documented == Documented
		default:
			st.Status = Unknown
		}
		s.Operations = append(s.Operations, st)
	}
	if m != nil {
		for _, e := range m.Evidence {
			if e.PhysicalValidation {
				s.PhysicalValidation = true
			}
		}
	}
	switch {
	case s.Qualification == QualUnsupported:
		s.Tier = TierUnsupported
	case s.Qualification == QualLabValidated || s.Qualification == QualProductionValidated:
		s.Tier = TierValidated
	case anyBoth:
		s.Tier = TierDocumentedObserved
	case anyObs:
		s.Tier = TierObservedOnly
	case anyDoc:
		s.Tier = TierDocumentedOnly
	default:
		s.Tier = TierUnknown
	}
	return s
}

// FamilyEvidenceSummary gives the documented side of every operation for a
// family (no device), for the catalogue table.
func (c *Catalogue) FamilyEvidenceSummary(f *Family) map[string]OpStatus {
	out := map[string]OpStatus{}
	for _, op := range Operations {
		st := c.DocumentedOp(f, "", op)
		st.Observed, st.ObservedBy = Unknown, []string{}
		out[op] = st
	}
	return out
}
