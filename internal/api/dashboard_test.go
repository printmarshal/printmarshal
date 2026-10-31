package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
)

type layoutView struct {
	Layout struct {
		Effective *DashboardLayout `json:"effective"`
		Personal  *DashboardLayout `json:"personal"`
		Default   *DashboardLayout `json:"default"`
	} `json:"layout"`
	Security struct {
		Devices     int `json:"devices"`
		Compliant   int `json:"compliant"`
		Unknown     int `json:"unknown"`
		NotAssessed int `json:"notAssessed"`
	} `json:"security"`
}

func dashboardView(t *testing.T, body string) layoutView {
	t.Helper()
	var v layoutView
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("dashboard payload: %v\n%s", err, body)
	}
	return v
}

// Personal layout wins over the instance default, which wins over nothing;
// unknown widget ids are dropped; only admins set the default; tokens are
// refused; deleting a user deletes their layout.
func TestDashboardLayouts(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")

	if v := dashboardView(t, second(do(t, ts, viewer, "GET", "/api/dashboard", ""))); v.Layout.Effective != nil || v.Layout.Personal != nil || v.Layout.Default != nil {
		t.Fatalf("fresh instance should have no layouts: %+v", v.Layout)
	}
	if code, _ := do(t, ts, viewer, "PUT", "/api/dashboard/layout/default", `{"widgets":[{"id":"fleet.health"}]}`); code != 403 {
		t.Fatalf("viewer set the default: %d", code)
	}
	code, body := do(t, ts, admin, "PUT", "/api/dashboard/layout/default", `{"widgets":[{"id":"security.posture"},{"id":"no.such.widget"},{"id":"kpi.devices","hidden":true},{"id":"security.posture"}]}`)
	if code != 200 || strings.Contains(body, "no.such.widget") || strings.Count(body, "security.posture") != 1 {
		t.Fatalf("default layout: %d %s", code, body)
	}
	v := dashboardView(t, second(do(t, ts, viewer, "GET", "/api/dashboard", "")))
	if v.Layout.Personal != nil || v.Layout.Default == nil || v.Layout.Effective == nil || len(v.Layout.Effective.Widgets) != 2 || v.Layout.Effective.Widgets[0].ID != "security.posture" || !v.Layout.Effective.Widgets[1].Hidden {
		t.Fatalf("default should be effective: %+v", v.Layout)
	}
	if code, _ := do(t, ts, viewer, "PUT", "/api/dashboard/layout", `{"widgets":[{"id":"alerts.recent","collapsed":true}]}`); code != 200 {
		t.Fatalf("viewer personal layout: %d", code)
	}
	v = dashboardView(t, second(do(t, ts, viewer, "GET", "/api/dashboard", "")))
	if v.Layout.Personal == nil || v.Layout.Effective.Widgets[0].ID != "alerts.recent" || !v.Layout.Effective.Widgets[0].Collapsed || v.Layout.Default == nil {
		t.Fatalf("personal should be effective: %+v", v.Layout)
	}
	if v := dashboardView(t, second(do(t, ts, admin, "GET", "/api/dashboard", ""))); v.Layout.Personal != nil || v.Layout.Effective.Widgets[0].ID != "security.posture" {
		t.Fatalf("another user must not see the viewer's personal layout: %+v", v.Layout)
	}
	if code, _ := do(t, ts, viewer, "DELETE", "/api/dashboard/layout", ""); code != 200 {
		t.Fatalf("reset personal: %d", code)
	}
	if v := dashboardView(t, second(do(t, ts, viewer, "GET", "/api/dashboard", ""))); v.Layout.Personal != nil || v.Layout.Effective.Widgets[0].ID != "security.posture" {
		t.Fatalf("after reset the default should be effective: %+v", v.Layout)
	}
	if code, body := do(t, ts, viewer, "PUT", "/api/dashboard/layout", `{"version":1}`); code != 400 {
		t.Fatalf("layout without widgets accepted: %d %s", code, body)
	}
	if code, _ := do(t, ts, admin, "DELETE", "/api/dashboard/layout/default", ""); code != 200 {
		t.Fatalf("clear default: %d", code)
	}
	if v := dashboardView(t, second(do(t, ts, viewer, "GET", "/api/dashboard", ""))); v.Layout.Effective != nil {
		t.Fatalf("no layouts left: %+v", v.Layout)
	}

	// API tokens cannot touch layouts, even admin ones.
	tok := mkToken(t, ts, admin, `{"name":"bot","role":"admin"}`).Secret
	for _, c := range []struct{ m, p string }{{"PUT", "/api/dashboard/layout"}, {"DELETE", "/api/dashboard/layout"}, {"PUT", "/api/dashboard/layout/default"}} {
		if code, _ := do(t, ts, tok, c.m, c.p, `{"widgets":[]}`); code != 403 {
			t.Errorf("token %s %s = %d, want 403", c.m, c.p, code)
		}
	}

	// A deleted user's layout goes with them.
	u, err := s.Auth.CreateUser(context.Background(), "temp", "temppass123", model.RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	temp := login(t, ts, "temp", "temppass123")
	if code, _ := do(t, ts, temp, "PUT", "/api/dashboard/layout", `{"widgets":[{"id":"kpi.jobs","hidden":true}]}`); code != 200 {
		t.Fatalf("temp layout: %d", code)
	}
	if code, _ := do(t, ts, admin, "DELETE", "/api/users/"+u.ID, ""); code != 200 {
		t.Fatalf("delete user: %d", code)
	}
	var left DashboardLayout
	if err := s.Store.GetSetting(context.Background(), layoutUserKey(u.ID), &left); err == nil {
		t.Fatalf("layout survived user deletion: %+v", left)
	}
}

// The posture summary on the dashboard counts unknown items separately and
// covers only the devices a scoped principal may see.
func TestDashboardSecuritySummaryIsScoped(t *testing.T) {
	ts, s := newTestServer(t)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	g, in, out := scopedFixture(t, s)
	for _, a := range []*model.Assessment{
		{DeviceID: in.ID, PolicyID: "baseline", Passed: 3, Failed: 1, Unknown: 2, Score: 75, Compliant: false, At: time.Now().UTC(),
			Items: []model.ItemResult{{ItemID: "telnet", Status: model.ItemFail}}},
		{DeviceID: out.ID, PolicyID: "baseline", Passed: 5, Failed: 0, Unknown: 4, Score: 100, Compliant: true, At: time.Now().UTC()},
	} {
		if err := s.Store.SaveAssessment(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	v := dashboardView(t, second(do(t, ts, admin, "GET", "/api/dashboard", "")))
	if v.Security.Devices != 2 || v.Security.Unknown != 6 || v.Security.NotAssessed != 0 || v.Security.Compliant != 1 {
		t.Fatalf("admin posture: %+v", v.Security)
	}
	tok := mkToken(t, ts, admin, `{"name":"bot","role":"viewer","scope":["`+g.ID+`"]}`).Secret
	v = dashboardView(t, second(do(t, ts, tok, "GET", "/api/dashboard", "")))
	if v.Security.Devices != 1 || v.Security.Unknown != 2 || v.Security.Compliant != 0 {
		t.Fatalf("scoped posture leaked: %+v", v.Security)
	}
	// The compliance route agrees and refuses a foreign device parameter.
	_, body := do(t, ts, tok, "GET", "/api/compliance", "")
	if strings.Contains(body, out.ID) || !strings.Contains(body, `"unknown":2`) {
		t.Fatalf("compliance route not scoped: %s", body)
	}
	if code, _ := do(t, ts, tok, "GET", "/api/compliance?device="+out.ID, ""); code != 403 {
		t.Fatalf("compliance for a foreign device = %d, want 403", code)
	}
}

func second(_ int, body string) string { return body }

// Every alert on the dashboard carries the device's display name, resolved by
// id on the server, so the widget never labels a row "device" (#53).
func TestDashboardAlertsCarryDeviceName(t *testing.T) {
	ts, s := newTestServer(t)
	ctx := context.Background()
	viewer := login(t, ts, "viewer", "viewerpass1")
	d, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.31", Serial: "DN1", Make: "Acme", Model: "Laser 1", SysName: "Reception", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Store.RaiseAlert(ctx, d.ID, "jam", "poll", model.SeverityCritical, "paper jam"); err != nil {
		t.Fatal(err)
	}
	code, body := do(t, ts, viewer, "GET", "/api/dashboard", "")
	if code != 200 {
		t.Fatalf("dashboard: %d %s", code, body)
	}
	var v struct {
		Alerts []struct {
			DeviceID   string `json:"deviceId"`
			DeviceName string `json:"deviceName"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Alerts) != 1 || v.Alerts[0].DeviceID != d.ID || v.Alerts[0].DeviceName != "Reception" {
		t.Errorf("dashboard alerts = %+v, want deviceName %q", v.Alerts, "Reception")
	}
}
