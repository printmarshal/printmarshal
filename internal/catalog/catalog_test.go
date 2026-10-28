package catalog

import (
	"strings"
	"testing"

	"printmarshal/internal/model"
)

func TestLoadAndConsistency(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s := c.Summary()
	if s.Families != 126 || s.BaseIdentifiers != 495 || s.Brands != 15 {
		t.Errorf("summary = %d families, %d ids, %d brands; want 126/495/15", s.Families, s.BaseIdentifiers, s.Brands)
	}
	if s.Families != c.Research.FamilyGroups || s.BaseIdentifiers != c.Research.BaseModelIdentifiers || s.Brands != c.Research.Brands {
		t.Errorf("computed summary %+v disagrees with the research's own counts %+v", s, c.Research)
	}
	for _, f := range c.Families {
		if _, ok := c.ManagementProfiles[f.ManagementProfile]; !ok {
			t.Errorf("%s: profile %q missing", f.ID, f.ManagementProfile)
		}
		if f.QualificationStatus != "research_candidate_unvalidated" {
			t.Errorf("%s: shipped qualification must stay research_candidate_unvalidated, got %q", f.ID, f.QualificationStatus)
		}
		for _, sid := range f.ModelSourceIDs {
			if _, ok := c.Sources[sid]; !ok {
				t.Errorf("%s: source %q missing", f.ID, sid)
			}
		}
	}
	for _, e := range c.OperationEvidence {
		for _, mid := range e.ModelIDs {
			if isPointerModelID(mid) {
				continue
			}
			if len(c.FamiliesForModelID(e.Brand, mid)) == 0 {
				t.Errorf("evidence %s: model id %q belongs to no %s family", e.ID, mid, e.Brand)
			}
		}
		if e.PhysicalValidation {
			t.Errorf("evidence %s claims physical validation; the research reports none", e.ID)
		}
	}
	if s.PhysicallyValidated != 0 {
		t.Errorf("physically validated = %d, research says 0", s.PhysicallyValidated)
	}
	// Validate is what Parse runs; a broken document must be rejected.
	if _, err := Parse([]byte(`{"families":[{"id":"X1","brand":"HP","model_ids":["Z1"],"management_profile":"NOPE"}],"management_profiles":{},"sources":{}}`)); err == nil || !strings.Contains(err.Error(), "NOPE") {
		t.Errorf("Parse should reject a missing profile, got %v", err)
	}
	if _, err := Parse([]byte(`{"families":[{"id":"X1","brand":"HP","model_ids":["Z1"],"management_profile":"P"}],"management_profiles":{"P":{"source_ids":["S9"]}},"sources":{}}`)); err == nil || !strings.Contains(err.Error(), "S9") {
		t.Errorf("Parse should reject a missing source, got %v", err)
	}
}

func TestFamilyListFilters(t *testing.T) {
	c := MustLoad()
	if n := len(c.FamilyList(Filter{Brand: "hp"})); n != 47 {
		t.Errorf("HP families = %d, want 47", n)
	}
	if n := len(c.FamilyList(Filter{Q: "E87770"})); n != 1 {
		t.Errorf("q=E87770 families = %d, want 1", n)
	}
	if n := len(c.FamilyList(Filter{Q: "im c3010"})); n != 2 { // RI02 and the RI04 "IM C3010SD" sibling
		t.Errorf("q='im c3010' families = %d, want 2", n)
	}
	if n := len(c.FamilyList(Filter{Segment: "A3 colour laser MFP", Brand: "Xerox"})); n == 0 {
		t.Error("segment filter returned nothing")
	}
	if len(c.Brands()) != 15 || len(c.Segments()) == 0 {
		t.Error("brands/segments")
	}
}

