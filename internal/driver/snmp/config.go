package snmp

// Configuration read/write over the standard MIBs (spec §8.4).
//
// SNMP exposes very little configuration in a vendor-neutral way, so this file
// maps only the read-write objects every compliant agent has:
//
//	sysName               RFC 3418 SNMPv2-MIB  -> identity.hostname
//	sysLocation           RFC 3418 SNMPv2-MIB  -> identity.location
//	sysContact            RFC 3418 SNMPv2-MIB  -> identity.contact
//	prtGeneralPrinterName RFC 3805 Printer MIB -> identity.printerName
//	prtInputDefaultIndex  RFC 3805 Printer MIB -> trays.defaultTray (as the tray's prtInputName)
//	prtMarkerDefaultIndex RFC 3805 Printer MIB -> print.defaultMarkerIndex
//
// Everything else in the catalogue that SNMP cannot express is simply absent
// from the document; the service merges a richer driver (IPP, vendor plugin)
// on top. Writes use one SetRequest per setting so results are per setting,
// and a dry run issues no SetRequest at all.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// ReadConfig implements engine.ConfigReader.
func (d *Driver) ReadConfig(ctx context.Context, dev *model.Device, creds model.Creds, sel model.ConfigSelector) (*model.ConfigDocument, error) {
	s, variant, err := d.dial(ctx, dev.Address, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	doc, err := ReadConfig(ctx, s, variant)
	if err != nil {
		return nil, err
	}
	doc.DeviceID = dev.ID
	doc.Settings = FilterSettings(doc.Settings, sel)
	return doc, nil
}

// ReadConfig builds a configuration document from a session (exported for
// tests and the simulator). variant is "v2c" or "v3" as reported by the dialer.
func ReadConfig(ctx context.Context, s Session, variant string) (*model.ConfigDocument, error) {
	doc := &model.ConfigDocument{Settings: []model.Setting{}, ReadAt: time.Now().UTC(), Sources: []string{Name}}
	sys, err := s.Get(ctx, []string{OIDSysName, OIDSysLocation, OIDSysContact})
	if err != nil {
		return nil, err
	}
	for _, p := range sys {
		if absent(p) {
			continue
		}
		switch p.OID {
		case OIDSysName:
			doc.Settings = append(doc.Settings, setting("identity.hostname", Str(p), true, ""))
		case OIDSysLocation:
			doc.Settings = append(doc.Settings, setting("identity.location", Str(p), true, ""))
		case OIDSysContact:
			doc.Settings = append(doc.Settings, setting("identity.contact", Str(p), true, ""))
		}
	}
	idx := generalIndex(ctx, s)
	if idx != "" {
		gen, err := s.Get(ctx, []string{OIDPrtGeneralPrinterName + "." + idx, OIDPrtInputDefaultIndex + "." + idx, OIDPrtMarkerDefaultIndex + "." + idx})
		if err == nil {
			for _, p := range gen {
				if absent(p) {
					continue
				}
				switch {
				case strings.HasPrefix(p.OID, OIDPrtGeneralPrinterName+"."):
					doc.Settings = append(doc.Settings, setting("identity.printerName", Str(p), true, ""))
				case strings.HasPrefix(p.OID, OIDPrtInputDefaultIndex+"."):
					if n, ok := Int(p); ok {
						doc.Settings = append(doc.Settings, setting("trays.defaultTray", trayName(ctx, s, n), true, ""))
					}
				case strings.HasPrefix(p.OID, OIDPrtMarkerDefaultIndex+"."):
					if n, ok := Int(p); ok {
						doc.Settings = append(doc.Settings, setting("print.defaultMarkerIndex", int(n), true, ""))
					}
				}
			}
		}
	}
	// What we can observe about the SNMP service itself: the version that answered.
	switch variant {
	case "v2c":
		doc.Settings = append(doc.Settings, setting("protocols.snmp.v1v2cEnabled", true, false, "observed: the device answered SNMPv2c"))
	case "v3":
		doc.Settings = append(doc.Settings, setting("protocols.snmp.v3Enabled", true, false, "observed: the device answered SNMPv3"))
	}
	return doc, nil
}

// generalIndex finds the hrDeviceIndex used by the prtGeneralTable ("" if none).
func generalIndex(ctx context.Context, s Session) string {
	for _, col := range []string{OIDPrtGeneralPrinterName, OIDPrtGeneralSerialNumber, OIDPrtInputDefaultIndex} {
		if rows, err := s.Walk(ctx, col); err == nil && len(rows) > 0 {
			return strings.TrimPrefix(rows[0].OID, col+".")
		}
	}
	return ""
}

// trayName renders a prtInputDefaultIndex as the tray's prtInputName, falling
// back to the bare index when the table has no name.
func trayName(ctx context.Context, s Session, index int64) string {
	names, _ := s.Walk(ctx, OIDPrtInputName)
	for _, p := range names {
		if lastIndex(strings.TrimPrefix(p.OID, OIDPrtInputName+".")) == int(index) {
			if n := Str(p); n != "" {
				return n
			}
		}
	}
	return strconv.FormatInt(index, 10)
}

// trayIndex resolves a desired trays.defaultTray value (a prtInputName or a
// bare index) to the prtInputTable index.
func trayIndex(ctx context.Context, s Session, want string) (int, error) {
	want = strings.TrimSpace(want)
	names, _ := s.Walk(ctx, OIDPrtInputName)
	for _, p := range names {
		if strings.EqualFold(Str(p), want) {
			return lastIndex(strings.TrimPrefix(p.OID, OIDPrtInputName+".")), nil
		}
	}
	if n, err := strconv.Atoi(want); err == nil && n > 0 {
		return n, nil
	}
	return 0, fmt.Errorf("no input tray named %q", want)
}

func setting(key string, value any, writable bool, reason string) model.Setting {
	def := model.CatalogByKey[key]
	return model.Setting{Key: key, Area: def.Area, Name: def.Name, Type: def.Type, Options: def.Options,
		Value: value, Writable: writable, Source: Name, ReadOnlyReason: reason}
}

// FilterSettings applies a ConfigSelector.
func FilterSettings(in []model.Setting, sel model.ConfigSelector) []model.Setting {
	if len(sel.Areas) == 0 && len(sel.Keys) == 0 {
		return in
	}
	out := make([]model.Setting, 0, len(in))
	for _, s := range in {
		keep := false
		for _, a := range sel.Areas {
			if s.Area == a {
				keep = true
			}
		}
		for _, k := range sel.Keys {
			if s.Key == k {
				keep = true
			}
		}
		if keep {
			out = append(out, s)
		}
	}
	return out
}

// snmpWritable describes how a catalogue key is written.
type snmpWritable struct {
	oid    string // scalar OID, or a columnar OID that gets ".<generalIndex>"
	column bool
	typ    gosnmp.Asn1BER
}

var writable = map[string]snmpWritable{
	"identity.hostname":        {oid: OIDSysName, typ: gosnmp.OctetString},
	"identity.location":        {oid: OIDSysLocation, typ: gosnmp.OctetString},
	"identity.contact":         {oid: OIDSysContact, typ: gosnmp.OctetString},
	"identity.printerName":     {oid: OIDPrtGeneralPrinterName, column: true, typ: gosnmp.OctetString},
	"trays.defaultTray":        {oid: OIDPrtInputDefaultIndex, column: true, typ: gosnmp.Integer},
	"print.defaultMarkerIndex": {oid: OIDPrtMarkerDefaultIndex, column: true, typ: gosnmp.Integer},
}

// WriteConfig implements engine.ConfigWriter. The dialer picks SNMPv3 when a USM
// user is configured, else the communities in priority order, so a write
// community (or v3 user with write access) must be configured as a credential
// with the highest priority for the device.
func (d *Driver) WriteConfig(ctx context.Context, dev *model.Device, creds model.Creds, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error) {
	s, variant, err := d.dial(ctx, dev.Address, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	return WriteConfig(ctx, s, variant, dev.ID, changes, opts)
}

// WriteConfig applies changes through a session (exported for tests and the
// simulator). With opts.DryRun no SetRequest is issued: it reads the current
// values and reports what would change.
func WriteConfig(ctx context.Context, s Session, variant, deviceID string, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error) {
	current, err := ReadConfig(ctx, s, variant)
	if err != nil {
		return nil, err
	}
	res := &model.WriteResult{DeviceID: deviceID, DryRun: opts.DryRun, Results: []model.SettingResult{}, At: time.Now().UTC()}
	idx := ""
	for _, ch := range changes {
		r := model.SettingResult{Key: ch.Key, Driver: Name}
		w, ok := writable[ch.Key]
		if !ok {
			r.Status, r.Error = model.SettingSkipped, "not writable over SNMP"
			res.Results = append(res.Results, r)
			continue
		}
		cur, present := current.Get(ch.Key)
		if !present {
			r.Status, r.Error = model.SettingSkipped, "the device does not expose this object"
			res.Results = append(res.Results, r)
			continue
		}
		desired, err := model.CoerceValue(ch.Key, ch.Value)
		if err != nil {
			r.Status, r.Error = model.SettingFailed, err.Error()
			res.Results = append(res.Results, r)
			continue
		}
		r.Before = cur.Value
		if model.ValuesEqual(cur.Value, desired) {
			r.Status, r.After = model.SettingUnchanged, cur.Value
			res.Results = append(res.Results, r)
			continue
		}
		// Build the binding.
		oid := w.oid
		if w.column {
			if idx == "" {
				idx = generalIndex(ctx, s)
			}
			if idx == "" {
				r.Status, r.Error = model.SettingSkipped, "no prtGeneralTable row"
				res.Results = append(res.Results, r)
				continue
			}
			oid += "." + idx
		}
		var value any
		switch w.typ {
		case gosnmp.OctetString:
			value = []byte(fmt.Sprint(desired))
		case gosnmp.Integer:
			if ch.Key == "trays.defaultTray" {
				n, err := trayIndex(ctx, s, fmt.Sprint(desired))
				if err != nil {
					r.Status, r.Error = model.SettingFailed, err.Error()
					res.Results = append(res.Results, r)
					continue
				}
				value = n
			} else {
				value, _ = desired.(int)
			}
		}
		r.After = desired
		if opts.DryRun {
			r.Status = model.SettingWouldWrite
			res.Results = append(res.Results, r)
			continue
		}
		if err := s.Set(ctx, []PDU{{OID: oid, Type: w.typ, Value: value}}); err != nil {
			r.Status, r.Error, r.After = model.SettingFailed, err.Error(), nil
			res.Results = append(res.Results, r)
			continue
		}
		// Read back to confirm and to report the value the device now holds.
		if got, err := s.Get(ctx, []string{oid}); err == nil && len(got) == 1 && !absent(got[0]) {
			switch w.typ {
			case gosnmp.OctetString:
				r.After = Str(got[0])
			case gosnmp.Integer:
				if n, ok := Int(got[0]); ok {
					if ch.Key == "trays.defaultTray" {
						r.After = trayName(ctx, s, n)
					} else {
						r.After = int(n)
					}
				}
			}
		}
		r.Status = model.SettingWritten
		res.Rollback = append(res.Rollback, model.SettingChange{Key: ch.Key, Value: r.Before})
		res.Results = append(res.Results, r)
	}
	return res, nil
}

var _ engine.ConfigReader = (*Driver)(nil)
var _ engine.ConfigWriter = (*Driver)(nil)
