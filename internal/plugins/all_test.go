package plugins

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

const fixtureDir = "../../testdata/fixtures"

// Contract test (spec §14): every plugin is an engine.Driver, declares a
// consistent capability set, and implements the typed interface for every
// capability it declares.
func TestContract(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range New(vendorkit.Options{}) {
		var drv engine.Driver = p
		name := drv.Name()
		if name == "" || seen[name] || name == "snmp" || name == "ipp" {
			t.Errorf("plugin name %q must be unique and not a standards driver", name)
		}
		seen[name] = true
		if _, ok := p.(engine.Identifier); !ok {
			t.Errorf("%s: must implement engine.Identifier", name)
		}
		d := p.Descriptor()
		if d.Name != name || d.Vendor == "" || len(d.Enterprises) == 0 {
			t.Errorf("%s: descriptor incomplete: %+v", name, d)
		}
		declared := map[model.Capability]bool{}
		for _, c := range drv.Capabilities() {
			declared[c] = true
			var ok bool
			switch c {
			case model.CapIdentify:
				_, ok = p.(engine.Identifier)
			case model.CapReadStatus:
				_, ok = p.(engine.StatusReader)
			case model.CapReadSupplies:
				_, ok = p.(engine.SupplyReader)
			case model.CapReadTrays:
				_, ok = p.(engine.TrayReader)
			case model.CapReadCounters:
				_, ok = p.(engine.CounterReader)
			case model.CapFirmwareRead:
				_, ok = p.(engine.FirmwareReader)
			case model.CapReadConfig:
				_, ok = p.(engine.ConfigReader)
			case model.CapWriteConfig:
				_, ok = p.(engine.ConfigWriter)
			case model.CapReadSecurity:
				_, ok = p.(engine.SecurityReader)
			case model.CapWriteSecurity:
				_, ok = p.(engine.SecurityWriter)
			case model.CapManageCerts:
				_, ok = p.(engine.CertManager)
			case model.CapFirmwareUpdate:
				_, ok = p.(engine.FirmwareUpdater)
			default:
				t.Errorf("%s: declares capability %q this contract test does not know", name, c)
				continue
			}
			if !ok {
				t.Errorf("%s: declares %s without implementing its interface", name, c)
			}
			if drv.Priority(c) <= 0 {
				t.Errorf("%s: priority for %s must be positive", name, c)
			}
		}
		// The evidence matrix must agree with the declared set: everything
		// declared has a documented/observed/unverified row and nothing
		// marked unsupported is declared.
		matrix := map[model.Capability]vendorkit.Evidence{}
		for _, e := range d.Capabilities {
			matrix[e.Capability] = e.Status
		}
		for c := range declared {
			if st, ok := matrix[c]; !ok || st == vendorkit.EvidenceUnsupported {
				t.Errorf("%s: declares %s but the evidence matrix says %q", name, c, st)
			}
		}
		for c, st := range matrix {
			if st != vendorkit.EvidenceUnsupported && !declared[c] {
				t.Errorf("%s: evidence matrix lists %s as %s but it is not declared", name, c, st)
			}
		}
		// Supports never does I/O and follows the profile rule.
		dev := &model.Device{Capabilities: map[model.Capability]string{model.CapIdentify: "snmp"}}
		if drv.Supports(dev, model.CapIdentify) {
			t.Errorf("%s: must not claim a capability the profile assigned to another driver", name)
		}
		dev = &model.Device{IdentifyMethods: []string{name}}
		if !drv.Supports(dev, model.CapIdentify) {
			t.Errorf("%s: must serve a device it identified when nobody else is assigned", name)
		}
	}
	if len(seen) != 13 {
		t.Errorf("expected 13 plugins, got %d", len(seen))
	}
}

