package api

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"printmarshal/internal/consumables"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Consumables API (consumables spec §5). Every stock write is audited and
// role-gated; stock items are operator-owned and never written by polls.

func (s *Server) consumableRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/consumables", s.require(model.RoleViewer, s.listConsumables))
	mux.Handle("POST /api/consumables", s.require(model.RoleOperator, s.createConsumable))
	mux.Handle("GET /api/consumables.csv", s.require(model.RoleViewer, s.exportConsumablesCSV))
	mux.Handle("POST /api/consumables/import", s.require(model.RoleOperator, s.importConsumablesCSV))
	mux.Handle("GET /api/consumables/settings", s.require(model.RoleViewer, s.getConsumableSettings))
	mux.Handle("PUT /api/consumables/settings", s.require(model.RoleAdmin, s.putConsumableSettings))
	mux.Handle("GET /api/consumables/forecast", s.require(model.RoleViewer, s.consumablesForecast))
	mux.Handle("GET /api/consumables/installed", s.require(model.RoleViewer, s.installedSupplies))
	mux.Handle("GET /api/consumables/replacements", s.require(model.RoleViewer, s.listReplacements))
	mux.Handle("POST /api/consumables/replacements/{id}/confirm", s.require(model.RoleOperator, s.confirmReplacement))
	mux.Handle("POST /api/consumables/replacements/{id}/dismiss", s.require(model.RoleOperator, s.dismissReplacement))
	mux.Handle("POST /api/consumables/bulk-stock", s.require(model.RoleOperator, s.bulkStock))
	mux.Handle("GET /api/consumables/{id}", s.require(model.RoleViewer, s.getConsumable))
	mux.Handle("PATCH /api/consumables/{id}", s.require(model.RoleOperator, s.patchConsumable))
	mux.Handle("DELETE /api/consumables/{id}", s.require(model.RoleOperator, s.deleteConsumable))
	mux.Handle("POST /api/consumables/{id}/adjust-stock", s.require(model.RoleOperator, s.adjustStock))
	mux.Handle("GET /api/consumables/{id}/devices", s.require(model.RoleViewer, s.consumableDevices))
	mux.Handle("GET /api/consumables/{id}/forecast", s.require(model.RoleViewer, s.consumableForecast))
	mux.Handle("GET /api/consumables/{id}/events", s.require(model.RoleViewer, s.consumableEvents))

	mux.Handle("GET /api/devices/{id}/supplies", s.require(model.RoleViewer, s.deviceSupplies))
	mux.Handle("PUT /api/devices/{id}/supplies/{slot}/link", s.require(model.RoleOperator, s.linkSupply))
	mux.Handle("DELETE /api/devices/{id}/supplies/{slot}/link", s.require(model.RoleOperator, s.unlinkSupply))
	mux.Handle("POST /api/devices/{id}/supplies/{slot}/mark-replaced", s.require(model.RoleOperator, s.markReplaced))
}

// consumableRow is a list entry: the item plus derived status and forecast.
type consumableRow struct {
	*model.Consumable
	Status        model.StockStatus `json:"status"`
	LinkedDevices int               `json:"linkedDevices"`
	Forecast      forecastSummary   `json:"forecast"`
}

type forecastSummary struct {
	Known       bool     `json:"known"`
	StockDays   *float64 `json:"stockDays"`
	NextNeed    *float64 `json:"nextNeedDays"`
	ReorderSoon bool     `json:"reorderSoon"`
	Reason      string   `json:"reason,omitempty"`
}

func summarize(f consumables.ItemForecast) forecastSummary {
	return forecastSummary{Known: f.Known, StockDays: f.StockDays, NextNeed: f.NextNeed, ReorderSoon: f.ReorderSoon, Reason: f.Reason}
}

func consumableFilter(r *http.Request) store.ConsumableFilter {
	q := r.URL.Query()
	return store.ConsumableFilter{Q: q.Get("q"), Type: q.Get("type"), Color: consumables.NormalizeColor(q.Get("color")), Status: q.Get("status"), Model: q.Get("model")}
}

