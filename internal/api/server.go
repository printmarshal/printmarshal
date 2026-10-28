// Package api is the REST + WebSocket surface (spec §10). Every endpoint that
// touches devices is read-only in this phase; every write to PrintMarshal's
// own records is audited and role-gated (spec §11).
package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/auth"
	"printmarshal/internal/config"
	"printmarshal/internal/configmgmt"
	"printmarshal/internal/firmware"
	"printmarshal/internal/fleet"
	"printmarshal/internal/model"
	"printmarshal/internal/security"
	"printmarshal/internal/store"
)

// Server holds dependencies for the handlers.
type Server struct {
	Store      store.Store
	Fleet      *fleet.Service
	ConfigMgmt *configmgmt.Service
	Security   *security.Service
	Firmware   *firmware.Service
	Auth       *auth.Service
	Hub        *Hub
	Cfg        *config.Config
	Log        *slog.Logger
	UI         http.Handler
	Version    string
}

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.Version})
	})

	// Auth
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.Handle("GET /api/auth/me", s.require(model.RoleViewer, s.me))
	mux.Handle("POST /api/auth/password", s.require(model.RoleViewer, s.changePassword))

	// Live stream
	mux.Handle("GET /api/stream", s.require(model.RoleViewer, s.Hub.ServeHTTP))

	// Dashboard, devices
	mux.Handle("GET /api/dashboard", s.require(model.RoleViewer, s.dashboard))
	mux.Handle("GET /api/devices", s.require(model.RoleViewer, s.listDevices))
	mux.Handle("GET /api/devices.csv", s.require(model.RoleViewer, s.exportDevicesCSV))
	mux.Handle("GET /api/supplies.csv", s.require(model.RoleViewer, s.exportSuppliesCSV))
	mux.Handle("GET /api/devices/{id}", s.require(model.RoleViewer, s.getDevice))
	mux.Handle("PATCH /api/devices/{id}", s.require(model.RoleOperator, s.patchDevice))
	mux.Handle("DELETE /api/devices/{id}", s.require(model.RoleAdmin, s.deleteDevice))
	mux.Handle("POST /api/devices/{id}/poll", s.require(model.RoleOperator, s.pollDevice))
	mux.Handle("POST /api/devices/{id}/state", s.require(model.RoleOperator, s.setDeviceState))
	mux.Handle("GET /api/devices/{id}/alerts", s.require(model.RoleViewer, s.deviceAlerts))
	mux.Handle("GET /api/devices/{id}/telemetry", s.require(model.RoleViewer, s.deviceTelemetry))
	mux.Handle("GET /api/telemetry", s.require(model.RoleViewer, s.telemetry))

	// Groups
	mux.Handle("GET /api/groups", s.require(model.RoleViewer, s.listGroups))
	mux.Handle("POST /api/groups", s.require(model.RoleOperator, s.createGroup))
	mux.Handle("GET /api/groups/{id}", s.require(model.RoleViewer, s.getGroup))
	mux.Handle("PATCH /api/groups/{id}", s.require(model.RoleOperator, s.updateGroup))
	mux.Handle("DELETE /api/groups/{id}", s.require(model.RoleOperator, s.deleteGroup))
	mux.Handle("PUT /api/groups/{id}/members", s.require(model.RoleOperator, s.setGroupMembers))
	mux.Handle("GET /api/groups/{id}/devices", s.require(model.RoleViewer, s.groupDevices))

	// Discovery and jobs
	mux.Handle("POST /api/discovery/runs", s.require(model.RoleOperator, s.startDiscovery))
	mux.Handle("GET /api/discovery/runs", s.require(model.RoleViewer, s.listDiscoveryRuns))
	mux.Handle("POST /api/poll", s.require(model.RoleOperator, s.pollAll))
	mux.Handle("GET /api/jobs", s.require(model.RoleViewer, s.listJobs))
	mux.Handle("GET /api/jobs/{id}", s.require(model.RoleViewer, s.getJob))

	// Alerts
	mux.Handle("GET /api/alerts", s.require(model.RoleViewer, s.listAlerts))
	mux.Handle("POST /api/alerts/{id}/ack", s.require(model.RoleOperator, s.ackAlert))
	mux.Handle("POST /api/alerts/{id}/clear", s.require(model.RoleOperator, s.clearAlert))

	// Credentials (write-only secrets)
	mux.Handle("GET /api/credentials", s.require(model.RoleOperator, s.listCredentials))
	mux.Handle("POST /api/credentials", s.require(model.RoleAdmin, s.createCredential))
	mux.Handle("DELETE /api/credentials/{id}", s.require(model.RoleAdmin, s.deleteCredential))

	// Consumables stock inventory and installed-supply links (consumables.go)
	s.consumableRoutes(mux)
	// Feature areas (each in its own file; see config.go, security.go, firmware.go)
	s.configRoutes(mux)
	s.securityRoutes(mux)
	s.firmwareRoutes(mux)
	s.catalogRoutes(mux)
	s.printProviderRoutes(mux)

	// Users, audit, notifications, settings
	mux.Handle("GET /api/users", s.require(model.RoleAdmin, s.listUsers))
	mux.Handle("POST /api/users", s.require(model.RoleAdmin, s.createUser))
	mux.Handle("DELETE /api/users/{id}", s.require(model.RoleAdmin, s.deleteUser))
	mux.Handle("GET /api/audit", s.require(model.RoleOperator, s.listAudit))
	mux.Handle("GET /api/notifications", s.require(model.RoleOperator, s.listNotifications))
	mux.Handle("GET /api/settings", s.require(model.RoleViewer, s.getSettings))
	mux.Handle("PATCH /api/settings", s.require(model.RoleAdmin, s.patchSettings))
	mux.Handle("DELETE /api/settings", s.require(model.RoleAdmin, s.resetSettings))
	mux.Handle("GET /api/drivers", s.require(model.RoleViewer, s.listDrivers))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "no such endpoint")
	})
	if s.UI != nil {
		mux.Handle("/", s.UI)
	}
	return s.logging(secureHeaders(mux))
}

