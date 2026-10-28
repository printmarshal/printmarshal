package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"printmarshal/internal/firmware"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Firmware API (spec §8.6, §10). Reads are open to viewers; packages are
// admin-managed; campaigns are operator-managed with two explicit gates
// (start, confirm). Every write is audited. No endpoint here touches a device
// directly: delivery happens inside the campaign job.

// firmwareRoutes mounts the firmware endpoints. Implemented by the firmware feature; keep
// handlers in this file (and firmware_*.go) so server.go stays shared.
func (s *Server) firmwareRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/firmware/inventory", s.require(model.RoleViewer, s.fw(s.firmwareInventory)))
	mux.Handle("GET /api/firmware/outdated", s.require(model.RoleViewer, s.fw(s.firmwareOutdated)))
	mux.Handle("GET /api/firmware/packages", s.require(model.RoleViewer, s.fw(s.listFirmwarePackages)))
	mux.Handle("POST /api/firmware/packages", s.require(model.RoleAdmin, s.fw(s.createFirmwarePackage)))
	mux.Handle("GET /api/firmware/packages/{id}", s.require(model.RoleViewer, s.fw(s.getFirmwarePackage)))
	mux.Handle("DELETE /api/firmware/packages/{id}", s.require(model.RoleAdmin, s.fw(s.deleteFirmwarePackage)))
	mux.Handle("GET /api/firmware/campaigns", s.require(model.RoleViewer, s.fw(s.listCampaigns)))
	mux.Handle("POST /api/firmware/campaigns", s.require(model.RoleOperator, s.fw(s.createCampaign)))
	mux.Handle("GET /api/firmware/campaigns/{id}", s.require(model.RoleViewer, s.fw(s.getCampaign)))
	mux.Handle("PATCH /api/firmware/campaigns/{id}", s.require(model.RoleOperator, s.fw(s.patchCampaign)))
	mux.Handle("DELETE /api/firmware/campaigns/{id}", s.require(model.RoleOperator, s.fw(s.deleteCampaign)))
	mux.Handle("POST /api/firmware/campaigns/{id}/start", s.require(model.RoleOperator, s.fw(s.startCampaign)))
	mux.Handle("POST /api/firmware/campaigns/{id}/confirm", s.require(model.RoleOperator, s.fw(s.campaignAction("confirm"))))
	mux.Handle("POST /api/firmware/campaigns/{id}/pause", s.require(model.RoleOperator, s.fw(s.campaignAction("pause"))))
	mux.Handle("POST /api/firmware/campaigns/{id}/resume", s.require(model.RoleOperator, s.fw(s.campaignAction("resume"))))
	mux.Handle("POST /api/firmware/campaigns/{id}/cancel", s.require(model.RoleOperator, s.fw(s.campaignAction("cancel"))))
	mux.Handle("GET /api/firmware/campaigns/{id}/devices", s.require(model.RoleViewer, s.fw(s.campaignDevices)))
	mux.Handle("GET /api/devices/{id}/firmware", s.require(model.RoleViewer, s.fw(s.deviceFirmwareHistory)))
}

// fw guards handlers against a server built without the firmware service.
func (s *Server) fw(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Firmware == nil {
			writeErr(w, http.StatusServiceUnavailable, "firmware service is not configured")
			return
		}
		next(w, r)
	}
}

// firmwareFail maps service errors to HTTP statuses.
func (s *Server) firmwareFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, firmware.ErrInvalid):
		writeErr(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), firmware.ErrInvalid.Error()+": "))
	case errors.Is(err, firmware.ErrConflict):
		writeErr(w, http.StatusConflict, strings.TrimPrefix(err.Error(), firmware.ErrConflict.Error()+": "))
	default:
		s.fail(w, err)
	}
}

// --- inventory ----------------------------------------------------------------

func (s *Server) firmwareInventory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	inv, err := s.Firmware.Inventory(r.Context(), firmware.InventoryFilter{Make: q.Get("make"), Model: q.Get("model"), GroupID: q.Get("group"), Q: q.Get("q")})
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) firmwareOutdated(w http.ResponseWriter, r *http.Request) {
	out, err := s.Firmware.Outdated(r.Context())
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// --- packages -----------------------------------------------------------------

func (s *Server) listFirmwarePackages(w http.ResponseWriter, r *http.Request) {
	pkgs, err := s.Firmware.ListPackages(r.Context())
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pkgs)
}

