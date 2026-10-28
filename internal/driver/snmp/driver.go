// Package snmp is the standards SNMP driver (spec §7): identity from the
// system group, status from the Host Resources MIB, and supplies, trays,
// counters and alerts from the Printer MIB.
package snmp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// Name is the driver's registry name.
const Name = "snmp"

// Driver implements engine.Driver plus the read capabilities.
type Driver struct {
	dial Dialer
}

// New returns a driver that opens sessions through dial.
func New(dial Dialer) *Driver { return &Driver{dial: dial} }

// Name implements engine.Driver.
func (d *Driver) Name() string { return Name }

// Capabilities implements engine.Driver.
func (d *Driver) Capabilities() []model.Capability {
	return []model.Capability{
		model.CapIdentify, model.CapReadStatus, model.CapReadSupplies,
		model.CapReadTrays, model.CapReadCounters, model.CapSubscribeAlerts,
		model.CapReadConfig, model.CapWriteConfig,
	}
}

// Supports implements engine.Driver.
func (d *Driver) Supports(dev *model.Device, cap model.Capability) bool {
	return engine.ProfileSupports(dev, Name, cap)
}

// Priority implements engine.Driver. SNMP is the most universal read path.
func (d *Driver) Priority(cap model.Capability) int {
	switch cap {
	case model.CapIdentify, model.CapReadStatus, model.CapReadSupplies, model.CapReadTrays, model.CapReadCounters:
		return 50
	case model.CapReadConfig, model.CapWriteConfig:
		// IPP (40) exposes far more settings; SNMP fills in the system group.
		return 30
	}
	return 10
}

func (d *Driver) session(ctx context.Context, dev *model.Device, creds model.Creds) (Session, error) {
	s, _, err := d.dial(ctx, dev.Address, creds)
	return s, err
}

