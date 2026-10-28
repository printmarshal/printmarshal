package snmp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/model"
)

// --- value helpers -------------------------------------------------------

// Str renders a PDU as text. Octet strings that are not printable are hex-encoded.
func Str(p PDU) string {
	switch v := p.Value.(type) {
	case string:
		return strings.TrimRight(v, "\x00 ")
	case []byte:
		s := strings.TrimRight(string(v), "\x00 ")
		if isPrintable(s) {
			return strings.TrimSpace(s)
		}
		return hexColon(v)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func hexColon(b []byte) string {
	parts := make([]string, len(b))
	for i, x := range b {
		parts[i] = fmt.Sprintf("%02x", x)
	}
	return strings.Join(parts, ":")
}

// Int renders a numeric PDU; ok is false for non-numeric values.
func Int(p PDU) (int64, bool) {
	switch v := p.Value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case uint8:
		return int64(v), true
	}
	return 0, false
}

// Column/index helpers for table walks. For a column OID like
// <table>.<col>.<idx...> this returns col and the remaining index string.
func splitColumn(table, oid string) (col int, index string, ok bool) {
	prefix := strings.TrimSuffix(table, ".") + "."
	if !strings.HasPrefix(oid, prefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(oid, prefix)
	parts := strings.SplitN(rest, ".", 2)
	c, err := strconv.Atoi(parts[0])
	if err != nil || len(parts) < 2 {
		return 0, "", false
	}
	return c, parts[1], true
}

// rows groups a table walk into index -> column -> PDU.
func rows(table string, pdus []PDU) (map[string]map[int]PDU, []string) {
	out := map[string]map[int]PDU{}
	for _, p := range pdus {
		col, idx, ok := splitColumn(table, p.OID)
		if !ok {
			continue
		}
		if out[idx] == nil {
			out[idx] = map[int]PDU{}
		}
		out[idx][col] = p
	}
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return CompareOID("."+keys[i], "."+keys[j]) < 0 })
	return out, keys
}

func lastIndex(idx string) int {
	parts := strings.Split(idx, ".")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}

func colInt(r map[int]PDU, col int, def int64) int64 {
	if p, ok := r[col]; ok {
		if n, ok := Int(p); ok {
			return n
		}
	}
	return def
}

func colStr(r map[int]PDU, col int) string {
	if p, ok := r[col]; ok {
		return Str(p)
	}
	return ""
}

// --- Printer MIB parsers -------------------------------------------------

// ParseSupplies converts a prtMarkerSuppliesTable walk (plus an optional
// prtMarkerColorantValue walk) into supplies.
func ParseSupplies(supplies, colorants []PDU) []model.Supply {
	colorant := map[string]string{} // hrDeviceIndex.colorantIndex -> name
	for _, p := range colorants {
		if strings.HasPrefix(p.OID, OIDPrtMarkerColorantValue+".") {
			colorant[strings.TrimPrefix(p.OID, OIDPrtMarkerColorantValue+".")] = Str(p)
		}
	}
	table, keys := rows(OIDPrtMarkerSuppliesTable, supplies)
	var out []model.Supply
	for _, idx := range keys {
		r := table[idx]
		s := model.Supply{
			Index:       lastIndex(idx),
			Description: colStr(r, colSuppliesDescription),
			Type:        supplyTypes[int(colInt(r, colSuppliesType, 2))],
			Unit:        supplyUnits[int(colInt(r, colSuppliesUnit, 2))],
			Level:       int(colInt(r, colSuppliesLevel, -2)),
			MaxCapacity: int(colInt(r, colSuppliesMaxCapacity, -2)),
		}
		switch colInt(r, colSuppliesClass, 1) {
		case 3:
			s.Class = "consumed"
		case 4:
			s.Class = "filled"
		default:
			s.Class = "other"
		}
		if ci := colInt(r, colSuppliesColorant, 0); ci > 0 {
			dev := strings.SplitN(idx, ".", 2)[0]
			s.Colorant = colorant[dev+"."+strconv.FormatInt(ci, 10)]
		}
		s.Percent = model.Percent(s.Level, s.MaxCapacity)
		if s.Description == "" {
			s.Description = strings.TrimSpace(s.Colorant + " " + s.Type)
		}
		out = append(out, s)
	}
	return out
}

