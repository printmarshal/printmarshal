// Package canon is the Canon vendor plugin (IANA enterprise 1602).
//
// Match rule: sysObjectID enterprise 1602; a leading "Canon" in sysDescr /
// hrDeviceDescr when no enterprise is reported; IPP MFG "Canon".
//
// Capability / evidence table (see internal/plugins/README.md for sources):
//
//	identify         documented   enterprise 1602; DX C3900 / DX 8900 manuals document SNMPv1/v3 and RFC 2790 formatting [C03] [NCA02] [NCA03]
//	read_config      documented   identity.* (SNMP system group) + vendor.canon.family / sysObjectId
//	read_counters    unverified   private-MIB counter table keyed by Canon's documented "Check Counter" ids (101 Total 1, 108 Black, 113 Full+Single Colour, 122 2-sided); OID root is community-sourced, probe-gated
//	read_security    unsupported  Remote UI only; no open admin API [C03] [C06]
//	write_config     unsupported  Device Settings Configurator / iW MC are vendor tools, not an open wire API [NCA01]
//	manage_certs     unsupported  device-side SCEP is documented for imageFORCE C5100 but no client-facing API [C08] [C12]
//	firmware_update  unsupported  CDS / eMaintenance; customer-accessible route and entitlement unestablished [C11]
package canon

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "canon"

// Enterprise is Canon's IANA enterprise number.
const Enterprise = 1602

// UNVERIFIED: Canon private-MIB counter table as used by community monitoring
// checks; the row index is the counter id shown on the device's Check
// Counter screen, whose numbering Canon documents in its user manuals.
const oidCounterTable = ".1.3.6.1.4.1.1602.1.11.1.3.1.4"

// Canon check-counter ids.
const (
	counterTotal1    = 101 // Total 1
	counterBlack1    = 108 // Total (Black 1)
	counterColor1    = 113 // Total (Full Color + Single Color 1)
	counterDuplex1   = 122 // Total 1 (2-Sided)
	counterScanTotal = 501 // Scan (Total 1)
)

// Plugin is the Canon vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	p := &Plugin{}
	p.Base = vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Canon",
		Enterprises:      []int{Enterprise},
		IPPManufacturers: []string{"Canon", "CANON INC."},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig, model.CapReadCounters},
		Priorities:       map[model.Capability]int{model.CapIdentify: 60, model.CapReadCounters: 60, model.CapReadConfig: 20},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "The counter table is the only enterprise object implemented; its OID is community-sourced and gated on a plausible Total 1 row.",
		Enrich:           p.enrich,
		Family:           family,
	}, opts)
	return p
}

// families are the Canon groups of the research inventory (catalogue group ids).
var families = []string{
	"imageRUNNER ADVANCE DX C3730/C3725/C3720, C3835/C3830/C3826/C3822, C3935/C3930/C3926/C3922 (CA01)",
	"imageRUNNER ADVANCE DX C5760-C5735, C5870-C5840 (CA02); C7780/C7770/C7765 (CA03)",
	"imageRUNNER ADVANCE DX 4751-4725, 4845-4825, 4945-4925 (CA04); 6780-6980 (CA05); 8705/8795/8786, 8905/8995/8986 (CA06)",
	"imageRUNNER ADVANCE DX C357/C257/C359/C259/C477/C478 (CA07); 717/617/527/719/619/529 (CA08)",
	"imageFORCE C5140/C5150/C5160/C5170 (CA09); 6155/6160/6170, 8105/8195/8186 (CA10); C3100/4100 series (2026)",
	"imageCLASS / i-SENSYS business fleets (separate protocol profile needed)",
}

