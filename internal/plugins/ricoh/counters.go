package ricoh

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
)

// UNVERIFIED enterprise objects. These OIDs come from community monitoring
// templates for the Ricoh private MIB, not from the licensed RiDP "Private
// MIB Specification" the research could not obtain [R08] [R09]. The parser
// therefore makes no assumption about column numbers: it pairs, per row index,
// the column that carries printable names with the column that carries
// integers, and only produces counters when a row is literally named "Total".
const (
	// ricohEngCounterTable: one row per named engine counter.
	oidEngCounterTable = ".1.3.6.1.4.1.367.3.2.1.2.19.5.1"
	// Private system group serial number; used only when prtGeneralSerialNumber is empty.
	oidPrivateSerial = ".1.3.6.1.4.1.367.3.2.1.2.1.4.0"
)

// NamedCounter is one row of the engine counter table.
type NamedCounter struct {
	Index int
	Name  string
	Value int64
}

// ParseCounterRows pairs names and values per row index.
func ParseCounterRows(pdus []snmp.PDU) []NamedCounter {
	type row struct {
		name  string
		value int64
		hasV  bool
	}
	rows := map[int]*row{}
	for _, p := range pdus {
		rest := strings.TrimPrefix(p.OID, oidEngCounterTable+".")
		parts := strings.Split(rest, ".")
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			continue
		}
		r := rows[idx]
		if r == nil {
			r = &row{}
			rows[idx] = r
		}
		switch p.Type {
		case gosnmp.OctetString:
			// Names look like "Copier: Color"; a non-printable blob renders as aa:bb:cc and is ignored.
			if s := snmp.Str(p); s != "" && r.name == "" && !isHexBlob(s) {
				r.name = s
			}
		case gosnmp.Integer, gosnmp.Counter32, gosnmp.Counter64, gosnmp.Gauge32:
			if n, ok := snmp.Int(p); ok && !r.hasV {
				r.value, r.hasV = n, true
			}
		}
	}
	var out []NamedCounter
	for idx, r := range rows {
		if r.name != "" && r.hasV {
			out = append(out, NamedCounter{Index: idx, Name: r.name, Value: r.value})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func isHexBlob(s string) bool {
	if len(s) < 5 || s[2] != ':' {
		return false
	}
	for i, c := range s {
		if i%3 == 2 {
			if c != ':' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// ParseCounters derives Counters from the named rows:
//
//	Total   = the row named "Total"
//	Mono    = sum of per-function rows ("Copier: B & W", "Printer: B & W", "Fax: ...") naming black/B&W
//	Color   = sum of per-function rows naming colour
//	Duplex  = a row named "Duplex" (or "...: Duplex")
//
// nil when there is no Total row; mono/colour are dropped when they exceed it.
func ParseCounters(pdus []snmp.PDU, at time.Time) *model.Counters {
	rows := ParseCounterRows(pdus)
	var total *int64
	var mono, color, duplex int64
	var hasMono, hasColor, hasDuplex bool
	for _, r := range rows {
		n := strings.ToLower(strings.TrimSpace(r.Name))
		switch {
		case n == "total":
			v := r.Value
			total = &v
		case n == "duplex" || strings.HasSuffix(n, ": duplex") || strings.HasSuffix(n, "duplex total"):
			duplex, hasDuplex = r.Value, true
		case isFunctionRow(n) && isColor(n):
			color += r.Value
			hasColor = true
		case isFunctionRow(n) && isMono(n):
			mono += r.Value
			hasMono = true
		}
	}
	if total == nil {
		return nil
	}
	c := &model.Counters{Total: *total, At: at}
	if hasMono && hasColor && mono+color <= *total+(*total/100)+1 {
		c.Mono, c.Color = &mono, &color
	} else if hasMono && !hasColor && mono <= *total {
		c.Mono = &mono
	}
	if hasDuplex && duplex <= *total {
		c.Duplex = &duplex
	}
	return c
}

// isFunctionRow keeps the mono/colour sums to printed output: copier, printer, fax.
func isFunctionRow(n string) bool {
	return strings.HasPrefix(n, "copier") || strings.HasPrefix(n, "printer") || strings.HasPrefix(n, "fax") || strings.HasPrefix(n, "total prints")
}

func isColor(n string) bool {
	return strings.Contains(n, "color") || strings.Contains(n, "colour") || strings.Contains(n, "full col")
}

func isMono(n string) bool {
	return strings.Contains(n, "b & w") || strings.Contains(n, "b&w") || strings.Contains(n, "black") || strings.Contains(n, "mono")
}
