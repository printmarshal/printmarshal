package model

// SettingDef describes a canonical, vendor-neutral setting key. Drivers and
// plugins map their device-specific fields onto these keys so profiles,
// drift reports and security policies work across manufacturers (spec §8.4).
// Plugins may add vendor-only keys under "vendor.<make>.<name>"; those never
// appear in cross-vendor profiles.
type SettingDef struct {
	Key      string
	Area     string
	Name     string
	Type     string // string | int | bool | enum | list | secret
	Options  []any
	Security bool // security-relevant: included in read_security documents
}

// Areas groups settings in the UI, in display order.
var Areas = []string{"identity", "network", "protocols", "security", "energy", "trays", "panel", "print", "scan", "other"}

// Catalog is the canonical setting catalogue. Keep keys stable; add, never rename.
var Catalog = []SettingDef{
	// identity
	{Key: "identity.hostname", Area: "identity", Name: "Hostname", Type: "string"},
	{Key: "identity.location", Area: "identity", Name: "Location (device-reported)", Type: "string"},
	{Key: "identity.contact", Area: "identity", Name: "Contact", Type: "string"},
	{Key: "identity.assetNumber", Area: "identity", Name: "Asset number (device)", Type: "string"},
	{Key: "identity.description", Area: "identity", Name: "Description (device-reported)", Type: "string"},
	{Key: "identity.printerName", Area: "identity", Name: "Printer name (Printer MIB)", Type: "string"},
	{Key: "identity.organization", Area: "identity", Name: "Organization", Type: "string"},
	{Key: "identity.organizationalUnit", Area: "identity", Name: "Organizational unit", Type: "string"},
	// network
	{Key: "network.ipv4.mode", Area: "network", Name: "IPv4 address mode", Type: "enum", Options: []any{"dhcp", "static", "bootp", "auto"}},
	{Key: "network.ipv4.address", Area: "network", Name: "IPv4 address", Type: "string"},
	{Key: "network.ipv4.gateway", Area: "network", Name: "IPv4 gateway", Type: "string"},
	{Key: "network.ipv4.netmask", Area: "network", Name: "IPv4 subnet mask", Type: "string"},
	{Key: "network.ipv6.enabled", Area: "network", Name: "IPv6 enabled", Type: "bool"},
	{Key: "network.dns.servers", Area: "network", Name: "DNS servers", Type: "list"},
	{Key: "network.dns.domain", Area: "network", Name: "DNS domain", Type: "string"},
	{Key: "network.ntp.server", Area: "network", Name: "NTP server", Type: "string"},
	{Key: "network.ntp.enabled", Area: "network", Name: "NTP enabled", Type: "bool"},
	{Key: "network.timezone", Area: "network", Name: "Time zone", Type: "string"},
	{Key: "network.linkSpeed", Area: "network", Name: "Link speed / duplex", Type: "enum", Options: []any{"auto", "10half", "10full", "100half", "100full", "1000full"}},
	{Key: "network.wifi.enabled", Area: "network", Name: "Wi-Fi enabled", Type: "bool"},
	{Key: "network.wifiDirect.enabled", Area: "network", Name: "Wi-Fi Direct enabled", Type: "bool", Security: true},
	// protocols (all security-relevant)
	{Key: "protocols.snmp.v1v2cEnabled", Area: "protocols", Name: "SNMP v1/v2c enabled", Type: "bool", Security: true},
	{Key: "protocols.snmp.v2cWriteEnabled", Area: "protocols", Name: "SNMP v2c write (set) enabled", Type: "bool", Security: true},
	{Key: "protocols.snmp.v3Enabled", Area: "protocols", Name: "SNMPv3 enabled", Type: "bool", Security: true},
	{Key: "protocols.snmp.readCommunity", Area: "protocols", Name: "SNMP read community", Type: "secret", Security: true},
	{Key: "protocols.snmp.writeCommunity", Area: "protocols", Name: "SNMP write community", Type: "secret", Security: true},
	{Key: "protocols.ipp.enabled", Area: "protocols", Name: "IPP enabled", Type: "bool", Security: true},
	{Key: "protocols.ipps.enabled", Area: "protocols", Name: "IPPS (IPP over TLS) enabled", Type: "bool", Security: true},
	{Key: "protocols.raw9100.enabled", Area: "protocols", Name: "Raw port 9100 printing enabled", Type: "bool", Security: true},
	{Key: "protocols.lpd.enabled", Area: "protocols", Name: "LPD enabled", Type: "bool", Security: true},
	{Key: "protocols.ftp.enabled", Area: "protocols", Name: "FTP enabled", Type: "bool", Security: true},
	{Key: "protocols.telnet.enabled", Area: "protocols", Name: "Telnet enabled", Type: "bool", Security: true},
	{Key: "protocols.http.enabled", Area: "protocols", Name: "HTTP (web UI) enabled", Type: "bool", Security: true},
	{Key: "protocols.https.enabled", Area: "protocols", Name: "HTTPS enabled", Type: "bool", Security: true},
	{Key: "protocols.https.redirect", Area: "protocols", Name: "Redirect HTTP to HTTPS", Type: "bool", Security: true},
	{Key: "protocols.mdns.enabled", Area: "protocols", Name: "mDNS / Bonjour enabled", Type: "bool", Security: true},
	{Key: "protocols.wsd.enabled", Area: "protocols", Name: "WS-Discovery / WSD enabled", Type: "bool", Security: true},
	{Key: "protocols.slp.enabled", Area: "protocols", Name: "SLP enabled", Type: "bool", Security: true},
	{Key: "protocols.llmnr.enabled", Area: "protocols", Name: "LLMNR enabled", Type: "bool", Security: true},
	{Key: "protocols.smb.enabled", Area: "protocols", Name: "SMB enabled", Type: "bool", Security: true},
	{Key: "protocols.airprint.enabled", Area: "protocols", Name: "AirPrint enabled", Type: "bool", Security: true},
	{Key: "protocols.escl.enabled", Area: "protocols", Name: "eSCL (driverless scan) enabled", Type: "bool", Security: true},
	{Key: "protocols.usb.hostEnabled", Area: "protocols", Name: "USB host ports enabled", Type: "bool", Security: true},
	// security
	{Key: "security.adminPassword.set", Area: "security", Name: "Administrator password set", Type: "bool", Security: true},
	{Key: "security.adminPassword", Area: "security", Name: "Administrator password", Type: "secret", Security: true},
	{Key: "security.tls.minVersion", Area: "security", Name: "Minimum TLS version", Type: "enum", Options: []any{"1.0", "1.1", "1.2", "1.3"}, Security: true},
	{Key: "security.tls.ciphers", Area: "security", Name: "TLS cipher suites", Type: "list", Security: true},
	{Key: "security.cert.identityPresent", Area: "security", Name: "Identity certificate present", Type: "bool", Security: true},
	{Key: "security.cert.identitySelfSigned", Area: "security", Name: "Identity certificate self-signed", Type: "bool", Security: true},
	{Key: "security.cert.identityExpiresAt", Area: "security", Name: "Identity certificate expiry (RFC 3339)", Type: "string", Security: true},
	{Key: "security.cert.identityDaysLeft", Area: "security", Name: "Identity certificate days until expiry", Type: "int", Security: true},
	{Key: "security.ieee8021x.enabled", Area: "security", Name: "802.1X enabled", Type: "bool", Security: true},
	{Key: "security.ipsec.enabled", Area: "security", Name: "IPsec enabled", Type: "bool", Security: true},
	{Key: "security.firewall.enabled", Area: "security", Name: "IP filter / firewall enabled", Type: "bool", Security: true},
	{Key: "security.firewall.allowList", Area: "security", Name: "IP allow list", Type: "list", Security: true},
	{Key: "security.panel.lock", Area: "security", Name: "Control panel admin lock", Type: "bool", Security: true},
	{Key: "security.jobLog.enabled", Area: "security", Name: "Job log / accounting enabled", Type: "bool", Security: true},
	{Key: "security.storage.encryption", Area: "security", Name: "Storage encryption enabled", Type: "bool", Security: true},
	{Key: "security.storage.overwrite", Area: "security", Name: "Storage overwrite on delete", Type: "bool", Security: true},
	{Key: "security.firmware.autoUpdate", Area: "security", Name: "Automatic firmware updates", Type: "bool", Security: true},
	{Key: "security.firmware.signedOnly", Area: "security", Name: "Accept only signed firmware", Type: "bool", Security: true},
	{Key: "security.pjl.deviceAccess", Area: "security", Name: "PJL device access / file system", Type: "bool", Security: true},
	{Key: "security.syslog.server", Area: "security", Name: "Syslog server", Type: "string", Security: true},
	// energy
	{Key: "energy.sleepDelayMinutes", Area: "energy", Name: "Sleep delay (minutes)", Type: "int"},
	{Key: "energy.autoOffMinutes", Area: "energy", Name: "Auto-off delay (minutes)", Type: "int"},
	{Key: "energy.wakeSchedule", Area: "energy", Name: "Wake schedule", Type: "string"},
	{Key: "energy.ecoMode", Area: "energy", Name: "Eco / toner-save mode", Type: "bool"},
	// trays & media
	{Key: "trays.defaultTray", Area: "trays", Name: "Default input tray", Type: "string"},
	{Key: "trays.defaultMediaSize", Area: "trays", Name: "Default media size", Type: "string"},
	{Key: "trays.defaultMediaType", Area: "trays", Name: "Default media type", Type: "string"},
	{Key: "trays.traySizes", Area: "trays", Name: "Per-tray media sizes", Type: "list"},
	// panel
	{Key: "panel.language", Area: "panel", Name: "Display language", Type: "string"},
	{Key: "panel.brightness", Area: "panel", Name: "Display brightness", Type: "int"},
	{Key: "panel.homeScreenMessage", Area: "panel", Name: "Home screen message", Type: "string"},
	// print
	{Key: "print.defaultDuplex", Area: "print", Name: "Default duplex", Type: "bool"},
	{Key: "print.defaultColorMode", Area: "print", Name: "Default colour mode", Type: "enum", Options: []any{"auto", "color", "monochrome"}},
	{Key: "print.defaultCopies", Area: "print", Name: "Default copies", Type: "int"},
	{Key: "print.defaultResolution", Area: "print", Name: "Default resolution", Type: "string"},
	{Key: "print.jobTimeoutSeconds", Area: "print", Name: "Job timeout (seconds)", Type: "int"},
	{Key: "print.defaultMarkerIndex", Area: "print", Name: "Default marker (Printer MIB index)", Type: "int"},
	{Key: "print.defaultDocumentFormat", Area: "print", Name: "Default document format", Type: "string"},
	// scan
	{Key: "scan.email.enabled", Area: "scan", Name: "Scan to email enabled", Type: "bool"},
	{Key: "scan.folder.enabled", Area: "scan", Name: "Scan to folder enabled", Type: "bool"},
	{Key: "scan.smtp.server", Area: "scan", Name: "SMTP server", Type: "string"},
}

// CatalogByKey indexes the catalogue.
var CatalogByKey = func() map[string]SettingDef {
	m := make(map[string]SettingDef, len(Catalog))
	for _, d := range Catalog {
		m[d.Key] = d
	}
	return m
}()

// SecurityKeys lists the keys included in read_security documents.
func SecurityKeys() []string {
	var out []string
	for _, d := range Catalog {
		if d.Security {
			out = append(out, d.Key)
		}
	}
	return out
}
