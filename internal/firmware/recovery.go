package firmware

import "strings"

// Generic recovery guidance that applies to every vendor. It is deliberately
// conservative: PrintMarshal cannot un-brick a device, and a wrong step can
// make an interrupted update worse.
const genericRecovery = `If an update fails or is interrupted:
1. Do not power-cycle the device while its panel or status LED shows an update, flash or reboot in progress. Many devices take 10-30 minutes to reboot after accepting an image.
2. Re-poll the device from PrintMarshal once it answers again and compare the reported version with the package version. A device that still reports the old version usually rejected the image and is safe to retry.
3. Retry once with the same package only after the device is idle and reachable. Repeated failures point at an incompatible package, a bad download or a device-side problem; stop and investigate.
4. To roll back, create a campaign with the previous package and "allow downgrade" enabled, if the vendor permits downgrades. Not every vendor does.
5. If the device never comes back or shows a recovery/error screen, use the vendor's documented recovery procedure (often a USB or service-mode flash) or contact vendor support. Keep the device powered and on the network while you wait.`

// vendorRecovery holds vendor-specific pointers. Keep entries honest: only
// state what is documented by the vendor or exercised here.
var vendorRecovery = []struct {
	match string
	notes string
}{
	{"sim", "Simulator (pm-sim): the device is a process. Restart pm-sim with -firmware <version> to reset the reported version; no recovery procedure is needed."},
	{"acme", "Simulator fixture vendor (pm-sim): restart pm-sim with -firmware <version> to reset the reported version."},
}

// RecoveryNotes returns rollback/recovery guidance for a vendor (spec §8.6
// "documented rollback/recovery guidance"). Unknown vendors get the generic
// guidance plus an explicit pointer to the vendor's service mode.
func RecoveryNotes(vendor string) string {
	v := strings.ToLower(strings.TrimSpace(vendor))
	for _, r := range vendorRecovery {
		if v != "" && strings.Contains(v, r.match) {
			return genericRecovery + "\n\nVendor note: " + r.notes
		}
	}
	name := vendor
	if strings.TrimSpace(name) == "" {
		name = "this vendor"
	}
	return genericRecovery + "\n\nVendor note: PrintMarshal has no verified recovery procedure for " + name +
		". Use the vendor's service mode or documented recovery flash, or contact vendor support; do not guess."
}