// subunitStatus decodes PrtSubUnitStatusTC (RFC 3805 §2.2.9) into a summary and flags.
func subunitStatus(v int64) (string, []string) {
	avail := v & 7
	var flags []string
	summary := "ok"
	switch avail {
	case 0:
		flags = append(flags, "availableAndIdle")
	case 2:
		flags = append(flags, "availableAndStandby")
	case 4:
		flags = append(flags, "availableAndActive")
	case 6:
		flags = append(flags, "availableAndBusy")
	case 1:
		flags = append(flags, "unavailableAndOnRequest")
		summary = "unavailable"
	case 3:
		flags = append(flags, "unavailableBecauseBroken")
		summary = "unavailable"
	case 5:
		flags = append(flags, "unknown")
		summary = "unknown"
	}
	if v&8 != 0 {
		flags = append(flags, "nonCriticalAlerts")
	}
	if v&16 != 0 {
		flags = append(flags, "criticalAlerts")
	}
	if v&32 != 0 {
		flags = append(flags, "offline")
		summary = "unavailable"
	}
	if v&64 != 0 {
		flags = append(flags, "transitioning")
	}
	return summary, flags
}

// ParseInputTrays converts a prtInputTable walk into input trays.
func ParseInputTrays(pdus []PDU) []model.Tray {
	table, keys := rows(OIDPrtInputTable, pdus)
	var out []model.Tray
	for _, idx := range keys {
		r := table[idx]
		t := model.Tray{
			Index:       lastIndex(idx),
			Kind:        "input",
			Name:        colStr(r, colInputName),
			Type:        inputTypes[int(colInt(r, colInputType, 2))],
			MaxCapacity: int(colInt(r, colInputMaxCapacity, -2)),
			Level:       int(colInt(r, colInputCurrentLevel, -2)),
			MediaName:   colStr(r, colInputMediaName),
			MediaType:   colStr(r, colInputMediaType),
		}
		if t.Name == "" {
			t.Name = fmt.Sprintf("Tray %d", t.Index)
		}
		t.MediaSize = mediaSize(colInt(r, colInputDimUnit, 0), colInt(r, colInputDimXFeedDecl, -2), colInt(r, colInputDimFeedDecl, -2))
		t.Percent = model.Percent(t.Level, t.MaxCapacity)
		summary, flags := subunitStatus(colInt(r, colInputStatus, 0))
		t.Flags = flags
		t.Status = trayStatus(summary, t.Level, t.Percent, flags)
		out = append(out, t)
	}
	return out
}

func trayStatus(summary string, level int, pct *int, flags []string) string {
	if level == 0 {
		return "empty" // an empty tray is often also reported "unavailable on request"
	}
	if summary != "ok" {
		return summary
	}
	switch {
	case level == -3:
		return "ok" // at least one sheet
	case pct != nil && *pct <= 10:
		return "low"
	}
	for _, f := range flags {
		if f == "criticalAlerts" {
			return "empty"
		}
		if f == "nonCriticalAlerts" {
			return "low"
		}
	}
	return "ok"
}

// mediaSize renders declared media dimensions as "210x297mm" (unit 4 = micrometers,
// 3 = ten-thousandths of an inch).
func mediaSize(unit, x, y int64) string {
	if x <= 0 || y <= 0 {
		return ""
	}
	switch unit {
	case 4:
		return fmt.Sprintf("%dx%dmm", x/1000, y/1000)
	case 3:
		return fmt.Sprintf("%.1fx%.1fin", float64(x)/10000, float64(y)/10000)
	}
	return ""
}

// ParseOutputTrays converts a prtOutputTable walk into output trays.
func ParseOutputTrays(pdus []PDU) []model.Tray {
	table, keys := rows(OIDPrtOutputTable, pdus)
	var out []model.Tray
	for _, idx := range keys {
		r := table[idx]
		t := model.Tray{
			Index:       lastIndex(idx),
			Kind:        "output",
			Name:        colStr(r, colOutputName),
			Type:        outputTypes[int(colInt(r, colOutputType, 2))],
			MaxCapacity: int(colInt(r, colOutputMaxCapacity, -2)),
			Level:       int(colInt(r, colOutputRemaining, -2)),
		}
		if t.Name == "" {
			t.Name = fmt.Sprintf("Output %d", t.Index)
		}
		t.Percent = model.Percent(t.Level, t.MaxCapacity)
		summary, flags := subunitStatus(colInt(r, colOutputStatus, 0))
		t.Flags = flags
		t.Status = summary
		if summary == "ok" {
			switch {
			case t.Level == 0:
				t.Status = "full"
			case t.Percent != nil && *t.Percent <= 10:
				t.Status = "nearFull"
			}
		}
		out = append(out, t)
	}
	return out
}

