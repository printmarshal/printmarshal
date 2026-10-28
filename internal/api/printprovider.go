package api

import (
	"errors"
	"net/http"
	"sync"

	"printmarshal/internal/model"
	"printmarshal/internal/printprovider"
	"printmarshal/internal/store"
)

// Optional print-provider add-on (spec §8.7). The module is wired from main
// through SetPrintProvider rather than a Server field, so the shared server
// file stays untouched; without it every endpoint answers 503.

var (
	ppMu sync.RWMutex
	pp   *printprovider.Service
)

// SetPrintProvider registers the print-provider service the routes use.
// main calls it once after building the service: api.SetPrintProvider(pp).
func SetPrintProvider(s *printprovider.Service) {
	ppMu.Lock()
	defer ppMu.Unlock()
	pp = s
}

func printProvider() *printprovider.Service {
	ppMu.RLock()
	defer ppMu.RUnlock()
	return pp
}

// printProviderRoutes mounts the optional print-provider (CUPS / IPP Everywhere
// bridge) endpoints.
func (s *Server) printProviderRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/print-provider", s.require(model.RoleViewer, s.getPrintProvider))
	mux.Handle("PUT /api/print-provider", s.require(model.RoleAdmin, s.putPrintProvider))
	mux.Handle("GET /api/print-provider/plan", s.require(model.RoleViewer, s.printProviderPlan))
	mux.Handle("POST /api/print-provider/sync", s.require(model.RoleOperator, s.printProviderSync))
	mux.Handle("DELETE /api/print-provider/queues/{deviceId}", s.require(model.RoleOperator, s.printProviderRemoveQueue))
}

// withPP resolves the service or answers 503.
func withPP(w http.ResponseWriter) *printprovider.Service {
	p := printProvider()
	if p == nil {
		writeErr(w, http.StatusServiceUnavailable, "print provider add-on is not enabled in this build")
	}
	return p
}

func ppStatus(err error) int {
	switch {
	case errors.Is(err, printprovider.ErrDisabled), errors.Is(err, printprovider.ErrListMode):
		return http.StatusConflict
	case errors.Is(err, printprovider.ErrNoQueue), errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// getPrintProvider returns settings, backend availability and the host's
// queue inventory (empty while the module is disabled).
func (s *Server) getPrintProvider(w http.ResponseWriter, r *http.Request) {
	p := withPP(w)
	if p == nil {
		return
	}
	st, err := p.Settings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	queues, err := p.Inventory(r.Context())
	inventoryError := ""
	if err != nil {
		inventoryError = err.Error()
		queues = []printprovider.Queue{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": st, "availability": p.Availability(), "queues": queues, "inventoryError": inventoryError,
	})
}

func settingsMap(st printprovider.Settings) map[string]any {
	return map[string]any{"enabled": st.Enabled, "mode": st.Mode, "queuePrefix": st.QueuePrefix, "defaultOptions": st.DefaultOptions,
		"autoCreateForGroupId": st.AutoCreateForGroupID, "driverless": st.Driverless}
}

// putPrintProvider replaces the settings (admin).
func (s *Server) putPrintProvider(w http.ResponseWriter, r *http.Request) {
	p := withPP(w)
	if p == nil {
		return
	}
	before, err := p.Settings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var in printprovider.Settings
	if err := readJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.AutoCreateForGroupID != "" {
		if _, err := s.Store.GetGroup(r.Context(), in.AutoCreateForGroupID); err != nil {
			writeErr(w, http.StatusBadRequest, "autoCreateForGroupId: unknown group")
			return
		}
	}
	saved, err := p.SaveSettings(r.Context(), in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r.Context(), r, "printprovider.settings.update", "printprovider", settingsMap(before), settingsMap(saved), "ok")
	writeJSON(w, http.StatusOK, saved)
}

// printProviderPlan previews which queues would be created, updated or removed.
func (s *Server) printProviderPlan(w http.ResponseWriter, r *http.Request) {
	p := withPP(w)
	if p == nil {
		return
	}
	plan, err := p.Plan(r.Context())
	if err != nil {
		writeErr(w, ppStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// printProviderSync queues a sync job. Body: {"dryRun": bool}.
func (s *Server) printProviderSync(w http.ResponseWriter, r *http.Request) {
	p := withPP(w)
	if p == nil {
		return
	}
	var req struct {
		DryRun bool `json:"dryRun"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	job, err := p.Sync(r.Context(), actor(r), req.DryRun)
	if err != nil {
		s.audit(r.Context(), r, "printprovider.sync", "printprovider", nil, map[string]any{"dryRun": req.DryRun}, "refused: "+err.Error())
		writeErr(w, ppStatus(err), err.Error())
		return
	}
	s.audit(r.Context(), r, "printprovider.sync", "printprovider", nil, map[string]any{"dryRun": req.DryRun, "jobId": job.ID}, "queued")
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

// printProviderRemoveQueue deletes the queue serving one device.
func (s *Server) printProviderRemoveQueue(w http.ResponseWriter, r *http.Request) {
	p := withPP(w)
	if p == nil {
		return
	}
	id := r.PathValue("deviceId")
	name, err := p.Remove(r.Context(), actor(r), id)
	if err != nil {
		writeErr(w, ppStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": name, "deviceId": id})
}
