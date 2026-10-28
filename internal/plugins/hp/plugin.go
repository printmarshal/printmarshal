package hp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "hp"

// Enterprise numbers claimed by this plugin.
const (
	EnterpriseHP      = 11  // Hewlett-Packard
	EnterpriseSamsung = 236 // Samsung Electronics: legacy SyncThru printers now owned by HP [M06]
)

// Plugin is the HP vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	p := &Plugin{}
	p.Base = vendorkit.New(vendorkit.Spec{
		Name:        Name,
		Vendor:      "HP",
		Enterprises: []int{EnterpriseHP, EnterpriseSamsung},
		Badges: []vendorkit.Badge{
			{Name: "Hewlett-Packard", Match: []string{"Hewlett-Packard", "Hewlett Packard"}},
			{Name: "Samsung", Match: []string{"Samsung"}},
		},
		IPPManufacturers: []string{"HP", "Hewlett-Packard", "Hewlett Packard"},
		Capabilities: []model.Capability{
			model.CapIdentify, model.CapReadStatus, model.CapReadCounters, model.CapFirmwareRead,
			model.CapReadConfig, model.CapReadSecurity,
		},
		Priorities: map[model.Capability]int{
			model.CapIdentify: 60, model.CapReadStatus: 60, model.CapReadCounters: 60, model.CapFirmwareRead: 60,
			model.CapReadConfig: 60, model.CapReadSecurity: 60,
		},
		Evidence:   evidence,
		Families:   families,
		Management: management,
		Notes:      notes,
		Enrich:     p.enrich,
		Config:     p.config,
		Family:     family,
	}, opts)
	return p
}

// families are the HP groups of the research inventory (catalogue group ids).
var families = []string{
	"Color LaserJet Managed MFP E87740/E87750/E87760/E87770, generic E877dn/z (HP01)",
	"Color LaserJet Managed MFP E87640/E87650/E87660 (HP02)",
	"Color LaserJet Managed MFP E78625/E78630/E78635, E78523/E78528, E78223/E78228, E78323-E78330, E77822-E77830, E77422/E77428, E78122 (HP03-HP09)",
	"LaserJet Managed MFP E82650/E82660/E82670, E82540/E82550/E82560, E73130-E73140, E73025/E73030, E72725/E72830, E72525-E72535, E72425/E72430 (HP10-HP16)",
	"LaserJet Managed E40040, E42540, E45028, E47528, E50045/E50145, E52545/E52645, E60055-E60175, E62555-E62675 (HP17-HP24)",
	"Color LaserJet Managed E55040, E57540, E65050-E65160, E67550-E67660, E75245, E85055 (HP25-HP29)",
	"PageWide Managed E55650, E58650, E75160, P75250, E77650/E77660, P77440-P77960 (HP30-HP34)",
	"LaserJet Enterprise M406-M612, M430-M636 (HP35-HP36)",
	"Color LaserJet Enterprise M455-M653, M480-M682, M751/M856, M776 (HP37-HP40)",
	"Color LaserJet Enterprise 5700/6700/6701, X55745/X65455/X65465; MFP 5800/6800/6801, X57945/X58045/X67755/X67765 (HP41-HP42)",
	"LaserJet Enterprise 8501, MFP 8601, Color MFP 8801 (HP43-HP45)",
	"LaserJet Enterprise 5501/5502/6500/6501, X50352-X60357; MFP 5601/5602/6600, X52952-X62757 (HP46-HP47)",
	"LaserJet Pro / Color LaserJet Pro (M283fdw validated; LEDM web service)",
	"Samsung MultiXpress X4220RX/X4250LX/X4300LX, ProXpress M40xx (SA01; identity only)",
}

