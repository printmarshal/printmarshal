package reports

import (
	"context"
	"sort"

	"printmarshal/internal/model"
)

// HealthDay is the fleet's reachability and worst-severity picture on one day.
type HealthDay struct {
	Day          string  `json:"day"`
	Polled       int     `json:"polled"`       // devices polled at least once
	Reachable    int     `json:"reachable"`    // devices reached at least once
	Unreachable  int     `json:"unreachable"`  // devices never reached that day
	ReachablePct float64 `json:"reachablePct"` // successful polls / polls
	OK           int     `json:"ok"`           // devices whose worst status that day was ok
	Warning      int     `json:"warning"`
	Critical     int     `json:"critical"`
}

// HealthDevice is one device's availability over the range.
type HealthDevice struct {
	DeviceID        string  `json:"deviceId"`
	Name            string  `json:"name"`
	Model           string  `json:"model"`
	Location        string  `json:"location"`
	Polls           int     `json:"polls"`
	Failed          int     `json:"failed"`
	AvailabilityPct float64 `json:"availabilityPct"` // successful polls / polls
	OfflineMinutes  float64 `json:"offlineMinutes"`
	Incidents       int     `json:"incidents"` // reachable → unreachable transitions
	MeanOfflineMin  float64 `json:"meanOfflineMinutes"`
	WorstSeverity   string  `json:"worstSeverity"` // ok | warning | critical | "" (no status samples)
	DaysCritical    int     `json:"daysCritical"`
	DaysWarning     int     `json:"daysWarning"`
}

// HealthSummary is the fleet-health KPI set.
type HealthSummary struct {
	Devices         int     `json:"devices"`
	Polled          int     `json:"polled"`
	AvailabilityPct float64 `json:"availabilityPct"`
	OfflineMinutes  float64 `json:"offlineMinutes"`
	Incidents       int     `json:"incidents"`
	DevicesOffline  int     `json:"devicesOffline"` // devices with at least one failed poll
	DaysCritical    int     `json:"daysCritical"`   // days with at least one critical device
	WorstDay        string  `json:"worstDay,omitempty"`
	WorstDayPct     float64 `json:"worstDayPct"`
}

// HealthReport is reachability and severity over time (spec §8.3).
type HealthReport struct {
	Range   Range          `json:"range"`
	Summary HealthSummary  `json:"summary"`
	Days    []HealthDay    `json:"days"`
	Devices []HealthDevice `json:"devices"` // sorted by offline minutes desc
}

func sevName(v float64) string {
	switch {
	case v >= 2:
		return string(model.SeverityCritical)
	case v >= 1:
		return string(model.SeverityWarning)
	}
	return string(model.SeverityOK)
}

