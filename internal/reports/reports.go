// Package reports computes fleet reports (spec §8.2 CSV export, §8.3 trends,
// §8.5 reporting) from aggregate store queries. Every report takes a time
// range and an optional group or device scope; the maths (counter deltas,
// consumption rates, projections, availability) lives here so it is unit
// testable without a database.
package reports

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// ErrInvalid marks a bad parameter (HTTP 400).
var ErrInvalid = errors.New("invalid report parameters")

// MaxRangeDays caps a report range.
const MaxRangeDays = 400

// Store is what the reports need from persistence: the aggregate surface plus
// device, group and firmware-package reads. *store.SQLite satisfies it.
type Store interface {
	store.ReportStore
	ListDevices(ctx context.Context, f store.DeviceFilter) ([]*model.Device, error)
	GetDevice(ctx context.Context, id string) (*model.Device, error)
	GetGroup(ctx context.Context, id string) (*model.Group, error)
	ListGroups(ctx context.Context) ([]*model.Group, error)
	ListFirmwarePackages(ctx context.Context) ([]*model.FirmwarePackage, error)
}

// Params select the range and scope of a report.
type Params struct {
	From   time.Time
	To     time.Time // exclusive
	Group  string    // group id (static or rule), optional
	Device string    // device id, optional
	TopN   int       // top-N lists; default 10
}

// Range describes the resolved range and scope echoed in every report.
type Range struct {
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Days    int       `json:"days"`
	Group   string    `json:"group,omitempty"`
	Device  string    `json:"device,omitempty"`
	Devices int       `json:"devices"` // devices in scope
}

// Table is a flat view of one report section for CSV export.
type Table struct {
	Name    string
	Columns []string
	Rows    [][]string
}

// Service computes reports.
type Service struct {
	Store Store
	Now   func() time.Time
}

// New returns a report service over st.
func New(st Store) *Service {
	return &Service{Store: st, Now: func() time.Time { return time.Now().UTC() }}
}

// scope is the resolved device set of a report.
type scope struct {
	rng     Range
	days    []string
	devices []*model.Device
	byID    map[string]*model.Device
	ids     []string // nil when unscoped (every device)
	groups  []*model.Group
	members map[string][]string // group id -> device ids within scope
}

func (s *Service) resolve(ctx context.Context, p Params) (*scope, error) {
	now := s.Now()
	if p.To.IsZero() {
		p.To = now
	}
	if p.From.IsZero() {
		p.From = p.To.Add(-30 * 24 * time.Hour)
	}
	p.From, p.To = p.From.UTC(), p.To.UTC()
	if !p.To.After(p.From) {
		return nil, fmt.Errorf("%w: to must be after from", ErrInvalid)
	}
	if p.To.Sub(p.From) > MaxRangeDays*24*time.Hour {
		return nil, fmt.Errorf("%w: range is longer than %d days", ErrInvalid, MaxRangeDays)
	}
	if p.Group != "" && p.Device != "" {
		return nil, fmt.Errorf("%w: give group or device, not both", ErrInvalid)
	}
	sc := &scope{rng: Range{From: p.From, To: p.To, Group: p.Group, Device: p.Device}, byID: map[string]*model.Device{}, members: map[string][]string{}}
	sc.days = dayList(p.From, p.To)
	sc.rng.Days = len(sc.days)

	all, err := s.Store.ListDevices(ctx, store.DeviceFilter{IncludeDecommissioned: true})
	if err != nil {
		return nil, err
	}
	groups, err := s.Store.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	sc.groups = groups
	switch {
	case p.Device != "":
		d, err := s.Store.GetDevice(ctx, p.Device)
		if err != nil {
			return nil, err
		}
		sc.devices = []*model.Device{d}
		sc.ids = []string{d.ID}
	case p.Group != "":
		g, err := s.Store.GetGroup(ctx, p.Group)
		if err != nil {
			return nil, err
		}
		sc.ids = []string{}
		for _, d := range all {
			if inGroup(g, d) {
				sc.devices = append(sc.devices, d)
				sc.ids = append(sc.ids, d.ID)
			}
		}
	default:
		sc.devices = all
	}
	for _, d := range sc.devices {
		sc.byID[d.ID] = d
	}
	for _, g := range groups {
		for _, d := range sc.devices {
			if inGroup(g, d) {
				sc.members[g.ID] = append(sc.members[g.ID], d.ID)
			}
		}
	}
	sc.rng.Devices = len(sc.devices)
	return sc, nil
}

func (sc *scope) rangeOf() store.ReportRange {
	return store.ReportRange{From: sc.rng.From, To: sc.rng.To, DeviceIDs: sc.ids}
}

// inGroup reports membership in a static or rule-based group.
func inGroup(g *model.Group, d *model.Device) bool {
	if g.Kind == "rule" {
		return store.MatchRule(g.Rule, d)
	}
	for _, id := range g.MemberIDs {
		if id == d.ID {
			return true
		}
	}
	return false
}

// dayList enumerates UTC days from from (inclusive) to to (exclusive).
func dayList(from, to time.Time) []string {
	var out []string
	last := to.Add(-time.Nanosecond)
	for d := from.UTC().Truncate(24 * time.Hour); !d.After(last); d = d.Add(24 * time.Hour) {
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

func (sc *scope) name(id string) string {
	if d, ok := sc.byID[id]; ok {
		return d.Name()
	}
	if id == "" {
		return "(stock)"
	}
	return id
}

func (sc *scope) field(id string, f func(*model.Device) string) string {
	if d, ok := sc.byID[id]; ok {
		return f(d)
	}
	return ""
}

// splitLabel splits a supply/tray label "index:description".
func splitLabel(label string) (int, string) {
	idx, desc, _ := strings.Cut(label, ":")
	n := 0
	fmt.Sscanf(idx, "%d", &n)
	return n, desc
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func fmtF(f float64) string { return fmt.Sprintf("%.1f", f) }

func fmtP(f *float64) string {
	if f == nil {
		return ""
	}
	return fmtF(*f)
}

func fmtI(n int) string { return fmt.Sprintf("%d", n) }

func fmtI64(n int64) string { return fmt.Sprintf("%d", n) }
