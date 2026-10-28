// Package konicaminolta is the Konica Minolta vendor plugin (IANA enterprise 18334).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Konica Minolta wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 18334; a leading "Konica Minolta" / "Develop" / "Olivetti" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "KONICA MINOLTA" / "Konica Minolta" / "KONICAMINOLTA".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 18334; Develop ineo identity declared by the official download centre; Olivetti aliasing needs explicit evidence (badge label only, never a selector)
//	read_config      documented   identity.* (SNMP system group) + vendor.konicaminolta.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; Web Connection documents SNMPv1/v2c/v3; Printer MIB conformance per model not stated
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  meter counts are exposed through bizhub OpenAPI, whose partner schema was not obtained [K03]
//	read_security    unsupported  OpenAPI describes security/authentication configuration but the operation specification is partner-only [K03]
//	write_config     unsupported  OpenAPI configuration needs partner operation specifications / version compatibility [K03]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  NDES/SCEP integration is Fleet RMM-mediated with firmware gates, not a device-native API [K06]
//	firmware_update  unsupported  Fleet RMM / CS Remote Care; third-party firmware transport unverified [K10] [K11]
package konicaminolta

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "konicaminolta"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 18334

// Plugin is the Konica Minolta vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Konica Minolta",
		Enterprises:      []int{Enterprise},
		Badges:           []vendorkit.Badge{{Name: "Develop", Match: []string{"Develop", "ineo"}}, {Name: "Olivetti", Match: []string{"Olivetti", "d-Color", "d-Copia"}}},
		IPPManufacturers: []string{"KONICA MINOLTA", "Konica Minolta", "KONICAMINOLTA"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "Develop and Olivetti badges resolve here only when the enterprise OID is 18334 [K08]; Muratec/Sindoh need identity evidence first.",
		Family:           family,
	}, opts)}
}

var families = []string{"bizhub C250i/C300i/C360i/C450i/C550i/C650i/C750i (KM01)", "bizhub C251i-C751i (KM02)", "bizhub 300i-750i (KM03)", "bizhub 301i-751i (KM04)", "AccurioPrint 850i/950i (KM05)", "bizhub C3350i/C4050i/C3351i/C4051i (KM06)", "bizhub 4050i/4750i/4051i/4751i (KM07)", "Develop ineo+ 250i/300i/360i [K08]"}

var management = vendorkit.ManagementPaths{
	Read:          "Web Connection and SNMPv1/v2c/v3 with separate read/write users; current MIB package to obtain",
	Configuration: "bizhub OpenAPI (TLS) acquires/configures device information, network settings, address books, meter counts; partner schema required",
	Firmware:      "Fleet RMM / CS Remote Care / service paths; third-party upload contract unverified",
	AdminAPI:      "Partner OpenAPI evidence; operation schema and licence not obtained",
	Sources:       []string{"K02", "K03", "K10", "NKM01"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 18334; Develop ineo identity declared by the official download centre; Olivetti aliasing needs explicit evidence (badge label only, never a selector)", Sources: []string{"K02", "K08", "NKM01"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.konicaminolta.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "Web Connection documents SNMPv1/v2c/v3; Printer MIB conformance per model not stated"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "meter counts are exposed through bizhub OpenAPI, whose partner schema was not obtained [K03]"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "OpenAPI describes security/authentication configuration but the operation specification is partner-only [K03]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "OpenAPI configuration needs partner operation specifications / version compatibility [K03]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "NDES/SCEP integration is Fleet RMM-mediated with firmware gates, not a device-native API [K06]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "Fleet RMM / CS Remote Care; third-party firmware transport unverified [K10] [K11]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"KONICA MINOLTA ", "DEVELOP ", "OLIVETTI ", "KONICAMINOLTA "} {
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
	{"ACCURIOPRINT", "AccurioPrint"},
	{"ACCURIOPRESS", "AccurioPress"},
	{"BIZHUB", "bizhub"},
	{"INEO", "ineo"},
	{"D-COLOR", "d-Color"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
