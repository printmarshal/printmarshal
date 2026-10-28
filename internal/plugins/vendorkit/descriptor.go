// Package vendorkit is the shared scaffolding for vendor plugins (spec §6,
// roadmap phase 7). A plugin is an engine.Driver that
//
//   - matches only its own manufacturer's devices (SNMP sysObjectID enterprise
//     number, sysDescr / hrDeviceDescr badge text, or the IPP printer-device-id
//     MFG field) and returns ErrNotVendor for everything else;
//   - enriches identity with the manufacturer's own objects where those are
//     documented or were observed on real hardware;
//   - serves a read-only configuration document (identity.* from the SNMP
//     system group plus vendor.<name>.* facts) that vendor packages extend.
//
// Nothing in the core imports this package; the core only sees engine.Driver.
package vendorkit

import "printmarshal/internal/model"

// Evidence classifies how a capability's implementation is backed.
type Evidence string

const (
	// EvidenceDocumented means the research report or primary vendor
	// documentation describes the interface the implementation uses.
	EvidenceDocumented Evidence = "documented"
	// EvidenceObserved means the objects/endpoints were exercised read-only
	// against real hardware by this build and the values cross-checked.
	EvidenceObserved Evidence = "observed"
	// EvidenceUnverified means the implementation relies on community MIB
	// knowledge that the research pass could not confirm; the code is guarded
	// by a runtime probe and only reports when the device answers sensibly.
	EvidenceUnverified Evidence = "unverified"
	// EvidenceUnsupported means the plugin does not implement the capability
	// because no documented, concrete interface exists for it.
	EvidenceUnsupported Evidence = "unsupported"
)

// CapabilityEvidence is one row of a plugin's capability / evidence matrix.
type CapabilityEvidence struct {
	Capability model.Capability `json:"capability"`
	Status     Evidence         `json:"status"`
	Note       string           `json:"note,omitempty"`
	// Sources are research source ids ([Rxx], [H11], [NHP05] ...).
	Sources []string `json:"sources,omitempty"`
}

// ManagementPaths summarises the documented vendor management routes from
// the research catalogue (management_profiles): what reads, configures and
// updates the device through the vendor's own tools. They describe the
// vendor's documented paths, not what PrintMarshal implements.
type ManagementPaths struct {
	Read          string   `json:"read,omitempty"`
	Configuration string   `json:"configuration,omitempty"`
	Firmware      string   `json:"firmware,omitempty"`
	AdminAPI      string   `json:"adminApi,omitempty"`
	Sources       []string `json:"sources,omitempty"`
}

// Descriptor describes a plugin for /api/drivers and the README matrix.
type Descriptor struct {
	Name         string               `json:"name"`
	Vendor       string               `json:"vendor"`
	Enterprises  []int                `json:"enterprises"`
	Badges       []string             `json:"badges"`
	Families     []string             `json:"families,omitempty"`
	Capabilities []CapabilityEvidence `json:"capabilities"`
	Management   ManagementPaths      `json:"management"`
	Notes        string               `json:"notes,omitempty"`
}

// Declared returns the capabilities whose evidence status is not unsupported,
// i.e. the set the plugin actually registers with the engine.
func (d Descriptor) Declared() []model.Capability {
	var out []model.Capability
	for _, c := range d.Capabilities {
		if c.Status != EvidenceUnsupported {
			out = append(out, c.Capability)
		}
	}
	return out
}
