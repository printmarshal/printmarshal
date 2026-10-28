package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

func main() {
	ip := net.ParseIP("198.51.100.72").To4()
	group := &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}
	for _, qu := range []bool{false, true} {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip, Port: 0})
		if err != nil {
			fmt.Println("listen:", err)
			return
		}
		m := new(dns.Msg)
		m.SetQuestion("_ipp._tcp.local.", dns.TypePTR)
		if qu {
			m.Question[0].Qclass |= 1 << 15
		}
		b, _ := m.Pack()
		n, err := conn.WriteToUDP(b, group)
		fmt.Printf("QU=%v sent %d bytes err=%v from %s\n", qu, n, err, conn.LocalAddr())
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		buf := make([]byte, 65535)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				fmt.Println("  read end:", err)
				break
			}
			var r dns.Msg
			_ = r.Unpack(buf[:n])
			fmt.Printf("  reply %d bytes from %s: %d answers, %d extra\n", n, from, len(r.Answer), len(r.Extra))
		}
		conn.Close()
	}
	// WS-Discovery variants
	for _, types := range []string{"", "wsdp:Device", "wprt:PrintDeviceType"} {
		conn, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: ip, Port: 0})
		typesXML := ""
		if types != "" {
			typesXML = "<wsd:Types>" + types + "</wsd:Types>"
		}
		msg := `<?xml version="1.0" encoding="utf-8"?><soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:wsd="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:wsdp="http://schemas.xmlsoap.org/ws/2006/02/devprof" xmlns:wprt="http://schemas.microsoft.com/windows/2006/08/wdp/print"><soap:Header><wsa:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</wsa:To><wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</wsa:Action><wsa:MessageID>urn:uuid:4c3a2b1a-0000-4000-8000-00000000000` + fmt.Sprint(len(types)%10) + `</wsa:MessageID></soap:Header><soap:Body><wsd:Probe>` + typesXML + `</wsd:Probe></soap:Body></soap:Envelope>`
		_, err := conn.WriteToUDP([]byte(msg), &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 3702})
		fmt.Printf("WSD types=%q err=%v\n", types, err)
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		buf := make([]byte, 65535)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			body := string(buf[:n])
			i := strings.Index(body, "XAddrs")
			snip := ""
			if i > 0 && i+120 < len(body) {
				snip = body[i : i+120]
			}
			fmt.Printf("  reply %d bytes from %s %s\n", n, from, snip)
		}
		conn.Close()
	}
}
