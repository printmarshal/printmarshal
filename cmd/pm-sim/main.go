// Command pm-sim is a printer simulator for development and CI (spec §13): an
// SNMPv2c agent that serves a recorded fixture, and an IPP server that answers
// Get-Printer-Attributes with data derived from the same fixture.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
)

func main() {
	fixture := flag.String("fixture", "testdata/fixtures/generic-laser.snmp", "SNMP fixture file")
	snmpAddr := flag.String("snmp", "127.0.0.1:1161", "SNMP agent UDP listen address")
	ippAddr := flag.String("ipp", "127.0.0.1:6631", "IPP server TCP listen address ('' to disable)")
	community := flag.String("community", "public", "accepted SNMPv2c community")
	firmware := flag.String("firmware", "2.14.7", "firmware version reported over IPP")
	flag.Parse()

	fx, err := snmp.LoadFixture(*fixture)
	if err != nil {
		log.Fatalf("fixture: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		if err := serveSNMP(ctx, *snmpAddr, *community, fx); err != nil {
			log.Fatalf("snmp agent: %v", err)
		}
	}()
	if *ippAddr != "" {
		go func() {
			if err := serveIPP(ctx, *ippAddr, fx, *firmware); err != nil && err != http.ErrServerClosed {
				log.Fatalf("ipp server: %v", err)
			}
		}()
	}
	log.Printf("pm-sim: snmp on udp://%s (community %q), ipp on http://%s, fixture %s", *snmpAddr, *community, *ippAddr, *fixture)
	<-ctx.Done()
}

// --- SNMP agent ---------------------------------------------------------------

func serveSNMP(ctx context.Context, addr, community string, fx *snmp.Fixture) error {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() { <-ctx.Done(); conn.Close() }()
	dec := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: community}
	buf := make([]byte, 64<<10)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		pkt, err := dec.SnmpDecodePacket(buf[:n])
		if err != nil || pkt.Community != community {
			continue
		}
		resp := &gosnmp.SnmpPacket{Version: pkt.Version, Community: pkt.Community, PDUType: gosnmp.GetResponse, RequestID: pkt.RequestID}
		switch pkt.PDUType {
		case gosnmp.GetRequest:
			for _, v := range pkt.Variables {
				got, _ := fx.Get(ctx, []string{v.Name})
				resp.Variables = append(resp.Variables, toVar(got[0]))
			}
		case gosnmp.GetNextRequest:
			for _, v := range pkt.Variables {
				resp.Variables = append(resp.Variables, next(fx, v.Name))
			}
		case gosnmp.GetBulkRequest:
			nr := int(pkt.NonRepeaters)
			if nr > len(pkt.Variables) {
				nr = len(pkt.Variables)
			}
			for _, v := range pkt.Variables[:nr] {
				resp.Variables = append(resp.Variables, next(fx, v.Name))
			}
			reps := int(pkt.MaxRepetitions)
			if reps <= 0 || reps > 100 {
				reps = 25
			}
			cursors := make([]string, 0, len(pkt.Variables)-nr)
			for _, v := range pkt.Variables[nr:] {
				cursors = append(cursors, v.Name)
			}
			for i := 0; i < reps; i++ {
				for j := range cursors {
					nv := next(fx, cursors[j])
					resp.Variables = append(resp.Variables, nv)
					if nv.Type != gosnmp.EndOfMibView {
						cursors[j] = nv.Name
					}
				}
			}
		default:
			continue
		}
		out, err := resp.MarshalMsg()
		if err != nil {
			log.Printf("marshal: %v", err)
			continue
		}
		_, _ = conn.WriteTo(out, from)
	}
}

func next(fx *snmp.Fixture, oid string) gosnmp.SnmpPDU {
	p, ok := fx.Next(oid)
	if !ok {
		return gosnmp.SnmpPDU{Name: oid, Type: gosnmp.EndOfMibView}
	}
	return toVar(p)
}

func toVar(p snmp.PDU) gosnmp.SnmpPDU {
	v := gosnmp.SnmpPDU{Name: p.OID, Type: p.Type, Value: p.Value}
	switch p.Type {
	case gosnmp.NoSuchInstance, gosnmp.NoSuchObject, gosnmp.EndOfMibView:
		v.Value = nil
	case gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks:
		switch x := p.Value.(type) {
		case uint:
			v.Value = uint32(x)
		case uint32:
			v.Value = x
		case int:
			v.Value = uint32(x)
		}
	case gosnmp.OctetString:
		if s, ok := p.Value.(string); ok {
			v.Value = []byte(s)
		}
	}
	return v
}

// --- IPP server -------------------------------------------------------------

func serveIPP(ctx context.Context, addr string, fx *snmp.Fixture, firmware string) error {
	attrs := printerAttributes(ctx, fx, firmware)
	srv := &http.Server{Addr: addr, ReadHeaderTimeout: 5 * time.Second}
	srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/ipp") {
			http.Error(w, "IPP endpoint", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		req, err := ipp.Decode(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := &ipp.Message{Version: [2]byte{2, 0}, RequestID: req.RequestID}
		ops := ipp.Group{Tag: ipp.TagOperation, Attributes: []ipp.Attribute{
			{Tag: ipp.TagCharset, Name: "attributes-charset", Values: []any{"utf-8"}},
			{Tag: ipp.TagLanguage, Name: "attributes-natural-language", Values: []any{"en"}},
		}}
		if req.StatusCode != ipp.OpGetPrinterAttributes {
			resp.StatusCode = 0x0501 // server-error-operation-not-supported
			resp.Groups = []ipp.Group{ops}
		} else {
			resp.StatusCode = 0x0000
			resp.Groups = []ipp.Group{ops, {Tag: ipp.TagPrinter, Attributes: attrs}}
		}
		out, _ := ipp.Encode(resp)
		w.Header().Set("Content-Type", "application/ipp")
		_, _ = w.Write(out)
	})
	go func() { <-ctx.Done(); _ = srv.Shutdown(context.Background()) }()
	return srv.ListenAndServe()
}

