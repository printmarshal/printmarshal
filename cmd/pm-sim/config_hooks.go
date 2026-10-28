package main

// Configuration-write simulation (spec §13): Set-Printer-Attributes over IPP
// (RFC 3380) and SNMP SetRequest for the standard read-write objects, so the
// configuration feature can be exercised end to end without touching real
// hardware. Everything here attaches through the hooks in main.go.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
)

// settableAttrs are the printer attributes the simulated device lets a client
// set, published as printer-settable-attributes-supported (RFC 3380 §6.1).
var settableAttrs = []string{
	"printer-location", "printer-info", "printer-organization", "printer-organizational-unit",
	"copies-default", "sides-default", "print-color-mode-default", "printer-resolution-default", "media-default",
	"document-format-default",
}

// snmpWritableOIDs are the standard read-write objects the SNMP agent accepts a
// SetRequest for (RFC 3418 system group, RFC 3805 prtGeneralTable).
var snmpWritableOIDs = map[string]gosnmp.Asn1BER{
	snmp.OIDSysName:                      gosnmp.OctetString,
	snmp.OIDSysLocation:                  gosnmp.OctetString,
	snmp.OIDSysContact:                   gosnmp.OctetString,
	snmp.OIDPrtGeneralPrinterName + ".1": gosnmp.OctetString,
	snmp.OIDPrtInputDefaultIndex + ".1":  gosnmp.Integer,
	snmp.OIDPrtMarkerDefaultIndex + ".1": gosnmp.Integer,
}

func init() {
	// Seed the writable defaults and advertise Set-Printer-Attributes. HTTPHooks
	// run once with the state before the IPP server starts serving.
	HTTPHooks = append(HTTPHooks, func(_ *http.ServeMux, state *SimState) { seedConfig(state) })
	IPPHooks = append(IPPHooks, setPrinterAttributes)
	SNMPSetHook = snmpSet
}

func strs(v ...string) []any {
	out := make([]any, len(v))
	for i, s := range v {
		out[i] = s
	}
	return out
}

func resolution(dpi int) []byte {
	b := make([]byte, 9)
	b[2], b[3] = byte(dpi>>8), byte(dpi)
	b[6], b[7] = byte(dpi>>8), byte(dpi)
	b[8] = 3 // dpi
	return b
}

// seedConfig installs the attributes a configuration read expects. The fixture
// has no notion of these, so they live in the override map from the start.
func seedConfig(state *SimState) {
	state.Lock()
	defer state.Unlock()
	state.Fixture.Writable = true
	if got, _ := state.Fixture.Get(context.Background(), []string{snmp.OIDSysContact}); len(got) == 1 && got[0].Type == gosnmp.NoSuchInstance {
		state.Fixture.Put(snmp.OIDSysContact, "Facilities <facilities@example.com>")
	}
	ops := []any{0x0002, 0x0004, 0x0008, 0x0009, 0x000A, 0x000B, int(ipp.OpSetPrinterAttributes)}
	sets := strs(settableAttrs...)
	defaults := []ipp.Attribute{
		{Tag: ipp.TagEnum, Name: "operations-supported", Values: ops},
		{Tag: ipp.TagKeyword, Name: "printer-settable-attributes-supported", Values: sets},
		{Tag: ipp.TagText, Name: "printer-info", Values: strs("Simulated printer")},
		{Tag: ipp.TagText, Name: "printer-organization", Values: strs("")},
		{Tag: ipp.TagText, Name: "printer-organizational-unit", Values: strs("")},
		{Tag: ipp.TagInteger, Name: "copies-default", Values: []any{1}},
		{Tag: ipp.TagRange, Name: "copies-supported", Values: []any{[2]int{1, 99}}},
		{Tag: ipp.TagKeyword, Name: "sides-default", Values: strs("one-sided")},
		{Tag: ipp.TagKeyword, Name: "sides-supported", Values: strs("one-sided", "two-sided-long-edge", "two-sided-short-edge")},
		{Tag: ipp.TagKeyword, Name: "print-color-mode-default", Values: strs("auto")},
		{Tag: ipp.TagKeyword, Name: "print-color-mode-supported", Values: strs("auto", "color", "monochrome")},
		{Tag: ipp.TagResolution, Name: "printer-resolution-default", Values: []any{resolution(600)}},
		{Tag: ipp.TagResolution, Name: "printer-resolution-supported", Values: []any{resolution(300), resolution(600), resolution(1200)}},
		{Tag: ipp.TagMime, Name: "document-format-default", Values: strs("application/pdf")},
		{Tag: ipp.TagMime, Name: "document-format-supported", Values: strs("application/pdf", "image/pwg-raster", "application/octet-stream")},
		{Tag: ipp.TagKeyword, Name: "media-default", Values: strs("iso_a4_210x297mm")},
		{Tag: ipp.TagKeyword, Name: "media-supported", Values: strs("iso_a4_210x297mm", "na_letter_8.5x11in", "iso_a5_148x210mm")},
		{Tag: ipp.TagInteger, Name: "multiple-operation-time-out", Values: []any{120}},
		{Tag: ipp.TagLanguage, Name: "natural-language-configured", Values: strs("en")},
		{Tag: ipp.TagURI, Name: "printer-uri-supported", Values: strs("ipp://127.0.0.1:6631/ipp/print")},
		{Tag: ipp.TagKeyword, Name: "uri-security-supported", Values: strs("none")},
	}
	for _, a := range defaults {
		if _, exists := state.Overrides[a.Name]; !exists {
			state.Overrides[a.Name] = a
		}
	}
}