// --- middleware -------------------------------------------------------------

func (s *Server) require(min model.Role, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.Auth.Authenticate(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !u.Role.Allows(min) {
			writeErr(w, http.StatusForbidden, fmt.Sprintf("requires %s role", min))
			return
		}
		next(w, r.WithContext(auth.WithUser(r.Context(), u)))
	})
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(c int) { w.status = c; w.ResponseWriter.WriteHeader(c) }

// Unwrap lets the WebSocket upgrader reach the underlying hijacker.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/stream" {
			s.Log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
		}
	})
}

// --- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	default:
		s.Log.Error("request failed", "err", err)
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func actor(r *http.Request) string {
	if u := auth.UserFrom(r.Context()); u != nil {
		return u.Username
	}
	return "anonymous"
}

func (s *Server) audit(ctx context.Context, r *http.Request, action, target string, before, after map[string]any, result string) {
	_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor(r), Action: action, Target: target, Before: before, After: after, Result: result})
}

func intQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// --- auth handlers ----------------------------------------------------------

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	token, u, err := s.Auth.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		time.Sleep(300 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	s.Auth.SetCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "token": token})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if tok := auth.TokenFromRequest(r); tok != "" {
		_ = s.Auth.Logout(r.Context(), tok)
	}
	s.Auth.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, auth.UserFrom(r.Context()))
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u := auth.UserFrom(r.Context())
	if _, _, err := s.Auth.Login(r.Context(), u.Username, body.Current); err != nil {
		writeErr(w, http.StatusForbidden, "current password is wrong")
		return
	}
	if err := s.Auth.SetPassword(r.Context(), u.ID, body.New); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r.Context(), r, "user.password", u.Username, nil, nil, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- dashboard / devices ----------------------------------------------------

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ds, err := s.Store.Dashboard(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	alerts, _ := s.Store.ListAlerts(r.Context(), store.AlertFilter{ActiveOnly: true, Limit: 20})
	jobs, _ := s.Store.ListJobs(r.Context(), store.JobFilter{Limit: 10})
	attention, _ := s.Store.ListDevices(r.Context(), store.DeviceFilter{Sort: "severity", Limit: 10})
	var needs []*model.Device
	for _, d := range attention {
		if d.Status != nil && (d.Status.Severity != model.SeverityOK || !d.Status.Online) {
			needs = append(needs, d)
		}
	}
	if needs == nil {
		needs = []*model.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": ds, "alerts": alerts, "jobs": jobs, "attention": needs, "consumables": s.dashboardConsumables(r.Context())})
}

func deviceFilter(r *http.Request) store.DeviceFilter {
	q := r.URL.Query()
	f := store.DeviceFilter{
		Q: q.Get("q"), State: q.Get("state"), Make: q.Get("make"), Model: q.Get("model"),
		Severity: q.Get("severity"), Sort: q.Get("sort"), Limit: intQuery(r, "limit", 0), Offset: intQuery(r, "offset", 0),
		IncludeDecommissioned: q.Get("includeDecommissioned") == "true",
	}
	if g := q.Get("group"); g != "" {
		f.GroupIDs = []string{g}
	}
	return f
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	f := deviceFilter(r)
	devs, err := s.resolveDevices(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devs, "total": len(devs)})
}