// Negative test: each plugin accepts only its own vendor's fixtures and
// rejects every other fixture with ErrNotVendor (never a transport error).
func TestFixtureMatrix(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(fixtureDir, "*.snmp"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	// Fixtures that carry no sysObjectID give no SNMP vendor evidence at all;
	// every plugin must reject them (hp-m283fdw.snmp is the first M283fdw
	// recording, which the recorder captured without the system group).
	noEvidence := map[string]bool{"hp-m283fdw.snmp": true, "generic-laser.snmp": true, "color-mfp.snmp": true}
	for _, f := range files {
		base := filepath.Base(f)
		fx, err := snmp.LoadFixture(f)
		if err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		owner := strings.SplitN(base, "-", 2)[0]
		if noEvidence[base] {
			owner = ""
		}
		accepted := map[string]bool{}
		for _, p := range New(vendorkit.Options{SNMP: vendorkit.FixtureDialer(fx)}) {
			id, err := p.(engine.Identifier).Identify(context.Background(), "10.0.0.1", model.Creds{})
			switch {
			case err == nil:
				accepted[p.Name()] = true
				if id.Make == "" || len(id.Capabilities) == 0 {
					t.Errorf("%s/%s: identity incomplete: %+v", base, p.Name(), id)
				}
			case errors.Is(err, vendorkit.ErrNotVendor):
			default:
				t.Errorf("%s/%s: unexpected error %v", base, p.Name(), err)
			}
		}
		var got []string
		for n := range accepted {
			got = append(got, n)
		}
		sort.Strings(got)
		switch {
		case owner == "" && len(got) != 0:
			t.Errorf("%s: no vendor evidence, but accepted by %v", base, got)
		case owner != "" && (len(got) != 1 || got[0] != owner):
			t.Errorf("%s: want exactly [%s], got %v", base, owner, got)
		}
	}
}

func TestRegisterAllAndList(t *testing.T) {
	reg := engine.NewRegistry()
	RegisterAllWith(reg, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})), vendorkit.Options{})
	if len(reg.Drivers()) != 13 {
		t.Errorf("registered %d drivers", len(reg.Drivers()))
	}
	if reg.Named("ricoh") == nil || reg.Named("hp") == nil {
		t.Error("ricoh / hp not registered")
	}
	list := List()
	if len(list) != 13 || list[0].Name != "ricoh" || list[1].Name != "hp" {
		t.Errorf("list = %+v", list)
	}
	for _, d := range list {
		if len(d.Capabilities) == 0 || d.Management.Read == "" {
			t.Errorf("%s: descriptor lacks evidence or management paths", d.Name)
		}
		for _, e := range d.Capabilities {
			if e.Status == vendorkit.EvidenceUnverified && e.Note == "" {
				t.Errorf("%s: unverified capability %s needs a note", d.Name, e.Capability)
			}
		}
	}
}

// The HP recording identifies through the registry exactly like production:
// the plugin wins identify (priority 60) and the standards profile falls back
// to snmp for what the plugin does not serve.
func TestEngineIdentifyMergesPluginIdentity(t *testing.T) {
	fx, err := snmp.LoadFixture(filepath.Join(fixtureDir, "hp-m283fdw-enterprise.snmp"))
	if err != nil {
		t.Fatal(err)
	}
	reg := engine.NewRegistry()
	RegisterAllWith(reg, nil, vendorkit.Options{SNMP: vendorkit.FixtureDialer(fx)})
	eng := engine.New(reg, slog.Default(), 4)
	id, caps, err := eng.Identify(context.Background(), "198.51.100.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.Make != "HP" || id.Serial != "VNB0TEST01" || id.Firmware != "20260622" || !strings.Contains(id.Method, "hp") {
		t.Errorf("identity = %+v", id)
	}
	if caps[model.CapReadCounters] != "hp" || caps[model.CapReadConfig] != "hp" || caps[model.CapIdentify] != "hp" {
		t.Errorf("profile = %v", caps)
	}
	if _, ok := caps[model.CapReadSupplies]; ok {
		t.Errorf("no plugin serves read_supplies; profile = %v", caps)
	}
}
