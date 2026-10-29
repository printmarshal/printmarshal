package consumables

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// SettingsKey is the settings-table key for ConsumableSettings.
const SettingsKey = "consumables"

// Service is the consumables façade used by fleet and the API.
type Service struct {
	Store store.Store
	Log   *slog.Logger
}

// New builds a service.
func New(st store.Store, log *slog.Logger) *Service { return &Service{Store: st, Log: log} }

// Settings returns the operator settings merged over the defaults.
func (s *Service) Settings(ctx context.Context) model.ConsumableSettings {
	cs := model.DefaultConsumableSettings()
	_ = s.Store.GetSetting(ctx, SettingsKey, &cs)
	return Validate(cs)
}

// SaveSettings validates and stores settings.
func (s *Service) SaveSettings(ctx context.Context, cs model.ConsumableSettings) (model.ConsumableSettings, error) {
	switch cs.AutoDecrement {
	case model.AutoDecrementOff, model.AutoDecrementPrompt, model.AutoDecrementAutomatic:
	default:
		return cs, fmt.Errorf("autoDecrement must be off, prompt or automatic")
	}
	cs = Validate(cs)
	return cs, s.Store.SetSetting(ctx, SettingsKey, cs)
}

// Validate clamps settings into sane ranges.
func Validate(cs model.ConsumableSettings) model.ConsumableSettings {
	d := model.DefaultConsumableSettings()
	if cs.AutoDecrement == "" {
		cs.AutoDecrement = d.AutoDecrement
	}
	if cs.ReplacementJumpPct < 10 || cs.ReplacementJumpPct > 95 {
		cs.ReplacementJumpPct = d.ReplacementJumpPct
	}
	if cs.ForecastMinDays < 1 {
		cs.ForecastMinDays = d.ForecastMinDays
	}
	if cs.ReorderLeadDays < 0 {
		cs.ReorderLeadDays = d.ReorderLeadDays
	}
	if cs.DefaultWarningAt < 0 {
		cs.DefaultWarningAt = 0
	}
	if cs.DefaultCriticalAt < 0 {
		cs.DefaultCriticalAt = 0
	}
	if cs.DefaultCriticalAt > cs.DefaultWarningAt {
		cs.DefaultWarningAt = cs.DefaultCriticalAt
	}
	return cs
}

// Resolve recomputes the auto links of one device from the current stock
// items. Manual links are left untouched (operator-owned).
func (s *Service) Resolve(ctx context.Context, dev *model.Device) error {
	items, err := s.Store.ListConsumables(ctx, store.ConsumableFilter{})
	if err != nil {
		return err
	}
	return s.resolveWith(ctx, dev, items)
}

func (s *Service) resolveWith(ctx context.Context, dev *model.Device, items []*model.Consumable) error {
	var links []*model.SupplyLink
	for _, sp := range dev.Supplies {
		if sp.Class == "other" {
			continue
		}
		if c := Match(items, dev, sp); c != nil {
			links = append(links, &model.SupplyLink{DeviceID: dev.ID, SupplyIndex: sp.Index, ConsumableID: c.ID, Source: "auto"})
		}
	}
	return s.Store.ReplaceAutoLinks(ctx, dev.ID, links)
}

// ResolveAll recomputes auto links for every device (after stock items change).
func (s *Service) ResolveAll(ctx context.Context) error {
	items, err := s.Store.ListConsumables(ctx, store.ConsumableFilter{})
	if err != nil {
		return err
	}
	devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{IncludeDecommissioned: true})
	if err != nil {
		return err
	}
	for _, d := range devs {
		if err := s.resolveWith(ctx, d, items); err != nil {
			return err
		}
	}
	return nil
}

// Installed is one installed supply decorated with its stock link and forecast
// (the fleet supplies view and GET /devices/{id}/supplies).
type Installed struct {
	DeviceID    string            `json:"deviceId"`
	DeviceName  string            `json:"deviceName"`
	DeviceModel string            `json:"deviceModel"`
	Address     string            `json:"address"`
	Supply      model.Supply      `json:"supply"`
	Consumable  *model.Consumable `json:"consumable,omitempty"`
	LinkSource  string            `json:"linkSource,omitempty"` // auto | manual | ""
	Forecast    *DeviceForecast   `json:"forecast,omitempty"`
	Suggestion  *model.Consumable `json:"suggestion,omitempty"` // for unmatched supplies
}

