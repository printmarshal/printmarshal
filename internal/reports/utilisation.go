package reports

import (
	"context"
	"sort"

	"printmarshal/internal/store"
)

// UtilDay is the fleet (in scope) page volume on one day.
type UtilDay struct {
	Day     string `json:"day"`
	Pages   int64  `json:"pages"`
	Mono    int64  `json:"mono"`
	Color   int64  `json:"color"`
	Devices int    `json:"devices"` // devices with counter data that day
}

// UtilDevice is one device's page volume over the range.
type UtilDevice struct {
	DeviceID   string  `json:"deviceId"`
	Name       string  `json:"name"`
	Model      string  `json:"model"`
	Location   string  `json:"location"`
	Pages      int64   `json:"pages"`
	Mono       int64   `json:"mono"`
	Color      int64   `json:"color"`
	PerDay     float64 `json:"perDay"`     // pages / days in range
	ActiveDays int     `json:"activeDays"` // days with at least one page
	DataDays   int     `json:"dataDays"`   // days with a counter sample
	Resets     int     `json:"resets"`
	Idle       bool    `json:"idle"` // counter data present, zero pages
}

// UtilGroup is a group's page volume over the range.
type UtilGroup struct {
	GroupID string  `json:"groupId"`
	Name    string  `json:"name"`
	Devices int     `json:"devices"`
	Pages   int64   `json:"pages"`
	PerDay  float64 `json:"perDay"`
}

// UtilSummary is the utilisation KPI set.
type UtilSummary struct {
	Pages            int64   `json:"pages"`
	Mono             int64   `json:"mono"`
	Color            int64   `json:"color"`
	HasSplit         bool    `json:"hasSplit"`
	PerDay           float64 `json:"perDay"`          // fleet pages per day
	PerDevicePerDay  float64 `json:"perDevicePerDay"` // over devices with data
	DevicesWithData  int     `json:"devicesWithData"`
	DevicesNoData    int     `json:"devicesNoData"`
	IdleDevices      int     `json:"idleDevices"`
	Resets           int     `json:"resets"`
	PeakDay          string  `json:"peakDay,omitempty"`
	PeakPages        int64   `json:"peakPages"`
	BusiestDevice    string  `json:"busiestDevice,omitempty"`
	BusiestDeviceID  string  `json:"busiestDeviceId,omitempty"`
	BusiestDevicePct float64 `json:"busiestDevicePct"`
}

// UtilisationReport is pages per day per device/group (spec §8.3 trends).
type UtilisationReport struct {
	Range   Range        `json:"range"`
	Summary UtilSummary  `json:"summary"`
	Days    []UtilDay    `json:"days"`
	Devices []UtilDevice `json:"devices"` // sorted by pages desc
	Top     []UtilDevice `json:"top"`
	Groups  []UtilGroup  `json:"groups"`
}

// counterSeries is one device's daily samples of a counter metric.
type counterSeries struct {
	baseline *float64
	days     map[string]store.DailyAgg
}

// DailyDeltas turns a counter series into pages per day. For each day the
// delta is last-sample(day) - last-sample(previous observed day), or the
// baseline (last sample before the range) for the first day. A drop means the
// counter was reset: that day counts 0 and the new value becomes the base.
// Without any baseline the first observed day uses the growth within the day.
// Returns pages per day (only days with data) and the number of resets.
func DailyDeltas(days []string, baseline *float64, byDay map[string]store.DailyAgg) (map[string]int64, int) {
	out := map[string]int64{}
	prev := baseline
	resets := 0
	for _, day := range days {
		a, ok := byDay[day]
		if !ok {
			continue
		}
		var delta float64
		switch {
		case prev == nil:
			delta = a.Last - a.First
			if delta < 0 {
				delta, resets = 0, resets+1
			}
		case a.Last < *prev:
			delta, resets = 0, resets+1
		default:
			delta = a.Last - *prev
		}
		out[day] = int64(delta + 0.5)
		v := a.Last
		prev = &v
	}
	return out, resets
}

func (s *Service) counterSeries(ctx context.Context, metric string, sc *scope) (map[string]*counterSeries, error) {
	daily, err := s.Store.DailyTelemetry(ctx, metric, sc.rangeOf())
	if err != nil {
		return nil, err
	}
	base, err := s.Store.LastBefore(ctx, metric, sc.rng.From, sc.ids)
	if err != nil {
		return nil, err
	}
	out := map[string]*counterSeries{}
	get := func(id string) *counterSeries {
		c, ok := out[id]
		if !ok {
			c = &counterSeries{days: map[string]store.DailyAgg{}}
			out[id] = c
		}
		return c
	}
	for _, b := range base {
		v := b.Value
		get(b.DeviceID).baseline = &v
	}
	for _, a := range daily {
		get(a.DeviceID).days[a.Day] = a
	}
	return out, nil
}

