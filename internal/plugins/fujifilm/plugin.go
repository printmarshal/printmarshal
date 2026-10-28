// Package fujifilm is the Fujifilm vendor plugin (IANA enterprise 297).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Fujifilm wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 297; a leading "Fujifilm" / "Fuji Xerox" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "FUJIFILM" / "FUJI XEROX" / "Fuji Xerox" / "FujiFilm".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 297 (Fuji Xerox); Internet Services SNMPv3 help confirms SNMPv3 in the Apeos 5570 family
//	read_config      documented   identity.* (SNMP system group) + vendor.fujifilm.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; exact SNMP/MIB support per C3570 variant not established; evaluated configurations may disable SNMP [NFU01]
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  no Fujifilm private counter objects documented in the research
//	read_security    unsupported  Internet Services only [F03]
//	write_config     unsupported  Internet Services / vendor tooling; no API [F05]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  enrolment matrix is a gap [F03]
//	firmware_update  unsupported  firmware process unverified [F02]
package fujifilm

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "fujifilm"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 297

// Plugin is the Fujifilm vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Fujifilm",
		Enterprises:      []int{Enterprise},
		Badges:           []vendorkit.Badge{{Name: "Fuji Xerox", Match: []string{"Fuji Xerox", "FUJI XEROX", "FujiXerox"}}},
		IPPManufacturers: []string{"FUJIFILM", "FUJI XEROX", "Fuji Xerox", "FujiFilm"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "Fujifilm / Fuji Xerox identifiers must never select the Xerox plugin or AltaLink firmware [F03].",
		Family:           family,
	}, opts)}
}

var families = []string{"Apeos C3570/C4570/C5570/C7070 (FU01)", "Apeos C6580/C7580/C8180 (FU02)", "Apeos C3530/C4030 (FU03)", "Apeos C325z/dw; legacy ApeosPort / DocuCentre identifiers [F04] [F06]"}

var management = vendorkit.ManagementPaths{
	Read:          "SNMPv3 and Internet Services in exact family documents; the evaluated secure configuration disables SNMP",
	Configuration: "Internet Services / vendor tooling",
	Firmware:      "exact regional customer/service route; no third-party contract",
	AdminAPI:      "Workflow integrations are separate from administrative API coverage",
	Sources:       []string{"F03", "F05", "F06", "NFU01"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 297 (Fuji Xerox); Internet Services SNMPv3 help confirms SNMPv3 in the Apeos 5570 family", Sources: []string{"F01", "F03", "F05"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.fujifilm.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "exact SNMP/MIB support per C3570 variant not established; evaluated configurations may disable SNMP [NFU01]"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "no Fujifilm private counter objects documented in the research"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "Internet Services only [F03]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "Internet Services / vendor tooling; no API [F05]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "enrolment matrix is a gap [F03]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "firmware process unverified [F02]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"FUJIFILM ", "FUJI XEROX "} {
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
	{"APEOSPORT", "ApeosPort"},
	{"APEOS", "Apeos"},
	{"DOCUCENTRE", "DocuCentre"},
	{"DOCUPRINT", "DocuPrint"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