// Installed lists every installed supply of the given devices, decorated.
func (s *Service) Installed(ctx context.Context, devs []*model.Device, withForecast bool) ([]Installed, error) {
	items, err := s.Store.ListConsumables(ctx, store.ConsumableFilter{})
	if err != nil {
		return nil, err
	}
	byID := map[string]*model.Consumable{}
	for _, it := range items {
		byID[it.ID] = it
	}
	cs := s.Settings(ctx)
	out := []Installed{}
	for _, d := range devs {
		links, err := s.Store.ListSupplyLinks(ctx, store.SupplyLinkFilter{DeviceID: d.ID})
		if err != nil {
			return nil, err
		}
		linkFor := map[int]*model.SupplyLink{}
		for _, l := range links {
			linkFor[l.SupplyIndex] = l
		}
		for _, sp := range d.Supplies {
			if sp.Class == "other" {
				continue
			}
			in := Installed{DeviceID: d.ID, DeviceName: d.Name(), DeviceModel: d.Model, Address: d.Address, Supply: sp}
			if l := linkFor[sp.Index]; l != nil {
				if c := byID[l.ConsumableID]; c != nil {
					in.Consumable, in.LinkSource = c, l.Source
				}
			}
			if in.Consumable == nil && cs.AutoSuggestFromScans {
				sug := Suggest(d, sp, cs)
				in.Suggestion = &sug
			}
			if withForecast {
				f := s.deviceForecast(ctx, d, sp, cs)
				in.Forecast = &f
			}
			out = append(out, in)
		}
	}
	return out, nil
}

func (s *Service) deviceForecast(ctx context.Context, d *model.Device, sp model.Supply, cs model.ConsumableSettings) DeviceForecast {
	f := DeviceForecast{DeviceID: d.ID, DeviceName: d.Name(), SupplyIndex: sp.Index, Description: sp.Description, Percent: sp.Percent}
	samples, err := s.Store.QueryTelemetry(ctx, store.TelemetryQuery{DeviceID: d.ID, Metric: "supply.percent", Label: SlotLabel(sp), From: time.Now().Add(-forecastWindow)})
	if err != nil {
		f.Rate.Reason = err.Error()
		return f
	}
	filled := sp.Class == "filled"
	f.Rate = ConsumptionRate(samples, filled, cs.ReplacementJumpPct, float64(cs.ForecastMinDays))
	f.DaysToEmpty = daysToEmpty(sp.Percent, filled, f.Rate)
	return f
}

// DevicesFor lists the installed supplies linked to one stock item, with forecasts.
func (s *Service) DevicesFor(ctx context.Context, item *model.Consumable) ([]Installed, error) {
	links, err := s.Store.ListSupplyLinks(ctx, store.SupplyLinkFilter{ConsumableID: item.ID})
	if err != nil {
		return nil, err
	}
	cs := s.Settings(ctx)
	out := []Installed{}
	devCache := map[string]*model.Device{}
	for _, l := range links {
		d := devCache[l.DeviceID]
		if d == nil {
			d, err = s.Store.GetDevice(ctx, l.DeviceID)
			if err != nil {
				continue
			}
			devCache[l.DeviceID] = d
		}
		for _, sp := range d.Supplies {
			if sp.Index != l.SupplyIndex {
				continue
			}
			f := s.deviceForecast(ctx, d, sp, cs)
			out = append(out, Installed{DeviceID: d.ID, DeviceName: d.Name(), DeviceModel: d.Model, Address: d.Address, Supply: sp, Consumable: item, LinkSource: l.Source, Forecast: &f})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceName < out[j].DeviceName })
	return out, nil
}