// management is the catalogue's HP FutureSmart management profile [NHP19] [NHP20] [H11].
var management = vendorkit.ManagementPaths{
	Read:          "SNMP plus EWS (FutureSmart); LEDM /DevMgmt XML on LaserJet Pro / OfficeJet class; exact object and firmware coverage must be probed",
	Configuration: "EWS and Web Jetadmin (SNMP Get/Set, HTTPS 7627 device web services); third-party management contract and private MIB still to obtain",
	Firmware:      "FutureSmart .bdl through EWS or the Web Jetadmin firmware repository; exact product number, FutureSmart branch, revision and date code must match",
	AdminAPI:      "No unrestricted public administrative REST contract verified; Workpath app-management APIs exist for app deployment only",
	Sources:       []string{"NHP19", "NHP20", "H11", "NHP07", "NHP08"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceObserved,
		Note: "enterprise 11 (and 236 for legacy Samsung badges); PML product number / model / serial / firmware date code, Jetdirect sysDescr SN/PID", Sources: []string{"H06", "H07", "M06", "NHP05"}},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceObserved,
		Note: "standard HR/Printer MIB status plus the PML panel status line; LEDM ProductStatusDyn alerts when the web service answers", Sources: []string{"H08"}},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceObserved,
		Note: "PML print-engine total / mono / colour / duplex (cross-checked with Printer MIB and LEDM on an M283fdw; mono/colour only reported when they sum to the total)", Sources: []string{"H08"}},
	{Capability: model.CapFirmwareRead, Status: vendorkit.EvidenceObserved,
		Note: "PML firmware date code and component table (formatter / DC controller / duplexer)", Sources: []string{"H07", "H11"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceObserved,
		Note: "LEDM /DevMgmt XML pages (ProductConfigDyn, NetAppsDyn, NetAppsSecureDyn, SecurityDyn, IoMgmt/Adapters) on LaserJet Pro / OfficeJet class devices; FutureSmart Enterprise devices answer identity.* only", Sources: []string{"H08", "H09"}},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceObserved,
		Note: "protocols.* and security.adminPassword.set from LEDM NetAppsDyn / ProductConfigDyn", Sources: []string{"H14", "NHP05"}},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported,
		Note: "EWS / Web Jetadmin writes are vendor-tool paths; no public write contract (LEDM PUT or FutureSmart) is documented", Sources: []string{"H08", "H10", "NHP08"}},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported,
		Note: "as write_config", Sources: []string{"NHP19", "NHP20"}},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported,
		Note: "HP's EST Connector is a Workforce Experience (manager-side) feature, not a device-native enrolment API", Sources: []string{"H12"}},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported,
		Note: "FutureSmart .bdl delivery is documented through EWS and Web Jetadmin only; no unattended upload contract is public, and LaserJet Pro uses a different (undocumented) path", Sources: []string{"H11", "NHP02"}},
}

const notes = "Samsung-badged devices (enterprise 236) are identified under this plugin with Make \"Samsung\" and receive no HP enrichment: SyncThru is a separate legacy management generation [G01] [M06]. Read-only validation on 2026-10-28 against an HP Color LaserJet MFP M283fdw at 198.51.100.5."

// family derives the HP product family from the model text.
func family(m string) string {
	u := strings.ToUpper(m)
	switch {
	case strings.Contains(u, "PAGEWIDE"):
		return "PageWide"
	case strings.Contains(u, "LASERJET MANAGED") || strings.Contains(u, "MANAGED"):
		return "LaserJet Managed"
	case strings.Contains(u, "LASERJET ENTERPRISE"):
		return "LaserJet Enterprise"
	case strings.Contains(u, "LASERJET PRO"):
		return "LaserJet Pro"
	case strings.Contains(u, "LASERJET"):
		return "LaserJet"
	case strings.Contains(u, "OFFICEJET"):
		return "OfficeJet"
	case strings.Contains(u, "DESIGNJET"):
		return "DesignJet"
	case strings.Contains(u, "SAMSUNG") || strings.Contains(u, "SYNCTHRU"):
		return "Samsung legacy"
	}
	return ""
}

