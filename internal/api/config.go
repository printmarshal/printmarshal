package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"printmarshal/internal/configmgmt"
	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Configuration management API (spec §8.4, §10). Reads need the viewer role;
// anything that writes PrintMarshal records or a device needs operator. Every
// device write is a job (202 Accepted) so dry runs and per-device results are
// reported the same way as polls and discovery.
func (s *Server) configRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/profiles", s.require(model.RoleViewer, s.listProfiles))
	mux.Handle("POST /api/profiles", s.require(model.RoleOperator, s.createProfile))
	mux.Handle("GET /api/profiles/{id}", s.require(model.RoleViewer, s.getProfile))
	mux.Handle("PATCH /api/profiles/{id}", s.require(model.RoleOperator, s.patchProfile))
	mux.Handle("DELETE /api/profiles/{id}", s.require(model.RoleOperator, s.deleteProfile))
	mux.Handle("POST /api/profiles/{id}/apply", s.require(model.RoleOperator, s.applyProfile))
	mux.Handle("POST /api/profiles/{id}/drift", s.require(model.RoleViewer, s.profileDrift))

	mux.Handle("GET /api/devices/{id}/config", s.require(model.RoleViewer, s.deviceConfig))
	mux.Handle("GET /api/devices/{id}/config/snapshots", s.require(model.RoleViewer, s.deviceConfigSnapshots))
	mux.Handle("POST /api/devices/{id}/config", s.require(model.RoleOperator, s.applyDeviceConfig))

	mux.Handle("GET /api/config/catalog", s.require(model.RoleViewer, s.configCatalog))
	mux.Handle("GET /api/config/diff", s.require(model.RoleViewer, s.configDiff))
	mux.Handle("GET /api/config/writes", s.require(model.RoleViewer, s.listConfigWrites))
	mux.Handle("GET /api/config/writes/{id}", s.require(model.RoleViewer, s.getConfigWrite))
	mux.Handle("POST /api/config/writes/{id}/revert", s.require(model.RoleOperator, s.revertConfigWrite))
}

// configFail maps service errors to statuses.
func (s *Server) configFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, configmgmt.ErrNoTarget), errors.Is(err, configmgmt.ErrNoChanges):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, engine.ErrUnsupported), errors.Is(err, model.ErrDecommissioned):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, err.Error())
	default:
		s.fail(w, err)
	}
}

// --- profiles ----------------------------------------------------------------

func validateProfileSettings(in []model.SettingChange) ([]model.SettingChange, error) {
	out := make([]model.SettingChange, 0, len(in))
	seen := map[string]bool{}
	for _, ch := range in {
		ch.Key = strings.TrimSpace(ch.Key)
		if ch.Key == "" {
			return nil, errors.New("a setting has no key")
		}
		if seen[ch.Key] {
			return nil, fmt.Errorf("duplicate setting %s", ch.Key)
		}
		seen[ch.Key] = true
		if _, ok := model.CatalogByKey[ch.Key]; !ok && !strings.HasPrefix(ch.Key, "vendor.") {
			return nil, fmt.Errorf("unknown setting %s", ch.Key)
		}
		if model.IsSecretKey(ch.Key) {
			return nil, fmt.Errorf("%s: secrets cannot be stored in a profile", ch.Key)
		}
		v, err := model.CoerceValue(ch.Key, ch.Value)
		if err != nil {
			return nil, err
		}
		ch.Value = v
		out = append(out, ch)
	}
	return out, nil
}

func profileAudit(p *model.Profile) map[string]any {
	return map[string]any{"name": p.Name, "description": p.Description, "settings": p.Settings}
}

