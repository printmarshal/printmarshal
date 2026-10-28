package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"printmarshal/internal/catalog"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// catalogRoutes mounts the device model catalogue endpoints (server.go stays
// shared). Everything is readable by viewers; the qualification override is
// an operator write and is audited.
func (s *Server) catalogRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/catalog/summary", s.require(model.RoleViewer, s.catalogSummary))
	mux.Handle("GET /api/catalog/families", s.require(model.RoleViewer, s.catalogFamilies))
	mux.Handle("GET /api/catalog/families/{id}", s.require(model.RoleViewer, s.catalogFamily))
	mux.Handle("PUT /api/catalog/families/{id}/qualification", s.require(model.RoleOperator, s.catalogSetQualification))
	mux.Handle("GET /api/catalog/profiles", s.require(model.RoleViewer, s.catalogProfiles))
	mux.Handle("GET /api/catalog/match", s.require(model.RoleViewer, s.catalogMatch))
	mux.Handle("GET /api/devices/{id}/support", s.require(model.RoleViewer, s.deviceSupport))
}

// catalogStore reaches the CatalogStore methods without changing the shared
// Store interface (store.go). Once CatalogStore is embedded there this can
// become a plain field access.
func (s *Server) catalogStore() (store.CatalogStore, error) {
	cs, ok := s.Store.(store.CatalogStore)
	if !ok {
		return nil, errors.New("the configured store does not implement the catalogue qualification table")
	}
	return cs, nil
}

func (s *Server) catalogue(w http.ResponseWriter) (*catalog.Catalogue, bool) {
	c, err := catalog.Load()
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return c, true
}

// fleetMatch is one device's match, computed over the whole fleet.
type fleetMatch struct {
	dev *model.Device
	m   *catalog.Match
}

// matchFleet matches every device (decommissioned included) against the catalogue.
func (s *Server) matchFleet(ctx context.Context, c *catalog.Catalogue) ([]fleetMatch, error) {
	devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{IncludeDecommissioned: true})
	if err != nil {
		return nil, err
	}
	out := make([]fleetMatch, 0, len(devs))
	for _, d := range devs {
		m, _ := c.Match(d.Make, d.Model, d.SysDescr, d.EnterpriseID)
		out = append(out, fleetMatch{dev: d, m: m})
	}
	return out, nil
}

func (s *Server) qualifications(ctx context.Context) map[string]*catalog.Qualification {
	cs, err := s.catalogStore()
	if err != nil {
		return map[string]*catalog.Qualification{}
	}
	q, err := cs.ListFamilyQualifications(ctx)
	if err != nil {
		s.Log.Warn("catalogue qualifications", "err", err)
		return map[string]*catalog.Qualification{}
	}
	return q
}

func effectiveQualification(q *catalog.Qualification) string {
	if q != nil && q.Status != "" {
		return q.Status
	}
	return catalog.QualResearchCandidate
}

// fleetDevice is the compact device row shown under a family.
type fleetDevice struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Make       string `json:"make"`
	Model      string `json:"model"`
	Address    string `json:"address"`
	State      string `json:"state"`
	Confidence string `json:"confidence"`
	ModelID    string `json:"modelId,omitempty"`
}

func toFleetDevice(fm fleetMatch) fleetDevice {
	return fleetDevice{ID: fm.dev.ID, Name: fm.dev.Name(), Make: fm.dev.Make, Model: fm.dev.Model, Address: fm.dev.Address, State: string(fm.dev.State), Confidence: fm.m.Confidence, ModelID: fm.m.ModelID}
}

func (s *Server) catalogSummary(w http.ResponseWriter, r *http.Request) {
	c, ok := s.catalogue(w)
	if !ok {
		return
	}
	fleet, err := s.matchFleet(r.Context(), c)
	if err != nil {
		s.fail(w, err)
		return
	}
	byConf := map[string]int{catalog.ConfidenceExact: 0, catalog.ConfidenceFamily: 0, catalog.ConfidenceBrandOnly: 0, catalog.ConfidenceNone: 0}
	matched := 0
	for _, fm := range fleet {
		byConf[fm.m.Confidence]++
		if fm.m.FamilyID != "" {
			matched++
		}
	}
	quals := s.qualifications(r.Context())
	overrides := map[string]int{}
	for _, q := range quals {
		overrides[q.Status]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"catalogue": c.Summary(),
		"fleet":     map[string]any{"devices": len(fleet), "matched": matched, "unmatched": len(fleet) - matched, "byConfidence": byConf},
		"overrides": overrides,
		"brands":    c.Brands(),
		"segments":  c.Segments(),
	})
}

// familyRow is the catalogue table row.
type familyRow struct {
	catalog.Family
	ProfileLabel  string                      `json:"profileLabel"`
	Evidence      map[string]catalog.OpStatus `json:"evidence"`
	EvidenceCount int                         `json:"evidenceCount"`
	FleetCount    int                         `json:"fleetCount"`
	Qualification string                      `json:"qualification"`
	Override      *catalog.Qualification      `json:"override,omitempty"`
}

func (s *Server) catalogFamilies(w http.ResponseWriter, r *http.Request) {
	c, ok := s.catalogue(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	fams := c.FamilyList(catalog.Filter{Brand: q.Get("brand"), Q: q.Get("q"), Segment: q.Get("segment")})
	fleet, err := s.matchFleet(r.Context(), c)
	if err != nil {
		s.fail(w, err)
		return
	}
	counts := map[string]int{}
	for _, fm := range fleet {
		if fm.m.FamilyID != "" {
			counts[fm.m.FamilyID]++
		}
	}
	quals := s.qualifications(r.Context())
	rows := make([]familyRow, 0, len(fams))
	for i := range fams {
		f := &fams[i]
		row := familyRow{Family: *f, Evidence: c.FamilyEvidenceSummary(f), FleetCount: counts[f.ID], Qualification: effectiveQualification(quals[f.ID]), Override: quals[f.ID]}
		if p, ok := c.Profile(f.ManagementProfile); ok {
			row.ProfileLabel = p.Label
		}
		row.EvidenceCount = len(c.EvidenceForFamily(f))
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"families": rows, "total": len(rows)})
}

