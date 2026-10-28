package engine

import "printmarshal/internal/model"

// ProfileSupports is the standard Supports() rule for a driver: it serves cap
// when the device's resolved profile assigns cap to it, or when the profile has
// no driver for cap but this driver did identify the device (so a later probe
// may still succeed).
func ProfileSupports(d *model.Device, driver string, cap model.Capability) bool {
	if d == nil {
		return false
	}
	if d.Capabilities != nil {
		if by, ok := d.Capabilities[cap]; ok {
			return by == driver
		}
	}
	for _, m := range d.IdentifyMethods {
		if m == driver {
			return true
		}
	}
	return false
}