// management is the catalogue's Canon management profile [NCA01] [NCA02] [NCA03] [C08].
var management = vendorkit.ManagementPaths{
	Read:          "Remote UI and SNMP (DX C3900 and DX 8900 manuals document SNMPv1/v3 and RFC 2790 formatting; v2c unverified)",
	Configuration: "Device Settings Configurator supports settings retrieval / distribution on listed DX and third-generation devices with operation exceptions; iW Management Console / Enterprise Management Console",
	Firmware:      "Content Delivery System / eMaintenance; exact customer-accessible update procedure and package entitlement to confirm per controller",
	AdminAPI:      "Settings-tool compatibility is not an open wire API; MEAP is an embedded application platform",
	Sources:       []string{"NCA01", "NCA02", "NCA03", "C08", "C11"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented,
		Note: "enterprise 1602; SNMP documented for DX C3900 / DX 8900 / DX C5800 (SNMPv1/v3; v2c not confirmed)", Sources: []string{"C03", "C06", "NCA02", "NCA03"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented,
		Note: "identity.* from the SNMP system group plus vendor.canon.family / sysObjectId", Sources: []string{"C03"}},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnverified,
		Note: "private-MIB counter table keyed by Canon's documented Check Counter ids (101/108/113/122/501); table OID from community checks, probe-gated", Sources: []string{"C03"}},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "Remote UI only", Sources: []string{"C03", "C06"}},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "Device Settings Configurator / iW MC are vendor tools; not an open wire API", Sources: []string{"NCA01"}},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config", Sources: []string{"NCA02"}},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "device-side SCEP documented for imageFORCE C5100; no client-facing enrolment API; EST/ACME U", Sources: []string{"C08", "C12"}},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "CDS / eMaintenance; route and entitlement unestablished", Sources: []string{"C11"}},
}

func family(m string) string {
	u := strings.ToUpper(m)
	switch {
	case strings.Contains(u, "IMAGERUNNER ADVANCE DX") || strings.Contains(u, "IR-ADV DX") || strings.Contains(u, "IR ADV DX"):
		return "imageRUNNER ADVANCE DX"
	case strings.Contains(u, "IMAGERUNNER ADVANCE") || strings.Contains(u, "IR-ADV"):
		return "imageRUNNER ADVANCE"
	case strings.Contains(u, "IMAGERUNNER") || strings.HasPrefix(strings.TrimPrefix(u, "CANON "), "IR"):
		return "imageRUNNER"
	case strings.Contains(u, "IMAGEFORCE"):
		return "imageFORCE"
	case strings.Contains(u, "IMAGEPRESS"):
		return "imagePRESS"
	case strings.Contains(u, "IMAGECLASS"):
		return "imageCLASS"
	case strings.Contains(u, "I-SENSYS"):
		return "i-SENSYS"
	case strings.Contains(u, "MAXIFY"):
		return "MAXIFY"
	case strings.Contains(u, "PIXMA"):
		return "PIXMA"
	}
	return ""
}

func (p *Plugin) enrich(ctx context.Context, pr *vendorkit.Probe, id *model.Identity) {
	if pr.Session == nil {
		return
	}
	if ParseCounters(pr.Walk(ctx, oidCounterTable), time.Now()) != nil {
		id.Capabilities = append(id.Capabilities, model.CapReadCounters)
	}
}

// ParseCounters maps the counter table rows (index = Canon counter id) onto
// Counters; nil without a plausible Total 1 row.
func ParseCounters(pdus []snmp.PDU, at time.Time) *model.Counters {
	vals := map[int]int64{}
	for _, pdu := range pdus {
		rest := strings.TrimPrefix(pdu.OID, oidCounterTable+".")
		id, err := strconv.Atoi(strings.SplitN(rest, ".", 2)[0])
		if err != nil {
			continue
		}
		if n, ok := snmp.Int(pdu); ok && n >= 0 {
			vals[id] = n
		}
	}
	total, ok := vals[counterTotal1]
	if !ok || total <= 0 {
		return nil
	}
	c := &model.Counters{Total: total, At: at}
	mono, okM := vals[counterBlack1]
	color, okC := vals[counterColor1]
	if okM && okC && mono+color <= total+total/100+1 {
		c.Mono, c.Color = &mono, &color
	}
	if d, ok := vals[counterDuplex1]; ok && d <= total {
		c.Duplex = &d
	}
	return c
}

// ReadCounters implements engine.CounterReader.
func (p *Plugin) ReadCounters(ctx context.Context, d *model.Device, creds model.Creds) (*model.Counters, error) {
	s, _, err := p.Dial(ctx, d.Address, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	pr := &vendorkit.Probe{Address: d.Address, Session: s}
	c := ParseCounters(pr.Walk(ctx, oidCounterTable), time.Now().UTC())
	if c == nil {
		return nil, errors.New("canon counter table did not answer with a Total 1 (101) row")
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
