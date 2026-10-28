package ricoh

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "ricoh"

// Enterprise is Ricoh's IANA enterprise number.
const Enterprise = 367

// Plugin is the Ricoh vendor plugin (Savin / Lanier / Gestetner / Nashuatec /
// Rex-Rotary / NRG / IKON badges resolve to it).
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	p := &Plugin{}
	p.Base = vendorkit.New(vendorkit.Spec{
		Name:        Name,
		Vendor:      "Ricoh",
		Enterprises: []int{Enterprise},
		Badges: []vendorkit.Badge{
			{Name: "Savin", Match: []string{"Savin"}},
			{Name: "Lanier", Match: []string{"Lanier"}},
			{Name: "Gestetner", Match: []string{"Gestetner"}},
			{Name: "Nashuatec", Match: []string{"Nashuatec"}},
			{Name: "Rex-Rotary", Match: []string{"Rex-Rotary", "Rex Rotary", "RexRotary"}},
			{Name: "NRG", Match: []string{"NRG"}},
			{Name: "IKON", Match: []string{"IKON"}},
			{Name: "infotec", Match: []string{"infotec"}},
		},
		IPPManufacturers: []string{"RICOH", "Ricoh", "SAVIN", "LANIER", "GESTETNER", "NASHUATEC", "REX-ROTARY", "NRG"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig, model.CapReadCounters},
		Priorities:       map[model.Capability]int{model.CapIdentify: 60, model.CapReadCounters: 60, model.CapReadConfig: 20},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            notes,
		Enrich:           p.enrich,
		Family:           family,
	}, opts)
	return p
}

// families are the Ricoh groups of the research inventory (catalogue group ids).
var families = []string{
	"IM C2000/C2500/C3000/C3500/C4500/C5500/C6000 (RI01, 2019)",
	"IM C2010/C2510/C3010/C3510/C4510/C5510/C6010 (RI02, 2023)",
	"IM C6510/C7010/C8010 (RI03); IM C6500/C8000 (RI06)",
	"IM C3010SD/C4510SD/C6010SD (RI04, 2025 JP); IM C2011-C6011 (RI05, 2026 JP)",
	"IM 2500/3000/3500/4000/5000/6000 (RI07); IM 2510-7010 (RI08, 2026); IM 7000/8000/9000 (RI09)",
	"IM C300/C300F/C400F/C400SRF (RI10); IM C320F/C401F/C401SRF (RI11)",
	"IM 370F/460F (RI12); IM 550F/600F/600SRF (RI13)",
	"P 800/P 801 (RI14); P C600/P C375 (RI15)",
	"MP 6503/7503/9003, MP 501/601, MP C3004ex-C6004ex (RI16, RI17; conditional legacy)",
	"Pro C7500, Pro C5400S/C5410S (Q01-Q03; DFE is a separate component)",
}

// management is the catalogue's Ricoh management profile [R08] [R09] [R10] [R11] [NRI01] [NRI05].
var management = vendorkit.ManagementPaths{
	Read:          "Web Image Monitor; Standard / Private MIB specifications and SmartDeviceMonitor API through the Ricoh developer program (RiDP); PMAPI, Common / Private / GJ MIB in the Japanese program",
	Configuration: "Ricoh Device Interfaces / PMAPI / private MIB coverage requires program access and an exact model matrix; SmartSDK WebAPI is workflow REST, not administration",
	Firmware:      "Always Current Technology and the Firmware Update Tool are vendor routes; a third-party unattended upload contract is unverified",
	AdminAPI:      "Partner management interfaces documented; unrestricted model-specific contract not verified",
	Sources:       []string{"R08", "R09", "R10", "R11", "NRI01", "NRI05"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented,
		Note: "enterprise 367; regional badges Savin / Lanier / Gestetner / Nashuatec / Rex-Rotary enumerated by the JISEC certification; NRG and IKON kept as labels only (an IKON dealer record never selects the plugin without the enterprise OID)", Sources: []string{"R23", "R24", "R25"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented,
		Note: "identity.* from the SNMP system group plus vendor.ricoh.badge / family / sysObjectId; Web Image Monitor settings are not exposed through a documented API", Sources: []string{"R08", "R16"}},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnverified,
		Note: "Ricoh private-MIB engine counter table (.1.3.6.1.4.1.367.3.2.1.2.19.5.1: name / value rows) as used by community monitoring templates; the licensed Private MIB Specification was not obtained, so the reader is probe-gated: counters are declared only when the device returns a row literally named \"Total\" and the derived mono/colour never exceed it", Sources: []string{"R08", "R09"}},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported,
		Note: "security settings live in Web Image Monitor; no documented read API (SmartSDK is workflow REST)", Sources: []string{"R10", "R11"}},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported,
		Note: "PMAPI / Device Interfaces require RiDP program access and an exact model matrix", Sources: []string{"R08", "R09"}},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config", Sources: []string{"R11"}},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported,
		Note: "native SCEP / EST / ACME not established; certificate import is not enrolment", Sources: []string{"R11"}},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported,
		Note: "Firmware Update Tool / ACT / @Remote are vendor routes; no public update endpoint may be inferred from them", Sources: []string{"R21", "R27"}},
}

