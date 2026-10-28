package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/printprovider"
)

// ppServer wires a print-provider service with a recording backend into the
// test server the same way main does (api.SetPrintProvider).
func ppServer(t *testing.T) (ts *httptest.Server, s *Server, dry *printprovider.DryBackend) {
	t.Helper()
	ts, s = newTestServer(t)
	dry = printprovider.NewDryBackend("test")
	svc := printprovider.NewWithBackend(s.Fleet.DepsOf(), dry)
	SetPrintProvider(svc)
	t.Cleanup(func() { SetPrintProvider(nil) })
	if _, err := s.Auth.CreateUser(context.Background(), "op", "operatorpass1", model.RoleOperator); err != nil {
		t.Fatal(err)
	}
	return ts, s, dry
}

func TestPrintProviderRolesAndFlow(t *testing.T) {
	ts, s, dry := ppServer(t)
	ctx := context.Background()
	dev, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.30.0.7", Model: "Everywhere 1"}, map[model.Capability]string{model.CapIdentify: "ipp"}, []string{"mdns"})
	if err != nil {
		t.Fatal(err)
	}
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")
	op := login(t, ts, "op", "operatorpass1")

	// Unauthenticated.
	if code, _ := do(t, ts, "", "GET", "/api/print-provider", ""); code != 401 {
		t.Errorf("anon get %d", code)
	}
	// Viewer: read-only, and the module reports itself off by default.
	code, body := do(t, ts, viewer, "GET", "/api/print-provider", "")
	if code != 200 {
		t.Fatalf("viewer get %d %s", code, body)
	}
	var st struct {
		Settings     printprovider.Settings     `json:"settings"`
		Availability printprovider.Availability `json:"availability"`
		Queues       []printprovider.Queue      `json:"queues"`
	}
	_ = json.Unmarshal([]byte(body), &st)
	if st.Settings.Enabled || st.Settings.QueuePrefix != "pm-" || st.Availability.Backend != "dry" || len(st.Queues) != 0 {
		t.Errorf("defaults %s", body)
	}
	if code, _ := do(t, ts, viewer, "PUT", "/api/print-provider", `{"enabled":true,"mode":"cups","queuePrefix":"pm-","driverless":true}`); code != 403 {
		t.Errorf("viewer put %d", code)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/print-provider/sync", `{"dryRun":true}`); code != 403 {
		t.Errorf("viewer sync %d", code)
	}
	if code, _ := do(t, ts, viewer, "DELETE", "/api/print-provider/queues/"+dev.ID, ""); code != 403 {
		t.Errorf("viewer delete %d", code)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/print-provider/plan", ""); code != 200 {
		t.Errorf("viewer plan %d", code)
	}
	// Operator may sync but not change settings.
	if code, _ := do(t, ts, op, "PUT", "/api/print-provider", `{"enabled":true,"mode":"cups","queuePrefix":"pm-","driverless":true}`); code != 403 {
		t.Errorf("operator put %d", code)
	}
	if code, body := do(t, ts, op, "POST", "/api/print-provider/sync", `{"dryRun":true}`); code != 409 {
		t.Errorf("sync while disabled %d %s", code, body)
	}
	if len(dry.Commands()) != 0 {
		t.Errorf("backend touched while disabled: %v", dry.Commands())
	}
	// Admin: validation then enable.
	if code, body := do(t, ts, admin, "PUT", "/api/print-provider", `{"enabled":true,"mode":"cups","queuePrefix":"has space","driverless":true}`); code != 400 {
		t.Errorf("bad prefix %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "PUT", "/api/print-provider", `{"enabled":true,"mode":"cups","queuePrefix":"pm-","driverless":true,"autoCreateForGroupId":"nope"}`); code != 400 {
		t.Errorf("bad group %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "PUT", "/api/print-provider", `{"enabled":true,"mode":"cups","queuePrefix":"pm-","driverless":true,"defaultOptions":"media=iso_a4_210x297mm"}`); code != 200 {
		t.Fatalf("enable %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/print-provider/plan", "")
	if code != 200 || !strings.Contains(body, `"queueName":"pm-everywhere-1"`) || !strings.Contains(body, `"action":"create"`) || !strings.Contains(body, `"inventoryRead":true`) {
		t.Errorf("plan %d %s", code, body)
	}
	// Operator syncs (dry run first, then for real) -> 202 + job.
	for _, dryRun := range []string{"true", "false"} {
		code, body = do(t, ts, op, "POST", "/api/print-provider/sync", `{"dryRun":`+dryRun+`}`)
		if code != 202 {
			t.Fatalf("sync %d %s", code, body)
		}
		var resp struct {
			Job model.Job `json:"job"`
		}
		_ = json.Unmarshal([]byte(body), &resp)
		if resp.Job.Type != printprovider.JobType || resp.Job.CreatedBy != "op" {
			t.Errorf("job %+v", resp.Job)
		}
		wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		j, err := s.Fleet.Jobs.Wait(wctx, resp.Job.ID)
		cancel()
		if err != nil || j.Status != model.JobSucceeded {
			t.Fatalf("job wait %v %+v", err, j)
		}
	}
	cmds := strings.Join(dry.Commands(), "\n")
	if strings.Count(cmds, "lpadmin -p pm-everywhere-1") != 1 {
		t.Errorf("expected exactly one real create, got:\n%s", cmds)
	}
	code, body = do(t, ts, viewer, "GET", "/api/print-provider", "")
	if code != 200 || !strings.Contains(body, `"name":"pm-everywhere-1"`) || !strings.Contains(body, `"managed":true`) {
		t.Errorf("inventory %d %s", code, body)
	}
	// Remove: unknown device 404, then the real one.
	if code, _ := do(t, ts, op, "DELETE", "/api/print-provider/queues/nope", ""); code != 404 {
		t.Errorf("delete unknown %d", code)
	}
	if code, body := do(t, ts, op, "DELETE", "/api/print-provider/queues/"+dev.ID, ""); code != 200 || !strings.Contains(body, `"removed":"pm-everywhere-1"`) {
		t.Errorf("delete %d %s", code, body)
	}
	if code, _ := do(t, ts, op, "DELETE", "/api/print-provider/queues/"+dev.ID, ""); code != 404 {
		t.Errorf("delete again %d", code)
	}
	// Every write left an audit trail.
	audit, _ := s.Store.ListAudit(ctx, 100)
	seen := map[string]int{}
	for _, a := range audit {
		seen[a.Action]++
	}
	if seen["printprovider.settings.update"] != 1 || seen["printprovider.sync"] != 3 || seen["printprovider.queue.create"] != 1 || seen["printprovider.queue.remove"] != 1 {
		t.Errorf("audit %v", seen)
	}
}

func TestPrintProviderUnwired(t *testing.T) {
	ts, _ := newTestServer(t)
	SetPrintProvider(nil)
	admin := login(t, ts, "admin", "adminpass123")
	if code, _ := do(t, ts, admin, "GET", "/api/print-provider", ""); code != 503 {
		t.Errorf("unwired %d", code)
	}
}