func TestMatch(t *testing.T) {
	c := MustLoad()
	cases := []struct {
		name, mk, model, descr    string
		ent                       int
		wantOK                    bool
		wantConf, wantFam, wantID string
		wantBrand                 string
	}{
		{"HP M283fdw is not an enterprise/managed model: brand-only", "HP", "HP Color LaserJet MFP M283fdw", "", 11, false, ConfidenceBrandOnly, "", "", "HP"},
		{"HP E87770z exact in HP01", "HP", "Color LaserJet Managed MFP E87770z", "", 11, true, ConfidenceExact, "HP01", "E87770", "HP"},
		{"HP generic E877dn maps to HP01 at family level", "HP", "HP Color LaserJet E877dn", "", 11, true, ConfidenceFamily, "HP01", "E87740", "HP"},
		{"Savin badge resolves to a Ricoh family", "Savin", "SAVIN IM C3010", "RICOH IM C3010 1.10 / RICOH Network Printer C model", 367, true, ConfidenceExact, "RI02", "IM C3010", "Ricoh"},
		{"Konica bizhub C300i", "KONICA MINOLTA", "KONICA MINOLTA bizhub C300i", "", 18334, true, ConfidenceExact, "KM01", "C300i", "Konica Minolta"},
		{"unknown make and enterprise: none", "Acme", "Acme LaserPrint 4200dn", "Acme LaserPrint 4200dn", 99999, false, ConfidenceNone, "", "", ""},
		{"enterprise number alone resolves the brand", "", "e-STUDIO6529A", "TOSHIBA e-STUDIO6529A", 1129, true, ConfidenceExact, "TO04", "6529A", "Toshiba"},
		{"Sharp negative-control A4 model", "SHARP", "SHARP MX-C407P", "", 2385, true, ConfidenceExact, "SH05", "C407P", "Sharp"},
		{"Lexmark bundle suffix", "Lexmark", "Lexmark CX943adse", "", 641, true, ConfidenceExact, "LE03", "CX943", "Lexmark"},
		{"Brother hyphenated id", "Brother", "Brother MFC-L9670CDN", "", 2435, true, ConfidenceExact, "BR02", "MFC-L9670CDN", "Brother"},
		{"HP Pro model: brand-only", "HP", "HP Color LaserJet Pro M454dw", "", 11, false, ConfidenceBrandOnly, "", "", "HP"},
		{"HP Enterprise M507 (prefix suffix dn)", "HP", "HP LaserJet Enterprise M507dn", "", 11, true, ConfidenceExact, "HP35", "M507", "HP"},
		{"wrong brand for a known id: brand-only", "Canon", "Canon E87770", "", 1602, false, ConfidenceBrandOnly, "", "", "Canon"},
	}
	for _, tc := range cases {
		m, ok := c.Match(tc.mk, tc.model, tc.descr, tc.ent)
		if ok != tc.wantOK || m.Confidence != tc.wantConf || m.FamilyID != tc.wantFam || m.ModelID != tc.wantID || m.Brand != tc.wantBrand {
			t.Errorf("%s: ok=%v conf=%s fam=%s id=%s brand=%s (%s)", tc.name, ok, m.Confidence, m.FamilyID, m.ModelID, m.Brand, m.Reason)
		}
		if m.Observations.Make != tc.mk || m.Observations.Model != tc.model || m.Observations.SysDescr != tc.descr || m.Observations.EnterpriseID != tc.ent {
			t.Errorf("%s: observations not kept verbatim: %+v", tc.name, m.Observations)
		}
		if m.Evidence == nil {
			t.Errorf("%s: evidence must be an empty list, not nil", tc.name)
		}
	}
	// E87770 carries the HP_E877 evidence; E82670 has none.
	m, _ := c.Match("HP", "E87770z", "", 0)
	if len(m.Evidence) != 1 || m.Evidence[0].ID != "HP_E877" || m.Profile == nil || m.ProfileID != "HP_FS" {
		t.Errorf("E87770 evidence/profile: %+v", m)
	}
	m, _ = c.Match("HP", "E82670z", "", 0)
	if len(m.Evidence) != 0 {
		t.Errorf("E82670 should have no model-specific evidence, got %v", m.Evidence)
	}
}