func (s *Server) catalogFamily(w http.ResponseWriter, r *http.Request) {
	c, ok := s.catalogue(w)
	if !ok {
		return
	}
	f, ok := c.Family(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "no such family")
		return
	}
	fleet, err := s.matchFleet(r.Context(), c)
	if err != nil {
		s.fail(w, err)
		return
	}
	devices := []fleetDevice{}
	for _, fm := range fleet {
		if fm.m.FamilyID == f.ID {
			devices = append(devices, toFleetDevice(fm))
		}
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Name < devices[j].Name })
	evidence := c.EvidenceForFamily(f)
	if evidence == nil {
		evidence = []catalog.OperationEvidence{}
	}
	brandEvidence := c.BrandEvidence(f.Brand)
	if brandEvidence == nil {
		brandEvidence = []catalog.OperationEvidence{}
	}
	// Collect every source id touched by the family, its profile, its evidence and conflicts.
	ids := append([]string{}, f.ModelSourceIDs...)
	profile, _ := c.Profile(f.ManagementProfile)
	if profile != nil {
		ids = append(ids, profile.SourceIDs...)
	}
	for _, e := range append(append([]catalog.OperationEvidence{}, evidence...), brandEvidence...) {
		for _, ev := range []*catalog.Evidence{e.Monitoring, e.Configuration, e.Security, e.Firmware, e.AppManagement} {
			if ev != nil {
				ids = append(ids, ev.SourceIDs...)
			}
		}
	}
	conflicts := c.ConflictsFor(f)
	if conflicts == nil {
		conflicts = []catalog.IdentityConflict{}
	}
	for _, k := range conflicts {
		for _, o := range k.Observations {
			ids = append(ids, o.SourceID)
		}
	}
	variants := c.VariantsFor(f)
	if variants == nil {
		variants = []catalog.VariantExample{}
	}
	for _, v := range variants {
		ids = append(ids, v.SourceIDs...)
	}
	quals := s.qualifications(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"family":                 f,
		"profile":                profile,
		"evidence":               evidence,
		"brandEvidence":          brandEvidence,
		"summary":                c.FamilyEvidenceSummary(f),
		"sources":                c.SourcesFor(ids...),
		"conflicts":              conflicts,
		"variants":               variants,
		"devices":                devices,
		"qualification":          effectiveQualification(quals[f.ID]),
		"override":               quals[f.ID],
		"catalogueQualification": f.QualificationStatus,
	})
}

func (s *Server) catalogProfiles(w http.ResponseWriter, r *http.Request) {
	c, ok := s.catalogue(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, c.ManagementProfiles)
}

func (s *Server) catalogMatch(w http.ResponseWriter, r *http.Request) {
	c, ok := s.catalogue(w)
	if !ok {
		return
	}
	q := r.URL.Query()
	if q.Get("make") == "" && q.Get("model") == "" && q.Get("sysDescr") == "" {
		writeErr(w, http.StatusBadRequest, "give at least one of make, model, sysDescr")
		return
	}
	m, _ := c.Match(q.Get("make"), q.Get("model"), q.Get("sysDescr"), intQuery(r, "enterpriseId", 0))
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) deviceSupport(w http.ResponseWriter, r *http.Request) {
	c, ok := s.catalogue(w)
	if !ok {
		return
	}
	d, err := s.Store.GetDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	m, _ := c.Match(d.Make, d.Model, d.SysDescr, d.EnterpriseID)
	var override *catalog.Qualification
	if m.FamilyID != "" {
		if cs, err := s.catalogStore(); err == nil {
			if q, err := cs.GetFamilyQualification(r.Context(), m.FamilyID); err == nil {
				override = q
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"match": m, "support": c.Support(d, m, override)})
}

func (s *Server) catalogSetQualification(w http.ResponseWriter, r *http.Request) {
	c, ok := s.catalogue(w)
	if !ok {
		return
	}
	f, ok := c.Family(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "no such family")
		return
	}
	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	body.Status = strings.TrimSpace(body.Status)
	if !catalog.ValidQualification(body.Status) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("status must be one of %s", strings.Join(catalog.QualificationStatuses, ", ")))
		return
	}
	if len(body.Note) > 2000 {
		writeErr(w, http.StatusBadRequest, "note is too long (max 2000 characters)")
		return
	}
	cs, err := s.catalogStore()
	if err != nil {
		s.fail(w, err)
		return
	}
	var before map[string]any
	if prev, err := cs.GetFamilyQualification(r.Context(), f.ID); err == nil {
		before = map[string]any{"status": prev.Status, "note": prev.Note, "actor": prev.Actor}
	} else {
		before = map[string]any{"status": catalog.QualResearchCandidate, "catalogue": f.QualificationStatus}
	}
	q := &catalog.Qualification{FamilyID: f.ID, Status: body.Status, Note: strings.TrimSpace(body.Note), Actor: actor(r)}
	if err := cs.SetFamilyQualification(r.Context(), q); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "catalog.qualification", f.ID, before, map[string]any{"status": q.Status, "note": q.Note}, "ok")
	writeJSON(w, http.StatusOK, q)
}
