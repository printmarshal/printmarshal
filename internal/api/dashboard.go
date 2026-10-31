package api

import (
	"context"
	"errors"
	"net/http"

	"printmarshal/internal/auth"
	"printmarshal/internal/model"
	"printmarshal/internal/security"
	"printmarshal/internal/store"
)

// Dashboard layouts (intent/dashboard-layout): which widgets a person sees,
// in what order, hidden or collapsed. A layout is presentation only; it never
// changes what the dashboard payload contains for a principal.

const (
	layoutVersion    = 1
	layoutDefaultKey = "dashboard.layout.default"
	layoutMaxBody    = 16 << 10
)

func layoutUserKey(userID string) string { return "dashboard.layout.user." + userID }

// widgetIDs is the server's copy of the SPA's widget catalogue: it validates
// ids, the SPA owns titles, sizes and rendering. Keep the two in step.
var widgetIDs = []string{
	"kpi.devices", "kpi.online", "kpi.alerts", "kpi.supplies", "kpi.stock", "kpi.jobs",
	"security.posture", "fleet.health", "alerts.recent", "devices.attention",
	"supplies.low", "jobs.inprogress", "activity.recent",
	"consumables.lowstock", "consumables.reorder",
}

// LayoutEntry is one widget's place in a layout.
type LayoutEntry struct {
	ID        string `json:"id"`
	Hidden    bool   `json:"hidden,omitempty"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

// DashboardLayout is an ordered list of entries. Widgets missing from the
// list are shown after it with their defaults; unknown ids are dropped.
type DashboardLayout struct {
	Version int           `json:"version"`
	Widgets []LayoutEntry `json:"widgets"`
}

// normalizeLayout drops unknown and repeated ids, keeps order, and sets the
// version, so a stored layout survives widgets being added or removed.
func normalizeLayout(l DashboardLayout) DashboardLayout {
	known := make(map[string]bool, len(widgetIDs))
	for _, id := range widgetIDs {
		known[id] = true
	}
	seen := map[string]bool{}
	out := DashboardLayout{Version: layoutVersion, Widgets: []LayoutEntry{}}
	for _, e := range l.Widgets {
		if !known[e.ID] || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out.Widgets = append(out.Widgets, e)
	}
	return out
}

// layouts returns the caller's personal layout and the instance default
// (nil when unset) and the effective one (personal, else default, else nil:
// the SPA then uses its built-in order).
func (s *Server) layouts(ctx context.Context, u *model.User) (effective, personal, def *DashboardLayout) {
	var d DashboardLayout
	if err := s.Store.GetSetting(ctx, layoutDefaultKey, &d); err == nil {
		d = normalizeLayout(d)
		def = &d
	}
	if u != nil {
		var p DashboardLayout
		if err := s.Store.GetSetting(ctx, layoutUserKey(u.ID), &p); err == nil {
			p = normalizeLayout(p)
			personal = &p
		}
	}
	switch {
	case personal != nil:
		effective = personal
	case def != nil:
		effective = def
	}
	return effective, personal, def
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ds, err := s.Store.Dashboard(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	alerts, _ := s.Store.ListAlerts(ctx, store.AlertFilter{ActiveOnly: true, Limit: 20})
	alerts = s.filterAlerts(ctx, alerts)
	jobs, _ := s.Store.ListJobs(ctx, store.JobFilter{Limit: 10})
	attention, _ := s.resolveDevices(ctx, store.DeviceFilter{Sort: "severity", Limit: 10})
	var needs []*model.Device
	for _, d := range attention {
		if d.Status != nil && (d.Status.Severity != model.SeverityOK || !d.Status.Online) {
			needs = append(needs, d)
		}
	}
	if needs == nil {
		needs = []*model.Device{}
	}
	u := auth.UserFrom(ctx)
	effective, personal, def := s.layouts(ctx, u)
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": ds, "alerts": alerts, "jobs": jobs, "attention": needs,
		"consumables": s.dashboardConsumables(ctx),
		"security":    s.dashboardSecurity(ctx, ds.Devices),
		"layout":      map[string]any{"effective": effective, "personal": personal, "default": def},
	})
}

// dashboardSecurity is the compliance roll-up over the devices the caller
// may see, with the unknown-item count kept apart from the score.
func (s *Server) dashboardSecurity(ctx context.Context, fleetSize int) security.ComplianceSummary {
	var ids []string
	inView := fleetSize
	if s.scopeDeviceIDs(ctx) != nil {
		visible, err := s.resolveDevices(ctx, store.DeviceFilter{})
		if err != nil {
			return security.Summarize(nil)
		}
		inView = len(visible)
		if len(visible) == 0 {
			return security.Summarize(nil)
		}
		ids = make([]string, 0, len(visible))
		for _, d := range visible {
			ids = append(ids, d.ID)
		}
	}
	as, err := s.Store.LatestAssessments(ctx, "", ids)
	if err != nil {
		return security.Summarize(nil)
	}
	return security.SummarizeFor(as, inView)
}

func (s *Server) readLayout(w http.ResponseWriter, r *http.Request) (DashboardLayout, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, layoutMaxBody)
	var l DashboardLayout
	if err := readJSON(r, &l); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return l, false
	}
	if l.Widgets == nil {
		writeErr(w, http.StatusBadRequest, "layout needs a widgets list")
		return l, false
	}
	return normalizeLayout(l), true
}

func (s *Server) putLayout(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "sign in required")
		return
	}
	l, ok := s.readLayout(w, r)
	if !ok {
		return
	}
	if err := s.Store.SetSetting(r.Context(), layoutUserKey(u.ID), l); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) deleteLayout(w http.ResponseWriter, r *http.Request) {
	u := auth.UserFrom(r.Context())
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "sign in required")
		return
	}
	if err := s.Store.DeleteSetting(r.Context(), layoutUserKey(u.ID)); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) putDefaultLayout(w http.ResponseWriter, r *http.Request) {
	l, ok := s.readLayout(w, r)
	if !ok {
		return
	}
	if err := s.Store.SetSetting(r.Context(), layoutDefaultKey, l); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "dashboard.layout.default", "dashboard", nil, map[string]any{"widgets": len(l.Widgets)}, "ok")
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) deleteDefaultLayout(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DeleteSetting(r.Context(), layoutDefaultKey); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "dashboard.layout.default", "dashboard", nil, map[string]any{"cleared": true}, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