func (s *Server) rows(ctx context.Context, items []*model.Consumable) ([]consumableRow, error) {
	fcs, err := s.Fleet.Consumables.Forecast(ctx, items)
	if err != nil {
		return nil, err
	}
	out := make([]consumableRow, 0, len(items))
	for i, it := range items {
		out = append(out, consumableRow{Consumable: it, Status: it.Status(), LinkedDevices: fcs[i].LinkedCount, Forecast: summarize(fcs[i])})
	}
	return out, nil
}

func (s *Server) listConsumables(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListConsumables(r.Context(), consumableFilter(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	rows, err := s.rows(r.Context(), items)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "total": len(rows)})
}

func consumableAudit(c *model.Consumable) map[string]any {
	return map[string]any{"name": c.Name, "type": c.Type, "color": c.Color, "partNumber": c.PartNumber, "compatibleModels": c.CompatibleModels,
		"unit": c.Unit, "onHand": c.OnHand, "warningAt": c.WarningAt, "criticalAt": c.CriticalAt, "location": c.Location, "reorderUrl": c.ReorderURL}
}

func validateConsumable(c *model.Consumable) error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("name is required")
	}
	if c.Type == "" {
		c.Type = model.ConsumableOther
	}
	if !model.ValidConsumableType(c.Type) {
		return fmt.Errorf("unknown type %q", c.Type)
	}
	if c.OnHand < 0 || c.WarningAt < 0 || c.CriticalAt < 0 {
		return errors.New("quantities cannot be negative")
	}
	c.Color = consumables.NormalizeColor(c.Color)
	return nil
}

// afterStockChange refreshes links, stock alerts and the live stream.
func (s *Server) afterStockChange(ctx context.Context, id string, relink bool) {
	if relink {
		if err := s.Fleet.Consumables.ResolveAll(ctx); err != nil {
			s.Log.Warn("resolve supply links", "err", err)
		}
	}
	s.Fleet.ReconcileConsumables(ctx)
	s.publish(nil, model.Event{Type: "consumable.updated", Data: map[string]any{"id": id}})
}