// resolveDevices lists devices, expanding rule-based groups in Go.
func (s *Server) resolveDevices(ctx context.Context, f store.DeviceFilter) ([]*model.Device, error) {
	if len(f.GroupIDs) == 1 {
		g, err := s.Store.GetGroup(ctx, f.GroupIDs[0])
		if err != nil {
			return nil, err
		}
		if g.Kind == "rule" {
			f.GroupIDs = nil
			all, err := s.Store.ListDevices(ctx, f)
			if err != nil {
				return nil, err
			}
			out := []*model.Device{}
			for _, d := range all {
				if store.MatchRule(g.Rule, d) {
					out = append(out, d)
				}
			}
			return out, nil
		}
	}
	return s.Store.ListDevices(ctx, f)
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	d, err := s.Store.GetDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	groups, _ := s.Store.GroupsForDevice(r.Context(), d.ID)
	alerts, _ := s.Store.ListAlerts(r.Context(), store.AlertFilter{DeviceID: d.ID, ActiveOnly: true})
	writeJSON(w, http.StatusOK, map[string]any{"device": d, "groups": groups, "alerts": alerts})
}

func (s *Server) patchDevice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName *string `json:"displayName"`
		Location    *string `json:"location"`
		AssetTag    *string `json:"assetTag"`
		Notes       *string `json:"notes"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	before, err := s.Store.GetDevice(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	d, err := s.Store.PatchDevice(r.Context(), id, store.DevicePatch{DisplayName: body.DisplayName, Location: body.Location, AssetTag: body.AssetTag, Notes: body.Notes})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "device.patch", id, operatorFields(before), operatorFields(d), "ok")
	s.Hub.Publish(model.Event{Type: "device.updated", Data: d})
	writeJSON(w, http.StatusOK, d)
}

func operatorFields(d *model.Device) map[string]any {
	return map[string]any{"displayName": d.DisplayName, "location": d.Location, "assetTag": d.AssetTag, "notes": d.Notes, "state": d.State}
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	before, err := s.Store.GetDevice(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.DeleteDevice(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "device.delete", id, map[string]any{"address": before.Address, "model": before.Model, "serial": before.Serial}, nil, "ok")
	s.Hub.Publish(model.Event{Type: "device.deleted", Data: map[string]any{"id": id}})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setDeviceState(w http.ResponseWriter, r *http.Request) {
	var body struct {
		State model.LifecycleState `json:"state"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	before, err := s.Store.GetDevice(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !model.ValidTransition(before.State, body.State) {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("cannot move from %s to %s", before.State, body.State))
		return
	}
	d, err := s.Store.SetDeviceState(r.Context(), id, body.State)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "device.state", id, map[string]any{"state": before.State}, map[string]any{"state": d.State}, "ok")
	s.Hub.Publish(model.Event{Type: "device.updated", Data: d})
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) pollDevice(w http.ResponseWriter, r *http.Request) {
	d, err := s.Store.GetDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	upd, res, err := s.Fleet.PollDevice(ctx, d)
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"device": upd, "servedBy": res.ServedBy, "errors": res.Errors, "reachable": res.Reachable})
}

