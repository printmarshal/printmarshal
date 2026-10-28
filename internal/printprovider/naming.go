package printprovider

import (
	"net"
	"strings"

	"printmarshal/internal/model"
)

// maxNameLen bounds the sanitised part of a queue name; CUPS allows 127
// characters in total and the prefix is at most 32.
const maxNameLen = 63

// Sanitize turns a display name into a CUPS-safe queue name: lower-case
// letters, digits, '_' and '-', runs of anything else collapsed to one '-',
// no leading/trailing '-', at most maxNameLen characters. It returns "" when
// nothing usable remains.
func Sanitize(name string) string {
	var b strings.Builder
	dash := true // suppress a leading dash
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
			dash = false
		default:
			if !dash {
				b.WriteByte('-')
				dash = true
			}
		}
		if b.Len() >= maxNameLen {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

// QueueName derives a device's queue name: prefix + sanitised display name
// (falling back to sysName, hostname, model, then a short id). Collision
// suffixes are added by the planner, not here.
func QueueName(prefix string, d *model.Device) string {
	for _, cand := range []string{d.DisplayName, d.SysName, d.Hostname, d.Model} {
		if s := Sanitize(cand); s != "" {
			return prefix + s
		}
	}
	id := d.ID
	if len(id) > 8 {
		id = id[:8]
	}
	if s := Sanitize(id); s != "" {
		return prefix + "printer-" + s
	}
	return prefix + "printer"
}

// DeviceURI is the IPP Everywhere endpoint for a device. IPP over TLS (ipps)
// is not inferred: CUPS negotiates encryption itself when the printer offers it.
func DeviceURI(d *model.Device) string {
	host := d.Address
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		host = "[" + host + "]"
	}
	return "ipp://" + host + ":631/ipp/print"
}

// HasIPP reports whether the device's capability profile is served (in any
// part) by the IPP driver — the devices a driverless queue can print to.
func HasIPP(d *model.Device) bool {
	for _, drv := range d.Capabilities {
		if drv == "ipp" {
			return true
		}
	}
	for _, m := range d.IdentifyMethods {
		if m == "ipp" {
			return true
		}
	}
	return false
}

// AdvertisesDNSSD reports whether the device was found by DNS-SD (mDNS), the
// usual way an IPP Everywhere printer announces _ipp._tcp / _ipps._tcp.
func AdvertisesDNSSD(d *model.Device) bool {
	for _, m := range d.DiscoveryMethods {
		if m == "mdns" {
			return true
		}
	}
	return false
}
