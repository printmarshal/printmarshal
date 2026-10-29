package model

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// The web page of a device (issue #7): the embedded administration UI an
// operator wants one click away. Three sources feed it, in order:
//
//   1. WebURLOverride — set by an operator, never touched by a scan.
//   2. AdminURL — what the device advertises (IPP printer-more-info, eSCL
//      AdminURI, a DNS-SD "adminurl" record), when it looks like a normal
//      web UI rather than a protocol endpoint.
//   3. WebURLProbed — what a probe of https:// then http:// on the device
//      found; WebURLNone records that the probe found nothing, so the same
//      device is not probed on every poll.
//
// WebURL is the resolved result, filled when a device is read from the store.

// WebURLNone is stored in WebURLProbed when a probe found no web server.
const WebURLNone = "none"

// ErrWebURL is returned by ValidWebURL for a URL an operator may not store.
var ErrWebURL = errors.New("web URL must be http or https, with a host and no credentials")

// ResolveWebURL returns the link to show for d, or "" when there is none.
func ResolveWebURL(d *Device) string {
	if d.WebURLOverride != "" {
		return d.WebURLOverride
	}
	if IsWebUI(d.AdminURL) {
		return d.AdminURL
	}
	if d.WebURLProbed != "" && d.WebURLProbed != WebURLNone {
		return d.WebURLProbed
	}
	return ""
}

// webUIPortsToSkip are ports a device advertises for a protocol, not a page:
// IPP (631), LPD (515) and raw print (9100) answer HTTP, if at all, with a
// status page that is not the administration UI.
var webUIPortsToSkip = map[string]bool{"631": true, "515": true, "9100": true}

// IsWebUI reports whether raw is an http(s) URL that is likely a device's
// administration page rather than a protocol endpoint.
func IsWebUI(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return false
	}
	if webUIPortsToSkip[u.Port()] {
		return false
	}
	// A path that names IPP or eSCL is a protocol endpoint, whatever the port.
	p := strings.ToLower(u.Path)
	return !strings.HasPrefix(p, "/ipp") && !strings.HasPrefix(p, "/escl") && !strings.HasPrefix(p, "/escl/")
}

// ValidWebURL checks a URL an operator wants stored as a device's web page.
// Empty clears the override. Credentials in the URL are refused outright: the
// link is opened by browsers and copied into tickets.
func ValidWebURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ErrWebURL
	}
	return nil
}

// WebHost is the host a device's web page should be reached at: the DNS or
// mDNS hostname when one is known, else the address. IPv6 addresses are
// bracketed so they can carry a port.
func WebHost(d *Device) string {
	h := strings.TrimSuffix(strings.TrimSpace(d.Hostname), ".")
	if h == "" {
		h = d.Address
	}
	if ip := net.ParseIP(h); ip != nil && ip.To4() == nil && !strings.HasPrefix(h, "[") {
		return "[" + h + "]"
	}
	return h
}