// Identify implements engine.Identifier.
func (d *Driver) Identify(ctx context.Context, address string, creds model.Creds) (*model.Identity, error) {
	s, variant, err := d.dial(ctx, address, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()

	sys, err := s.Get(ctx, []string{OIDSysDescr, OIDSysObjectID, OIDSysName, OIDSysLocation})
	if err != nil {
		return nil, err
	}
	id := &model.Identity{Address: address, Method: Name + "/" + variant}
	for _, p := range sys {
		if absent(p) {
			continue
		}
		switch p.OID {
		case OIDSysDescr:
			id.SysDescr = Str(p)
		case OIDSysObjectID:
			id.EnterpriseID = EnterpriseID(Str(p))
		case OIDSysName:
			id.SysName = Str(p)
		case OIDSysLocation:
			id.SysLocation = Str(p)
		}
	}

	// Is this a printer at all? (hrDeviceType == hrDevicePrinter, or a Printer MIB general entry.)
	isPrinter := false
	printerDescr := ""
	types, _ := s.Walk(ctx, OIDHrDeviceType)
	descrs, _ := s.Walk(ctx, OIDHrDeviceDescr)
	descrByIdx := map[string]string{}
	for _, p := range descrs {
		descrByIdx[strings.TrimPrefix(p.OID, OIDHrDeviceDescr+".")] = Str(p)
	}
	for _, p := range types {
		if Str(p) == OIDHrDevicePrinterType {
			isPrinter = true
			if printerDescr == "" {
				printerDescr = descrByIdx[strings.TrimPrefix(p.OID, OIDHrDeviceType+".")]
			}
		}
	}
	general, _ := s.Walk(ctx, OIDPrtGeneralSerialNumber)
	names, _ := s.Walk(ctx, OIDPrtGeneralPrinterName)
	if len(general) > 0 || len(names) > 0 {
		isPrinter = true
	}
	if !isPrinter {
		return nil, errors.New("responds to SNMP but is not a printer (no hrDevicePrinter / Printer MIB)")
	}
	for _, p := range general {
		if v := Str(p); v != "" {
			id.Serial = v
			break
		}
	}
	id.Make, id.Model = MakeModel(firstNonEmpty(printerDescr, id.SysDescr))
	id.DeviceType = "printer"

	if macs, err := s.Walk(ctx, OIDIfPhysAddress); err == nil {
		for _, p := range macs {
			if b, ok := p.Value.([]byte); ok && len(b) == 6 && !allZero(b) {
				id.MAC = hexColon(b)
				break
			}
		}
	}

	// The system group is always readable and (with a write community / v3 user)
	// writable, so every SNMP printer gets read_config and write_config.
	id.Capabilities = []model.Capability{model.CapIdentify, model.CapReadStatus, model.CapSubscribeAlerts, model.CapReadConfig, model.CapWriteConfig}
	if sup, err := s.Walk(ctx, OIDPrtMarkerSuppliesTable+"."+strconv.Itoa(colSuppliesLevel)); err == nil && len(sup) > 0 {
		id.Capabilities = append(id.Capabilities, model.CapReadSupplies)
	}
	if in, err := s.Walk(ctx, OIDPrtInputTable+"."+strconv.Itoa(colInputStatus)); err == nil && len(in) > 0 {
		id.Capabilities = append(id.Capabilities, model.CapReadTrays)
	}
	if lc, err := s.Walk(ctx, OIDPrtMarkerLifeCount); err == nil && len(lc) > 0 {
		id.Capabilities = append(id.Capabilities, model.CapReadCounters)
	}
	return id, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// ReadStatus implements engine.StatusReader.
func (d *Driver) ReadStatus(ctx context.Context, dev *model.Device, creds model.Creds) (*model.Status, error) {
	s, err := d.session(ctx, dev, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	return ReadStatus(ctx, s)
}

// ReadStatus builds a Status from a session (exported for tests/simulator).
func ReadStatus(ctx context.Context, s Session) (*model.Status, error) {
	st := &model.Status{Online: true, State: "unknown", Severity: model.SeverityOK, At: time.Now().UTC()}

	up, err := s.Get(ctx, []string{OIDSysUpTime})
	if err != nil {
		return nil, err
	}
	if len(up) > 0 {
		if n, ok := Int(up[0]); ok {
			st.UptimeSecs = n / 100
		}
	}

	// Find the printer device index in the Host Resources device table.
	types, _ := s.Walk(ctx, OIDHrDeviceType)
	printerIdx := ""
	for _, p := range types {
		if Str(p) == OIDHrDevicePrinterType {
			printerIdx = strings.TrimPrefix(p.OID, OIDHrDeviceType+".")
			break
		}
	}
	if printerIdx != "" {
		res, err := s.Get(ctx, []string{
			OIDHrDeviceStatus + "." + printerIdx,
			OIDHrPrinterStatus + "." + printerIdx,
			OIDHrPrinterDetectedError + "." + printerIdx,
		})
		if err == nil {
			var devStatus, prtStatus int64 = -1, -1
			for _, p := range res {
				if absent(p) {
					continue
				}
				switch {
				case strings.HasPrefix(p.OID, OIDHrPrinterStatus+"."):
					prtStatus, _ = Int(p)
				case strings.HasPrefix(p.OID, OIDHrDeviceStatus+"."):
					devStatus, _ = Int(p)
				case strings.HasPrefix(p.OID, OIDHrPrinterDetectedError+"."):
					st.Errors = ParseDetectedErrors(p)
				}
			}
			switch prtStatus { // hrPrinterStatus: 1 other, 2 unknown, 3 idle, 4 printing, 5 warmup
			case 3:
				st.State = "idle"
			case 4:
				st.State = "printing"
			case 5:
				st.State = "warmup"
			}
			switch devStatus { // hrDeviceStatus: 1 unknown, 2 running, 3 warning, 4 testing, 5 down
			case 3:
				if st.Severity == model.SeverityOK {
					st.Severity = model.SeverityWarning
				}
				if st.State == "unknown" {
					st.State = "warning"
				}
			case 5:
				st.Severity = model.SeverityCritical
				st.State = "down"
			}
		}
	}

	if cons, err := s.Walk(ctx, OIDPrtConsoleDisplayText); err == nil {
		var lines []string
		for _, p := range cons {
			if t := Str(p); t != "" {
				lines = append(lines, t)
			}
		}
		st.Message = strings.Join(lines, " / ")
	}

	if alerts, err := s.Walk(ctx, OIDPrtAlertTable); err == nil {
		st.Alerts = ParseDeviceAlerts(alerts)
	}

	// Escalate severity from error bits and device alerts.
	for _, e := range st.Errors {
		switch e {
		case "noPaper", "noToner", "doorOpen", "jammed", "offline", "serviceRequested", "markerSupplyMissing", "outputFull", "inputTrayMissing":
			st.Severity = model.SeverityCritical
		case "lowPaper", "lowToner", "outputNearFull", "inputTrayEmpty", "overduePreventMaint", "outputTrayMissing":
			if st.Severity == model.SeverityOK {
				st.Severity = model.SeverityWarning
			}
		}
	}
	for _, a := range st.Alerts {
		if a.Severity == model.SeverityCritical {
			st.Severity = model.SeverityCritical
		} else if a.Severity == model.SeverityWarning && st.Severity == model.SeverityOK {
			st.Severity = model.SeverityWarning
		}
	}
	if st.State == "unknown" && st.Severity == model.SeverityOK {
		st.State = "idle"
	}
	return st, nil
}

// ReadSupplies implements engine.SupplyReader.
func (d *Driver) ReadSupplies(ctx context.Context, dev *model.Device, creds model.Creds) ([]model.Supply, error) {
	s, err := d.session(ctx, dev, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	sup, err := s.Walk(ctx, OIDPrtMarkerSuppliesTable)
	if err != nil {
		return nil, err
	}
	col, _ := s.Walk(ctx, OIDPrtMarkerColorantValue)
	out := ParseSupplies(sup, col)
	if len(out) == 0 {
		return nil, fmt.Errorf("prtMarkerSuppliesTable is empty")
	}
	return out, nil
}

// ReadTrays implements engine.TrayReader.
func (d *Driver) ReadTrays(ctx context.Context, dev *model.Device, creds model.Creds) ([]model.Tray, error) {
	s, err := d.session(ctx, dev, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	in, err := s.Walk(ctx, OIDPrtInputTable)
	if err != nil {
		return nil, err
	}
	out, _ := s.Walk(ctx, OIDPrtOutputTable)
	trays := append(ParseInputTrays(in), ParseOutputTrays(out)...)
	if len(trays) == 0 {
		return nil, fmt.Errorf("prtInputTable is empty")
	}
	return trays, nil
}

// ReadCounters implements engine.CounterReader.
func (d *Driver) ReadCounters(ctx context.Context, dev *model.Device, creds model.Creds) (*model.Counters, error) {
	s, err := d.session(ctx, dev, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	m, err := s.Walk(ctx, OIDPrtMarkerTable)
	if err != nil {
		return nil, err
	}
	c := ParseCounters(m, time.Now().UTC())
	if c == nil {
		return nil, fmt.Errorf("prtMarkerTable is empty")
	}
	return c, nil
}

var _ engine.Identifier = (*Driver)(nil)
var _ engine.StatusReader = (*Driver)(nil)
var _ engine.SupplyReader = (*Driver)(nil)
var _ engine.TrayReader = (*Driver)(nil)
var _ engine.CounterReader = (*Driver)(nil)
