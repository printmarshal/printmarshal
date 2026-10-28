package firmware

import (
	"context"
	"sort"
	"strings"

	"printmarshal/internal/firmware/fwversion"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// InventoryFilter narrows the firmware inventory.
type InventoryFilter struct {
	Make    string
	Model   string
	GroupID string
	Q       string
}

// InventoryEntry is one make/model/version cell of the fleet.
type InventoryEntry struct {
	Make            string   `json:"make"`
	Model           string   `json:"model"`
	Version         string   `json:"version"` // "" = unknown
	Count           int      `json:"count"`
	Latest          string   `json:"latest,omitempty"` // latest registered package version for this make/model
	LatestPackageID string   `json:"latestPackageId,omitempty"`
	Outdated        bool     `json:"outdated"`
	DeviceIDs       []string `json:"deviceIds"`
}

// Inventory is the fleet-wide firmware picture.
type Inventory struct {
	Entries  []InventoryEntry `json:"entries"`
	Devices  int              `json:"devices"`
	Outdated int              `json:"outdated"`
	Unknown  int              `json:"unknown"` // devices that report no firmware version
}

// OutdatedDevice is a device older than a registered compatible package.
type OutdatedDevice struct {
	DeviceID  string `json:"deviceId"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Make      string `json:"make"`
	Model     string `json:"model"`
	Installed string `json:"installed"`
	Latest    string `json:"latest"`
	PackageID string `json:"packageId"`
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

func (s *Service) devicesFor(ctx context.Context, f InventoryFilter) ([]*model.Device, error) {
	df := store.DeviceFilter{Make: f.Make, Model: f.Model, Q: f.Q}
	if f.GroupID != "" {
		g, err := s.Store.GetGroup(ctx, f.GroupID)
		if err != nil {
			return nil, err
		}
		if g.Kind == "rule" {
			all, err := s.Store.ListDevices(ctx, df)
			if err != nil {
				return nil, err
			}
			out := []*model.Device{}
			for _, d := range all {
				if store.MatchRule(g.Rule, d) {
					out = append(out, d)
				}
			}
			return out, nil
		}
		df.GroupIDs = []string{f.GroupID}
	}
	return s.Store.ListDevices(ctx, df)
}

// Inventory groups devices by make, model and installed version and marks the
// cells older than the latest registered compatible package.
func (s *Service) Inventory(ctx context.Context, f InventoryFilter) (*Inventory, error) {
	devs, err := s.devicesFor(ctx, f)
	if err != nil {
		return nil, err
	}
	pkgs, err := s.Store.ListFirmwarePackages(ctx)
	if err != nil {
		return nil, err
	}
	inv := &Inventory{Entries: []InventoryEntry{}}
	byKey := map[string]*InventoryEntry{}
	for _, d := range devs {
		inv.Devices++
		fw := strings.TrimSpace(d.Firmware)
		if fw == "" {
			inv.Unknown++
		}
		key := strings.ToLower(d.Make) + "|" + strings.ToLower(d.Model) + "|" + fwversion.Normalize(fw)
		e, ok := byKey[key]
		if !ok {
			e = &InventoryEntry{Make: d.Make, Model: d.Model, Version: fw, DeviceIDs: []string{}}
			if p := latestFor(pkgs, d); p != nil {
				e.Latest, e.LatestPackageID = p.Version, p.ID
				e.Outdated = fw != "" && fwversion.Less(fw, p.Version)
			}
			byKey[key] = e
		}
		e.Count++
		e.DeviceIDs = append(e.DeviceIDs, d.ID)
		if e.Outdated {
			inv.Outdated++
		}
	}
	for _, e := range byKey {
		inv.Entries = append(inv.Entries, *e)
	}
	sort.Slice(inv.Entries, func(i, j int) bool {
		a, b := inv.Entries[i], inv.Entries[j]
		if a.Make != b.Make {
			return a.Make < b.Make
		}
		if a.Model != b.Model {
			return a.Model < b.Model
		}
		return fwversion.Compare(a.Version, b.Version) > 0
	})
	return inv, nil
}

// Outdated lists devices whose installed version is older than a registered
// package that applies to them. Devices with an unknown version are not listed.
func (s *Service) Outdated(ctx context.Context) ([]OutdatedDevice, error) {
	devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{})
	if err != nil {
		return nil, err
	}
	pkgs, err := s.Store.ListFirmwarePackages(ctx)
	if err != nil {
		return nil, err
	}
	out := []OutdatedDevice{}
	for _, d := range devs {
		if strings.TrimSpace(d.Firmware) == "" {
			continue
		}
		p := latestFor(pkgs, d)
		if p == nil || !fwversion.Less(d.Firmware, p.Version) {
			continue
		}
		out = append(out, OutdatedDevice{DeviceID: d.ID, Name: d.Name(), Address: d.Address, Make: d.Make, Model: d.Model,
			Installed: d.Firmware, Latest: p.Version, PackageID: p.ID})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
