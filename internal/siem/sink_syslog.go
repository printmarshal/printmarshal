package siem

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"printmarshal/internal/events"
)

// syslogSink sends RFC 5424 messages over UDP (one datagram per message),
// TCP or TLS (octet-counted framing, RFC 6587 / RFC 5425). No third-party
// library: the framing is a length prefix.
type syslogSink struct {
	scheme, addr string
	format       string
	facility     int
	version      string
	hostname     string
	tlsCfg       *tls.Config

	mu       sync.Mutex
	conn     net.Conn
	lastDial time.Time
	counters

	// DialTimeout / WriteTimeout bound every network step.
	DialTimeout, WriteTimeout time.Duration
	// RedialAfter is the minimum pause between failed dial attempts.
	RedialAfter time.Duration
}

const (
	maxUDPMessage = 8 * 1024
	maxTCPMessage = 64 * 1024
)

func newSyslogSink(cfg struct {
	Address     string `json:"address"`
	Format      string `json:"format"`
	Facility    int    `json:"facility,omitempty"`
	TLSInsecure bool   `json:"tlsInsecure,omitempty"`
	CAFile      string `json:"caFile,omitempty"`
}, version string) (*syslogSink, error) {
	u, err := url.Parse(cfg.Address)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "udp", "tcp", "tls":
	default:
		return nil, fmt.Errorf("syslog address must be udp://, tcp:// or tls://, got %q", cfg.Address)
	}
	host := u.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "514")
	}
	format := cfg.Format
	switch format {
	case "", "syslog":
		format = "syslog"
	case "cef", "json":
	default:
		return nil, fmt.Errorf("syslog format must be syslog, cef or json, got %q", format)
	}
	s := &syslogSink{scheme: u.Scheme, addr: host, format: format, facility: cfg.Facility, version: version, hostname: hostname(),
		DialTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, RedialAfter: 2 * time.Second}
	if u.Scheme == "tls" {
		h, _, _ := net.SplitHostPort(host)
		s.tlsCfg = &tls.Config{ServerName: h, MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.TLSInsecure} //nolint:gosec // operator opt-in
		if cfg.CAFile != "" {
			pem, err := os.ReadFile(cfg.CAFile)
			if err != nil {
				return nil, fmt.Errorf("syslog caFile: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("syslog caFile %s: no certificates found", cfg.CAFile)
			}
			s.tlsCfg.RootCAs = pool
		}
	}
	return s, nil
}

func (s *syslogSink) Name() string { return "syslog" }

func (s *syslogSink) dial() (net.Conn, error) {
	if s.conn != nil {
		return s.conn, nil
	}
	if !s.lastDial.IsZero() && time.Since(s.lastDial) < s.RedialAfter {
		return nil, fmt.Errorf("syslog %s unreachable; retrying shortly", s.addr)
	}
	s.lastDial = time.Now()
	d := &net.Dialer{Timeout: s.DialTimeout}
	var c net.Conn
	var err error
	switch s.scheme {
	case "udp":
		c, err = d.Dial("udp", s.addr)
	case "tcp":
		c, err = d.Dial("tcp", s.addr)
	case "tls":
		c, err = tls.DialWithDialer(d, "tcp", s.addr, s.tlsCfg)
	}
	if err != nil {
		return nil, err
	}
	s.conn = c
	return c, nil
}

func (s *syslogSink) Write(env events.Envelope) error {
	msg := formatSyslog(env, s.format, s.facility, s.hostname, s.version)
	err := s.send(msg)
	if err != nil {
		s.bad(err)
		return err
	}
	s.good()
	return nil
}

func (s *syslogSink) send(msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.dial()
	if err != nil {
		return err
	}
	var frame []byte
	if s.scheme == "udp" {
		if len(msg) > maxUDPMessage {
			msg = msg[:maxUDPMessage]
		}
		frame = []byte(msg)
	} else {
		if len(msg) > maxTCPMessage {
			msg = msg[:maxTCPMessage]
		}
		frame = append([]byte(strconv.Itoa(len(msg))+" "), msg...)
	}
	_ = c.SetWriteDeadline(time.Now().Add(s.WriteTimeout))
	if _, err := c.Write(frame); err != nil {
		_ = c.Close()
		s.conn = nil
		return err
	}
	return nil
}

func (s *syslogSink) Flush() error { return nil }

func (s *syslogSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		err := s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}

func (s *syslogSink) Status() SinkStatus {
	st := SinkStatus{Name: s.Name(), Type: "syslog", Target: s.scheme + "://" + s.addr, Format: s.format}
	s.counters.fill(&st)
	return st
}
