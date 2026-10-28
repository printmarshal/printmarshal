package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/security"
	"printmarshal/internal/store"
)

// Security API (spec §8.5, §10). Reads are open to viewers; assessment,
// remediation, rotation and enrollment need an operator; CA profiles are
// admin-only. Every device write runs as a job (202) so it is dry-runnable,
// resumable and audited; remediation and rotation default to dry run unless
// the body says "dryRun": false.

// securityRoutes mounts the security endpoints. Implemented by the security feature; keep
// handlers in this file (and security_*.go) so server.go stays shared.
func (s *Server) securityRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/policies", s.require(model.RoleViewer, s.listPolicies))
	mux.Handle("POST /api/policies", s.require(model.RoleOperator, s.createPolicy))
	mux.Handle("GET /api/policies/{id}", s.require(model.RoleViewer, s.getPolicy))
	mux.Handle("PATCH /api/policies/{id}", s.require(model.RoleOperator, s.patchPolicy))
	mux.Handle("DELETE /api/policies/{id}", s.require(model.RoleOperator, s.deletePolicy))
	mux.Handle("POST /api/policies/{id}/assess", s.require(model.RoleOperator, s.assessPolicy))
	mux.Handle("POST /api/policies/{id}/remediate", s.require(model.RoleOperator, s.remediatePolicy))

	mux.Handle("GET /api/compliance", s.require(model.RoleViewer, s.compliance))
	mux.Handle("GET /api/compliance/history", s.require(model.RoleViewer, s.complianceHistory))

	mux.Handle("GET /api/devices/{id}/security", s.require(model.RoleViewer, s.deviceSecurity))
	mux.Handle("GET /api/devices/{id}/certificates", s.require(model.RoleViewer, s.deviceCertificates))
	mux.Handle("POST /api/devices/{id}/certificates/enroll", s.require(model.RoleOperator, s.enrollCertificate))
	mux.Handle("POST /api/devices/{id}/certificates/{certId}/install", s.require(model.RoleOperator, s.installCertificate))

	mux.Handle("GET /api/ca-profiles", s.require(model.RoleViewer, s.listCAProfiles))
	mux.Handle("POST /api/ca-profiles", s.require(model.RoleAdmin, s.createCAProfile))
	mux.Handle("DELETE /api/ca-profiles/{id}", s.require(model.RoleAdmin, s.deleteCAProfile))

	mux.Handle("POST /api/security/rotate", s.require(model.RoleOperator, s.rotateCredentials))
	mux.Handle("GET /api/security/catalog", s.require(model.RoleViewer, s.securityCatalog))
}

// --- policies -----------------------------------------------------------------

type policyBody struct {
	Name        *string             `json:"name"`
	Description *string             `json:"description"`
	Items       *[]model.PolicyItem `json:"items"`
}