func (s *Server) createConsumable(w http.ResponseWriter, r *http.Request) {
	var c model.Consumable
	if err := readJSON(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateConsumable(&c); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	c.ID = ""
	cs := s.Fleet.Consumables.Settings(r.Context())
	if c.WarningAt == 0 && c.CriticalAt == 0 {
		c.WarningAt, c.CriticalAt = cs.DefaultWarningAt, cs.DefaultCriticalAt
	}
	if err := s.Store.CreateConsumable(r.Context(), &c); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "consumable.create", c.ID, nil, consumableAudit(&c), "ok")
	s.afterStockChange(r.Context(), c.ID, true)
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) getConsumable(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.GetConsumable(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	devs, err := s.Fleet.Consumables.DevicesFor(r.Context(), c)
	if err != nil {
		s.fail(w, err)
		return
	}
	fc, err := s.Fleet.Consumables.Forecast(r.Context(), []*model.Consumable{c})
	if err != nil {
		s.fail(w, err)
		return
	}
	events, _ := s.Store.ListStockEvents(r.Context(), store.StockEventFilter{ConsumableID: c.ID, Limit: 200})
	writeJSON(w, http.StatusOK, map[string]any{"item": c, "status": c.Status(), "devices": devs, "forecast": fc[0], "events": events})
}

func (s *Server) patchConsumable(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Name             *string               `json:"name"`
		Type             *model.ConsumableType `json:"type"`
		Color            *string               `json:"color"`
		PartNumber       *string               `json:"partNumber"`
		CompatibleModels *[]string             `json:"compatibleModels"`
		Unit             *string               `json:"unit"`
		WarningAt        *int                  `json:"warningAt"`
		CriticalAt       *int                  `json:"criticalAt"`
		Location         *string               `json:"location"`
		Notes            *string               `json:"notes"`
		ReorderURL       *string               `json:"reorderUrl"`
		UnitCost         *float64              `json:"unitCost"`
		ClearUnitCost    *bool                 `json:"clearUnitCost"`
	}
	if err := readJSON(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := s.Store.GetConsumable(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	before := consumableAudit(c)
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&c.Name, p.Name)
	set(&c.Color, p.Color)
	set(&c.PartNumber, p.PartNumber)
	set(&c.Unit, p.Unit)
	set(&c.Location, p.Location)
	set(&c.Notes, p.Notes)
	set(&c.ReorderURL, p.ReorderURL)
	if p.Type != nil {
		c.Type = *p.Type
	}
	if p.CompatibleModels != nil {
		c.CompatibleModels = *p.CompatibleModels
	}
	if p.WarningAt != nil {
		c.WarningAt = *p.WarningAt
	}
	if p.CriticalAt != nil {
		c.CriticalAt = *p.CriticalAt
	}
	if p.UnitCost != nil {
		c.UnitCost = p.UnitCost
	}
	if p.ClearUnitCost != nil && *p.ClearUnitCost {
		c.UnitCost = nil
	}
	if err := validateConsumable(c); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Store.UpdateConsumable(r.Context(), c); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "consumable.update", c.ID, before, consumableAudit(c), "ok")
	s.afterStockChange(r.Context(), c.ID, true)
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) deleteConsumable(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	before, err := s.Store.GetConsumable(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if err := s.Store.DeleteConsumable(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "consumable.delete", id, consumableAudit(before), nil, "ok")
	s.afterStockChange(r.Context(), id, false)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) adjustStock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Set    *int   `json:"set"`
		Delta  *int   `json:"delta"`
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if (body.Set == nil) == (body.Delta == nil) {
		writeErr(w, http.StatusBadRequest, "give exactly one of set or delta")
		return
	}
	id := r.PathValue("id")
	before, err := s.Store.GetConsumable(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	ev := &model.StockEvent{Actor: actor(r), Reason: strings.TrimSpace(body.Reason)}
	switch {
	case body.Set != nil:
		if *body.Set < 0 {
			writeErr(w, http.StatusBadRequest, "quantity cannot be negative")
			return
		}
		ev.Kind, ev.Resulting = model.StockEventSet, *body.Set
	case *body.Delta >= 0:
		ev.Kind, ev.Delta = model.StockEventAdd, *body.Delta
	default:
		ev.Kind, ev.Delta = model.StockEventRemove, *body.Delta
	}
	c, err := s.Store.AdjustStock(r.Context(), id, ev)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "consumable.adjust", id, map[string]any{"onHand": before.OnHand}, map[string]any{"onHand": c.OnHand, "kind": ev.Kind, "delta": ev.Delta, "reason": ev.Reason}, "ok")
	s.afterStockChange(r.Context(), id, false)
	writeJSON(w, http.StatusOK, map[string]any{"item": c, "status": c.Status(), "event": ev})
}

