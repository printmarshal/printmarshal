package model

import "time"

// DeviceLog is one syslog line a printer sent to PrintMarshal's receiver
// (docs/export.md). Lines are attributed to a device by source address only;
// they are evidence, never a trigger for any automatic action.
type DeviceLog struct {
	ID       int64     `json:"id"`
	DeviceID string    `json:"deviceId"`
	At       time.Time `json:"at"`       // time of receipt (printer clocks are not trusted)
	Facility int       `json:"facility"` // syslog facility 0-23
	Severity int       `json:"severity"` // syslog severity 0 (emergency) - 7 (debug)
	Message  string    `json:"message"`
}

// SeverityName returns the RFC 5424 severity keyword.
func (l DeviceLog) SeverityName() string { return SyslogSeverityName(l.Severity) }

// SyslogSeverityName maps a syslog severity number to its keyword.
func SyslogSeverityName(n int) string {
	names := [...]string{"emergency", "alert", "critical", "error", "warning", "notice", "info", "debug"}
	if n < 0 || n >= len(names) {
		return "unknown"
	}
	return names[n]
}

// SyslogFacilityName maps a syslog facility number to its keyword.
func SyslogFacilityName(n int) string {
	names := [...]string{"kern", "user", "mail", "daemon", "auth", "syslog", "lpr", "news", "uucp", "cron", "authpriv", "ftp", "ntp", "audit", "alert", "clock",
		"local0", "local1", "local2", "local3", "local4", "local5", "local6", "local7"}
	if n < 0 || n >= len(names) {
		return "unknown"
	}
	return names[n]
}