func (s *Server) pollAll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceIDs []string `json:"deviceIds"`
		GroupID   string   `json:"groupId"`
	}
	if r.ContentLength > 0 {
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if body.GroupID != "" {
		ids, err := s.Fleet.GroupDeviceIDs(r.Context(), body.GroupID)
		if err != nil {
			s.fail(w, err)
			return
		}
		if len(ids) == 0 {
			writeErr(w, http.StatusBadRequest, "the group has no devices")
			return
		}
		body.DeviceIDs = ids
	}
	j, err := s.Fleet.SubmitPoll(r.Context(), body.DeviceIDs, actor(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, j)
}

func (s *Server) deviceAlerts(w http.ResponseWriter, r *http.Request) {
	a, err := s.Store.ListAlerts(r.Context(), store.AlertFilter{DeviceID: r.PathValue("id"), ActiveOnly: r.URL.Query().Get("all") != "true", Limit: intQuery(r, "limit", 200)})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func telemetryQuery(r *http.Request, deviceID string) store.TelemetryQuery {
	q := store.TelemetryQuery{DeviceID: deviceID, Metric: r.URL.Query().Get("metric"), Label: r.URL.Query().Get("label"), Limit: intQuery(r, "limit", 0)}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.From = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.To = t
		}
	}
	if q.From.IsZero() {
		q.From = time.Now().Add(-7 * 24 * time.Hour)
	}
	return q
}

func (s *Server) deviceTelemetry(w http.ResponseWriter, r *http.Request) {
	samples, err := s.Store.QueryTelemetry(r.Context(), telemetryQuery(r, r.PathValue("id")))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

func (s *Server) telemetry(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("device")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "device is required")
		return
	}
	samples, err := s.Store.QueryTelemetry(r.Context(), telemetryQuery(r, id))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