func (s *Server) listPolicies(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Store.ListPolicies(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) createPolicy(w http.ResponseWriter, r *http.Request) {
	var body policyBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p := &model.Policy{CreatedBy: actor(r)}
	if body.Name != nil {
		p.Name = strings.TrimSpace(*body.Name)
	}
	if body.Description != nil {
		p.Description = *body.Description
	}
	if body.Items != nil {
		p.Items = *body.Items
	}
	if err := security.ValidatePolicy(p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Store.CreatePolicy(r.Context(), p); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "a policy with that name exists")
			return
		}
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "policy.create", p.ID, nil, map[string]any{"name": p.Name, "items": len(p.Items)}, "ok")
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) getPolicy(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.GetPolicy(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) patchPolicy(w http.ResponseWriter, r *http.Request) {
	var body policyBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.Store.GetPolicy(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	before := map[string]any{"name": p.Name, "items": len(p.Items)}
	if body.Name != nil {
		p.Name = strings.TrimSpace(*body.Name)
	}
	if body.Description != nil {
		p.Description = *body.Description
	}
	if body.Items != nil {
		p.Items = *body.Items
	}
	if err := security.ValidatePolicy(p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Store.UpdatePolicy(r.Context(), p); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "policy.update", p.ID, before, map[string]any{"name": p.Name, "items": len(p.Items)}, "ok")
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deletePolicy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.Store.GetPolicy(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.DeletePolicy(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "policy.delete", id, map[string]any{"name": p.Name}, nil, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type targetBody struct {
	security.Target
	DryRun *bool `json:"dryRun"`
}

func (s *Server) assessPolicy(w http.ResponseWriter, r *http.Request) {
	var body targetBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	j, err := s.Security.Assess(r.Context(), r.PathValue("id"), body.Target, actor(r))
	if err != nil {
		s.failTarget(w, err)
		return
	}
	s.audit(r.Context(), r, "security.assess", j.ID, nil, j.Target, "queued")
	writeJSON(w, http.StatusAccepted, j)
}

func (s *Server) remediatePolicy(w http.ResponseWriter, r *http.Request) {
	var body targetBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Dry run unless the caller explicitly turns it off (spec §3 principle 4).
	opts := security.Options{DryRun: body.DryRun == nil || *body.DryRun}
	j, err := s.Security.Remediate(r.Context(), r.PathValue("id"), body.Target, opts, actor(r))
	if err != nil {
		s.failTarget(w, err)
		return
	}
	s.audit(r.Context(), r, "security.remediate", j.ID, nil, map[string]any{"target": j.Target, "dryRun": opts.DryRun}, "queued")
	writeJSON(w, http.StatusAccepted, j)
}

// failTarget maps target errors to 400/404.
func (s *Server) failTarget(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	case strings.Contains(err.Error(), "target must") || strings.Contains(err.Error(), "unknown rotation kind") || strings.Contains(err.Error(), "use must"):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		s.fail(w, err)
	}
}

// --- compliance -----------------------------------------------------------------

func (s *Server) compliance(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var deviceIDs []string
	if g := q.Get("group"); g != "" {
		devs, err := s.Security.ResolveTarget(r.Context(), security.Target{GroupID: g})
		if err != nil {
			s.failTarget(w, err)
			return
		}
		deviceIDs = make([]string, 0, len(devs))
		for _, d := range devs {
			deviceIDs = append(deviceIDs, d.ID)
		}
		if len(deviceIDs) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"results": []*model.Assessment{}, "summary": security.Summarize(nil)})
			return
		}
	}
	if d := q.Get("device"); d != "" {
		deviceIDs = []string{d}
	}
	as, err := s.Store.LatestAssessments(r.Context(), q.Get("policy"), deviceIDs)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": as, "summary": security.Summarize(as)})
}

func (s *Server) complianceHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var from, to time.Time
	var err error
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse(time.RFC3339, v); err != nil {
			writeErr(w, http.StatusBadRequest, "from must be RFC 3339")
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339, v); err != nil {
			writeErr(w, http.StatusBadRequest, "to must be RFC 3339")
			return
		}
	}
	pts, err := s.Store.ComplianceHistory(r.Context(), q.Get("policy"), from, to)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": q.Get("policy"), "points": pts})
}

// --- device views ------------------------------------------------------------------

