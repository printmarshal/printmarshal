package reports

import (
	"context"
	"sort"

	"printmarshal/internal/model"
)

// AlertDay is the alerts raised on one day.
type AlertDay struct {
	Day      string         `json:"day"`
	Total    int            `json:"total"`
	Critical int            `json:"critical"`
	Warning  int            `json:"warning"`
	BySource map[string]int `json:"bySource"`
}

// NameCount is a labelled count.
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// AlertDevice is one device's alert statistics over the range.
type AlertDevice struct {
	DeviceID         string   `json:"deviceId"`
	Name             string   `json:"name"`
	Model            string   `json:"model"`
	Location         string   `json:"location"`
	Alerts           int      `json:"alerts"`
	Critical         int      `json:"critical"`
	Warning          int      `json:"warning"`
	Acked            int      `json:"acked"`
	Cleared          int      `json:"cleared"`
	MeanAckMinutes   *float64 `json:"meanAckMinutes"`
	MeanClearMinutes *float64 `json:"meanClearMinutes"`
}

// AlertsSummary is the alerts KPI set.
type AlertsSummary struct {
	Total            int      `json:"total"`
	Critical         int      `json:"critical"`
	Warning          int      `json:"warning"`
	Acked            int      `json:"acked"`
	Cleared          int      `json:"cleared"`
	PerDay           float64  `json:"perDay"`
	MeanAckMinutes   *float64 `json:"meanAckMinutes"`   // mean time to acknowledge, acknowledged alerts only
	MeanClearMinutes *float64 `json:"meanClearMinutes"` // mean time to clear, cleared alerts only
	TopSource        string   `json:"topSource,omitempty"`
	PeakDay          string   `json:"peakDay,omitempty"`
	PeakCount        int      `json:"peakCount"`
	DevicesAffected  int      `json:"devicesAffected"`
}

// AlertsReport is alert volume by day, source and device.
type AlertsReport struct {
	Range   Range         `json:"range"`
	Summary AlertsSummary `json:"summary"`
	Days    []AlertDay    `json:"days"`
	Sources []NameCount   `json:"sources"`
	Devices []AlertDevice `json:"devices"` // sorted by alerts desc
}

// MeanMinutes returns sum/n rounded, or nil when n is 0.
func MeanMinutes(sum float64, n int) *float64 {
	if n <= 0 {
		return nil
	}
	m := round1(sum / float64(n))
	return &m
}

// Alerts computes the alerts report.
func (s *Service) Alerts(ctx context.Context, p Params) (*AlertsReport, error) {
	sc, err := s.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	days, err := s.Store.AlertsByDay(ctx, sc.rangeOf())
	if err != nil {
		return nil, err
	}
	devs, err := s.Store.AlertsByDevice(ctx, sc.rangeOf())
	if err != nil {
		return nil, err
	}
	rep := &AlertsReport{Range: sc.rng, Days: []AlertDay{}, Sources: []NameCount{}, Devices: []AlertDevice{}}
	dayIdx := map[string]int{}
	for i, d := range sc.days {
		dayIdx[d] = i
		rep.Days = append(rep.Days, AlertDay{Day: d, BySource: map[string]int{}})
	}
	sources := map[string]int{}
	for _, c := range days {
		i, ok := dayIdx[c.Day]
		if !ok {
			continue
		}
		d := &rep.Days[i]
		d.Total += c.Count
		switch model.Severity(c.Severity) {
		case model.SeverityCritical:
			d.Critical += c.Count
		case model.SeverityWarning:
			d.Warning += c.Count
		}
		d.BySource[c.Source] += c.Count
		sources[c.Source] += c.Count
		if d.Total > rep.Summary.PeakCount {
			rep.Summary.PeakCount, rep.Summary.PeakDay = d.Total, d.Day
		}
	}
	for name, n := range sources {
		rep.Sources = append(rep.Sources, NameCount{Name: name, Count: n})
	}
	sort.SliceStable(rep.Sources, func(i, j int) bool {
		if rep.Sources[i].Count != rep.Sources[j].Count {
			return rep.Sources[i].Count > rep.Sources[j].Count
		}
		return rep.Sources[i].Name < rep.Sources[j].Name
	})
	if len(rep.Sources) > 0 {
		rep.Summary.TopSource = rep.Sources[0].Name
	}
	var ackSum, clearSum float64
	for _, st := range devs {
		rep.Summary.Total += st.Count
		rep.Summary.Critical += st.Critical
		rep.Summary.Warning += st.Warning
		rep.Summary.Acked += st.Acked
		rep.Summary.Cleared += st.Cleared
		ackSum += st.AckMinutesSum
		clearSum += st.ClearMinutesSum
		if st.DeviceID != "" {
			rep.Summary.DevicesAffected++
		}
		rep.Devices = append(rep.Devices, AlertDevice{DeviceID: st.DeviceID, Name: sc.name(st.DeviceID), Model: sc.field(st.DeviceID, func(d *model.Device) string { return d.Model }),
			Location: sc.field(st.DeviceID, func(d *model.Device) string { return d.Location }), Alerts: st.Count, Critical: st.Critical, Warning: st.Warning, Acked: st.Acked, Cleared: st.Cleared,
			MeanAckMinutes: MeanMinutes(st.AckMinutesSum, st.Acked), MeanClearMinutes: MeanMinutes(st.ClearMinutesSum, st.Cleared)})
	}
	rep.Summary.MeanAckMinutes = MeanMinutes(ackSum, rep.Summary.Acked)
	rep.Summary.MeanClearMinutes = MeanMinutes(clearSum, rep.Summary.Cleared)
	rep.Summary.PerDay = round1(float64(rep.Summary.Total) / float64(len(sc.days)))
	sort.SliceStable(rep.Devices, func(i, j int) bool {
		if rep.Devices[i].Alerts != rep.Devices[j].Alerts {
			return rep.Devices[i].Alerts > rep.Devices[j].Alerts
		}
		return rep.Devices[i].Name < rep.Devices[j].Name
	})
	return rep, nil
}

// Tables implements CSV export: devices (default), days, sources.
func (r *AlertsReport) Tables() []Table {
	dev := Table{Name: "devices", Columns: []string{"deviceId", "name", "model", "location", "alerts", "critical", "warning", "acked", "cleared", "meanAckMinutes", "meanClearMinutes"}}
	for _, d := range r.Devices {
		dev.Rows = append(dev.Rows, []string{d.DeviceID, d.Name, d.Model, d.Location, fmtI(d.Alerts), fmtI(d.Critical), fmtI(d.Warning), fmtI(d.Acked), fmtI(d.Cleared), fmtP(d.MeanAckMinutes), fmtP(d.MeanClearMinutes)})
	}
	days := Table{Name: "days", Columns: []string{"day", "total", "critical", "warning"}}
	for _, d := range r.Days {
		days.Rows = append(days.Rows, []string{d.Day, fmtI(d.Total), fmtI(d.Critical), fmtI(d.Warning)})
	}
	src := Table{Name: "sources", Columns: []string{"source", "alerts"}}
	for _, s := range r.Sources {
		src.Rows = append(src.Rows, []string{s.Name, fmtI(s.Count)})
	}
	return []Table{dev, days, src}
}