// Utilisation computes the utilisation report.
func (s *Service) Utilisation(ctx context.Context, p Params) (*UtilisationReport, error) {
	sc, err := s.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	topN := p.TopN
	if topN <= 0 {
		topN = 10
	}
	total, err := s.counterSeries(ctx, "counter.total", sc)
	if err != nil {
		return nil, err
	}
	mono, err := s.counterSeries(ctx, "counter.mono", sc)
	if err != nil {
		return nil, err
	}
	color, err := s.counterSeries(ctx, "counter.color", sc)
	if err != nil {
		return nil, err
	}
	rep := &UtilisationReport{Range: sc.rng, Days: []UtilDay{}, Devices: []UtilDevice{}, Top: []UtilDevice{}, Groups: []UtilGroup{}}
	dayIdx := map[string]int{}
	for i, d := range sc.days {
		dayIdx[d] = i
		rep.Days = append(rep.Days, UtilDay{Day: d})
	}
	perDevice := map[string]int64{}
	for _, d := range sc.devices {
		cs, ok := total[d.ID]
		if !ok {
			rep.Summary.DevicesNoData++
			continue
		}
		pages, resets := DailyDeltas(sc.days, cs.baseline, cs.days)
		row := UtilDevice{DeviceID: d.ID, Name: d.Name(), Model: d.Model, Location: d.Location, Resets: resets, DataDays: len(cs.days)}
		for day, n := range pages {
			row.Pages += n
			if n > 0 {
				row.ActiveDays++
			}
			rep.Days[dayIdx[day]].Pages += n
			rep.Days[dayIdx[day]].Devices++
		}
		if m, ok := mono[d.ID]; ok {
			pm, _ := DailyDeltas(sc.days, m.baseline, m.days)
			for day, n := range pm {
				row.Mono += n
				rep.Days[dayIdx[day]].Mono += n
			}
			rep.Summary.HasSplit = true
		}
		if c, ok := color[d.ID]; ok {
			pc, _ := DailyDeltas(sc.days, c.baseline, c.days)
			for day, n := range pc {
				row.Color += n
				rep.Days[dayIdx[day]].Color += n
			}
			rep.Summary.HasSplit = true
		}
		row.PerDay = round1(float64(row.Pages) / float64(len(sc.days)))
		row.Idle = row.Pages == 0
		if row.Idle {
			rep.Summary.IdleDevices++
		}
		perDevice[d.ID] = row.Pages
		rep.Summary.Pages += row.Pages
		rep.Summary.Mono += row.Mono
		rep.Summary.Color += row.Color
		rep.Summary.Resets += resets
		rep.Summary.DevicesWithData++
		rep.Devices = append(rep.Devices, row)
	}
	sort.SliceStable(rep.Devices, func(i, j int) bool {
		if rep.Devices[i].Pages != rep.Devices[j].Pages {
			return rep.Devices[i].Pages > rep.Devices[j].Pages
		}
		return rep.Devices[i].Name < rep.Devices[j].Name
	})
	if len(rep.Devices) > topN {
		rep.Top = rep.Devices[:topN]
	} else {
		rep.Top = rep.Devices
	}
	days := float64(len(sc.days))
	rep.Summary.PerDay = round1(float64(rep.Summary.Pages) / days)
	if rep.Summary.DevicesWithData > 0 {
		rep.Summary.PerDevicePerDay = round1(float64(rep.Summary.Pages) / days / float64(rep.Summary.DevicesWithData))
	}
	for _, d := range rep.Days {
		if d.Pages > rep.Summary.PeakPages {
			rep.Summary.PeakPages, rep.Summary.PeakDay = d.Pages, d.Day
		}
	}
	if len(rep.Devices) > 0 && rep.Summary.Pages > 0 {
		b := rep.Devices[0]
		rep.Summary.BusiestDevice, rep.Summary.BusiestDeviceID = b.Name, b.DeviceID
		rep.Summary.BusiestDevicePct = round1(float64(b.Pages) * 100 / float64(rep.Summary.Pages))
	}
	for _, g := range sc.groups {
		ids := sc.members[g.ID]
		if len(ids) == 0 {
			continue
		}
		row := UtilGroup{GroupID: g.ID, Name: g.Name, Devices: len(ids)}
		for _, id := range ids {
			row.Pages += perDevice[id]
		}
		row.PerDay = round1(float64(row.Pages) / days)
		rep.Groups = append(rep.Groups, row)
	}
	sort.SliceStable(rep.Groups, func(i, j int) bool { return rep.Groups[i].Pages > rep.Groups[j].Pages })
	return rep, nil
}

// Tables implements CSV export: devices (default), days, groups.
func (r *UtilisationReport) Tables() []Table {
	dev := Table{Name: "devices", Columns: []string{"deviceId", "name", "model", "location", "pages", "mono", "color", "perDay", "activeDays", "dataDays", "resets", "idle"}}
	for _, d := range r.Devices {
		dev.Rows = append(dev.Rows, []string{d.DeviceID, d.Name, d.Model, d.Location, fmtI64(d.Pages), fmtI64(d.Mono), fmtI64(d.Color), fmtF(d.PerDay), fmtI(d.ActiveDays), fmtI(d.DataDays), fmtI(d.Resets), fmtB(d.Idle)})
	}
	days := Table{Name: "days", Columns: []string{"day", "pages", "mono", "color", "devices"}}
	for _, d := range r.Days {
		days.Rows = append(days.Rows, []string{d.Day, fmtI64(d.Pages), fmtI64(d.Mono), fmtI64(d.Color), fmtI(d.Devices)})
	}
	groups := Table{Name: "groups", Columns: []string{"groupId", "name", "devices", "pages", "perDay"}}
	for _, g := range r.Groups {
		groups.Rows = append(groups.Rows, []string{g.GroupID, g.Name, fmtI(g.Devices), fmtI64(g.Pages), fmtF(g.PerDay)})
	}
	return []Table{dev, days, groups}
}

func fmtB(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
