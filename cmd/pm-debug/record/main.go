// Command record captures read-only SNMP walks and an IPP Get-Printer-Attributes
// response from a real printer into fixture files for the test suite (spec §13).
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/driver/ipp"
)

func main() {
	host := flag.String("host", "", "printer address")
	community := flag.String("community", "public", "SNMPv2c community")
	out := flag.String("out", "", "output base path (writes <out>.snmp and <out>.ipp)")
	flag.Parse()
	if *host == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: record -host IP -out testdata/fixtures/name")
		os.Exit(2)
	}
	g := &gosnmp.GoSNMP{Target: *host, Port: 161, Community: *community, Version: gosnmp.Version2c, Timeout: 5 * time.Second, Retries: 2, MaxOids: 20}
	if err := g.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, "snmp connect:", err)
		os.Exit(1)
	}
	defer g.Close()
	var lines []string
	lines = append(lines, "# Recorded from a real device with cmd/pm-debug/record (read-only walk). Subtrees: system, ifPhysAddress, Host Resources, Printer MIB.")
	for _, root := range []string{".1.3.6.1.2.1.1", ".1.3.6.1.2.1.2.2.1.6", ".1.3.6.1.2.1.25.3.2", ".1.3.6.1.2.1.25.3.5",
		".1.3.6.1.2.1.43.5", ".1.3.6.1.2.1.43.8", ".1.3.6.1.2.1.43.9", ".1.3.6.1.2.1.43.10", ".1.3.6.1.2.1.43.11", ".1.3.6.1.2.1.43.12",
		".1.3.6.1.2.1.43.13", ".1.3.6.1.2.1.43.16", ".1.3.6.1.2.1.43.17", ".1.3.6.1.2.1.43.18"} {
		pdus, err := g.WalkAll(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "walk", root, err)
			continue
		}
		for _, p := range pdus {
			if l, ok := line(p); ok {
				lines = append(lines, l)
			}
		}
	}
	if err := os.WriteFile(*out+".snmp", []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s.snmp (%d bindings)\n", *out, len(lines)-1)

	// IPP: request everything so the fixture is complete.
	opts := ipp.DefaultOptions()
	opts.Timeout = 25 * time.Second
	tr := ipp.NewTransport(opts)
	for _, ep := range []string{"http://%s:631/ipp/print", "http://%s:631/ipp", "http://%s:631/"} {
		url := fmt.Sprintf(ep, *host)
		req := ipp.NewGetPrinterAttributes("ipp://"+strings.TrimPrefix(url, "http://"), 1, ipp.RequestedAttrs)
		body, _ := ipp.Encode(req)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		resp, err := tr.Do(ctx, url, body)
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ipp", url, err)
			continue
		}
		if _, err := ipp.Decode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "ipp decode", url, err)
			continue
		}
		if err := os.WriteFile(*out+".ipp", resp, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s.ipp (%d bytes) from %s\n", *out, len(resp), url)
		return
	}
}

func line(p gosnmp.SnmpPDU) (string, bool) {
	switch p.Type {
	case gosnmp.OctetString:
		b, _ := p.Value.([]byte)
		printable := true
		for _, c := range b {
			if c < 32 && c != '\n' && c != '\r' && c != '\t' || c > 126 {
				printable = false
				break
			}
		}
		if printable && !strings.Contains(string(b), "\n") {
			return fmt.Sprintf("%s STRING %s", p.Name, string(b)), true
		}
		return fmt.Sprintf("%s HEX %s", p.Name, hex.EncodeToString(b)), true
	case gosnmp.Integer:
		return fmt.Sprintf("%s INTEGER %v", p.Name, p.Value), true
	case gosnmp.Gauge32:
		return fmt.Sprintf("%s GAUGE %v", p.Name, p.Value), true
	case gosnmp.Counter32:
		return fmt.Sprintf("%s COUNTER32 %v", p.Name, p.Value), true
	case gosnmp.Counter64:
		return fmt.Sprintf("%s COUNTER64 %v", p.Name, p.Value), true
	case gosnmp.TimeTicks:
		return fmt.Sprintf("%s TIMETICKS %v", p.Name, p.Value), true
	case gosnmp.ObjectIdentifier:
		return fmt.Sprintf("%s OID %v", p.Name, p.Value), true
	case gosnmp.IPAddress:
		return fmt.Sprintf("%s IPADDRESS %v", p.Name, p.Value), true
	}
	return "", false
}