// bulkStock commits a batch of inline on-hand edits (consumables spec §3).
func (s *Server) bulkStock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			ID     string `json:"id"`
			OnHand int    `json:"onHand"`
		} `json:"items"`
		Reason string `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "items is empty")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		reason = "batch update"
	}
	updated := []*model.Consumable{}
	errs := map[string]string{}
	for _, it := range body.Items {
		if it.OnHand < 0 {
			errs[it.ID] = "quantity cannot be negative"
			continue
		}
		before, err := s.Store.GetConsumable(r.Context(), it.ID)
		if err != nil {
			errs[it.ID] = "not found"
			continue
		}
		if before.OnHand == it.OnHand {
			updated = append(updated, before)
			continue
		}
		ev := &model.StockEvent{Actor: actor(r), Kind: model.StockEventSet, Resulting: it.OnHand, Reason: reason}
		c, err := s.Store.AdjustStock(r.Context(), it.ID, ev)
		if err != nil {
			errs[it.ID] = err.Error()
			continue
		}
		s.audit(r.Context(), r, "consumable.adjust", it.ID, map[string]any{"onHand": before.OnHand}, map[string]any{"onHand": c.OnHand, "kind": ev.Kind, "reason": reason}, "ok")
		updated = append(updated, c)
	}
	s.afterStockChange(r.Context(), "", false)
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated, "errors": errs})
}

func (s *Server) consumableDevices(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.GetConsumable(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	devs, err := s.Fleet.Consumables.DevicesFor(r.Context(), c)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, devs)
}

func (s *Server) consumableForecast(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.GetConsumable(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	fc, err := s.Fleet.Consumables.Forecast(r.Context(), []*model.Consumable{c})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fc[0])
}

func (s *Server) consumableEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.Store.ListStockEvents(r.Context(), store.StockEventFilter{ConsumableID: r.PathValue("id"), Limit: intQuery(r, "limit", 500)})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) consumablesForecast(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListConsumables(r.Context(), consumableFilter(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	fcs, err := s.Fleet.Consumables.Forecast(r.Context(), items)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fcs)
}

// installedSupplies is the fleet supplies view: every installed supply across
// the fleet with its stock link. ?unmatched=true keeps only unlinked ones.
func (s *Server) installedSupplies(w http.ResponseWriter, r *http.Request) {
	devs, err := s.resolveDevices(r.Context(), deviceFilter(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	ins, err := s.Fleet.Consumables.Installed(r.Context(), devs, r.URL.Query().Get("forecast") != "false")
	if err != nil {
		s.fail(w, err)
		return
	}
	if r.URL.Query().Get("unmatched") == "true" {
		var only []consumables.Installed
		for _, in := range ins {
			if in.Consumable == nil {
				only = append(only, in)
			}
		}
		if only == nil {
			only = []consumables.Installed{}
		}
		ins = only
	}
	writeJSON(w, http.StatusOK, ins)
}

func (s *Server) listReplacements(w http.ResponseWriter, r *http.Request) {
	f := store.StockEventFilter{Kind: model.StockEventReplacement, Limit: intQuery(r, "limit", 200)}
	if r.URL.Query().Get("all") != "true" {
		f.Status = model.StockEventPending
	}
	events, err := s.Store.ListStockEvents(r.Context(), f)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) confirmReplacement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	ev, c, err := s.Fleet.Consumables.ConfirmReplacement(r.Context(), id, actor(r))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.fail(w, err)
			return
		}
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	s.audit(r.Context(), r, "consumable.replacement.confirm", c.ID, nil, map[string]any{"event": id, "onHand": c.OnHand, "device": ev.DeviceID}, "ok")
	s.afterStockChange(r.Context(), c.ID, false)
	writeJSON(w, http.StatusOK, map[string]any{"item": c, "event": ev})
}

func (s *Server) dismissReplacement(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	ev, err := s.Store.GetStockEvent(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if ev.Status != model.StockEventPending {
		writeErr(w, http.StatusConflict, "replacement is not pending")
		return
	}
	if err := s.Store.SetStockEventStatus(r.Context(), id, model.StockEventDismissed); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "consumable.replacement.dismiss", ev.ConsumableID, nil, map[string]any{"event": id, "device": ev.DeviceID}, "ok")
	s.publish(r, model.Event{Type: "consumable.updated", Data: map[string]any{"id": ev.ConsumableID}})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- settings ---------------------------------------------------------------

func (s *Server) getConsumableSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Fleet.Consumables.Settings(r.Context()))
}

func (s *Server) putConsumableSettings(w http.ResponseWriter, r *http.Request) {
	var cs model.ConsumableSettings
	if err := readJSON(r, &cs); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	before := s.Fleet.Consumables.Settings(r.Context())
	saved, err := s.Fleet.Consumables.SaveSettings(r.Context(), cs)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r.Context(), r, "consumable.settings", "consumables", map[string]any{"settings": before}, map[string]any{"settings": saved}, "ok")
	writeJSON(w, http.StatusOK, saved)
}

// --- per-device supplies ----------------------------------------------------

func (s *Server) deviceSupplies(w http.ResponseWriter, r *http.Request) {
	d, err := s.Store.GetDevice(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	ins, err := s.Fleet.Consumables.Installed(r.Context(), []*model.Device{d}, true)
	if err != nil {
		s.fail(w, err)
		return
	}
	pending, _ := s.Store.ListStockEvents(r.Context(), store.StockEventFilter{DeviceID: d.ID, Kind: model.StockEventReplacement, Status: model.StockEventPending})
	writeJSON(w, http.StatusOK, map[string]any{"supplies": ins, "pendingReplacements": pending})
}

func slot(r *http.Request) (int, error) {
	n, err := strconv.Atoi(r.PathValue("slot"))
	if err != nil || n < 0 {
		return 0, errors.New("bad supply slot")
	}
	return n, nil
}

func (s *Server) findSupply(ctx context.Context, deviceID string, idx int) (*model.Device, *model.Supply, error) {
	d, err := s.Store.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, nil, err
	}
	for i := range d.Supplies {
		if d.Supplies[i].Index == idx {
			return d, &d.Supplies[i], nil
		}
	}
	return d, nil, store.ErrNotFound
}

func (s *Server) linkSupply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConsumableID string `json:"consumableId"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	idx, err := slot(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d, sp, err := s.findSupply(r.Context(), r.PathValue("id"), idx)
	if err != nil {
		s.fail(w, err)
		return
	}
	if _, err := s.Store.GetConsumable(r.Context(), body.ConsumableID); err != nil {
		s.fail(w, err)
		return
	}
	l := &model.SupplyLink{DeviceID: d.ID, SupplyIndex: idx, ConsumableID: body.ConsumableID, Source: "manual"}
	if err := s.Store.SetSupplyLink(r.Context(), l); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "consumable.link", d.ID, nil, map[string]any{"supply": sp.Description, "slot": idx, "consumableId": body.ConsumableID}, "ok")
	s.publish(r, model.Event{Type: "consumable.updated", Data: map[string]any{"id": body.ConsumableID, "deviceId": d.ID}})
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) unlinkSupply(w http.ResponseWriter, r *http.Request) {
	idx, err := slot(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	if err := s.Store.DeleteSupplyLink(r.Context(), id, idx); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "consumable.unlink", id, map[string]any{"slot": idx}, nil, "ok")
	s.publish(r, model.Event{Type: "consumable.updated", Data: map[string]any{"deviceId": id}})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// markReplaced records that an operator swapped an installed supply and,
// when asked, decrements the linked stock item (consumables spec §5).
func (s *Server) markReplaced(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decrement    bool   `json:"decrement"`
		ConsumableID string `json:"consumableId"`
		Reason       string `json:"reason"`
	}
	if r.ContentLength > 0 {
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	idx, err := slot(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	d, sp, err := s.findSupply(r.Context(), r.PathValue("id"), idx)
	if err != nil {
		s.fail(w, err)
		return
	}
	cid := body.ConsumableID
	if cid == "" {
		links, _ := s.Store.ListSupplyLinks(r.Context(), store.SupplyLinkFilter{DeviceID: d.ID})
		for _, l := range links {
			if l.SupplyIndex == idx {
				cid = l.ConsumableID
			}
		}
	}
	if cid == "" {
		writeErr(w, http.StatusBadRequest, "this supply is not linked to a stock item")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		reason = fmt.Sprintf("%s replaced on %s", sp.Description, d.Name())
	}
	ev := &model.StockEvent{Actor: actor(r), Kind: model.StockEventReplacement, Delta: -1, DeviceID: d.ID, SupplyIndex: idx, Reason: reason, Status: model.StockEventApplied}
	if !body.Decrement {
		ev.Status = model.StockEventLogged
	}
	c, err := s.Store.AdjustStock(r.Context(), cid, ev)
	if err != nil {
		s.fail(w, err)
		return
	}
	// Any pending auto-detected replacement for this slot is now resolved.
	pend, _ := s.Store.ListStockEvents(r.Context(), store.StockEventFilter{DeviceID: d.ID, Kind: model.StockEventReplacement, Status: model.StockEventPending})
	for _, p := range pend {
		if p.SupplyIndex == idx {
			_ = s.Store.SetStockEventStatus(r.Context(), p.ID, model.StockEventConfirmed)
		}
	}
	s.audit(r.Context(), r, "consumable.replaced", cid, nil, map[string]any{"device": d.ID, "slot": idx, "supply": sp.Description, "decrement": body.Decrement, "onHand": c.OnHand}, "ok")
	s.afterStockChange(r.Context(), cid, false)
	writeJSON(w, http.StatusOK, map[string]any{"item": c, "status": c.Status(), "event": ev})
}

// --- CSV ---------------------------------------------------------------------

var consumableCSVHeader = []string{"id", "name", "type", "color", "partNumber", "compatibleModels", "unit", "onHand", "warningAt", "criticalAt", "location", "notes", "reorderUrl", "unitCost"}

func (s *Server) exportConsumablesCSV(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListConsumables(r.Context(), consumableFilter(r))
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="consumables.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write(consumableCSVHeader)
	for _, c := range items {
		cost := ""
		if c.UnitCost != nil {
			cost = strconv.FormatFloat(*c.UnitCost, 'f', -1, 64)
		}
		_ = cw.Write([]string{c.ID, c.Name, string(c.Type), c.Color, c.PartNumber, strings.Join(c.CompatibleModels, ";"), c.Unit,
			strconv.Itoa(c.OnHand), strconv.Itoa(c.WarningAt), strconv.Itoa(c.CriticalAt), c.Location, c.Notes, c.ReorderURL, cost})
	}
	cw.Flush()
}

// importConsumablesCSV upserts items from CSV (the export format). Rows match
// an existing item by id, then part number, then name; on-hand changes are
// recorded as "import" stock events so history stays complete.
func (s *Server) importConsumablesCSV(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "cannot read body")
		return
	}
	cr := csv.NewReader(strings.NewReader(string(body)))
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	records, err := cr.ReadAll()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid CSV: "+err.Error())
		return
	}
	if len(records) < 2 {
		writeErr(w, http.StatusBadRequest, "CSV needs a header row and at least one item")
		return
	}
	col := map[string]int{}
	for i, h := range records[0] {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))] = i
	}
	if _, ok := col["name"]; !ok {
		writeErr(w, http.StatusBadRequest, "CSV needs a name column")
		return
	}
	existing, err := s.Store.ListConsumables(r.Context(), store.ConsumableFilter{})
	if err != nil {
		s.fail(w, err)
		return
	}
	byID, byPart, byName := map[string]*model.Consumable{}, map[string]*model.Consumable{}, map[string]*model.Consumable{}
	for _, c := range existing {
		byID[c.ID] = c
		if c.PartNumber != "" {
			byPart[strings.ToLower(c.PartNumber)] = c
		}
		byName[strings.ToLower(c.Name)] = c
	}
	cs := s.Fleet.Consumables.Settings(r.Context())
	get := func(rec []string, key string) (string, bool) {
		i, ok := col[strings.ToLower(key)]
		if !ok || i >= len(rec) {
			return "", false
		}
		return strings.TrimSpace(rec[i]), true
	}
	atoi := func(v string, def int) int {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
		return def
	}
	created, updated := 0, 0
	errs := []string{}
	for n, rec := range records[1:] {
		name, _ := get(rec, "name")
		if name == "" {
			continue
		}
		id, _ := get(rec, "id")
		part, _ := get(rec, "partNumber")
		target := byID[id]
		if target == nil && part != "" {
			target = byPart[strings.ToLower(part)]
		}
		if target == nil {
			target = byName[strings.ToLower(name)]
		}
		var c model.Consumable
		if target != nil {
			c = *target
		} else {
			c = model.Consumable{WarningAt: cs.DefaultWarningAt, CriticalAt: cs.DefaultCriticalAt, Unit: "each"}
		}
		c.Name = name
		if v, ok := get(rec, "type"); ok && v != "" {
			c.Type = model.ConsumableType(strings.ToLower(strings.ReplaceAll(v, " ", "_")))
		}
		if v, ok := get(rec, "color"); ok {
			c.Color = v
		}
		if part != "" || target == nil {
			c.PartNumber = part
		}
		if v, ok := get(rec, "compatibleModels"); ok {
			c.CompatibleModels = nil
			for _, m := range strings.FieldsFunc(v, func(r rune) bool { return r == ';' || r == '|' }) {
				if m = strings.TrimSpace(m); m != "" {
					c.CompatibleModels = append(c.CompatibleModels, m)
				}
			}
		}
		if v, ok := get(rec, "unit"); ok && v != "" {
			c.Unit = v
		}
		if v, ok := get(rec, "warningAt"); ok && v != "" {
			c.WarningAt = atoi(v, c.WarningAt)
		}
		if v, ok := get(rec, "criticalAt"); ok && v != "" {
			c.CriticalAt = atoi(v, c.CriticalAt)
		}
		if v, ok := get(rec, "location"); ok {
			c.Location = v
		}
		if v, ok := get(rec, "notes"); ok {
			c.Notes = v
		}
		if v, ok := get(rec, "reorderUrl"); ok {
			c.ReorderURL = v
		}
		if v, ok := get(rec, "unitCost"); ok {
			if v == "" {
				c.UnitCost = nil
			} else if f, err := strconv.ParseFloat(v, 64); err == nil {
				c.UnitCost = &f
			}
		}
		onHand, hasQty := get(rec, "onHand")
		if err := validateConsumable(&c); err != nil {
			errs = append(errs, fmt.Sprintf("row %d: %v", n+2, err))
			continue
		}
		if target == nil {
			c.ID = ""
			if hasQty {
				c.OnHand = atoi(onHand, 0)
			}
			if err := s.Store.CreateConsumable(r.Context(), &c); err != nil {
				errs = append(errs, fmt.Sprintf("row %d: %v", n+2, err))
				continue
			}
			byID[c.ID] = &c
			if c.PartNumber != "" {
				byPart[strings.ToLower(c.PartNumber)] = &c
			}
			byName[strings.ToLower(c.Name)] = &c
			s.audit(r.Context(), r, "consumable.create", c.ID, nil, consumableAudit(&c), "import")
			created++
			continue
		}
		if err := s.Store.UpdateConsumable(r.Context(), &c); err != nil {
			errs = append(errs, fmt.Sprintf("row %d: %v", n+2, err))
			continue
		}
		if hasQty && onHand != "" && atoi(onHand, target.OnHand) != target.OnHand {
			ev := &model.StockEvent{Actor: actor(r), Kind: model.StockEventImport, Resulting: atoi(onHand, target.OnHand), Reason: "CSV import"}
			if _, err := s.Store.AdjustStock(r.Context(), c.ID, ev); err != nil {
				errs = append(errs, fmt.Sprintf("row %d: %v", n+2, err))
			}
			c.OnHand = ev.Resulting
		}
		s.audit(r.Context(), r, "consumable.update", c.ID, consumableAudit(target), consumableAudit(&c), "import")
		updated++
	}
	s.afterStockChange(r.Context(), "", true)
	writeJSON(w, http.StatusOK, map[string]any{"created": created, "updated": updated, "errors": errs})
}

