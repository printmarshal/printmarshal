// Package toshiba is the Toshiba vendor plugin (IANA enterprise 1129).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Toshiba wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 1129; a leading "Toshiba" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "TOSHIBA" / "Toshiba" / "TOSHIBA TEC".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 1129 (Toshiba TEC e-BRIDGE); 25AC specification lists SNMPv3
//	read_config      documented   identity.* (SNMP system group) + vendor.toshiba.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; private MIB files and complete RFC 3805/2790 conformance not verified [T01]
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  no Toshiba private counter objects documented in the research
//	read_security    unsupported  TopAccess only; SCEP/TPM/secure boot documented as features [T01] [T04]
//	write_config     unsupported  TopAccess is the administration UI; no schema [T07]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  SCEP is documented as a device feature on 25AC; EST/ACME U; no enrolment API [T01]
//	firmware_update  unsupported  TopAccess shows component versions; no customer-accessible unattended upload API established [T07]
package toshiba

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "toshiba"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 1129

// Plugin is the Toshiba vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Toshiba",
		Enterprises:      []int{Enterprise},
		Badges:           nil,
		IPPManufacturers: []string{"TOSHIBA", "Toshiba", "TOSHIBA TEC"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "Preserve the component firmware list rather than one system string when TopAccess access exists [T07].",
		Family:           family,
	}, opts)}
}

var families = []string{"e-STUDIO2525AC/3025AC/3525AC/4525AC/5525AC/6525AC (TO01)", "e-STUDIO6526AC/6527AC/7527AC (TO02)", "e-STUDIO2528A-6528A (TO03)", "e-STUDIO6529A/7529A/9029A (TO04)"}

var management = vendorkit.ManagementPaths{
	Read:          "TopAccess and SNMP; TopAccess exposes a component-level Current Software List",
	Configuration: "TopAccess; third-party administrative schema unverified",
	Firmware:      "component versions documented; system firmware upload rights/protocol need confirmation",
	AdminAPI:      "No general open administrative REST contract verified",
	Sources:       []string{"T01", "T04", "T07"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 1129 (Toshiba TEC e-BRIDGE); 25AC specification lists SNMPv3", Sources: []string{"T01", "T02", "T07"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.toshiba.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "private MIB files and complete RFC 3805/2790 conformance not verified [T01]"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "no Toshiba private counter objects documented in the research"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "TopAccess only; SCEP/TPM/secure boot documented as features [T01] [T04]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "TopAccess is the administration UI; no schema [T07]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "SCEP is documented as a device feature on 25AC; EST/ACME U; no enrolment API [T01]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "TopAccess shows component versions; no customer-accessible unattended upload API established [T07]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"TOSHIBA ", "TOSHIBA TEC "} {
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
	{"E-STUDIO", "e-STUDIO"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
