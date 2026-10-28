// Package plugins registers vendor plugins with the driver registry. The core
// has no compile-time knowledge of any vendor beyond this list; each plugin
// lives in its own sub-package and implements engine.Driver plus the
// capability interfaces it can serve (docs/adding-a-driver.md).
//
// Plugins are ordered by the research's support priority (Ricoh first).
// Each one matches only its own manufacturer (SNMP enterprise number, badge
// text or IPP manufacturer) and returns vendorkit.ErrNotVendor otherwise, so
// registering all of them is safe for any fleet.
package plugins

import (
	"log/slog"

	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/plugins/brother"
	"printmarshal/internal/plugins/canon"
	"printmarshal/internal/plugins/epson"
	"printmarshal/internal/plugins/fujifilm"
	"printmarshal/internal/plugins/hp"
	"printmarshal/internal/plugins/konicaminolta"
	"printmarshal/internal/plugins/kyocera"
	"printmarshal/internal/plugins/lexmark"
	"printmarshal/internal/plugins/oki"
	"printmarshal/internal/plugins/ricoh"
	"printmarshal/internal/plugins/sharp"
	"printmarshal/internal/plugins/toshiba"
	"printmarshal/internal/plugins/vendorkit"
	"printmarshal/internal/plugins/xerox"
)

// Plugin is what every vendor package exports: an engine.Driver with a descriptor.
type Plugin interface {
	engine.Driver
	Descriptor() vendorkit.Descriptor
}

// Options configure the transports the plugins use. Zero values select the
// production SNMP dialer, IPP transport and read-only HTTP client.
type Options struct {
	SNMP snmp.Options
	IPP  ipp.Options
	HTTP vendorkit.HTTPOptions
}

// DefaultOptions returns LAN-suitable defaults.
func DefaultOptions() Options {
	return Options{SNMP: snmp.DefaultOptions(), IPP: ipp.DefaultOptions(), HTTP: vendorkit.DefaultHTTPOptions()}
}

// New builds every plugin over the given transports, in priority order.
func New(t vendorkit.Options) []Plugin {
	return []Plugin{
		ricoh.New(t),
		hp.New(t),
		canon.New(t),
		xerox.New(t),
		konicaminolta.New(t),
		kyocera.New(t),
		brother.New(t),
		lexmark.New(t),
		epson.New(t),
		sharp.New(t),
		toshiba.New(t),
		oki.New(t),
		fujifilm.New(t),
	}
}

// Transports builds production transports from Options.
func Transports(o Options) vendorkit.Options {
	return vendorkit.Options{
		SNMP: snmp.NewDialer(o.SNMP),
		IPP:  ipp.NewTransport(o.IPP),
		HTTP: vendorkit.NewHTTP(o.HTTP),
	}
}

// RegisterAll registers every built-in vendor plugin with production transports.
func RegisterAll(reg *engine.Registry, log *slog.Logger) {
	RegisterAllWith(reg, log, Transports(DefaultOptions()))
}

// RegisterAllWith registers every plugin over explicit transports (tests, simulator).
func RegisterAllWith(reg *engine.Registry, log *slog.Logger, t vendorkit.Options) {
	for _, p := range New(t) {
		reg.Register(p)
		if log != nil {
			d := p.Descriptor()
			log.Info("vendor plugin registered", "plugin", d.Name, "vendor", d.Vendor, "enterprises", d.Enterprises, "capabilities", d.Declared())
		}
	}
}

// List returns the descriptors of every built-in plugin (for /api/drivers).
func List() []vendorkit.Descriptor {
	var out []vendorkit.Descriptor
	for _, p := range New(vendorkit.Options{}) {
		out = append(out, p.Descriptor())
	}
	return out
}