// FleetHealth computes the fleet-health report.
func (s *Service) FleetHealth(ctx context.Context, p Params) (*HealthReport, error) {
	sc, err := s.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	online, err := s.Store.DailyTelemetry(ctx, "status.online", sc.rangeOf())
	if err != nil {
		return nil, err
	}
	sev, err := s.Store.DailyTelemetry(ctx, "status.severity", sc.rangeOf())
	if err != nil {
		return nil, err
	}
	off, err := s.Store.OfflineStats(ctx, sc.rangeOf())
	if err != nil {
		return nil, err
	}
	rep := &HealthReport{Range: sc.rng, Days: []HealthDay{}, Devices: []HealthDevice{}}
	dayIdx := map[string]int{}
	for i, d := range sc.days {
		dayIdx[d] = i
		rep.Days = append(rep.Days, HealthDay{Day: d})
	}
	rows := map[string]*HealthDevice{}
	row := func(id string) *HealthDevice {
		r, ok := rows[id]
		if !ok {
			r = &HealthDevice{DeviceID: id, Name: sc.name(id), Model: sc.field(id, func(d *model.Device) string { return d.Model }), Location: sc.field(id, func(d *model.Device) string { return d.Location })}
			rows[id] = r
		}
		return r
	}
	type dayAcc struct{ polls, ok float64 }
	acc := map[string]*dayAcc{}
	for _, a := range online {
		if _, known := sc.byID[a.DeviceID]; !known && sc.ids != nil {
			continue
		}
		i, ok := dayIdx[a.Day]
		if !ok {
			continue
		}
		d := &rep.Days[i]
		d.Polled++
		if a.Max >= 1 {
			d.Reachable++
		} else {
			d.Unreachable++
		}
		ac, ok := acc[a.Day]
		if !ok {
			ac = &dayAcc{}
			acc[a.Day] = ac
		}
		ac.polls += float64(a.Count)
		ac.ok += a.Avg * float64(a.Count)
		row(a.DeviceID)
	}
	worst := map[string]float64{}
	for _, a := range sev {
		i, ok := dayIdx[a.Day]
		if !ok {
			continue
		}
		d := &rep.Days[i]
		r := row(a.DeviceID)
		switch sevName(a.Max) {
		case string(model.SeverityCritical):
			d.Critical++
			r.DaysCritical++
		case string(model.SeverityWarning):
			d.Warning++
			r.DaysWarning++
		default:
			d.OK++
		}
		if a.Max > worst[a.DeviceID] || r.WorstSeverity == "" {
			worst[a.DeviceID] = a.Max
			r.WorstSeverity = sevName(worst[a.DeviceID])
		}
	}
	for i := range rep.Days {
		d := &rep.Days[i]
		if ac, ok := acc[d.Day]; ok && ac.polls > 0 {
			d.ReachablePct = round1(ac.ok * 100 / ac.polls)
		}
		if d.Critical > 0 {
			rep.Summary.DaysCritical++
		}
		if d.Polled > 0 && (rep.Summary.WorstDay == "" || d.ReachablePct < rep.Summary.WorstDayPct) {
			rep.Summary.WorstDay, rep.Summary.WorstDayPct = d.Day, d.ReachablePct
		}
	}
	var polls, okPolls float64
	for _, st := range off {
		r := row(st.DeviceID)
		r.Polls, r.Failed, r.OfflineMinutes, r.Incidents = st.Polls, st.Failed, round1(st.OfflineMinutes), st.Incidents
		if st.Polls > 0 {
			r.AvailabilityPct = round1(float64(st.Polls-st.Failed) * 100 / float64(st.Polls))
		}
		if st.Incidents > 0 {
			r.MeanOfflineMin = round1(st.OfflineMinutes / float64(st.Incidents))
			rep.Summary.DevicesOffline++
		}
		polls += float64(st.Polls)
		okPolls += float64(st.Polls - st.Failed)
		rep.Summary.OfflineMinutes += st.OfflineMinutes
		rep.Summary.Incidents += st.Incidents
	}
	rep.Summary.OfflineMinutes = round1(rep.Summary.OfflineMinutes)
	if polls > 0 {
		rep.Summary.AvailabilityPct = round1(okPolls * 100 / polls)
	}
	rep.Summary.Devices = len(sc.devices)
	rep.Summary.Polled = len(rows)
	for _, r := range rows {
		rep.Devices = append(rep.Devices, *r)
	}
	sort.SliceStable(rep.Devices, func(i, j int) bool {
		a, b := rep.Devices[i], rep.Devices[j]
		if a.OfflineMinutes != b.OfflineMinutes {
			return a.OfflineMinutes > b.OfflineMinutes
		}
		if a.DaysCritical != b.DaysCritical {
			return a.DaysCritical > b.DaysCritical
		}
		return a.Name < b.Name
	})
	return rep, nil
}

// Tables implements CSV export: devices (default), days.
func (r *HealthReport) Tables() []Table {
	dev := Table{Name: "devices", Columns: []string{"deviceId", "name", "model", "location", "polls", "failed", "availabilityPct", "offlineMinutes", "incidents", "meanOfflineMinutes", "worstSeverity", "daysCritical", "daysWarning"}}
	for _, d := range r.Devices {
		dev.Rows = append(dev.Rows, []string{d.DeviceID, d.Name, d.Model, d.Location, fmtI(d.Polls), fmtI(d.Failed), fmtF(d.AvailabilityPct), fmtF(d.OfflineMinutes), fmtI(d.Incidents), fmtF(d.MeanOfflineMin), d.WorstSeverity, fmtI(d.DaysCritical), fmtI(d.DaysWarning)})
	}
	days := Table{Name: "days", Columns: []string{"day", "polled", "reachable", "unreachable", "reachablePct", "ok", "warning", "critical"}}
	for _, d := range r.Days {
		days.Rows = append(days.Rows, []string{d.Day, fmtI(d.Polled), fmtI(d.Reachable), fmtI(d.Unreachable), fmtF(d.ReachablePct), fmtI(d.OK), fmtI(d.Warning), fmtI(d.Critical)})
	}
	return []Table{dev, days}
}
