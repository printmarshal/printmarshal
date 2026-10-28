package reports

import (
	"context"
	"sort"
	"strings"

	"printmarshal/internal/firmware/fwversion"
	"printmarshal/internal/model"
)

// Firmware status values.
const (
	FirmwareCurrent      = "current"      // matches or exceeds the latest registered package
	FirmwareOutdated     = "outdated"     // older than a registered compatible package
	FirmwareUnregistered = "unregistered" // no compatible package registered
	FirmwareUnknown      = "unknown"      // the device reports no version
)

// VersionCount is one installed version within a model.
type VersionCount struct {
	Version  string `json:"version"` // "" = unknown
	Count    int    `json:"count"`
	Outdated bool   `json:"outdated"`
}

// FirmwareModel is the version distribution of one make/model.
type FirmwareModel struct {
	Make     string         `json:"make"`
	Model    string         `json:"model"`
	Devices  int            `json:"devices"`
	Versions []VersionCount `json:"versions"` // sorted newest first
	Latest   string         `json:"latest,omitempty"`
	Outdated int            `json:"outdated"`
	Unknown  int            `json:"unknown"`
}

// FirmwareDevice is one device's firmware status.
type FirmwareDevice struct {
	DeviceID  string `json:"deviceId"`
	Name      string `json:"name"`
	Make      string `json:"make"`
	Model     string `json:"model"`
	Location  string `json:"location"`
	Installed string `json:"installed"`
	Latest    string `json:"latest,omitempty"`
	Status    string `json:"status"`
}

// FirmwareSummary is the firmware KPI set.
type FirmwareSummary struct {
	Devices      int `json:"devices"`
	Versions     int `json:"versions"` // distinct installed versions
	Models       int `json:"models"`
	Current      int `json:"current"`
	Outdated     int `json:"outdated"`
	Unregistered int `json:"unregistered"`
	Unknown      int `json:"unknown"`
	Packages     int `json:"packages"`
}

// FirmwareReport is the version distribution per model and outdated counts. It
// is a point-in-time view of the device records; the range only scopes devices.
type FirmwareReport struct {
	Range   Range            `json:"range"`
	Summary FirmwareSummary  `json:"summary"`
	Models  []FirmwareModel  `json:"models"`
	Devices []FirmwareDevice `json:"devices"` // outdated first, then by name
}

// latestFor returns the newest registered package that applies to dev.
func latestFor(pkgs []*model.FirmwarePackage, dev *model.Device) *model.FirmwarePackage {
	var best *model.FirmwarePackage
	for _, p := range pkgs {
		if !fwversion.Applies(p, dev) {
			continue
		}
		if best == nil || fwversion.Less(best.Version, p.Version) {
			best = p
		}
	}
	return best
}

// FirmwareStatus classifies an installed version against the latest package.
func FirmwareStatus(installed, latest string) string {
	switch {
	case strings.TrimSpace(installed) == "":
		return FirmwareUnknown
	case latest == "":
		return FirmwareUnregistered
	case fwversion.Less(installed, latest):
		return FirmwareOutdated
	}
	return FirmwareCurrent
}

// Firmware computes the firmware report.
func (s *Service) Firmware(ctx context.Context, p Params) (*FirmwareReport, error) {
	sc, err := s.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	pkgs, err := s.Store.ListFirmwarePackages(ctx)
	if err != nil {
		return nil, err
	}
	rep := &FirmwareReport{Range: sc.rng, Models: []FirmwareModel{}, Devices: []FirmwareDevice{}}
	rep.Summary.Packages = len(pkgs)
	type mkey struct{ mk, md string }
	models := map[mkey]*FirmwareModel{}
	versions := map[string]bool{}
	for _, d := range sc.devices {
		latest := ""
		if pk := latestFor(pkgs, d); pk != nil {
			latest = pk.Version
		}
		status := FirmwareStatus(d.Firmware, latest)
		rep.Devices = append(rep.Devices, FirmwareDevice{DeviceID: d.ID, Name: d.Name(), Make: d.Make, Model: d.Model, Location: d.Location, Installed: d.Firmware, Latest: latest, Status: status})
		rep.Summary.Devices++
		switch status {
		case FirmwareCurrent:
			rep.Summary.Current++
		case FirmwareOutdated:
			rep.Summary.Outdated++
		case FirmwareUnregistered:
			rep.Summary.Unregistered++
		default:
			rep.Summary.Unknown++
		}
		k := mkey{d.Make, d.Model}
		m, ok := models[k]
		if !ok {
			m = &FirmwareModel{Make: d.Make, Model: d.Model, Versions: []VersionCount{}}
			models[k] = m
		}
		m.Devices++
		if latest != "" {
			m.Latest = latest
		}
		fw := strings.TrimSpace(d.Firmware)
		if fw == "" {
			m.Unknown++
		} else {
			versions[fw] = true
		}
		if status == FirmwareOutdated {
			m.Outdated++
		}
		found := false
		for i := range m.Versions {
			if m.Versions[i].Version == fw {
				m.Versions[i].Count++
				m.Versions[i].Outdated = m.Versions[i].Outdated || status == FirmwareOutdated
				found = true
			}
		}
		if !found {
			m.Versions = append(m.Versions, VersionCount{Version: fw, Count: 1, Outdated: status == FirmwareOutdated})
		}
	}
	rep.Summary.Versions = len(versions)
	rep.Summary.Models = len(models)
	for _, m := range models {
		sort.SliceStable(m.Versions, func(i, j int) bool { return fwversion.Less(m.Versions[j].Version, m.Versions[i].Version) })
		rep.Models = append(rep.Models, *m)
	}
	sort.SliceStable(rep.Models, func(i, j int) bool {
		a, b := rep.Models[i], rep.Models[j]
		if a.Outdated != b.Outdated {
			return a.Outdated > b.Outdated
		}
		if a.Devices != b.Devices {
			return a.Devices > b.Devices
		}
		return a.Make+a.Model < b.Make+b.Model
	})
	rank := map[string]int{FirmwareOutdated: 0, FirmwareUnknown: 1, FirmwareUnregistered: 2, FirmwareCurrent: 3}
	sort.SliceStable(rep.Devices, func(i, j int) bool {
		a, b := rep.Devices[i], rep.Devices[j]
		if rank[a.Status] != rank[b.Status] {
			return rank[a.Status] < rank[b.Status]
		}
		return a.Name < b.Name
	})
	return rep, nil
}

// Tables implements CSV export: devices (default), models.
func (r *FirmwareReport) Tables() []Table {
	dev := Table{Name: "devices", Columns: []string{"deviceId", "name", "make", "model", "location", "installed", "latest", "status"}}
	for _, d := range r.Devices {
		dev.Rows = append(dev.Rows, []string{d.DeviceID, d.Name, d.Make, d.Model, d.Location, d.Installed, d.Latest, d.Status})
	}
	models := Table{Name: "models", Columns: []string{"make", "model", "version", "devices", "outdated", "latest"}}
	for _, m := range r.Models {
		for _, v := range m.Versions {
			models.Rows = append(models.Rows, []string{m.Make, m.Model, v.Version, fmtI(v.Count), fmtB(v.Outdated), m.Latest})
		}
	}
	return []Table{dev, models}
}
