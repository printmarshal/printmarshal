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
	"sync"

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
	mu   sync.RWMutex
	pdus []PDU
	idx  map[string]int
	sets []PDU
	// Writable makes Set apply the bindings (the simulator's mode). When false
	// (the default for tests) Set records the attempt and rejects it with
	// notWritable, so a test can assert that a dry run never reached it.
	Writable bool
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
func (f *Fixture) All() []PDU {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]PDU, len(f.pdus))
	copy(out, f.pdus)
	return out
}

// Get returns the exact bindings; missing OIDs come back as NoSuchInstance.
func (f *Fixture) Get(_ context.Context, oids []string) ([]PDU, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
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
	f.mu.RLock()
	defer f.mu.RUnlock()
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
	f.mu.RLock()
	defer f.mu.RUnlock()
	i := sort.Search(len(f.pdus), func(i int) bool { return CompareOID(f.pdus[i].OID, oid) > 0 })
	if i >= len(f.pdus) {
		return PDU{}, false
	}
	return f.pdus[i], true
}

// Set implements Session. Every attempt is recorded (see Sets). Unless the
// fixture is Writable the request is rejected with notWritable, mirroring an
// agent whose community has no write access; when Writable, bindings for OIDs
// the image already holds are updated and unknown OIDs are rejected with
// noCreation.
func (f *Fixture) Set(_ context.Context, pdus []PDU) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets = append(f.sets, pdus...)
	if !f.Writable {
		return fmt.Errorf("snmp set error notWritable (fixture is read-only)")
	}
	for _, p := range pdus {
		if _, ok := f.idx[p.OID]; !ok {
			return fmt.Errorf("snmp set error noCreation at %s", p.OID)
		}
	}
	for _, p := range pdus {
		f.putLocked(p.OID, p.Type, p.Value)
	}
	return nil
}

// Sets returns every binding handed to Set so far, in order.
func (f *Fixture) Sets() []PDU {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]PDU, len(f.sets))
	copy(out, f.sets)
	return out
}

// ResetSets forgets the recorded Set attempts.
func (f *Fixture) ResetSets() {
	f.mu.Lock()
	f.sets = nil
	f.mu.Unlock()
}

// Put inserts or replaces one binding directly (the simulator uses it to
// reflect writes). Strings become OctetString, ints Integer; any other value
// keeps the existing binding's type or is stored as given.
func (f *Fixture) Put(oid string, value any) {
	if !strings.HasPrefix(oid, ".") {
		oid = "." + oid
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	typ := gosnmp.Null
	if i, ok := f.idx[oid]; ok {
		typ = f.pdus[i].Type
	}
	switch v := value.(type) {
	case string:
		typ, value = gosnmp.OctetString, []byte(v)
	case []byte:
		typ = gosnmp.OctetString
	case int:
		typ = gosnmp.Integer
	}
	f.putLocked(oid, typ, value)
}

func (f *Fixture) putLocked(oid string, typ gosnmp.Asn1BER, value any) {
	if i, ok := f.idx[oid]; ok {
		f.pdus[i] = PDU{OID: oid, Type: typ, Value: value}
		return
	}
	f.pdus = append(f.pdus, PDU{OID: oid, Type: typ, Value: value})
	sort.Slice(f.pdus, func(i, j int) bool { return CompareOID(f.pdus[i].OID, f.pdus[j].OID) < 0 })
	f.idx = make(map[string]int, len(f.pdus))
	for i, p := range f.pdus {
		f.idx[p.OID] = i
	}
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
