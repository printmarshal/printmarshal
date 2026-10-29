package hp

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// HP enterprise objects (1.3.6.1.4.1.11). Every OID below answered on an HP
// Color LaserJet MFP M283fdw during this build's read-only validation and the
// values were cross-checked against the Printer MIB and the device's own LEDM
// usage page (testdata/fixtures/hp-m283fdw-enterprise.snmp). The arc names are
// HP's PML naming (hp.nm.system.net-peripheral.netdm.dm.*).
const (
	// gdStatusDisplay: the control-panel status line ("Sleep mode is on.").
	oidStatusDisplay = ".1.3.6.1.4.1.11.2.3.9.1.1.3.0"
	// gdStatusId: the IEEE 1284 device id (MFG/MDL/CMD/SN...).
	oidDeviceID = ".1.3.6.1.4.1.11.2.3.9.1.1.7.0"

	// device.system.id.*
	oidProductNumber = ".1.3.6.1.4.1.11.2.3.9.4.2.1.1.3.1.0" // "7KW75A"
	oidModelName     = ".1.3.6.1.4.1.11.2.3.9.4.2.1.1.3.2.0" // "HP Color LaserJet MFP M283fdw"
	oidSerialNumber  = ".1.3.6.1.4.1.11.2.3.9.4.2.1.1.3.3.0" // "VNB0TEST01"
	oidFWDateCode    = ".1.3.6.1.4.1.11.2.3.9.4.2.1.1.3.5.0" // "20260622"

	// device.system.firmware component tables: name in .20.6.N.0, version in .20.7.N.0.
	oidFWComponentName    = ".1.3.6.1.4.1.11.2.3.9.4.2.1.1.20.6"
	oidFWComponentVersion = ".1.3.6.1.4.1.11.2.3.9.4.2.1.1.20.7"

	// device.print-engine.* page counters.
	oidTotalEnginePages = ".1.3.6.1.4.1.11.2.3.9.4.2.1.4.1.2.5.0" // equals prtMarkerLifeCount
	// On the M283fdw .6.0 equalled the LEDM MonochromeImpressions value and
	// .7.0 the ColorImpressions value; some community templates label them the
	// other way round. The reader cross-checks that mono+colour == total.
	oidMonoPages   = ".1.3.6.1.4.1.11.2.3.9.4.2.1.4.1.2.6.0"
	oidColorPages  = ".1.3.6.1.4.1.11.2.3.9.4.2.1.4.1.2.7.0"
	oidDuplexPages = ".1.3.6.1.4.1.11.2.3.9.4.2.1.4.1.2.22.0" // equals LEDM DuplexSheets

	// device.print-engine.consumables: per-cartridge part number (.56.N.0) and serial (.3.N.0).
	oidSupplyPartNumber = ".1.3.6.1.4.1.11.2.3.9.4.2.1.4.1.10.1.1.56"
	oidSupplySerial     = ".1.3.6.1.4.1.11.2.3.9.4.2.1.4.1.10.1.1.3"

	// Jetdirect (hp.nm.interface.npCard): product number and mDNS names.
	oidJDProductNumber = ".1.3.6.1.4.1.11.2.4.3.1.10.0"
	oidJDMDNSHostname  = ".1.3.6.1.4.1.11.2.4.3.5.46.0" // "NPI0A1B2C.local."
)

// pmlString strips the 2-byte PML type header HP agents prefix octet-string
// objects with (0x01 0x15 = string; newer firmware sends 0xFD 0xE8), then
// trims NULs and padding.
func pmlString(p snmp.PDU) string {
	b, ok := p.Value.([]byte)
	if !ok {
		if s, ok := p.Value.(string); ok {
			b = []byte(s)
		} else {
			return ""
		}
	}
	if len(b) >= 2 && b[0] == 0x01 && b[1] == 0x15 {
		b = b[2:]
	}
	return model.CleanText(string(b))
}

func pmlGet(ctx context.Context, p *vendorkit.Probe, oid string) string {
	pdu, ok := p.Get(ctx, oid)
	if !ok {
		return ""
	}
	return pmlString(pdu)
}