func (s *Server) listProfiles(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Store.ListProfiles(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": ps, "total": len(ps)})
}

func (s *Server) createProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
		Settings    []model.SettingChange `json:"settings"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	settings, err := validateProfileSettings(body.Settings)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p := &model.Profile{Name: strings.TrimSpace(body.Name), Description: body.Description, Settings: settings, CreatedBy: actor(r)}
	if err := s.Store.CreateProfile(r.Context(), p); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "a profile with that name exists")
			return
		}
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "profile.create", p.ID, nil, profileAudit(p), "ok")
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	drift, _ := s.Store.ListDriftReports(r.Context(), p.ID, 0)
	writes, _ := s.Store.ListWriteRecords(r.Context(), store.WriteRecordFilter{ProfileID: p.ID, Limit: 50})
	writeJSON(w, http.StatusOK, map[string]any{"profile": p, "drift": drift, "writes": writes})
}

func (s *Server) patchProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        *string                `json:"name"`
		Description *string                `json:"description"`
		Settings    *[]model.SettingChange `json:"settings"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := s.Store.GetProfile(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	before := profileAudit(p)
	if body.Name != nil {
		if strings.TrimSpace(*body.Name) == "" {
			writeErr(w, http.StatusBadRequest, "name is required")
			return
		}
		p.Name = strings.TrimSpace(*body.Name)
	}
	if body.Description != nil {
		p.Description = *body.Description
	}
	if body.Settings != nil {
		settings, err := validateProfileSettings(*body.Settings)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		p.Settings = settings
	}
	if err := s.Store.UpdateProfile(r.Context(), p); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeErr(w, http.StatusConflict, "a profile with that name exists")
			return
		}
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "profile.update", p.ID, before, profileAudit(p), "ok")
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.Store.GetProfile(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.DeleteProfile(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "profile.delete", id, profileAudit(p), nil, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type applyBody struct {
	Target model.ConfigTarget `json:"target"`
	DryRun *bool              `json:"dryRun"`
}

// dryRunOf is the write gate for every device-changing endpoint: a body
// that omits dryRun gets a dry run; only an explicit false writes (spec §3
// principle 4, code review H-007).
func dryRunOf(p *bool) bool { return p == nil || *p }

func (s *Server) applyProfile(w http.ResponseWriter, r *http.Request) {
	var body applyBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	dryRun := dryRunOf(body.DryRun)
	job, err := s.ConfigMgmt.Apply(r.Context(), configmgmt.ApplyRequest{ProfileID: id, Target: body.Target, DryRun: dryRun}, actor(r))
	if err != nil {
		s.configFail(w, err)
		return
	}
	s.audit(r.Context(), r, "profile.apply", id, nil, map[string]any{"jobId": job.ID, "target": job.Target, "dryRun": dryRun}, "queued")
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (s *Server) profileDrift(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Target  model.ConfigTarget `json:"target"`
		Refresh bool               `json:"refresh"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	reports, err := s.ConfigMgmt.DriftGroup(r.Context(), body.Target, r.PathValue("id"), body.Refresh)
	if err != nil {
		s.configFail(w, err)
		return
	}
	drifted := 0
	for _, rep := range reports {
		if rep.Drifted > 0 || len(rep.Items) > 0 {
			drifted++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports, "devices": len(reports), "drifted": drifted})
}

// --- device config -------------------------------------------------------------

func (s *Server) deviceConfig(w http.ResponseWriter, r *http.Request) {
	refresh := r.URL.Query().Get("refresh") == "true"
	dev, err := s.Store.GetDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		s.configFail(w, err)
		return
	}
	if dev.Decommissioned() {
		s.decommissionedConfig(w, r, dev, refresh)
		return
	}
	doc, sn, err := s.ConfigMgmt.CurrentConfig(r.Context(), dev.ID, refresh)
	if err != nil {
		s.configFail(w, err)
		return
	}
	out := map[string]any{"document": doc, "fresh": refresh}
	if sn != nil {
		out["snapshotId"] = sn.ID
		out["at"] = sn.At
	}
	writeJSON(w, http.StatusOK, out)
}

// decommissionedConfig answers from the store alone: the last snapshot if there
// is one, otherwise a null document, and a field saying why nothing was read.
// Unread settings stay absent; nothing is inferred about them.
func (s *Server) decommissionedConfig(w http.ResponseWriter, r *http.Request, dev *model.Device, refresh bool) {
	if refresh {
		s.configFail(w, model.ErrDecommissioned)
		return
	}
	out := map[string]any{"document": nil, "fresh": false, "contacted": false, "notContactedReason": string(model.StateDecommissioned)}
	sn, err := s.Store.LatestConfigSnapshot(r.Context(), dev.ID)
	switch {
	case err == nil:
		out["document"], out["snapshotId"], out["at"] = sn.Document, sn.ID, sn.At
	case !errors.Is(err, store.ErrNotFound):
		s.configFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deviceConfigSnapshots(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetDevice(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	sns, err := s.Store.ListConfigSnapshots(r.Context(), id, intQuery(r, "limit", 0))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": sns, "total": len(sns)})
}

func (s *Server) applyDeviceConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Changes []model.SettingChange `json:"changes"`
		DryRun  *bool                 `json:"dryRun"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	for _, ch := range body.Changes {
		if _, ok := model.CatalogByKey[ch.Key]; !ok && !strings.HasPrefix(ch.Key, "vendor.") {
			writeErr(w, http.StatusBadRequest, "unknown setting "+ch.Key)
			return
		}
	}
	job, err := s.ConfigMgmt.Apply(r.Context(), configmgmt.ApplyRequest{Changes: body.Changes, Target: model.ConfigTarget{DeviceIDs: []string{id}}, DryRun: dryRunOf(body.DryRun)}, actor(r))
	if err != nil {
		s.configFail(w, err)
		return
	}
	s.audit(r.Context(), r, "device.config.apply", id, nil, map[string]any{"jobId": job.ID, "changes": job.Params["changes"], "dryRun": dryRunOf(body.DryRun)}, "queued")
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

// --- catalogue, diff, writes -----------------------------------------------------

func (s *Server) configCatalog(w http.ResponseWriter, r *http.Request) {
	type def struct {
		Key      string `json:"key"`
		Area     string `json:"area"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Options  []any  `json:"options,omitempty"`
		Security bool   `json:"security"`
		Help     string `json:"help,omitempty"`
	}
	out := make([]def, 0, len(model.Catalog))
	for _, d := range model.Catalog {
		out = append(out, def{d.Key, d.Area, d.Name, d.Type, d.Options, d.Security, model.SettingHelp(d.Key)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"areas": model.Areas, "settings": out})
}

func (s *Server) configDiff(w http.ResponseWriter, r *http.Request) {
	a, b := r.URL.Query().Get("a"), r.URL.Query().Get("b")
	if a == "" || b == "" {
		writeErr(w, http.StatusBadRequest, "a and b device ids are required")
		return
	}
	refresh := r.URL.Query().Get("refresh") == "true"
	da, _, err := s.ConfigMgmt.CurrentConfig(r.Context(), a, refresh)
	if err != nil {
		s.configFail(w, err)
		return
	}
	db, _, err := s.ConfigMgmt.CurrentConfig(r.Context(), b, refresh)
	if err != nil {
		s.configFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, configmgmt.Diff(da, db))
}

func (s *Server) listConfigWrites(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ws, err := s.Store.ListWriteRecords(r.Context(), store.WriteRecordFilter{DeviceID: q.Get("device"), ProfileID: q.Get("profile"), JobID: q.Get("job"), Limit: intQuery(r, "limit", 0)})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"writes": ws, "total": len(ws)})
}

func (s *Server) getConfigWrite(w http.ResponseWriter, r *http.Request) {
	rec, err := s.Store.GetWriteRecord(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"write": rec, "reversible": rec.Reversible()})
}

func (s *Server) revertConfigWrite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DryRun *bool `json:"dryRun"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	id := r.PathValue("id")
	job, err := s.ConfigMgmt.Revert(r.Context(), id, dryRunOf(body.DryRun), actor(r))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.fail(w, err)
			return
		}
		if errors.Is(err, model.ErrDecommissioned) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r.Context(), r, "config.revert", id, nil, map[string]any{"jobId": job.ID, "dryRun": dryRunOf(body.DryRun)}, "queued")
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}