// ParseCounters converts a prtMarkerTable walk into counters.
func ParseCounters(pdus []PDU, at time.Time) *model.Counters {
	table, keys := rows(OIDPrtMarkerTable, pdus)
	if len(keys) == 0 {
		return nil
	}
	c := &model.Counters{At: at, ByMarker: map[string]int64{}}
	var powerOn int64 = -1
	for _, idx := range keys {
		r := table[idx]
		life := colInt(r, 4, -1)
		if life >= 0 {
			c.ByMarker[strconv.Itoa(lastIndex(idx))] = life
			if life > c.Total {
				c.Total = life
			}
		}
		if p := colInt(r, 5, -1); p > powerOn {
			powerOn = p
		}
	}
	if powerOn >= 0 {
		c.PowerOn = &powerOn
	}
	return c
}

// ParseDeviceAlerts converts a prtAlertTable walk into device alerts.
func ParseDeviceAlerts(pdus []PDU) []model.DeviceAlert {
	table, keys := rows(OIDPrtAlertTable, pdus)
	var out []model.DeviceAlert
	for _, idx := range keys {
		r := table[idx]
		a := model.DeviceAlert{
			Index:       lastIndex(idx),
			Group:       alertGroups[int(colInt(r, colAlertGroup, 1))],
			GroupIndex:  int(colInt(r, colAlertGroupIndex, 0)),
			Description: colStr(r, colAlertDescription),
		}
		code := int(colInt(r, colAlertCode, 0))
		if name, ok := alertCodes[code]; ok {
			a.Code = name
		} else if code != 0 {
			a.Code = strconv.Itoa(code)
		}
		if a.Description == "" {
			a.Description = humanise(a.Code)
		}
		switch colInt(r, colAlertSeverity, 1) {
		case 3:
			a.Severity = model.SeverityCritical
		case 4:
			a.Severity = model.SeverityWarning
		default:
			a.Severity = model.SeverityOK
		}
		out = append(out, a)
	}
	return out
}

// humanise turns "markerTonerAlmostEmpty" into "Marker toner almost empty".
func humanise(code string) string {
	if code == "" {
		return "Unknown alert"
	}
	var b strings.Builder
	for i, r := range code {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune(' ')
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		if i == 0 {
			b.WriteRune(unicode.ToUpper(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ParseDetectedErrors decodes hrPrinterDetectedErrorState (RFC 2790).
func ParseDetectedErrors(p PDU) []string {
	b, ok := p.Value.([]byte)
	if !ok {
		if s, ok := p.Value.(string); ok {
			b = []byte(s)
		}
	}
	var out []string
	for bit, name := range detectedErrorBits {
		byteIdx := bit / 8
		if byteIdx >= len(b) {
			break
		}
		if b[byteIdx]&(0x80>>(bit%8)) != 0 {
			out = append(out, name)
		}
	}
	return out
}

// EnterpriseID extracts the IANA enterprise number from a sysObjectID
// (.1.3.6.1.4.1.<enterprise>...). It is stored raw; only plugins interpret it.
func EnterpriseID(sysObjectID string) int {
	const prefix = ".1.3.6.1.4.1."
	s := strings.TrimPrefix(sysObjectID, ".")
	if !strings.HasPrefix("."+s, prefix) {
		return 0
	}
	rest := strings.TrimPrefix("."+s, prefix)
	n, _ := strconv.Atoi(strings.SplitN(rest, ".", 2)[0])
	return n
}

// MakeModel guesses make and model from a free-text device description such as
// "Brand LaserJet 4000 Series". The make is the first word; the model is the
// whole description. Plugins may refine this.
func MakeModel(descr string) (make, modelName string) {
	d := strings.TrimSpace(descr)
	if d == "" {
		return "", ""
	}
	// Drop trailing qualifiers after ';' or ',' that are typically firmware/serial blurbs.
	if i := strings.IndexAny(d, ";,"); i > 0 {
		d = strings.TrimSpace(d[:i])
	}
	fields := strings.Fields(d)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], d
}

// StatusFlag type codes for the gosnmp Asn1BER types we treat as "absent".
func absent(p PDU) bool {
	return p.Type == gosnmp.NoSuchObject || p.Type == gosnmp.NoSuchInstance || p.Type == gosnmp.EndOfMibView || p.Type == gosnmp.Null
}

// SubunitStatus is the exported PrtSubUnitStatusTC decoder; PWG 5100.13 reuses
// the same encoding in IPP printer-input-tray / printer-output-tray strings.
func SubunitStatus(v int64) (summary string, flags []string) { return subunitStatus(v) }

// TrayStatus is the exported tray summary rule shared with the IPP driver.
func TrayStatus(summary string, level int, pct *int, flags []string) string {
	return trayStatus(summary, level, pct, flags)
}

// SupplyTypeName maps a Printer MIB supply type number to its keyword.
func SupplyTypeName(n int) string { return supplyTypes[n] }