const notes = "Priority-1 vendor in the research. The engine counter table is the only enterprise object implemented and it is community-sourced (unverified); everything else is standards-based until RiDP MIB specifications are obtained."

// family derives the Ricoh product family from the model text.
func family(m string) string {
	u := strings.ToUpper(m)
	for _, bd := range []string{"RICOH ", "SAVIN ", "LANIER ", "GESTETNER ", "NASHUATEC ", "REX-ROTARY ", "NRG ", "IKON ", "INFOTEC "} {
		u = strings.TrimPrefix(u, bd)
	}
	switch {
	case strings.HasPrefix(u, "PRO C"):
		return "Pro C"
	case strings.HasPrefix(u, "PRO "):
		return "Pro"
	case strings.HasPrefix(u, "IM C"):
		return "IM C"
	case strings.HasPrefix(u, "IM "):
		return "IM"
	case strings.HasPrefix(u, "MP C"):
		return "MP C"
	case strings.HasPrefix(u, "MP "):
		return "MP"
	case strings.HasPrefix(u, "P C"):
		return "P C"
	case strings.HasPrefix(u, "P "):
		return "P"
	case strings.HasPrefix(u, "SP C"):
		return "SP C"
	case strings.HasPrefix(u, "SP "):
		return "SP"
	case strings.HasPrefix(u, "M C"):
		return "M C"
	case strings.HasPrefix(u, "AFICIO"):
		return "Aficio"
	}
	return ""
}

var serialRe = regexp.MustCompile(`^[A-Za-z0-9]{6,24}$`)

// enrich probes the (unverified) counter table and, when the device answers
// sensibly, declares read_counters. It never changes identity except to fill
// an empty serial from the private system group.
func (p *Plugin) enrich(ctx context.Context, pr *vendorkit.Probe, id *model.Identity) {
	if pr.Session == nil {
		return
	}
	if id.Serial == "" {
		if v := pr.GetStr(ctx, oidPrivateSerial); serialRe.MatchString(v) {
			id.Serial = v
		}
	}
	if c := ParseCounters(pr.Walk(ctx, oidEngCounterTable), time.Now()); c != nil {
		id.Capabilities = append(id.Capabilities, model.CapReadCounters)
	}
}

// ReadCounters implements engine.CounterReader from the engine counter table,
// falling back to the Printer MIB marker life counts for the total.
func (p *Plugin) ReadCounters(ctx context.Context, d *model.Device, creds model.Creds) (*model.Counters, error) {
	s, _, err := p.Dial(ctx, d.Address, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	pr := &vendorkit.Probe{Address: d.Address, Session: s}
	c := ParseCounters(pr.Walk(ctx, oidEngCounterTable), time.Now().UTC())
	if c == nil {
		return nil, errors.New("ricoh engine counter table did not answer with a Total row")
	}
	if std := snmp.ParseCounters(pr.Walk(ctx, snmp.OIDPrtMarkerTable), c.At); std != nil {
		c.ByMarker = std.ByMarker
		c.PowerOn = std.PowerOn
	}
	return c, nil
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.CounterReader = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
