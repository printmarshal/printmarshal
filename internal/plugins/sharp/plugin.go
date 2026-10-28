// Package sharp is the Sharp vendor plugin (IANA enterprise 2385).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Sharp wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 2385; a leading "Sharp" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "SHARP" / "Sharp".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 2385; BP-70C documentation lists SNMPv3, IPP/IPPS, TLS 1.3
//	read_config      documented   identity.* (SNMP system group) + vendor.sharp.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; private MIB available only through Sharp [A01] [A08]
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  SRDM meters are a vendor-tool path; no private counter objects documented [NSH01]
//	read_security    unsupported  BP-70C sheet documents port/protocol controls without a read API [A01]
//	write_config     unsupported  web management / cloning / SRDM are vendor tools; A4 OEM-derived rows are narrower [NSH02]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  native SCEP/EST/ACME U [A01]
//	firmware_update  unsupported  SRDM one-to-many update documented; no third-party transport [A01] [NSH03]
package sharp

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "sharp"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 2385

// Plugin is the Sharp vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Sharp",
		Enterprises:      []int{Enterprise},
		Badges:           nil,
		IPPManufacturers: []string{"SHARP", "Sharp"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "Keep the MX-C407P/C507P/C607P row as a negative control: monitoring only in the SRDM matrix [NSH02].",
		Family:           family,
	}, opts)}
}

var families = []string{"BP-70M75/BP-70M90 (SH01)", "BP-50M26-BP-70M65 (SH02)", "BP-50C26-BP-70C65 (SH03)", "MX-M1056/MX-M1206 (SH04)", "MX-C407P/C507P/C607P (SH05: SRDM marks power/security/clone/firmware unsupported)", "BP-51C26/31/36/45, BP-71C (2025) [A03] [A04]", "MX-3071/3571/4071, MX-2651-4051 [A07]"}

var management = vendorkit.ManagementPaths{
	Read:          "SRDM discovery/status/meters; exact matrix row determines fields",
	Configuration: "Web management / cloning and the SRDM capability matrix",
	Firmware:      "Network and one-to-many updates through Sharp Remote Device Manager on supported rows",
	AdminAPI:      "Sharp OSA is an application framework, not an admin REST",
	Sources:       []string{"NSH01", "NSH02", "NSH03"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 2385; BP-70C documentation lists SNMPv3, IPP/IPPS, TLS 1.3", Sources: []string{"A01", "NSH01", "NSH02"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.sharp.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "private MIB available only through Sharp [A01] [A08]"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "SRDM meters are a vendor-tool path; no private counter objects documented [NSH01]"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "BP-70C sheet documents port/protocol controls without a read API [A01]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "web management / cloning / SRDM are vendor tools; A4 OEM-derived rows are narrower [NSH02]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "native SCEP/EST/ACME U [A01]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "SRDM one-to-many update documented; no third-party transport [A01] [NSH03]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"SHARP "} {
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
	{"BP-", "BP"},
	{"MX-", "MX"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
