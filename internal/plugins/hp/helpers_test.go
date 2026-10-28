package hp

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/driver/snmp"
)

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }

// fixtureLine renders a PDU back into fixture-file syntax (tests rewrite recordings).
func fixtureLine(p snmp.PDU) string {
	switch p.Type {
	case gosnmp.OctetString:
		b, _ := p.Value.([]byte)
		return fmt.Sprintf("%s HEX %s\n", p.OID, hex.EncodeToString(b))
	case gosnmp.Integer:
		return fmt.Sprintf("%s INTEGER %v\n", p.OID, p.Value)
	case gosnmp.Counter32:
		return fmt.Sprintf("%s COUNTER32 %v\n", p.OID, p.Value)
	case gosnmp.Gauge32:
		return fmt.Sprintf("%s GAUGE %v\n", p.OID, p.Value)
	case gosnmp.TimeTicks:
		return fmt.Sprintf("%s TIMETICKS %v\n", p.OID, p.Value)
	case gosnmp.ObjectIdentifier:
		return fmt.Sprintf("%s OID %v\n", p.OID, p.Value)
	case gosnmp.IPAddress:
		return fmt.Sprintf("%s IPADDRESS %v\n", p.OID, p.Value)
	}
	return ""
}