func (s *Server) getFirmwarePackage(w http.ResponseWriter, r *http.Request) {
	p, err := s.Firmware.GetPackage(r.Context(), r.PathValue("id"))
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// createFirmwarePackage accepts a multipart form: "file" (the image) plus
// vendor, models (comma-separated or repeated), version, minVersion, notes.
func (s *Server) createFirmwarePackage(w http.ResponseWriter, r *http.Request) {
	const maxUpload = 512 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "expected a multipart form with a \"file\" field: "+err.Error())
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "a firmware file is required in the \"file\" field")
		return
	}
	defer file.Close()
	var models []string
	for _, v := range r.MultipartForm.Value["models"] {
		for _, m := range strings.Split(v, ",") {
			if m = strings.TrimSpace(m); m != "" {
				models = append(models, m)
			}
		}
	}
	meta := firmware.PackageMeta{Vendor: r.FormValue("vendor"), Models: models, Version: r.FormValue("version"),
		MinVersion: r.FormValue("minVersion"), Notes: r.FormValue("notes"), FileName: hdr.Filename}
	p, err := s.Firmware.RegisterPackage(r.Context(), meta, io.Reader(file), actor(r))
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	s.audit(r.Context(), r, "firmware.package.create", p.ID, nil,
		map[string]any{"vendor": p.Vendor, "models": p.Models, "version": p.Version, "fileName": p.FileName, "size": p.Size, "sha256": p.SHA256}, "ok")
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) deleteFirmwarePackage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.Firmware.GetPackage(r.Context(), id)
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	if err := s.Firmware.DeletePackage(r.Context(), id); err != nil {
		s.firmwareFail(w, err)
		return
	}
	s.audit(r.Context(), r, "firmware.package.delete", id, map[string]any{"vendor": p.Vendor, "version": p.Version, "fileName": p.FileName}, nil, "ok")
	w.WriteHeader(http.StatusNoContent)
}

// --- campaigns ----------------------------------------------------------------

// campaignRequest is the create/patch body. Pointers distinguish "absent" from
// zero. An empty window object clears the window; clearScheduledAt clears the
// schedule.
type campaignRequest struct {
	Name                    *string                  `json:"name"`
	Description             *string                  `json:"description"`
	PackageID               *string                  `json:"packageId"`
	GroupID                 *string                  `json:"groupId"`
	DeviceIDs               *[]string                `json:"deviceIds"`
	CanaryCount             *int                     `json:"canaryCount"`
	Concurrency             *int                     `json:"concurrency"`
	Window                  *model.MaintenanceWindow `json:"window"`
	FailureThresholdPercent *int                     `json:"failureThresholdPercent"`
	ScheduledAt             *time.Time               `json:"scheduledAt"`
	ClearScheduledAt        bool                     `json:"clearScheduledAt"`
	VerifyTimeoutSecs       *int                     `json:"verifyTimeoutSecs"`
	AllowDowngrade          *bool                    `json:"allowDowngrade"`
}

func (req *campaignRequest) apply(c *model.Campaign) {
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	set(&c.Name, req.Name)
	set(&c.Description, req.Description)
	set(&c.PackageID, req.PackageID)
	set(&c.GroupID, req.GroupID)
	if req.DeviceIDs != nil {
		c.DeviceIDs = *req.DeviceIDs
	}
	if req.CanaryCount != nil {
		c.CanaryCount = *req.CanaryCount
	}
	if req.Concurrency != nil {
		c.Concurrency = *req.Concurrency
	}
	if req.Window != nil {
		c.Window = req.Window
		if len(req.Window.Days) == 0 && req.Window.Start == "" && req.Window.End == "" {
			c.Window = nil
		}
	}
	if req.FailureThresholdPercent != nil {
		c.FailureThresholdPercent = *req.FailureThresholdPercent
	}
	if req.ScheduledAt != nil {
		t := req.ScheduledAt.UTC()
		c.ScheduledAt = &t
	}
	if req.ClearScheduledAt {
		c.ScheduledAt = nil
	}
	if req.VerifyTimeoutSecs != nil {
		c.VerifyTimeoutSecs = *req.VerifyTimeoutSecs
	}
	if req.AllowDowngrade != nil {
		c.AllowDowngrade = *req.AllowDowngrade
	}
}

func campaignAudit(c *model.Campaign) map[string]any {
	return map[string]any{"name": c.Name, "packageId": c.PackageID, "groupId": c.GroupID, "deviceIds": c.DeviceIDs, "canaryCount": c.CanaryCount,
		"concurrency": c.Concurrency, "failureThresholdPercent": c.FailureThresholdPercent, "window": c.Window, "scheduledAt": c.ScheduledAt,
		"allowDowngrade": c.AllowDowngrade, "status": c.Status, "dryRun": c.DryRun}
}