func (s *Server) exportDevicesCSV(w http.ResponseWriter, r *http.Request) {
	devs, err := s.resolveDevices(r.Context(), deviceFilter(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="devices.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "name", "make", "model", "serial", "address", "hostname", "mac", "firmware", "type", "state", "status", "severity", "location", "assetTag", "pagesTotal", "lastSeen", "lastPolled"})
	for _, d := range devs {
		status, sev := "", ""
		if d.Status != nil {
			status, sev = d.Status.State, string(d.Status.Severity)
		}
		pages := ""
		if d.Counters != nil {
			pages = strconv.FormatInt(d.Counters.Total, 10)
		}
		lp := ""
		if d.LastPolled != nil {
			lp = d.LastPolled.Format(time.RFC3339)
		}
		_ = cw.Write([]string{d.ID, d.Name(), d.Make, d.Model, d.Serial, d.Address, d.Hostname, d.MAC, d.Firmware, d.DeviceType, string(d.State), status, sev, d.Location, d.AssetTag, pages, d.LastSeen.Format(time.RFC3339), lp})
	}
	cw.Flush()
}

func (s *Server) exportSuppliesCSV(w http.ResponseWriter, r *http.Request) {
	devs, err := s.resolveDevices(r.Context(), deviceFilter(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="supplies.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"device", "model", "serial", "address", "supply", "type", "colorant", "percent", "level", "maxCapacity", "unit"})
	for _, d := range devs {
		for _, sp := range d.Supplies {
			pct := ""
			if sp.Percent != nil {
				pct = strconv.Itoa(*sp.Percent)
			}
			_ = cw.Write([]string{d.Name(), d.Model, d.Serial, d.Address, sp.Description, sp.Type, sp.Colorant, pct, strconv.Itoa(sp.Level), strconv.Itoa(sp.MaxCapacity), sp.Unit})
		}
	}
	cw.Flush()
}

// --- groups -----------------------------------------------------------------

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	gs, err := s.Store.ListGroups(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, gs)
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	var g model.Group
	if err := readJSON(r, &g); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(g.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if g.Kind == "rule" && g.Rule == nil {
		writeErr(w, http.StatusBadRequest, "rule groups need a rule")
		return
	}
	g.ID = ""
	if err := s.Store.CreateGroup(r.Context(), &g); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "group.create", g.ID, nil, map[string]any{"name": g.Name, "kind": g.Kind}, "ok")
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) getGroup(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.GetGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.GetGroup(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	before := map[string]any{"name": g.Name, "description": g.Description, "kind": g.Kind, "rule": g.Rule}
	var patch struct {
		Name        *string          `json:"name"`
		Description *string          `json:"description"`
		Kind        *string          `json:"kind"`
		Rule        *model.GroupRule `json:"rule"`
	}
	if err := readJSON(r, &patch); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if patch.Name != nil {
		g.Name = *patch.Name
	}
	if patch.Description != nil {
		g.Description = *patch.Description
	}
	if patch.Kind != nil {
		g.Kind = *patch.Kind
	}
	if patch.Rule != nil {
		g.Rule = patch.Rule
	}
	if err := s.Store.UpdateGroup(r.Context(), g); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "group.update", g.ID, before, map[string]any{"name": g.Name, "description": g.Description, "kind": g.Kind, "rule": g.Rule}, "ok")
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteGroup(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "group.delete", id, nil, nil, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setGroupMembers(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceIDs []string `json:"deviceIds"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	if err := s.Store.SetGroupMembers(r.Context(), id, body.DeviceIDs); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "group.members", id, nil, map[string]any{"count": len(body.DeviceIDs)}, "ok")
	g, _ := s.Store.GetGroup(r.Context(), id)
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) groupDevices(w http.ResponseWriter, r *http.Request) {
	devs, err := s.resolveDevices(r.Context(), store.DeviceFilter{GroupIDs: []string{r.PathValue("id")}})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, devs)
}

// --- discovery / jobs -------------------------------------------------------

func (s *Server) startDiscovery(w http.ResponseWriter, r *http.Request) {
	var p fleet.DiscoveryParams
	p.ZeroConf = true
	if r.ContentLength > 0 {
		if err := readJSON(r, &p); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if !p.ZeroConf && len(p.Targets) == 0 {
		writeErr(w, http.StatusBadRequest, "enable zeroconf or give targets")
		return
	}
	j, err := s.Fleet.SubmitDiscovery(r.Context(), p, actor(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "discovery.run", j.ID, nil, map[string]any{"targets": p.Targets, "zeroconf": p.ZeroConf, "allowLarge": p.AllowLarge}, "queued")
	writeJSON(w, http.StatusAccepted, j)
}

func (s *Server) listDiscoveryRuns(w http.ResponseWriter, r *http.Request) {
	js, err := s.Store.ListJobs(r.Context(), store.JobFilter{Type: "discovery", Limit: intQuery(r, "limit", 50)})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, js)
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	js, err := s.Store.ListJobs(r.Context(), store.JobFilter{Type: r.URL.Query().Get("type"), Status: r.URL.Query().Get("status"), Limit: intQuery(r, "limit", 100)})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, js)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	j, err := s.Store.GetJob(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	results, err := s.Store.ListJobResults(r.Context(), j.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"job": j, "results": results})
}

// --- alerts -----------------------------------------------------------------

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	as, err := s.Store.ListAlerts(r.Context(), store.AlertFilter{DeviceID: r.URL.Query().Get("device"), ActiveOnly: r.URL.Query().Get("all") != "true",
		Severity: r.URL.Query().Get("severity"), Limit: intQuery(r, "limit", 500)})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, as)
}

func (s *Server) ackAlert(w http.ResponseWriter, r *http.Request) {
	a, err := s.Store.AckAlert(r.Context(), r.PathValue("id"), actor(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "alert.ack", a.ID, nil, map[string]any{"device": a.DeviceID, "key": a.Key}, "ok")
	s.Hub.Publish(model.Event{Type: "alert.acked", Data: a})
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) clearAlert(w http.ResponseWriter, r *http.Request) {
	a, err := s.Store.ClearAlert(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "alert.clear", a.ID, nil, map[string]any{"device": a.DeviceID, "key": a.Key}, "ok")
	s.Hub.Publish(model.Event{Type: "alert.cleared", Data: a})
	writeJSON(w, http.StatusOK, a)
}

// --- credentials ------------------------------------------------------------

func (s *Server) listCredentials(w http.ResponseWriter, r *http.Request) {
	cs, err := s.Store.ListCredentials(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cs) // model.Credential carries no secret fields by construction
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string               `json:"name"`
		Type     model.CredentialType `json:"type"`
		Scope    string               `json:"scope"`
		Username string               `json:"username"`
		Priority int                  `json:"priority"`
		Secret   model.Secret         `json:"secret"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	switch body.Type {
	case model.CredSNMPCommunity:
		if body.Secret.Community == "" {
			writeErr(w, http.StatusBadRequest, "secret.community is required")
			return
		}
	case model.CredSNMPv3:
		if body.Username == "" {
			writeErr(w, http.StatusBadRequest, "username is required for snmpv3")
			return
		}
	case model.CredAdminPassword, model.CredCertificate:
	default:
		writeErr(w, http.StatusBadRequest, "unknown credential type")
		return
	}
	if body.Scope != "" && !strings.HasPrefix(body.Scope, "device:") && !strings.HasPrefix(body.Scope, "group:") {
		writeErr(w, http.StatusBadRequest, `scope must be "", "device:<id>" or "group:<id>"`)
		return
	}
	c := &model.Credential{Name: body.Name, Type: body.Type, Scope: body.Scope, Username: body.Username, Priority: body.Priority}
	if err := s.Store.CreateCredential(r.Context(), c, &body.Secret); err != nil {
		s.fail(w, err)
		return
	}
	// Audit records metadata only; never the secret.
	s.audit(r.Context(), r, "credential.create", c.ID, nil, map[string]any{"name": c.Name, "type": c.Type, "scope": c.Scope}, "ok")
	cs, _ := s.Store.ListCredentials(r.Context())
	for _, cc := range cs {
		if cc.ID == c.ID {
			writeJSON(w, http.StatusCreated, cc)
			return
		}
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) deleteCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteCredential(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "credential.delete", id, nil, nil, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- users / audit / settings ----------------------------------------------

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := s.Store.ListUsers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, us)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string     `json:"username"`
		Password string     `json:"password"`
		Role     model.Role `json:"role"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.Auth.CreateUser(r.Context(), body.Username, body.Password, body.Role)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r.Context(), r, "user.create", u.ID, nil, map[string]any{"username": u.Username, "role": u.Role}, "ok")
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if me := auth.UserFrom(r.Context()); me != nil && me.ID == id {
		writeErr(w, http.StatusBadRequest, "cannot delete yourself")
		return
	}
	if err := s.Store.DeleteUser(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "user.delete", id, nil, nil, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	es, err := s.Store.ListAudit(r.Context(), intQuery(r, "limit", 200))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, es)
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	ls, err := s.Store.ListNotificationLog(r.Context(), intQuery(r, "limit", 200))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ls)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	// Expose non-secret runtime settings only: effective values plus the file defaults.
	notifiers := []map[string]any{}
	for _, n := range s.Cfg.Notifiers {
		notifiers = append(notifiers, map[string]any{"name": n.Name, "type": n.Type, "events": n.Events})
	}
	eff := s.Fleet.Runtime(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"version":   s.Version,
		"poll":      map[string]any{"interval": eff.PollInterval, "concurrency": eff.PollConcurrency},
		"discovery": map[string]any{"enabled": eff.DiscoveryEnabled, "interval": eff.DiscoveryInterval, "targets": eff.DiscoveryTargets, "maxAddresses": s.Cfg.Discovery.MaxAddresses},
		"trap":      map[string]any{"enabled": s.Cfg.Trap.Enabled, "listen": s.Cfg.Trap.Listen},
		"alerts":    map[string]any{"supplyWarningPercent": eff.SupplyWarningPercent, "supplyCriticalPercent": eff.SupplyCriticalPercent, "offlineAfterFailures": eff.OfflineAfterFailures},
		"telemetry": map[string]any{"retention": eff.TelemetryRetention},
		"runtime":   eff,
		"defaults":  s.Fleet.Defaults(),
		"notifiers": notifiers,
	})
}

// patchSettings saves runtime overrides (admin). Scheduler and alert rules pick
// them up on the next tick; file/env values stay as the defaults.
func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	cur := s.Fleet.Runtime(r.Context())
	before := cur
	if err := readJSON(r, &cur); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Fleet.SaveRuntime(r.Context(), cur); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r.Context(), r, "settings.update", "runtime", map[string]any{"runtime": before}, map[string]any{"runtime": cur}, "ok")
	writeJSON(w, http.StatusOK, s.Fleet.Runtime(r.Context()))
}

func (s *Server) resetSettings(w http.ResponseWriter, r *http.Request) {
	before := s.Fleet.Runtime(r.Context())
	if err := s.Fleet.ResetRuntime(r.Context()); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "settings.reset", "runtime", map[string]any{"runtime": before}, nil, "ok")
	writeJSON(w, http.StatusOK, s.Fleet.Runtime(r.Context()))
}

func (s *Server) listDrivers(w http.ResponseWriter, r *http.Request) {
	type drv struct {
		Name         string             `json:"name"`
		Capabilities []model.Capability `json:"capabilities"`
	}
	var out []drv
	for _, d := range s.Fleet.Engine.Registry.Drivers() {
		out = append(out, drv{d.Name(), d.Capabilities()})
	}
	writeJSON(w, http.StatusOK, out)
}