// enrich fills identity from the HP enterprise objects and declares the
// capabilities the device actually answered for.
func (p *Plugin) enrich(ctx context.Context, pr *vendorkit.Probe, id *model.Identity) {
	if pr.Enterprise == EnterpriseSamsung || pr.Badge == "Samsung" {
		id.Make = "Samsung"
		// Identity only: no PML / LEDM on SyncThru devices.
		return
	}
	jd := jetdirectSysDescr(pr.SysDescr)
	if id.Model == "" || strings.HasPrefix(strings.ToUpper(id.Model), "HP ETHERNET") {
		if pid := jd["PID"]; pid != "" {
			id.Model = pid
		}
	}
	if id.Serial == "" {
		id.Serial = jd["SN"]
	}
	if pr.Session == nil {
		// IPP-only identification: the PML/LEDM capabilities need SNMP/HTTP, which
		// the poll will probe again through the standard profile fallback.
		id.Capabilities = append(id.Capabilities, model.CapReadStatus)
		return
	}
	pml, ok := readPMLIdentity(ctx, pr)
	if !ok {
		id.Capabilities = append(id.Capabilities, model.CapReadStatus)
		return
	}
	if pml.ModelName != "" {
		id.Model = pml.ModelName
	}
	if pml.Serial != "" {
		id.Serial = pml.Serial
	}
	if pml.FWDateCode != "" {
		id.Firmware = pml.FWDateCode
	}
	if strings.Contains(strings.ToLower(pml.DeviceID["CMD"]), "scan") || strings.Contains(strings.ToUpper(pml.DeviceID["DES"]), "MFP") {
		id.DeviceType = "mfp"
	}
	id.Capabilities = append(id.Capabilities, model.CapReadStatus)
	if _, ok := pr.GetInt(ctx, oidTotalEnginePages); ok {
		id.Capabilities = append(id.Capabilities, model.CapReadCounters)
	}
	if pml.FWDateCode != "" || len(pml.Components) > 0 {
		id.Capabilities = append(id.Capabilities, model.CapFirmwareRead)
	}
}

func (p *Plugin) probe(ctx context.Context, d *model.Device, creds model.Creds) (*vendorkit.Probe, func(), error) {
	s, variant, err := p.Dial(ctx, d.Address, creds)
	if err != nil {
		return nil, nil, err
	}
	pr := &vendorkit.Probe{Address: d.Address, Session: s, Variant: variant}
	return pr, func() { _ = s.Close() }, nil
}

// ReadStatus implements engine.StatusReader: the standard status reading plus
// the PML panel message, and LEDM alerts when the web service answers quickly.
func (p *Plugin) ReadStatus(ctx context.Context, d *model.Device, creds model.Creds) (*model.Status, error) {
	pr, done, err := p.probe(ctx, d, creds)
	if err != nil {
		return nil, err
	}
	defer done()
	st, err := snmp.ReadStatus(ctx, pr.Session)
	if err != nil {
		return nil, err
	}
	if msg := pr.GetStr(ctx, oidStatusDisplay); msg != "" && st.Message == "" {
		st.Message = msg
	}
	if p.Opts.HTTP != nil {
		lctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		c := &ledmClient{http: p.Opts.HTTP, address: d.Address}
		if n, err := c.get(lctx, ledmProductStatus); err == nil {
			mergeLEDMStatus(st, statusFromNode(n))
		}
	}
	return st, nil
}

func mergeLEDMStatus(st *model.Status, ls *LEDMStatus) {
	if st.Message == "" && ls.Message != "" {
		st.Message = ls.Message
	}
	base := len(st.Alerts)
	for _, a := range ls.Alerts {
		a.Index += base
		st.Alerts = append(st.Alerts, a)
		if a.Severity == model.SeverityCritical {
			st.Severity = model.SeverityCritical
		} else if a.Severity == model.SeverityWarning && st.Severity == model.SeverityOK {
			st.Severity = model.SeverityWarning
		}
	}
}

// ReadCounters implements engine.CounterReader from the PML print-engine
// objects (total, mono, colour, duplex), with the Printer MIB per-marker life
// counts merged in for traceability.
func (p *Plugin) ReadCounters(ctx context.Context, d *model.Device, creds model.Creds) (*model.Counters, error) {
	pr, done, err := p.probe(ctx, d, creds)
	if err != nil {
		return nil, err
	}
	defer done()
	c := readPMLCounters(ctx, pr, time.Now().UTC())
	if c == nil {
		return nil, errors.New("PML total-engine-page-count not available")
	}
	if std := snmp.ParseCounters(pr.Walk(ctx, snmp.OIDPrtMarkerTable), c.At); std != nil {
		c.ByMarker = std.ByMarker
		c.PowerOn = std.PowerOn
	}
	return c, nil
}