func (s *Server) listCampaigns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cs, err := s.Firmware.ListCampaigns(r.Context(), store.CampaignFilter{Status: q.Get("status"), PackageID: q.Get("package"), Active: q.Get("active") == "true", Limit: intQuery(r, "limit", 0)})
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	views := make([]*firmware.View, 0, len(cs))
	for _, c := range cs {
		v, err := s.Firmware.View(r.Context(), c)
		if err != nil {
			s.firmwareFail(w, err)
			return
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) createCampaign(w http.ResponseWriter, r *http.Request) {
	var req campaignRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	c := &model.Campaign{CanaryCount: firmware.CanaryDefault, Concurrency: firmware.DefaultConcurrency}
	req.apply(c)
	if err := s.Firmware.CreateCampaign(r.Context(), c, actor(r)); err != nil {
		s.firmwareFail(w, err)
		return
	}
	s.audit(r.Context(), r, "firmware.campaign.create", c.ID, nil, campaignAudit(c), "ok")
	s.writeCampaign(w, r, http.StatusCreated, c)
}

func (s *Server) writeCampaign(w http.ResponseWriter, r *http.Request, status int, c *model.Campaign) {
	v, err := s.Firmware.View(r.Context(), c)
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	writeJSON(w, status, v)
}

// campaignDeviceRow is a campaign device joined with the device's identity.
type campaignDeviceRow struct {
	*model.CampaignDevice
	Name    string `json:"name"`
	Address string `json:"address"`
	Model   string `json:"model"`
}

func (s *Server) campaignRows(r *http.Request, id string) ([]campaignDeviceRow, error) {
	devs, err := s.Firmware.Devices(r.Context(), id)
	if err != nil {
		return nil, err
	}
	rows := make([]campaignDeviceRow, 0, len(devs))
	for _, d := range devs {
		row := campaignDeviceRow{CampaignDevice: d}
		if dev, err := s.Store.GetDevice(r.Context(), d.DeviceID); err == nil {
			row.Name, row.Address, row.Model = dev.Name(), dev.Address, dev.Model
		} else {
			row.Name = "(deleted device)"
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *Server) getCampaign(w http.ResponseWriter, r *http.Request) {
	c, err := s.Firmware.GetCampaign(r.Context(), r.PathValue("id"))
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	v, err := s.Firmware.View(r.Context(), c)
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	rows, err := s.campaignRows(r, c.ID)
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"campaign": v, "devices": rows})
}

func (s *Server) patchCampaign(w http.ResponseWriter, r *http.Request) {
	c, err := s.Firmware.GetCampaign(r.Context(), r.PathValue("id"))
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	var req campaignRequest
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	before := campaignAudit(c)
	req.apply(c)
	if err := s.Firmware.UpdateCampaign(r.Context(), c); err != nil {
		s.firmwareFail(w, err)
		return
	}
	s.audit(r.Context(), r, "firmware.campaign.update", c.ID, before, campaignAudit(c), "ok")
	s.writeCampaign(w, r, http.StatusOK, c)
}

func (s *Server) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Firmware.GetCampaign(r.Context(), id)
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	if err := s.Firmware.DeleteCampaign(r.Context(), id); err != nil {
		s.firmwareFail(w, err)
		return
	}
	s.audit(r.Context(), r, "firmware.campaign.delete", id, campaignAudit(c), nil, "ok")
	w.WriteHeader(http.StatusNoContent)
}

// startCampaign is the explicit confirmation gate for wave 1. Body: {"dryRun": bool} (optional).
func (s *Server) startCampaign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DryRun bool `json:"dryRun"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	id := r.PathValue("id")
	c, err := s.Firmware.Start(r.Context(), id, actor(r), firmware.StartOptions{DryRun: req.DryRun})
	if err != nil {
		s.audit(r.Context(), r, "firmware.campaign.start", id, nil, map[string]any{"dryRun": req.DryRun}, "refused: "+err.Error())
		s.firmwareFail(w, err)
		return
	}
	s.audit(r.Context(), r, "firmware.campaign.start", c.ID, nil, campaignAudit(c), "ok")
	s.writeCampaign(w, r, http.StatusOK, c)
}

// campaignAction handles confirm / pause / resume / cancel.
func (s *Server) campaignAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var (
			c   *model.Campaign
			err error
		)
		switch action {
		case "confirm":
			c, err = s.Firmware.Confirm(r.Context(), id, actor(r))
		case "pause":
			c, err = s.Firmware.Pause(r.Context(), id, actor(r))
		case "resume":
			c, err = s.Firmware.Resume(r.Context(), id, actor(r))
		case "cancel":
			c, err = s.Firmware.Cancel(r.Context(), id, actor(r))
		default:
			err = fmt.Errorf("unknown action %q", action)
		}
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				s.audit(r.Context(), r, "firmware.campaign."+action, id, nil, nil, "refused: "+err.Error())
			}
			s.firmwareFail(w, err)
			return
		}
		s.audit(r.Context(), r, "firmware.campaign."+action, c.ID, nil, map[string]any{"status": c.Status, "wave": c.Wave, "confirmed": c.Confirmed}, "ok")
		s.writeCampaign(w, r, http.StatusOK, c)
	}
}

func (s *Server) campaignDevices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.campaignRows(r, r.PathValue("id"))
	if err != nil {
		s.firmwareFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) deviceFirmwareHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dev, err := s.Store.GetDevice(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	hist, err := s.Store.FirmwareHistory(r.Context(), id, intQuery(r, "limit", 50))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deviceId": id, "firmware": dev.Firmware, "history": hist})
}