func (s *Server) deviceSecurity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetDevice(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	as, err := s.Store.LatestAssessments(r.Context(), r.URL.Query().Get("policy"), []string{id})
	if err != nil {
		s.fail(w, err)
		return
	}
	certs, err := s.Security.StoredCertificates(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	alerts, _ := s.Store.ListAlerts(r.Context(), store.AlertFilter{DeviceID: id, ActiveOnly: true})
	var secAlerts []*model.Alert
	for _, a := range alerts {
		if a.Source == security.AlertSource {
			secAlerts = append(secAlerts, a)
		}
	}
	if secAlerts == nil {
		secAlerts = []*model.Alert{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"assessments": as, "certificates": certs, "alerts": secAlerts})
}

func (s *Server) deviceCertificates(w http.ResponseWriter, r *http.Request) {
	dev, err := s.Store.GetDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	var certs *security.DeviceCertificates
	if r.URL.Query().Get("live") == "true" {
		certs, err = s.Security.ListCertificates(r.Context(), dev)
	} else {
		certs, err = s.Security.StoredCertificates(r.Context(), dev.ID)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, certs)
}

func (s *Server) enrollCertificate(w http.ResponseWriter, r *http.Request) {
	var req security.EnrollRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.CAProfileID == "" {
		writeErr(w, http.StatusBadRequest, "caProfileId is required")
		return
	}
	j, err := s.Security.EnrollCertificate(r.Context(), r.PathValue("id"), req, actor(r))
	if err != nil {
		s.failTarget(w, err)
		return
	}
	s.audit(r.Context(), r, "security.enroll", j.ID, nil, map[string]any{"device": r.PathValue("id"), "caProfileId": req.CAProfileID, "use": req.Use, "dryRun": req.DryRun}, "queued")
	writeJSON(w, http.StatusAccepted, j)
}

func (s *Server) installCertificate(w http.ResponseWriter, r *http.Request) {
	j, err := s.Security.InstallIssued(r.Context(), r.PathValue("id"), r.PathValue("certId"), actor(r))
	if err != nil {
		s.failTarget(w, err)
		return
	}
	s.audit(r.Context(), r, "security.certificate.install", j.ID, nil, map[string]any{"device": r.PathValue("id"), "issuedId": r.PathValue("certId")}, "queued")
	writeJSON(w, http.StatusAccepted, j)
}

// --- CA profiles ----------------------------------------------------------------------

func (s *Server) listCAProfiles(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Store.ListCAProfiles(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) createCAProfile(w http.ResponseWriter, r *http.Request) {
	var p model.CAProfile
	if err := readJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p.ID, p.Name, p.URL = "", strings.TrimSpace(p.Name), strings.TrimSpace(p.URL)
	switch {
	case p.Name == "":
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	case p.URL == "" || !(strings.HasPrefix(p.URL, "http://") || strings.HasPrefix(p.URL, "https://")):
		writeErr(w, http.StatusBadRequest, "url must be an http(s) URL")
		return
	case p.Protocol != model.EnrollSCEP && p.Protocol != model.EnrollEST && p.Protocol != model.EnrollACME:
		writeErr(w, http.StatusBadRequest, "protocol must be scep, est or acme")
		return
	}
	if p.KeyType != "" && p.KeyType != "rsa2048" && p.KeyType != "rsa3072" && p.KeyType != "ecdsa-p256" {
		writeErr(w, http.StatusBadRequest, "keyType must be rsa2048, rsa3072 or ecdsa-p256")
		return
	}
	if p.CredentialID != "" {
		found := false
		if cs, err := s.Store.ListCredentials(r.Context()); err == nil {
			for _, c := range cs {
				if c.ID == p.CredentialID {
					found = true
				}
			}
		}
		if !found {
			writeErr(w, http.StatusBadRequest, "credentialId does not exist")
			return
		}
	}
	if err := s.Store.CreateCAProfile(r.Context(), &p); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "a CA profile with that name exists")
			return
		}
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "caprofile.create", p.ID, nil, map[string]any{"name": p.Name, "protocol": p.Protocol, "url": p.URL}, "ok")
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) deleteCAProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.Store.GetCAProfile(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.DeleteCAProfile(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "caprofile.delete", id, map[string]any{"name": p.Name}, nil, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- rotation --------------------------------------------------------------------------

func (s *Server) rotateCredentials(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Kind   string          `json:"kind"`
		Target security.Target `json:"target"`
		DryRun *bool           `json:"dryRun"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	opts := security.Options{DryRun: body.DryRun == nil || *body.DryRun}
	j, err := s.Security.Rotate(r.Context(), body.Kind, body.Target, opts, actor(r))
	if err != nil {
		s.failTarget(w, err)
		return
	}
	s.audit(r.Context(), r, "security.rotate", j.ID, nil, map[string]any{"kind": body.Kind, "target": j.Target, "dryRun": opts.DryRun}, "queued")
	writeJSON(w, http.StatusAccepted, j)
}

// securityCatalog lists the security-relevant setting keys, operators and
// severities so a policy editor can offer them.
func (s *Server) securityCatalog(w http.ResponseWriter, r *http.Request) {
	var defs []model.SettingDef
	for _, d := range model.Catalog {
		if d.Security {
			defs = append(defs, d)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": defs, "operators": model.Operators,
		"severities":      []string{model.SeverityItemCritical, model.SeverityItemHigh, model.SeverityItemMedium, model.SeverityItemLow},
		"managementPaths": []string{"snmp", "ipp", "http"}})
}
