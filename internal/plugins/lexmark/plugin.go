// Package lexmark is the Lexmark vendor plugin (IANA enterprise 641).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Lexmark wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 641; a leading "Lexmark" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "Lexmark" / "LEXMARK".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 641; CX625 guide documents SNMPv1/v2c and v3 controls
//	read_config      documented   identity.* (SNMP system group) + vendor.lexmark.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; Lexmark's MIB-pack article says Printer MIB v1 for the products it discusses; do not assume full RFC 3805 [L06]
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  no Lexmark private counter objects documented; MVE reads inventory through proprietary channels [NLE02]
//	read_security    unsupported  EWS only; certificate / 802.1X / IPsec documented as features [L01] [L02]
//	write_config     unsupported  MVE enforces configurations over LST/NPAP/XMLNT, which are proprietary and undocumented [L09] [NLE02]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  MVE enrols against the CA and installs to the printer; its EST connector is not native device EST [NLE04]
//	firmware_update  unsupported  MVE firmware upload with access-control prerequisite; package rules per model unconfirmed [NLE03]
package lexmark

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "lexmark"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 641

// Plugin is the Lexmark vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Lexmark",
		Enterprises:      []int{Enterprise},
		Badges:           nil,
		IPPManufacturers: []string{"Lexmark", "LEXMARK"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "Devices past End of Firmware Support drop out of MVE; a historical compatibility entry needs a lifecycle check [NLE01].",
		Family:           family,
	}, opts)}
}

var families = []string{"CS730/CS735/CS737/CS820/CS827 (LE01)", "CX730-CX860 (LE02)", "CX930/CX931/CX942/CX943/CX944 (LE03)", "MS725/MS821-MS826 (LE04)", "MX721-MX826 (LE05)", "MX931 (LE06)", "XC9325-XC9465 (LE07)", "XM5365/XM5370 (LE08)", "CX532/CX635/MX632 (LE09)", "CX625/CX622, MX622/MX521/MX522 [L04] [L05]"}

var management = vendorkit.ManagementPaths{
	Read:          "SNMPv1/v2c/v3 (CX625 guide); Markvision Enterprise requires SNMPv3 on footnoted models; MIB pack is Printer MIB v1 for the products it covers",
	Configuration: "Embedded Web Server; MVE over SNMP/HTTPS plus proprietary LST/NPAP/XMLNT",
	Firmware:      "MVE file upload with scheduled window and Firmware Updates access control",
	AdminAPI:      "Fleet Tracker REST and LDD JSAPI are server/job APIs, not device administration",
	Sources:       []string{"NLE01", "NLE02", "NLE03", "NLE04", "L07", "L08"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 641; CX625 guide documents SNMPv1/v2c and v3 controls", Sources: []string{"L02", "L05", "L06", "NLE01"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.lexmark.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "Lexmark's MIB-pack article says Printer MIB v1 for the products it discusses; do not assume full RFC 3805 [L06]"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "no Lexmark private counter objects documented; MVE reads inventory through proprietary channels [NLE02]"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "EWS only; certificate / 802.1X / IPsec documented as features [L01] [L02]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "MVE enforces configurations over LST/NPAP/XMLNT, which are proprietary and undocumented [L09] [NLE02]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "MVE enrols against the CA and installs to the printer; its EST connector is not native device EST [NLE04]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "MVE firmware upload with access-control prerequisite; package rules per model unconfirmed [NLE03]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"LEXMARK "} {
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
	{"XC9", "XC"},
	{"XM", "XM"},
	{"CX", "CX"},
	{"CS", "CS"},
	{"MX", "MX"},
	{"MS", "MS"},
	{"MC", "MC"},
	{"MB", "MB"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