// ReadFirmware implements engine.FirmwareReader: the PML firmware date code
// (the version HP's support tables key on for LaserJet Pro), else the first
// component version.
func (p *Plugin) ReadFirmware(ctx context.Context, d *model.Device, creds model.Creds) (string, error) {
	pr, done, err := p.probe(ctx, d, creds)
	if err != nil {
		return "", err
	}
	defer done()
	if v := pmlGet(ctx, pr, oidFWDateCode); fwDateCode.MatchString(v) {
		return v, nil
	}
	for _, c := range readFirmwareComponents(ctx, pr) {
		return c.Version, nil
	}
	return "", errors.New("no PML firmware objects")
}

// config appends LEDM-derived settings and the PML firmware components to the
// base configuration document. Devices without LEDM (FutureSmart Enterprise)
// still get the PML facts.
func (p *Plugin) config(ctx context.Context, b *vendorkit.Base, d *model.Device, creds model.Creds, doc *model.ConfigDocument) error {
	if d.EnterpriseID == EnterpriseSamsung || strings.EqualFold(d.Make, "Samsung") {
		return nil
	}
	if pr, done, err := p.probe(ctx, d, creds); err == nil {
		pml, ok := readPMLIdentity(ctx, pr)
		done()
		if ok {
			const ro = "read-only PML object"
			if pml.ProductNumber != "" {
				vendorkit.AddSetting(doc, Name, "vendor.hp.productNumber", pml.ProductNumber, false, ro)
			}
			if pml.FWDateCode != "" {
				vendorkit.AddSetting(doc, Name, "vendor.hp.firmwareDateCode", pml.FWDateCode, false, ro)
			}
			if len(pml.Components) > 0 {
				var comps []string
				for _, c := range pml.Components {
					comps = append(comps, fmt.Sprintf("%s=%s", c.Name, c.Version))
				}
				vendorkit.AddSetting(doc, Name, "vendor.hp.firmware.components", comps, false, ro)
			}
		}
	}
	if p.Opts.HTTP == nil {
		return nil
	}
	c := &ledmClient{http: p.Opts.HTTP, address: d.Address}
	pages, err := readLEDMConfig(ctx, c)
	if err != nil {
		// Not an error for the document: FutureSmart devices have no LEDM.
		vendorkit.AddSetting(doc, Name, "vendor.hp.ledm", false, false, "device does not expose the LEDM web service")
		return nil
	}
	vendorkit.AddSetting(doc, Name, "vendor.hp.ledm", true, false, "read-only")
	// Drop duplicates the LEDM pages override (product number appears in both).
	doc.Settings = dedupe(doc.Settings, "vendor.hp.productNumber")
	ConfigFromLEDM(pages, doc)
	doc.Settings = dedupeAll(doc.Settings)
	return nil
}

func dedupe(in []model.Setting, key string) []model.Setting {
	var out []model.Setting
	for _, s := range in {
		if s.Key != key {
			out = append(out, s)
		}
	}
	return out
}

// dedupeAll keeps the last occurrence of each key, preserving order of first sight.
func dedupeAll(in []model.Setting) []model.Setting {
	last := map[string]model.Setting{}
	var order []string
	for _, s := range in {
		if _, seen := last[s.Key]; !seen {
			order = append(order, s.Key)
		}
		last[s.Key] = s
	}
	out := make([]model.Setting, 0, len(order))
	for _, k := range order {
		out = append(out, last[k])
	}
	return out
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.StatusReader = (*Plugin)(nil)
var _ engine.CounterReader = (*Plugin)(nil)
var _ engine.FirmwareReader = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
var _ engine.SecurityReader = (*Plugin)(nil)