func TestMatchRangesAndWildcards(t *testing.T) {
	c, err := Parse([]byte(`{"families":[
		{"id":"T1","brand":"HP","model_prefix":"Color LaserJet Pro MFP","model_ids":["M282-M285"],"management_profile":"P","model_source_ids":[]},
		{"id":"T2","brand":"HP","model_prefix":"LaserJet Pro","model_ids":["M40x"],"management_profile":"P","model_source_ids":[]},
		{"id":"T3","brand":"HP","model_prefix":"LaserJet Enterprise","model_ids":["M406"],"management_profile":"P","model_source_ids":[]}],
		"management_profiles":{"P":{"label":"p"}},"sources":{},"operation_evidence":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := c.Match("HP", "HP Color LaserJet Pro MFP M283fdw", "", 11)
	if !ok || m.Confidence != ConfidenceExact || m.FamilyID != "T1" {
		t.Errorf("range: %+v", m)
	}
	if m, ok := c.Match("HP", "HP Color LaserJet Pro MFP M286fdw", "", 11); ok || m.Confidence != ConfidenceBrandOnly {
		t.Errorf("outside range: %+v", m)
	}
	// An explicit id beats a wildcard; a wildcard alone is only family-level.
	if m, _ := c.Match("HP", "HP LaserJet Enterprise M406dn", "", 11); m.Confidence != ConfidenceExact || m.FamilyID != "T3" {
		t.Errorf("exact over wildcard: %+v", m)
	}
	if m, _ := c.Match("HP", "HP LaserJet Pro M404dn", "", 11); m.Confidence != ConfidenceFamily || m.FamilyID != "T2" {
		t.Errorf("wildcard: %+v", m)
	}
}

func TestBrands(t *testing.T) {
	for in, want := range map[string]string{"HP": "HP", "Hewlett-Packard": "HP", "HP Inc.": "HP", "RICOH COMPANY, LTD.": "Ricoh", "Lanier": "Ricoh", "konica minolta": "Konica Minolta", "TRIUMPH-ADLER": "Kyocera", "Fuji Xerox": "Fujifilm"} {
		if got, ok := BrandFromMake(in); !ok || got != want {
			t.Errorf("BrandFromMake(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := BrandFromMake("Acme"); ok {
		t.Error("Acme must not resolve")
	}
	if b, ok := BrandFromEnterprise(11); !ok || b != "HP" {
		t.Error("enterprise 11 = HP")
	}
	if _, ok := BrandFromEnterprise(99999); ok {
		t.Error("unknown enterprise must not resolve")
	}
}

func fakeDevice(caps map[model.Capability]string) *model.Device {
	return &model.Device{ID: "d1", Make: "HP", Model: "HP Color LaserJet Managed MFP E87770z", EnterpriseID: 11, Capabilities: caps, IdentifyMethods: []string{"snmp"}}
}

func TestSupportStatus(t *testing.T) {
	c := MustLoad()
	d := fakeDevice(map[model.Capability]string{model.CapIdentify: "snmp", model.CapReadStatus: "snmp", model.CapReadSupplies: "snmp", model.CapReadConfig: "ipp", model.CapWriteConfig: "snmp"})
	m, _ := c.Match(d.Make, d.Model, d.SysDescr, d.EnterpriseID)
	s := c.Support(d, m, nil)
	ops := map[string]OpStatus{}
	for _, o := range s.Operations {
		ops[o.Operation] = o
	}
	if len(ops) != 5 {
		t.Fatalf("operations = %d", len(ops))
	}
	// Monitoring is documented by HP_E877 and observed by snmp.
	if o := ops["monitor"]; o.Documented != Documented || o.Observed != Observed || o.Status != "documented+observed" || !strings.HasPrefix(o.DocumentedBasis, "operation_evidence:HP_E877") || len(o.SourceIDs) == 0 {
		t.Errorf("monitor: %+v", o)
	}
	if o := ops["configure"]; o.Documented != Documented || o.Observed != Observed || len(o.ObservedBy) != 2 {
		t.Errorf("configure: %+v", o)
	}
	// Security is documented (E877dn spec) but no driver serves it: documented only, never "supported".
	if o := ops["secure"]; o.Documented != Documented || o.Observed != Unknown || o.Status != Documented || len(o.ObservedBy) != 0 {
		t.Errorf("secure: %+v", o)
	}
	if o := ops["update"]; o.Documented != Documented || o.Observed != Unknown || o.Status != Documented {
		t.Errorf("update: %+v", o)
	}
	if s.Tier != TierDocumentedObserved || s.Qualification != QualResearchCandidate || s.CatalogueQualification != "research_candidate_unvalidated" || s.PhysicalValidation {
		t.Errorf("tier/qualification: %+v", s)
	}

	// Unknown device: nothing documented; observed only what drivers answered.
	u := &model.Device{ID: "u", Make: "Acme", Model: "Acme 1", Capabilities: map[model.Capability]string{model.CapReadStatus: "snmp"}}
	um, _ := c.Match(u.Make, u.Model, "", 0)
	us := c.Support(u, um, nil)
	if us.Tier != TierObservedOnly {
		t.Errorf("unknown device tier = %s", us.Tier)
	}
	for _, o := range us.Operations {
		if o.Documented != Unknown {
			t.Errorf("unknown device: %s documented = %s, must stay unknown", o.Operation, o.Documented)
		}
		if o.Operation == "secure" && (o.Status != Unknown || o.Observed != Unknown) {
			t.Errorf("unknown device: secure = %+v", o)
		}
	}
	// A device with no capabilities at all and no family: everything unknown.
	e := &model.Device{ID: "e", Make: "Acme"}
	em, _ := c.Match(e.Make, "", "", 0)
	if es := c.Support(e, em, nil); es.Tier != TierUnknown {
		t.Errorf("empty device tier = %s", es.Tier)
	}

	// Sharp negative control: the checked tool path documents "not supported" for configure/update.
	sh := &model.Device{ID: "s", Make: "SHARP", Model: "SHARP MX-C407P", Capabilities: map[model.Capability]string{model.CapReadStatus: "snmp"}}
	sm, _ := c.Match(sh.Make, sh.Model, "", 0)
	ss := c.Support(sh, sm, nil)
	for _, o := range ss.Operations {
		switch o.Operation {
		case "configure", "update":
			if o.Documented != Unsupported || o.Status != Unsupported {
				t.Errorf("sharp %s: %+v", o.Operation, o)
			}
		case "monitor":
			if o.Status != "documented+observed" {
				t.Errorf("sharp monitor: %+v", o)
			}
		}
	}

	// Operator overrides drive the tier; they never alter documented/observed rows.
	ov := &Qualification{FamilyID: "HP01", Status: QualLabValidated, Actor: "ops"}
	if vs := c.Support(d, m, ov); vs.Tier != TierValidated || vs.Qualification != QualLabValidated || vs.Override == nil {
		t.Errorf("override: %+v", vs)
	}
	if vs := c.Support(d, m, &Qualification{FamilyID: "HP01", Status: QualUnsupported}); vs.Tier != TierUnsupported {
		t.Errorf("unsupported override: %+v", vs)
	}
	if !ValidQualification(QualProductionValidated) || ValidQualification("supported") {
		t.Error("ValidQualification")
	}
}

func TestFamilyEvidenceSummary(t *testing.T) {
	c := MustLoad()
	f, _ := c.Family("HP01")
	sum := c.FamilyEvidenceSummary(f)
	if sum["secure"].Documented != Documented || sum["monitor"].Documented != Documented || sum["identify"].Documented != Documented {
		t.Errorf("HP01 summary: %+v", sum)
	}
	f10, _ := c.Family("HP10")
	s10 := c.FamilyEvidenceSummary(f10)
	if s10["secure"].Documented != Unknown || s10["monitor"].Documented != Documented || !strings.HasPrefix(s10["monitor"].DocumentedBasis, "management_profile:") {
		t.Errorf("HP10 summary should be profile-level only: %+v", s10)
	}
	for _, o := range s10 {
		if o.Observed != Unknown {
			t.Errorf("family summary must never claim observation: %+v", o)
		}
	}
}
