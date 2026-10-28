// Package xerox is the Xerox vendor plugin (IANA enterprise 253).
//
// It is a skeleton in the sense of docs/adding-a-driver.md and the plugin
// honesty rule: it implements the match rule, identity enrichment from the
// standard groups and a read-only configuration document, and declares
// nothing else, because the research did not surface documented, concrete
// Xerox wire interfaces for counters, configuration or firmware (see the
// evidence table below and internal/plugins/README.md).
//
// Match rule: sysObjectID enterprise 253; a leading "Xerox" in
// sysDescr / hrDeviceDescr when no enterprise is reported; IPP MFG "Xerox" / "XEROX".
//
// Capability / evidence table:
//
//	identify         documented   enterprise 253; AltaLink C8200 specification explicitly names RFC 2790 / RFC 3805 and SNMPv1/v2c/v3
//	read_config      documented   identity.* (SNMP system group) + vendor.xerox.badge / family / sysObjectId
//	read_status      unsupported  served by the standards drivers; AltaLink documents RFC 3805 / RFC 2790 explicitly, so the standards drivers are the baseline
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_counters    unsupported  no Xerox private counter objects documented in the research; AltaLink meters are read through EWS / clone data
//	read_security    unsupported  EWS / secure-installation guides describe settings; no documented read API [X02] [X06]
//	write_config     unsupported  clone files are a vendor-tool path with cross-version exceptions [X02]
//	write_security   unsupported  as write_config
//	manage_certs     unsupported  SCEP/EST/ACME not found in the shared AltaLink administrator guide (U) [X02]
//	firmware_update  unsupported  .dlm delivery is documented through EWS / Fleet Orchestrator / FTP directory; no third-party upload contract [X02]
package xerox

import (
	"strings"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// Name is the registry name.
const Name = "xerox"

// Enterprise is the vendor's IANA enterprise number.
const Enterprise = 253

// Plugin is the Xerox vendor plugin.
type Plugin struct {
	*vendorkit.Base
}

// New returns the plugin.
func New(opts vendorkit.Options) *Plugin {
	return &Plugin{Base: vendorkit.New(vendorkit.Spec{
		Name:             Name,
		Vendor:           "Xerox",
		Enterprises:      []int{Enterprise},
		Badges:           nil,
		IPPManufacturers: []string{"Xerox", "XEROX"},
		Capabilities:     []model.Capability{model.CapIdentify, model.CapReadConfig},
		Evidence:         evidence,
		Families:         families,
		Management:       management,
		Notes:            "VersaLink is a separate qualification track from AltaLink; the Lexmark acquisition does not merge their controllers [M02].",
		Family:           family,
	}, opts)}
}

var families = []string{"AltaLink C8130/C8135/C8145/C8155/C8170 (XE01)", "AltaLink B8145/B8155/B8170 (XE03)", "AltaLink C8230-C8270 (XE02)", "AltaLink B8245/B8255/B8270 (XE04)", "VersaLink C620/B620 (XE05)", "VersaLink C625/B625/C415/B415 (XE06)", "VersaLink C405/B405, C7120-C7130, B7100 [X06]", "PrimeLink C9265/C9275/C9281 (Q04)"}

var management = vendorkit.ManagementPaths{
	Read:          "SNMP and EWS; AltaLink specification names SNMPv1/v2c/v3, RFC 2790 and RFC 3805",
	Configuration: "EWS clone files / Fleet Orchestrator (AltaLink B/C81xx, B/C82xx)",
	Firmware:      ".dlm upgrades through EWS / Fleet Orchestrator / scheduled FTP directory, restricted downgrades",
	AdminAPI:      "EIP administrative coverage and SDK access unverified per operation",
	Sources:       []string{"X02", "X03"},
}

var evidence = []vendorkit.CapabilityEvidence{
	{Capability: model.CapIdentify, Status: vendorkit.EvidenceDocumented, Note: "enterprise 253; AltaLink C8200 specification explicitly names RFC 2790 / RFC 3805 and SNMPv1/v2c/v3", Sources: []string{"X01", "X02", "X03", "NXE01"}},
	{Capability: model.CapReadConfig, Status: vendorkit.EvidenceDocumented, Note: "identity.* from the SNMP system group plus vendor.xerox.badge / family / sysObjectId"},
	{Capability: model.CapReadStatus, Status: vendorkit.EvidenceUnsupported, Note: "AltaLink documents RFC 3805 / RFC 2790 explicitly, so the standards drivers are the baseline"},
	{Capability: model.CapReadSupplies, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadTrays, Status: vendorkit.EvidenceUnsupported, Note: "served by the standards drivers"},
	{Capability: model.CapReadCounters, Status: vendorkit.EvidenceUnsupported, Note: "no Xerox private counter objects documented in the research; AltaLink meters are read through EWS / clone data"},
	{Capability: model.CapReadSecurity, Status: vendorkit.EvidenceUnsupported, Note: "EWS / secure-installation guides describe settings; no documented read API [X02] [X06]"},
	{Capability: model.CapWriteConfig, Status: vendorkit.EvidenceUnsupported, Note: "clone files are a vendor-tool path with cross-version exceptions [X02]"},
	{Capability: model.CapWriteSecurity, Status: vendorkit.EvidenceUnsupported, Note: "as write_config"},
	{Capability: model.CapManageCerts, Status: vendorkit.EvidenceUnsupported, Note: "SCEP/EST/ACME not found in the shared AltaLink administrator guide (U) [X02]"},
	{Capability: model.CapFirmwareUpdate, Status: vendorkit.EvidenceUnsupported, Note: ".dlm delivery is documented through EWS / Fleet Orchestrator / FTP directory; no third-party upload contract [X02]"},
}

// family derives the product family from the model text.
func family(m string) string {
	u := strings.ToUpper(strings.TrimSpace(m))
	for _, prefix := range []string{"XEROX "} {
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
	{"ALTALINK", "AltaLink"},
	{"VERSALINK", "VersaLink"},
	{"PRIMELINK", "PrimeLink"},
	{"WORKCENTRE", "WorkCentre"},
	{"PHASER", "Phaser"},
	{"COLORQUBE", "ColorQube"},
}

var _ engine.Driver = (*Plugin)(nil)
var _ engine.Identifier = (*Plugin)(nil)
var _ engine.ConfigReader = (*Plugin)(nil)
