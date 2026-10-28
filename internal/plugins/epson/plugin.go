// Package epson is the Epson vendor plugin (IANA enterprise 1248).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Epson wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 1248; a leading "Epson" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "EPSON" / "Epson" / "Seiko Epson".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 1248; AM-C6000 specifications enumerate SNMPv1/v2c/v3, mDNS, SLP, WSD, LLMNR, IPP/IPPS
//	read_config      documented   identity.* (SNMP system group) + vendor.epson.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; enterprise MIB definitions and complete standard-MIB conformance remain U [E01]
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  no Epson private counter objects documented in the research
//	read_security    unsupported  Web Config documents CA certificates, IPsec/IP filtering, startup verification; no read API [E03] [E04]
//	write_config     unsupported  Epson Device Admin / Web Config are vendor tools [E05]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  native SCEP/EST/ACME not established [E04]
//	firmware_update  unsupported  Web Config / Device Admin update paths; recovery mode may require USB, so a remote plugin needs an escalation path [E03] [E04]
package epson

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "epson"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 1248

// Plugin is the Epson vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Epson",
		Enterprises:      []int{Enterprise},
		Badges:           nil,
		IPPManufacturers: []string{"EPSON", "Epson", "Seiko Epson"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "Do not inherit AM-C6000 details onto AM-C10000 or older WF models.",
		Family:           family,
	}, opts)}
}

var families = []string{"WorkForce Enterprise AM-C4000/C5000/C6000 (EP01)", "WorkForce Enterprise AM-C10000 (EP02)", "WorkForce Enterprise WF-C21000 (EP03)", "WorkForce Pro WF-C5390/WF-C5890 (EP04)", "AM-C400/C550, AM-M5500, WF-C20600/C20750 [E05] [E07]"}

var management = vendorkit.ManagementPaths{
	Read:          "SNMPv1/v2c/v3 and Web Config (AM-C6000 specification)",
	Configuration: "Epson Device Admin batch settings and Web Config",
	Firmware:      "Web Config checks/downloads updates with internet access; scheduled automatic updates; Epson Device Admin; recovery may need USB",
	AdminAPI:      "Open Platform is a workflow platform, not a general management REST",
	Sources:       []string{"E03", "E04", "E05", "NEP02"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 1248; AM-C6000 specifications enumerate SNMPv1/v2c/v3, mDNS, SLP, WSD, LLMNR, IPP/IPPS", Sources: []string{"E01", "E03"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.epson.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "enterprise MIB definitions and complete standard-MIB conformance remain U [E01]"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "no Epson private counter objects documented in the research"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "Web Config documents CA certificates, IPsec/IP filtering, startup verification; no read API [E03] [E04]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "Epson Device Admin / Web Config are vendor tools [E05]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "native SCEP/EST/ACME not established [E04]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "Web Config / Device Admin update paths; recovery mode may require USB, so a remote plugin needs an escalation path [E03] [E04]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"EPSON ", "SEIKO EPSON "} {
		u = strings.TrimPrefix(u, prefix)
	}
	for _, r := range familyRules {
		if strings.Contains(u, r.match) {
			return r.family
		}
	}
	return ""
}

var familyRules = []struct{ match, family string }{
	{"AM-", "WorkForce Enterprise AM"},
	{"WF-C2", "WorkForce Enterprise"},
	{"WF-", "WorkForce Pro"},
	{"ET-", "EcoTank"},
	{"ECOTANK", "EcoTank"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
