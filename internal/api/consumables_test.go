package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

func pctp(n int) *int { return &n }

// Consumables: role-gated and audited writes; stock values are operator-owned.
func TestConsumablesAPI(t *testing.T) {
	ts, s := newTestServer(t)
	ctx := context.Background()
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")

	if code, _ := do(t, ts, viewer, "POST", "/api/consumables", `{"name":"x"}`); code != 403 {
		t.Errorf("viewer create = %d", code)
	}
	if code, _ := do(t, ts, admin, "POST", "/api/consumables", `{"name":"","type":"toner"}`); code != 400 {
		t.Errorf("empty name = %d", code)
	}
	if code, _ := do(t, ts, admin, "POST", "/api/consumables", `{"name":"x","type":"gizmo"}`); code != 400 {
		t.Errorf("bad type = %d", code)
	}
	code, body := do(t, ts, admin, "POST", "/api/consumables", `{"name":"Black toner","type":"toner","color":"K","partNumber":"TN-4200","compatibleModels":["Acme 4200"],"onHand":3,"location":"Store A"}`)
	if code != 201 || !strings.Contains(body, `"color":"black"`) || !strings.Contains(body, `"warningAt":2`) {
		t.Fatalf("create: %d %s", code, body)
	}
	var c model.Consumable
	_ = json.Unmarshal([]byte(body), &c)

	// A device with a black toner slot links to it; the device view shows the stock.
	d, _, _ := s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.5", Serial: "S1", Make: "Acme", Model: "Acme 4200", Method: "snmp"}, nil, nil)
	_, _ = s.Store.ApplyPoll(ctx, d.ID, store.PollUpdate{Reachable: true, At: time.Now(), Supplies: []model.Supply{
		{Index: 1, Description: "Black Toner Cartridge", Type: "toner", Class: "consumed", Colorant: "black", Percent: pctp(12), Level: 12, MaxCapacity: 100},
		{Index: 2, Description: "Imaging Drum", Type: "opc", Class: "consumed", Percent: pctp(71), Level: 71, MaxCapacity: 100},
	}})
	if err := s.Fleet.Consumables.ResolveAll(ctx); err != nil {
		t.Fatal(err)
	}
	code, body = do(t, ts, viewer, "GET", "/api/devices/"+d.ID+"/supplies", "")
	if code != 200 || !strings.Contains(body, `"linkSource":"auto"`) || !strings.Contains(body, `"onHand":3`) || !strings.Contains(body, `"suggestion"`) {
		t.Fatalf("device supplies: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/consumables/installed?unmatched=true", "")
	if code != 200 || !strings.Contains(body, "Imaging Drum") || strings.Contains(body, "Black Toner Cartridge") {
		t.Errorf("unmatched: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/consumables", "")
	if code != 200 || !strings.Contains(body, `"status":"ok"`) || !strings.Contains(body, `"linkedDevices":1`) || !strings.Contains(body, `"known":false`) {
		t.Errorf("list: %d %s", code, body)
	}

	// Stock adjustments are audited and raise/clear stock alerts.
	if code, _ := do(t, ts, viewer, "POST", "/api/consumables/"+c.ID+"/adjust-stock", `{"delta":-1}`); code != 403 {
		t.Errorf("viewer adjust = %d", code)
	}
	if code, _ := do(t, ts, admin, "POST", "/api/consumables/"+c.ID+"/adjust-stock", `{"set":1,"delta":1}`); code != 400 {
		t.Error("set and delta together must be rejected")
	}
	code, body = do(t, ts, admin, "POST", "/api/consumables/"+c.ID+"/adjust-stock", `{"delta":-2,"reason":"installed"}`)
	if code != 200 || !strings.Contains(body, `"onHand":1`) || !strings.Contains(body, `"status":"critical"`) {
		t.Fatalf("adjust: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "GET", "/api/alerts", "")
	if code != 200 || !strings.Contains(body, `"source":"consumable"`) || !strings.Contains(body, "consumable:"+c.ID) {
		t.Errorf("stock alert expected: %s", body)
	}
	code, body = do(t, ts, admin, "GET", "/api/audit", "")
	if !strings.Contains(body, "consumable.adjust") || !strings.Contains(body, "consumable.create") {
		t.Errorf("audit: %s", body)
	}
	// Batch save (inline edit) sets quantities and clears the alert.
	code, body = do(t, ts, admin, "POST", "/api/consumables/bulk-stock", `{"items":[{"id":"`+c.ID+`","onHand":6},{"id":"nope","onHand":1}],"reason":"stocktake"}`)
	if code != 200 || !strings.Contains(body, `"onHand":6`) || !strings.Contains(body, `"nope":"not found"`) {
		t.Fatalf("bulk: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "GET", "/api/alerts", "")
	if strings.Contains(body, "consumable:"+c.ID) {
		t.Errorf("stock alert should clear once replenished: %s", body)
	}
	// PATCH never touches on-hand.
	code, body = do(t, ts, admin, "PATCH", "/api/consumables/"+c.ID, `{"location":"Store B","warningAt":4}`)
	if code != 200 || !strings.Contains(body, `"onHand":6`) || !strings.Contains(body, "Store B") {
		t.Errorf("patch: %d %s", code, body)
	}
	if code, _ := do(t, ts, admin, "PATCH", "/api/consumables/"+c.ID, `{"onHand":0}`); code != 400 {
		t.Error("onHand is not patchable; use adjust-stock")
	}

	// Mark replaced decrements the linked item and logs the event.
	code, body = do(t, ts, admin, "POST", "/api/devices/"+d.ID+"/supplies/1/mark-replaced", `{"decrement":true}`)
	if code != 200 || !strings.Contains(body, `"onHand":5`) || !strings.Contains(body, `"kind":"replacement"`) {
		t.Fatalf("mark replaced: %d %s", code, body)
	}
	if code, _ := do(t, ts, admin, "POST", "/api/devices/"+d.ID+"/supplies/2/mark-replaced", `{"decrement":true}`); code != 400 {
		t.Error("unlinked supply cannot decrement")
	}
	// Manual link of the drum, then it is no longer unmatched; unlink restores.
	code, body = do(t, ts, admin, "POST", "/api/consumables", `{"name":"Drum","type":"drum","onHand":1}`)
	var drum model.Consumable
	_ = json.Unmarshal([]byte(body), &drum)
	code, _ = do(t, ts, admin, "PUT", "/api/devices/"+d.ID+"/supplies/2/link", `{"consumableId":"`+drum.ID+`"}`)
	if code != 200 {
		t.Fatalf("link = %d", code)
	}
	code, body = do(t, ts, viewer, "GET", "/api/consumables/"+drum.ID+"/devices", "")
	if code != 200 || !strings.Contains(body, `"linkSource":"manual"`) {
		t.Errorf("linked devices: %d %s", code, body)
	}
	if code, _ := do(t, ts, admin, "DELETE", "/api/devices/"+d.ID+"/supplies/2/link", ""); code != 200 {
		t.Error("unlink")
	}

	// Detail view and history.
	code, body = do(t, ts, viewer, "GET", "/api/consumables/"+c.ID, "")
	if code != 200 || !strings.Contains(body, `"events":[`) || !strings.Contains(body, `"forecast":{`) || !strings.Contains(body, `"devices":[`) {
		t.Errorf("detail: %d %s", code, body)
	}
	code, body = do(t, ts, viewer, "GET", "/api/consumables/forecast", "")
	if code != 200 || !strings.Contains(body, `"linkedDevices":1`) {
		t.Errorf("forecast: %d %s", code, body)
	}

	// Pending replacement confirm/dismiss.
	ev := &model.StockEvent{Actor: "system", Kind: model.StockEventReplacement, Delta: -1, DeviceID: d.ID, SupplyIndex: 1, Status: model.StockEventPending}
	if _, err := s.Store.AdjustStock(ctx, c.ID, ev); err != nil {
		t.Fatal(err)
	}
	code, body = do(t, ts, viewer, "GET", "/api/consumables/replacements", "")
	if code != 200 || !strings.Contains(body, `"status":"pending"`) {
		t.Errorf("pending list: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "POST", fmt.Sprintf("/api/consumables/replacements/%d/confirm", ev.ID), "")
	if code != 200 || !strings.Contains(body, `"onHand":4`) {
		t.Errorf("confirm: %d %s", code, body)
	}
	if code, _ := do(t, ts, admin, "POST", fmt.Sprintf("/api/consumables/replacements/%d/dismiss", ev.ID), ""); code != 409 {
		t.Error("confirmed replacement cannot be dismissed")
	}

	// Settings: viewer can read, only admin can write.
	code, body = do(t, ts, viewer, "GET", "/api/consumables/settings", "")
	if code != 200 || !strings.Contains(body, `"autoDecrement":"prompt"`) {
		t.Errorf("settings: %d %s", code, body)
	}
	if code, _ := do(t, ts, viewer, "PUT", "/api/consumables/settings", `{"autoDecrement":"automatic"}`); code != 403 {
		t.Error("viewer settings write")
	}
	if code, _ := do(t, ts, admin, "PUT", "/api/consumables/settings", `{"autoDecrement":"sometimes"}`); code != 400 {
		t.Error("bad mode")
	}
	code, body = do(t, ts, admin, "PUT", "/api/consumables/settings", `{"autoDecrement":"automatic","defaultWarningAt":3,"defaultCriticalAt":1,"replacementJumpPct":40,"forecastMinDays":2,"reorderLeadDays":21,"autoSuggestFromScans":true}`)
	if code != 200 || !strings.Contains(body, `"reorderLeadDays":21`) {
		t.Errorf("save settings: %d %s", code, body)
	}

	// CSV round trip: export, then import with a new row and an updated quantity.
	code, body = do(t, ts, viewer, "GET", "/api/consumables.csv", "")
	if code != 200 || !strings.Contains(body, "TN-4200") || !strings.Contains(body, "Acme 4200") {
		t.Fatalf("csv export: %d %s", code, body)
	}
	csvIn := "name,type,color,partNumber,compatibleModels,onHand,warningAt,criticalAt,location\n" +
		"Black toner,toner,black,TN-4200,Acme 4200;Acme 4300,9,2,1,Store C\n" +
		"Cyan toner,toner,cyan,TN-4201,Acme 4300,2,2,1,Store C\n" +
		",toner,,,,1,,,\n"
	code, body = do(t, ts, admin, "POST", "/api/consumables/import", csvIn)
	if code != 200 || !strings.Contains(body, `"created":1`) || !strings.Contains(body, `"updated":1`) {
		t.Fatalf("csv import: %d %s", code, body)
	}
	got, _ := s.Store.GetConsumable(ctx, c.ID)
	if got.OnHand != 9 || got.Location != "Store C" || len(got.CompatibleModels) != 2 {
		t.Errorf("imported update = %+v", got)
	}
	events, _ := s.Store.ListStockEvents(ctx, store.StockEventFilter{ConsumableID: c.ID, Limit: 1})
	if len(events) != 1 || events[0].Reason != "CSV import" || events[0].Resulting != 9 {
		t.Errorf("import must log a stock event: %+v", events)
	}
	if code, _ := do(t, ts, viewer, "POST", "/api/consumables/import", csvIn); code != 403 {
		t.Error("viewer import")
	}

	// Dashboard carries the consumables panels; delete is audited.
	code, body = do(t, ts, viewer, "GET", "/api/dashboard", "")
	if code != 200 || !strings.Contains(body, `"lowStock"`) || !strings.Contains(body, `"pendingReplacements"`) {
		t.Errorf("dashboard: %d %s", code, body)
	}
	if code, _ := do(t, ts, admin, "DELETE", "/api/consumables/"+drum.ID, ""); code != 200 {
		t.Error("delete")
	}
	if code, _ := do(t, ts, viewer, "GET", "/api/consumables/"+drum.ID, ""); code != 404 {
		t.Error("deleted item should be gone")
	}
}