func printerAttributes(ctx context.Context, fx *snmp.Fixture, firmware string) []ipp.Attribute {
	str := func(oid string) string {
		got, _ := fx.Get(ctx, []string{oid})
		return snmp.Str(got[0])
	}
	descr := str(snmp.OIDHrDeviceDescr + ".1")
	if descr == "" {
		descr = str(snmp.OIDSysDescr)
	}
	mk, mdl := snmp.MakeModel(descr)
	serial := str(snmp.OIDPrtGeneralSerialNumber + ".1")
	st, _ := snmp.ReadStatus(ctx, fx)

	strs := func(v ...string) []any {
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out
	}
	state := 3
	if st != nil && st.State == "down" {
		state = 5
	}
	reasons := []string{"none"}
	if st != nil && len(st.Errors) > 0 {
		reasons = nil
		for _, e := range st.Errors {
			switch e {
			case "jammed":
				reasons = append(reasons, "media-jam-error")
			case "lowToner":
				reasons = append(reasons, "toner-low-warning")
			case "noPaper":
				reasons = append(reasons, "media-empty-error")
			case "doorOpen":
				reasons = append(reasons, "door-open-error")
			default:
				reasons = append(reasons, e+"-warning")
			}
		}
	}
	attrs := []ipp.Attribute{
		{Tag: ipp.TagText, Name: "printer-make-and-model", Values: strs(mdl)},
		{Tag: ipp.TagName, Name: "printer-name", Values: strs(str(snmp.OIDSysName))},
		{Tag: ipp.TagText, Name: "printer-location", Values: strs(str(snmp.OIDSysLocation))},
		{Tag: ipp.TagText, Name: "printer-device-id", Values: strs(fmt.Sprintf("MFG:%s;MDL:%s;CMD:PCL,PDF;SN:%s;", mk, strings.TrimPrefix(mdl, mk+" "), serial))},
		{Tag: ipp.TagURI, Name: "printer-uuid", Values: strs("urn:uuid:7f1d6b2e-5c3a-4b8e-9d2f-" + fmt.Sprintf("%012x", hash(serial)))},
		{Tag: ipp.TagText, Name: "printer-firmware-string-version", Values: strs(firmware)},
		{Tag: ipp.TagName, Name: "printer-firmware-name", Values: strs("SIMFW")},
		{Tag: ipp.TagEnum, Name: "printer-state", Values: []any{state}},
		{Tag: ipp.TagKeyword, Name: "printer-state-reasons", Values: strs(reasons...)},
		{Tag: ipp.TagText, Name: "printer-state-message", Values: strs(st.Message)},
		{Tag: ipp.TagInteger, Name: "printer-up-time", Values: []any{int(st.UptimeSecs)}},
	}
	// Supplies and trays as PWG 5100.13 strings.
	sup, _ := fx.Walk(ctx, snmp.OIDPrtMarkerSuppliesTable)
	col, _ := fx.Walk(ctx, snmp.OIDPrtMarkerColorantValue)
	var supplyVals, descVals []any
	for _, s := range snmp.ParseSupplies(sup, col) {
		class := "supplyThatIsConsumed"
		if s.Class == "filled" {
			class = "receptacleThatIsFilled"
		}
		unit := s.Unit
		if unit == "" {
			unit = "percent"
		}
		supplyVals = append(supplyVals, fmt.Sprintf("index=%d;class=%s;type=%s;unit=%s;maxcapacity=%d;level=%d;colorantname=%s;", s.Index, class, s.Type, unit, s.MaxCapacity, s.Level, s.Colorant))
		descVals = append(descVals, s.Description)
	}
	if len(supplyVals) > 0 {
		attrs = append(attrs, ipp.Attribute{Tag: ipp.TagOctet, Name: "printer-supply", Values: supplyVals},
			ipp.Attribute{Tag: ipp.TagText, Name: "printer-supply-description", Values: descVals})
	}
	in, _ := fx.Walk(ctx, snmp.OIDPrtInputTable)
	var trayVals []any
	for _, t := range snmp.ParseInputTrays(in) {
		status := 0
		if t.Status == "empty" {
			status = 16
		}
		trayVals = append(trayVals, fmt.Sprintf("type=%s;mediafeed=29700;mediaxfeed=21000;maxcapacity=%d;level=%d;status=%d;name=%s;", t.Type, t.MaxCapacity, t.Level, status, t.Name))
	}
	if len(trayVals) > 0 {
		attrs = append(attrs, ipp.Attribute{Tag: ipp.TagOctet, Name: "printer-input-tray", Values: trayVals})
	}
	mk2, _ := fx.Walk(ctx, snmp.OIDPrtMarkerTable)
	if c := snmp.ParseCounters(mk2, time.Now()); c != nil {
		attrs = append(attrs, ipp.Attribute{Tag: ipp.TagInteger, Name: "printer-impressions-completed", Values: []any{int(c.Total)}})
	}
	_ = model.SeverityOK
	return attrs
}

func hash(s string) uint64 {
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h & 0xffffffffffff
}
