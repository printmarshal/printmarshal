// Package brother is the Brother vendor plugin (IANA enterprise 2435).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Brother wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 2435; a leading "Brother" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "Brother" / "BROTHER".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 2435; MFC-L6915DW documentation lists SNMPv1/v2c/v3, IPP/IPPS, mDNS, LLMNR
//	read_config      documented   identity.* (SNMP system group) + vendor.brother.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; SNMPv3-only modes documented on older models can disable v1/v2c reads [B05]
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  no Brother private counter objects documented in the research
//	read_security    unsupported  Web Based Management only [B02]
//	write_config     unsupported  Mass Deployment Tool / BRAdmin are vendor tools; no device REST interface verified [B02] [B04]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  native SCEP/EST/ACME U [B01]
//	firmware_update  unsupported  Firmware Update Tool obtains firmware from Brother's internet server; no redistributable package API [B01] [B04]
package brother

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "brother"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 2435

// Plugin is the Brother vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Brother",
		Enterprises:      []int{Enterprise},
		Badges:           nil,
		IPPManufacturers: []string{"Brother", "BROTHER"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "",
		Family:           family,
	}, opts)}
}

var families = []string{"HL-L9410CDN/L9430CDN/L9470CDN, HL-EX470W (BR01)", "MFC-L9610CDN/L9630CDN/L9670CDN, MFC-EX670W (BR02)", "HL-L6415DW, MFC-L6810DW, MFC-L6915DW (BR03)", "HL-L6200DW/L6400DW, MFC-L6900DW, HL-L8360CDW, MFC-L8900CDW/L9570CDW [B02] [B04]"}

var management = vendorkit.ManagementPaths{
	Read:          "SNMPv1/v2c/v3 and Web Based Management (MFC-L6915DW documentation)",
	Configuration: "BRAdmin Professional and Mass Deployment Tool",
	Firmware:      "Firmware Update Tool fetches firmware from Brother's server; offline/API route unverified",
	AdminAPI:      "No general administrative REST contract verified",
	Sources:       []string{"B02", "B04", "B05", "NBR01"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 2435; MFC-L6915DW documentation lists SNMPv1/v2c/v3, IPP/IPPS, mDNS, LLMNR", Sources: []string{"B01", "B02", "NBR01"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.brother.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "SNMPv3-only modes documented on older models can disable v1/v2c reads [B05]"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "no Brother private counter objects documented in the research"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "Web Based Management only [B02]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "Mass Deployment Tool / BRAdmin are vendor tools; no device REST interface verified [B02] [B04]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "native SCEP/EST/ACME U [B01]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "Firmware Update Tool obtains firmware from Brother's internet server; no redistributable package API [B01] [B04]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"BROTHER "} {
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
	{"MFC-", "MFC"},
	{"DCP-", "DCP"},
	{"HL-", "HL"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
