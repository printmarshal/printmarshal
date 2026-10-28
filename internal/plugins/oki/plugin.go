// Package oki is the OKI vendor plugin (IANA enterprise 2001).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// OKI wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 2001; a leading "OKI" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "OKI" / "OKI DATA CORP" / "Oki Data".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 2001 (Oki Data); C650/ES6450 specifications list SNMPv1/v3
//	read_config      documented   identity.* (SNMP system group) + vendor.oki.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; SNMPv2c is not confirmed for C650 [O02]
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  model MIB definitions ship on the supplied DVD (Misc/MIB Files); not obtained [O03]
//	read_security    unsupported  Configuration Tool / EWS only [O04]
//	write_config     unsupported  Configuration Tool backs up / changes settings with explicit exceptions [O04]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  certificate enrolment needs model-specific evidence [O05]
//	firmware_update  unsupported  Firmware Update Tool requires the admin password; network transport and offline package rights U [O05]
package oki

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "oki"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 2001

// Plugin is the OKI vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "OKI",
		Enterprises:      []int{Enterprise},
		Badges:           nil,
		IPPManufacturers: []string{"OKI", "OKI DATA CORP", "Oki Data"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "ETRIA engine sharing with Ricoh/Toshiba does not imply management compatibility [M03] [M04].",
		Family:           family,
	}, opts)}
}

var families = []string{"C650dn/dnw, ES6450 (OK01)", "C824/C834/C844, MC853/MC873/MC883 (OK02)"}

var management = vendorkit.ManagementPaths{
	Read:          "C650 guide documents SNMPv1/v3 (v2c not confirmed); MIB files on the supplied media",
	Configuration: "Configuration Tool / model EWS; passwords and network settings excluded from cloning",
	Firmware:      "official Firmware Update Tool for C650 (administrator password required)",
	AdminAPI:      "No general administrative REST contract verified",
	Sources:       []string{"O02", "O03", "O04", "O05"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 2001 (Oki Data); C650/ES6450 specifications list SNMPv1/v3", Sources: []string{"O01", "O02", "O03"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.oki.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "SNMPv2c is not confirmed for C650 [O02]"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "model MIB definitions ship on the supplied DVD (Misc/MIB Files); not obtained [O03]"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "Configuration Tool / EWS only [O04]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "Configuration Tool backs up / changes settings with explicit exceptions [O04]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "certificate enrolment needs model-specific evidence [O05]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "Firmware Update Tool requires the admin password; network transport and offline package rights U [O05]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"OKI ", "OKI DATA CORP ", "OKI DATA "} {
		u = strings.TrimPrefix(u, prefix)
	}
	for _, r := range familyRules {
		if strings.HasPrefix(u, r.match) {
			return r.family
		}
	}
	return ""
}

var familyRules = []struct{ match, family string }{
	{"ES", "ES"},
	{"MC", "MC"},
	{"C", "C"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