// Forecast projects every given stock item.
func (s *Service) Forecast(ctx context.Context, items []*model.Consumable) ([]ItemForecast, error) {
	cs := s.Settings(ctx)
	out := []ItemForecast{}
	for _, it := range items {
		ins, err := s.DevicesFor(ctx, it)
		if err != nil {
			return nil, err
		}
		devs := make([]DeviceForecast, 0, len(ins))
		for _, in := range ins {
			if in.Forecast != nil {
				devs = append(devs, *in.Forecast)
			}
		}
		out = append(out, Project(it, devs, cs.ReorderLeadDays))
	}
	return out, nil
}

// AfterPoll runs after a successful poll: it refreshes the device's auto links
// and records inferred replacements as stock events, decrementing stock when
// the auto-decrement setting is "automatic". It returns the events it recorded.
func (s *Service) AfterPoll(ctx context.Context, prev, cur *model.Device) []*model.StockEvent {
	if err := s.Resolve(ctx, cur); err != nil {
		s.Log.Warn("resolve supply links", "device", cur.ID, "err", err)
	}
	if prev == nil {
		return nil
	}
	cs := s.Settings(ctx)
	reps := DetectReplacements(prev.Supplies, cur.Supplies, cs.ReplacementJumpPct)
	if len(reps) == 0 {
		return nil
	}
	links, err := s.Store.ListSupplyLinks(ctx, store.SupplyLinkFilter{DeviceID: cur.ID})
	if err != nil {
		return nil
	}
	linkFor := map[int]string{}
	for _, l := range links {
		linkFor[l.SupplyIndex] = l.ConsumableID
	}
	var events []*model.StockEvent
	for _, r := range reps {
		cid := linkFor[r.Index]
		if cid == "" {
			s.Log.Info("supply replaced (no stock item linked)", "device", cur.Name(), "supply", r.Description, "from", r.From, "to", r.To)
			continue
		}
		ev := &model.StockEvent{Actor: "system", Kind: model.StockEventReplacement, Delta: -1, DeviceID: cur.ID, SupplyIndex: r.Index,
			Reason: fmt.Sprintf("%s on %s rose from %d%% to %d%%", r.Description, cur.Name(), r.From, r.To)}
		switch cs.AutoDecrement {
		case model.AutoDecrementAutomatic:
			ev.Status = model.StockEventApplied
		case model.AutoDecrementPrompt:
			ev.Status = model.StockEventPending
		default:
			ev.Status = model.StockEventLogged
		}
		if _, err := s.Store.AdjustStock(ctx, cid, ev); err != nil {
			s.Log.Warn("record replacement", "device", cur.ID, "err", err)
			continue
		}
		events = append(events, ev)
	}
	return events
}

// ConfirmReplacement applies a pending replacement's decrement.
func (s *Service) ConfirmReplacement(ctx context.Context, id int64, actor string) (*model.StockEvent, *model.Consumable, error) {
	ev, err := s.Store.GetStockEvent(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if ev.Status != model.StockEventPending {
		return nil, nil, fmt.Errorf("replacement is %s, not pending", ev.Status)
	}
	// Claim the event first (pending -> confirmed is conditional, so a
	// concurrent confirmation of the same replacement loses and no unit is
	// subtracted twice); put it back if the stock update then fails so the
	// confirmation can be retried (code review H-010).
	if err := s.Store.SetStockEventStatus(ctx, id, model.StockEventPending, model.StockEventConfirmed); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, fmt.Errorf("replacement is no longer pending")
		}
		return nil, nil, err
	}
	applied := &model.StockEvent{Actor: actor, Kind: model.StockEventReplacement, Delta: -1, DeviceID: ev.DeviceID, SupplyIndex: ev.SupplyIndex,
		Reason: "confirmed: " + ev.Reason, Status: model.StockEventApplied}
	c, err := s.Store.AdjustStock(ctx, ev.ConsumableID, applied)
	if err != nil {
		if rb := s.Store.SetStockEventStatus(context.WithoutCancel(ctx), id, model.StockEventConfirmed, model.StockEventPending); rb != nil {
			s.Log.Error("replacement confirmation: stock update failed and the event could not be returned to pending", "event", id, "err", err, "rollback", rb)
		}
		return nil, nil, err
	}
	return applied, c, nil
}