// setPrinterAttributes answers Set-Printer-Attributes (RFC 3380 §4.1). Settable
// attributes update the overrides (and, for the name/location, the SNMP image
// so both protocols agree); anything else is echoed in the unsupported group
// with client-error-attributes-not-settable.
func setPrinterAttributes(req *ipp.Message, state *SimState) *ipp.Message {
	if req.StatusCode != ipp.OpSetPrinterAttributes {
		return nil
	}
	resp := &ipp.Message{Version: [2]byte{2, 0}, RequestID: req.RequestID, StatusCode: ipp.StatusOK}
	ops := ipp.Group{Tag: ipp.TagOperation, Attributes: []ipp.Attribute{
		{Tag: ipp.TagCharset, Name: "attributes-charset", Values: []any{"utf-8"}},
		{Tag: ipp.TagLanguage, Name: "attributes-natural-language", Values: []any{"en"}},
	}}
	resp.Groups = []ipp.Group{ops}
	settable := map[string]bool{}
	for _, a := range settableAttrs {
		settable[a] = true
	}
	var unsupported []ipp.Attribute
	state.Lock()
	defer state.Unlock()
	for _, g := range req.Groups {
		if g.Tag != ipp.TagPrinter {
			continue
		}
		for _, a := range g.Attributes {
			if !settable[a.Name] || len(a.Values) == 0 {
				unsupported = append(unsupported, ipp.Attribute{Tag: ipp.TagUnsupportedValue, Name: a.Name, Values: []any{""}})
				continue
			}
			state.Overrides[a.Name] = a
			switch a.Name {
			case "printer-location":
				state.Fixture.Put(snmp.OIDSysLocation, fmt.Sprint(a.Values[0]))
			}
		}
	}
	if len(unsupported) > 0 {
		resp.StatusCode = ipp.StatusClientErrorAttrsNotSettable
		resp.Groups = append(resp.Groups, ipp.Group{Tag: ipp.TagUnsupported, Attributes: unsupported})
	}
	return resp
}

// snmpSet applies a SetRequest to the standard read-write objects. Unknown or
// read-only OIDs fail the whole request with notWritable, as RFC 3416 requires.
func snmpSet(state *SimState, vars []gosnmp.SnmpPDU) ([]gosnmp.SnmpPDU, gosnmp.SNMPError) {
	state.Lock()
	defer state.Unlock()
	for _, v := range vars {
		typ, ok := snmpWritableOIDs[v.Name]
		if !ok {
			return vars, gosnmp.NotWritable
		}
		if v.Type != typ {
			return vars, gosnmp.WrongType
		}
	}
	for _, v := range vars {
		switch val := v.Value.(type) {
		case []byte:
			state.Fixture.Put(v.Name, string(val))
			if v.Name == snmp.OIDSysLocation {
				delete(state.Overrides, "printer-location") // IPP re-derives it from the image
			}
		case string:
			state.Fixture.Put(v.Name, val)
		case int:
			state.Fixture.Put(v.Name, val)
		default:
			return vars, gosnmp.WrongType
		}
	}
	return vars, gosnmp.NoError
}
