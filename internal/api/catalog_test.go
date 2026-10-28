package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"printmarshal/internal/model"
)

func TestCatalogEndpoints(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")
	ctx := context.Background()
	hp, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.10", Serial: "HP1", Make: "HP", Model: "HP Color LaserJet Managed MFP E87770z", EnterpriseID: 11, Method: "snmp", Capabilities: []model.Capability{model.CapIdentify, model.CapReadStatus}}, map[model.Capability]string{model.CapIdentify: "snmp", model.CapReadStatus: "snmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	acme, _, err := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.11", Serial: "AC1", Make: "Acme", Model: "Acme LaserPrint 4200dn", Method: "snmp"}, map[model.Capability]string{model.CapReadStatus: "snmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if code, _ := do(t, ts, "", "GET", "/api/catalog/summary", ""); code != 401 {
		t.Errorf("unauthenticated summary = %d", code)
	}
	code, body := do(t, ts, viewer, "GET", "/api/catalog/summary", "")
	var sum struct {
		Catalogue struct{ Families, BaseIdentifiers, Brands int }
		Fleet     struct {
			Devices, Matched, Unmatched int
			ByConfidence                map[string]int
		}
	}
	if code != 200 {
		t.Fatalf("summary: %d %s", code, body)
	}
	_ = json.Unmarshal([]byte(body), &sum)
	if sum.Catalogue.Families != 126 || sum.Catalogue.BaseIdentifiers != 495 || sum.Fleet.Devices != 2 || sum.Fleet.Matched != 1 || sum.Fleet.Unmatched != 1 || sum.Fleet.ByConfidence["none"] != 1 {
		t.Errorf("summary = %+v", sum)
	}

	code, body = do(t, ts, viewer, "GET", "/api/catalog/families?brand=HP&q=E877", "")
	var fams struct {
		Families []struct {
			ID, Qualification string
			FleetCount        int
			Evidence          map[string]struct{ Documented, Observed, Status string }
		}
		Total int
	}
	_ = json.Unmarshal([]byte(body), &fams)
	if code != 200 || fams.Total != 1 || fams.Families[0].ID != "HP01" || fams.Families[0].FleetCount != 1 || fams.Families[0].Qualification != "research_candidate" {
		t.Fatalf("families: %d %s", code, body)
	}
	if ev := fams.Families[0].Evidence; ev["secure"].Documented != "documented" || ev["secure"].Observed != "unknown" || ev["monitor"].Status != "documented" {
		t.Errorf("family evidence must be documented-only: %+v", ev)
	}
	if code, body := do(t, ts, viewer, "GET", "/api/catalog/families?segment=A3+mono+laser+MFP", ""); code != 200 || !strings.Contains(body, `"RI07"`) {
		t.Errorf("segment filter: %d", code)
	}

	code, body = do(t, ts, viewer, "GET", "/api/catalog/families/HP01", "")
	if code != 200 || !strings.Contains(body, `"HP_E877"`) || !strings.Contains(body, `"NHP14"`) || !strings.Contains(body, hp.ID) {
		t.Fatalf("family detail: %d %s", code, body)
	}
	// HP02 carries the documented X3A87A identity conflict and its variant examples.
	if code, body := do(t, ts, viewer, "GET", "/api/catalog/families/HP02", ""); code != 200 || !strings.Contains(body, `"X3A87A"`) || !strings.Contains(body, `"product_number"`) {
		t.Errorf("HP02 detail should include the identity conflict and variants: %d", code)
	}
	if !strings.Contains(body, `"label":"HP FutureSmart enterprise/managed candidate"`) {
		t.Error("family detail should carry the management profile")
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/catalog/families/NOPE", ""); code != 404 {
		t.Errorf("unknown family = %d", code)
	}
	if code, body := do(t, ts, viewer, "GET", "/api/catalog/profiles", ""); code != 200 || !strings.Contains(body, `"RICOH"`) {
		t.Errorf("profiles: %d", code)
	}

	// Match endpoint: the HP M283fdw must not be forced into an Enterprise/Managed family.
	code, body = do(t, ts, viewer, "GET", "/api/catalog/match?make=HP&model=HP+Color+LaserJet+MFP+M283fdw", "")
	var m struct{ Confidence, FamilyID, Brand string }
	_ = json.Unmarshal([]byte(body), &m)
	if code != 200 || m.Confidence != "brand-only" || m.FamilyID != "" || m.Brand != "HP" {
		t.Errorf("match M283fdw: %d %+v", code, m)
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/catalog/match", ""); code != 400 {
		t.Errorf("match without inputs = %d", code)
	}

	// Device support: documented/observed/unknown are separate.
	code, body = do(t, ts, viewer, "GET", "/api/devices/"+hp.ID+"/support", "")
	var sup struct {
		Match   struct{ Confidence, FamilyID string }
		Support struct {
			Tier, Qualification string
			Operations          []struct{ Operation, Documented, Observed, Status string }
		}
	}
	_ = json.Unmarshal([]byte(body), &sup)
	if code != 200 || sup.Match.FamilyID != "HP01" || sup.Match.Confidence != "exact" || sup.Support.Tier != "documented+observed" || len(sup.Support.Operations) != 5 {
		t.Fatalf("support: %d %s", code, body)
	}
	for _, o := range sup.Support.Operations {
		if o.Operation == "secure" && (o.Documented != "documented" || o.Observed != "unknown" || o.Status != "documented") {
			t.Errorf("secure must be documented but not observed: %+v", o)
		}
		if o.Operation == "update" && o.Observed != "unknown" {
			t.Errorf("update must not be observed: %+v", o)
		}
	}
	if strings.Contains(body, `"supported"`) {
		t.Error("the API must never say \"supported\"")
	}
	code, body = do(t, ts, viewer, "GET", "/api/devices/"+acme.ID+"/support", "")
	_ = json.Unmarshal([]byte(body), &sup)
	if code != 200 || sup.Match.Confidence != "none" || sup.Support.Tier != "observed only (standards)" {
		t.Errorf("unknown device support: %d %s", code, body)
	}

	// Qualification override: operator only, validated, audited, reflected everywhere.
	if code, _ := do(t, ts, viewer, "PUT", "/api/catalog/families/HP01/qualification", `{"status":"lab_validated"}`); code != 403 {
		t.Errorf("viewer override = %d", code)
	}
	if code, _ := do(t, ts, admin, "PUT", "/api/catalog/families/HP01/qualification", `{"status":"supported"}`); code != 400 {
		t.Errorf("invalid status = %d", code)
	}
	if code, _ := do(t, ts, admin, "PUT", "/api/catalog/families/NOPE/qualification", `{"status":"lab_validated"}`); code != 404 {
		t.Errorf("unknown family override = %d", code)
	}
	code, body = do(t, ts, admin, "PUT", "/api/catalog/families/HP01/qualification", `{"status":"lab_validated","note":"bench-tested E87770z 2026-10"}`)
	if code != 200 || !strings.Contains(body, `"actor":"admin"`) || !strings.Contains(body, `"lab_validated"`) {
		t.Fatalf("override: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "GET", "/api/audit", "")
	if code != 200 || !strings.Contains(body, "catalog.qualification") || !strings.Contains(body, "bench-tested") {
		t.Errorf("audit: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/devices/"+hp.ID+"/support", "")
	_ = json.Unmarshal([]byte(body), &sup)
	if sup.Support.Tier != "validated" || sup.Support.Qualification != "lab_validated" {
		t.Errorf("override not reflected in support: %+v", sup.Support)
	}
	code, body = do(t, ts, viewer, "GET", "/api/catalog/families/HP01", "")
	if code != 200 || !strings.Contains(body, `"qualification":"lab_validated"`) {
		t.Errorf("override not reflected in family detail: %d", code)
	}
}