// jetdirectSysDescr parses the Jetdirect sysDescr format
// "HP ETHERNET MULTI-ENVIRONMENT,SN:...,FN:...,SVCID:...,PID:HP Color LaserJet MFP M283fdw".
func jetdirectSysDescr(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(part, ":")
		if ok {
			out[strings.ToUpper(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	return out
}

// PMLIdentity is what the enterprise objects say about the device.
type PMLIdentity struct {
	ProductNumber string
	ModelName     string
	Serial        string
	FWDateCode    string
	Components    []FirmwareComponent
	DeviceID      map[string]string
}

// FirmwareComponent is one row of the PML firmware component table.
type FirmwareComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// readPMLIdentity reads the identity objects; ok is false when none answered.
func readPMLIdentity(ctx context.Context, p *vendorkit.Probe) (PMLIdentity, bool) {
	id := PMLIdentity{
		ProductNumber: pmlGet(ctx, p, oidProductNumber),
		ModelName:     pmlGet(ctx, p, oidModelName),
		Serial:        pmlGet(ctx, p, oidSerialNumber),
		FWDateCode:    pmlGet(ctx, p, oidFWDateCode),
		DeviceID:      vendorkit.ParseDeviceID(p.GetStr(ctx, oidDeviceID)),
	}
	if id.ProductNumber == "" {
		id.ProductNumber = p.GetStr(ctx, oidJDProductNumber)
	}
	if id.ModelName == "" {
		// One lost SNMP reply (2 s timeout, 64 scan workers at once) would leave the
		// device with the print server's name as its model, so ask once more.
		id.ModelName = pmlGet(ctx, p, oidModelName)
	}
	id.Components = readFirmwareComponents(ctx, p)
	ok := id.Serial != "" || id.ModelName != "" || id.FWDateCode != "" || id.ProductNumber != ""
	return id, ok
}

var fwDateCode = regexp.MustCompile(`^20\d{6}$`)

func readFirmwareComponents(ctx context.Context, p *vendorkit.Probe) []FirmwareComponent {
	names := map[string]string{}
	for _, pdu := range p.Walk(ctx, oidFWComponentName) {
		names[strings.TrimPrefix(pdu.OID, oidFWComponentName+".")] = pmlString(pdu)
	}
	var idx []string
	versions := map[string]string{}
	for _, pdu := range p.Walk(ctx, oidFWComponentVersion) {
		i := strings.TrimPrefix(pdu.OID, oidFWComponentVersion+".")
		versions[i] = pmlString(pdu)
		idx = append(idx, i)
	}
	sort.Slice(idx, func(a, b int) bool { return snmp.CompareOID("."+idx[a], "."+idx[b]) < 0 })
	var out []FirmwareComponent
	for _, i := range idx {
		n, v := names[i], versions[i]
		if n == "" || v == "" || strings.HasPrefix(v, "?") {
			continue
		}
		out = append(out, FirmwareComponent{Name: n, Version: v})
	}
	return out
}

// readPMLCounters reads the print-engine page counters. Mono/colour are only
// reported when they add up to the engine total (±1%), which guards against
// the mono/colour labelling ambiguity documented above.
func readPMLCounters(ctx context.Context, p *vendorkit.Probe, at time.Time) *model.Counters {
	total, ok := p.GetInt(ctx, oidTotalEnginePages)
	if !ok || total < 0 {
		return nil
	}
	c := &model.Counters{Total: total, At: at}
	mono, okM := p.GetInt(ctx, oidMonoPages)
	color, okC := p.GetInt(ctx, oidColorPages)
	if okM && okC && mono >= 0 && color >= 0 {
		sum := mono + color
		tol := total / 100
		if sum >= total-tol && sum <= total+tol {
			c.Mono, c.Color = &mono, &color
		}
	}
	if d, ok := p.GetInt(ctx, oidDuplexPages); ok && d >= 0 {
		c.Duplex = &d
	}
	return c
}

// readSupplyPartNumbers returns cartridge slot -> part number (e.g. 1 -> "W2210A").
func readSupplyPartNumbers(ctx context.Context, p *vendorkit.Probe) map[int]string {
	out := map[int]string{}
	for _, pdu := range p.Walk(ctx, oidSupplyPartNumber) {
		rest := strings.TrimPrefix(pdu.OID, oidSupplyPartNumber+".")
		n, err := strconv.Atoi(strings.SplitN(rest, ".", 2)[0])
		if err != nil {
			continue
		}
		if v := pmlString(pdu); v != "" {
			out[n] = v
		}
	}
	return out
}
