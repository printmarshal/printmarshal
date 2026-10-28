// Package kyocera is the Kyocera vendor plugin (IANA enterprise 1347).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Kyocera wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 1347; a leading "Kyocera" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "KYOCERA" / "Kyocera" / "KYOCERA Document Solutions".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 1347; TASKalfa 3554ci / 7054ci sheets document SNMPv1/v2c/v3, MA4500ifx names Printer MIB
//	read_config      documented   identity.* (SNMP system group) + vendor.kyocera.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; MA4500ifx sheet names Printer MIB; complete RFC 3805 conformance per TASKalfa not established [Y07]
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  no Kyocera private counter objects documented in the research
//	read_security    unsupported  Command Center RX only; SCEP, TPM, secure boot documented as features without a read API [Y01] [NKY01]
//	write_config     unsupported  Command Center RX / Net Viewer / KFS are vendor tools [Y08]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  SCEP is documented on 3554ci / 7054ci / MZ as a device feature; EST/ACME U; no enrolment API [Y01] [Y04]
//	firmware_update  unsupported  Net Viewer can update; transport/API unverified; the IB-5x Software Upgrade Tool cannot update other devices [Y04] [Y09]
package kyocera

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "kyocera"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 1347

// Plugin is the Kyocera vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Kyocera",
		Enterprises:      []int{Enterprise},
		Badges:           nil,
		IPPManufacturers: []string{"KYOCERA", "Kyocera", "KYOCERA Document Solutions"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "Regional MA/PA suffixes are preserved in the model text.",
		Family:           family,
	}, opts)}
}

var families = []string{"TASKalfa 2554ci-7054ci (KY01)", "TASKalfa 5004i-9003i (KY02)", "TASKalfa MZ2501ci-MZ7001ci (KY03)", "TASKalfa MZ5001i-MZ7001i (KY04)", "ECOSYS PA4500x-PA6000x (KY05)", "ECOSYS MA4500ix/ifx, MA5500ifx, MA6000ifx (KY06)", "ECOSYS MA4000cix (KY07)"}

var management = vendorkit.ManagementPaths{
	Read:          "SNMP and Command Center RX; 7054ci / 3554ci sheets list SNMPv1/v2c/v3",
	Configuration: "Command Center RX, Net Viewer / Device Manager / KFS; HyPAS is an app platform, not an admin REST",
	Firmware:      "Net Viewer monitors/updates locally and remotely; package/transport unverified",
	AdminAPI:      "No unrestricted device administrative REST contract verified",
	Sources:       []string{"NKY01", "NKY03", "Y07", "Y08"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 1347; TASKalfa 3554ci / 7054ci sheets document SNMPv1/v2c/v3, MA4500ifx names Printer MIB", Sources: []string{"Y01", "Y07", "NKY01"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.kyocera.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "MA4500ifx sheet names Printer MIB; complete RFC 3805 conformance per TASKalfa not established [Y07]"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "no Kyocera private counter objects documented in the research"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "Command Center RX only; SCEP, TPM, secure boot documented as features without a read API [Y01] [NKY01]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "Command Center RX / Net Viewer / KFS are vendor tools [Y08]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "SCEP is documented on 3554ci / 7054ci / MZ as a device feature; EST/ACME U; no enrolment API [Y01] [Y04]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: "Net Viewer can update; transport/API unverified; the IB-5x Software Upgrade Tool cannot update other devices [Y04] [Y09]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"KYOCERA ", "KYOCERA DOCUMENT SOLUTIONS "} {
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
	{"TASKALFA", "TASKalfa"},
	{"ECOSYS", "ECOSYS"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
