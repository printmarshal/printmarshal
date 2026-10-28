package snmp

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// Fixture is an in-memory SNMP agent image: a sorted list of OID/value pairs
// loaded from a recorded walk (spec §13 "record real device responses as
// fixtures"). It implements Session and also backs the simulator.
//
// File format, one binding per line, blank lines and '#' comments ignored:
//
//	<oid> <TYPE> <value>
//
// TYPE is one of STRING, HEX (aa:bb:cc), INTEGER, GAUGE, COUNTER32, COUNTER64,
// TIMETICKS, OID, IPADDRESS, NULL.
type Fixture struct {
	pdus []PDU
	idx  map[string]int
}

// LoadFixture reads a fixture file.
func LoadFixture(path string) (*Fixture, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseFixture(f)
}

// ParseFixture reads fixture text.
func ParseFixture(r io.Reader) (*Fixture, error) {
	fx := &Fixture{idx: map[string]int{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.SplitN(text, " ", 3)
		if len(parts) < 2 {
			return nil, fmt.Errorf("fixture line %d: expected '<oid> <TYPE> <value>'", line)
		}
		oid := parts[0]
		if !strings.HasPrefix(oid, ".") {
			oid = "." + oid
		}
		val := ""
		if len(parts) == 3 {
			val = parts[2]
		}
		pdu, err := parseFixtureValue(oid, strings.ToUpper(parts[1]), val)
		if err != nil {
			return nil, fmt.Errorf("fixture line %d: %w", line, err)
		}
		fx.pdus = append(fx.pdus, pdu)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	sort.Slice(fx.pdus, func(i, j int) bool { return CompareOID(fx.pdus[i].OID, fx.pdus[j].OID) < 0 })
	for i, p := range fx.pdus {
		fx.idx[p.OID] = i
	}
	return fx, nil
}

func parseFixtureValue(oid, typ, val string) (PDU, error) {
	p := PDU{OID: oid}
	switch typ {
	case "STRING":
		p.Type, p.Value = gosnmp.OctetString, []byte(val)
	case "HEX":
		b, err := hex.DecodeString(strings.ReplaceAll(strings.ReplaceAll(val, ":", ""), " ", ""))
		if err != nil {
			return p, err
		}
		p.Type, p.Value = gosnmp.OctetString, b
	case "INTEGER":
		n, err := strconv.Atoi(val)
		if err != nil {
			return p, err
		}
		p.Type, p.Value = gosnmp.Integer, n
	case "GAUGE", "GAUGE32":
		n, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return p, err
		}
		p.Type, p.Value = gosnmp.Gauge32, uint(n)
	case "COUNTER", "COUNTER32":
		n, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return p, err
		}
		p.Type, p.Value = gosnmp.Counter32, uint(n)
	case "COUNTER64":
		n, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return p, err
		}
		p.Type, p.Value = gosnmp.Counter64, n
	case "TIMETICKS":
		n, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return p, err
		}
		p.Type, p.Value = gosnmp.TimeTicks, uint32(n)
	case "OID":
		if !strings.HasPrefix(val, ".") {
			val = "." + val
		}
		p.Type, p.Value = gosnmp.ObjectIdentifier, val
	case "IPADDRESS":
		p.Type, p.Value = gosnmp.IPAddress, val
	case "NULL":
		p.Type, p.Value = gosnmp.Null, nil
	default:
		return p, fmt.Errorf("unknown type %q", typ)
	}
	return p, nil
}

// All returns every binding in OID order.
func (f *Fixture) All() []PDU { return f.pdus }

// Get returns the exact bindings; missing OIDs come back as NoSuchInstance.
func (f *Fixture) Get(_ context.Context, oids []string) ([]PDU, error) {
	out := make([]PDU, 0, len(oids))
	for _, o := range oids {
		if i, ok := f.idx[o]; ok {
			out = append(out, f.pdus[i])
		} else {
			out = append(out, PDU{OID: o, Type: gosnmp.NoSuchInstance})
		}
	}
	return out, nil
}

// Walk returns every binding under root.
func (f *Fixture) Walk(_ context.Context, root string) ([]PDU, error) {
	prefix := strings.TrimSuffix(root, ".") + "."
	var out []PDU
	for _, p := range f.pdus {
		if strings.HasPrefix(p.OID, prefix) {
			out = append(out, p)
		}
	}
	return out, nil
}

// Next returns the first binding whose OID sorts after oid (GetNext semantics).
func (f *Fixture) Next(oid string) (PDU, bool) {
	i := sort.Search(len(f.pdus), func(i int) bool { return CompareOID(f.pdus[i].OID, oid) > 0 })
	if i >= len(f.pdus) {
		return PDU{}, false
	}
	return f.pdus[i], true
}

// Close is a no-op.
func (f *Fixture) Close() error { return nil }

// CompareOID orders two dotted OIDs numerically, component by component.
func CompareOID(a, b string) int {
	as := strings.Split(strings.TrimPrefix(a, "."), ".")
	bs := strings.Split(strings.TrimPrefix(b, "."), ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, _ := strconv.ParseUint(as[i], 10, 64)
		y, _ := strconv.ParseUint(bs[i], 10, 64)
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return len(as) - len(bs)
}