// dashboardConsumables feeds the dashboard's stock tile and reorder panel.
func (s *Server) dashboardConsumables(ctx context.Context) map[string]any {
	items, err := s.Store.ListConsumables(ctx, store.ConsumableFilter{})
	if err != nil {
		return map[string]any{"lowStock": []consumableRow{}, "reorder": []consumableRow{}, "pendingReplacements": 0}
	}
	rows, err := s.rows(ctx, items)
	if err != nil {
		return map[string]any{"lowStock": []consumableRow{}, "reorder": []consumableRow{}, "pendingReplacements": 0}
	}
	low, reorder := []consumableRow{}, []consumableRow{}
	for _, row := range rows {
		if row.Status != model.StockOK {
			low = append(low, row)
		}
		if row.Forecast.Known && (row.Forecast.ReorderSoon || row.Forecast.NextNeed != nil) {
			reorder = append(reorder, row)
		}
	}
	sortRowsBySoonest(reorder)
	if len(reorder) > 8 {
		reorder = reorder[:8]
	}
	pending, _ := s.Store.ListStockEvents(ctx, store.StockEventFilter{Kind: model.StockEventReplacement, Status: model.StockEventPending, Limit: 100})
	return map[string]any{"lowStock": low, "reorder": reorder, "pendingReplacements": len(pending)}
}

func sortRowsBySoonest(rows []consumableRow) {
	key := func(r consumableRow) float64 {
		if r.Forecast.StockDays != nil {
			return *r.Forecast.StockDays
		}
		if r.Forecast.NextNeed != nil {
			return *r.Forecast.NextNeed + 1e6
		}
		return 1e9
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && key(rows[j]) < key(rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}
