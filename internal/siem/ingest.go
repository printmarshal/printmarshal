package siem

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"printmarshal/internal/config"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Ingest is the printer syslog receiver: a UDP listener that accepts RFC 5424
// and BSD (RFC 3164) lines, attributes each to a known device by its source
// address (the same trust model as the SNMP trap listener), stores it and
// prunes old lines. It never answers, never polls and never writes to a
// printer; a line is evidence, not a trigger.
type Ingest struct {
	addr      string
	store     store.Store
	logs      store.LogStore
	log       *slog.Logger
	retention time.Duration
	rate      int

	mu     sync.Mutex
	bound  string // actual listen address once bound
	seen   map[string][]time.Time
	denied map[string]time.Time
	cache  map[string]cacheEntry // source ip -> device id

	received, stored, unknown, dropped atomic.Int64
	lastAt                             atomic.Int64 // unix nanos of the last stored line
	ready                              atomic.Bool
}

type cacheEntry struct {
	id string
	at time.Time
}

// IngestStatus is what GET /api/export/status reports for the receiver.
type IngestStatus struct {
	Enabled   bool       `json:"enabled"`
	Listen    string     `json:"listen,omitempty"`
	Listening bool       `json:"listening"`
	Received  int64      `json:"received"`
	Stored    int64      `json:"stored"`
	Unknown   int64      `json:"unknownSource"`
	Dropped   int64      `json:"dropped"`
	LastAt    *time.Time `json:"lastAt,omitempty"`
	Retention string     `json:"retention"`
}

// NewIngest builds the receiver from config.
func NewIngest(cfg config.Export, st store.Store, logs store.LogStore, log *slog.Logger) *Ingest {
	ret := cfg.IngestRetention.D()
	if ret <= 0 {
		ret = 30 * 24 * time.Hour
	}
	rate := cfg.IngestRatePerMinute
	if rate <= 0 {
		rate = 120
	}
	return &Ingest{addr: cfg.IngestListen, store: st, logs: logs, log: log, retention: ret, rate: rate,
		seen: map[string][]time.Time{}, denied: map[string]time.Time{}, cache: map[string]cacheEntry{}}
}

// Addr returns the bound listen address ("" until Run has bound it).
func (in *Ingest) Addr() string {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.bound
}

// Status reports counters.
func (in *Ingest) Status() IngestStatus {
	st := IngestStatus{Enabled: true, Listen: in.addr, Listening: in.ready.Load(), Received: in.received.Load(), Stored: in.stored.Load(),
		Unknown: in.unknown.Load(), Dropped: in.dropped.Load(), Retention: in.retention.String()}
	if n := in.lastAt.Load(); n > 0 {
		t := time.Unix(0, n).UTC()
		st.LastAt = &t
	}
	return st
}

// Run listens until ctx ends and prunes old lines hourly.
func (in *Ingest) Run(ctx context.Context) error {
	pc, err := net.ListenPacket("udp", in.addr)
	if err != nil {
		return err
	}
	in.mu.Lock()
	in.bound = pc.LocalAddr().String()
	in.mu.Unlock()
	in.ready.Store(true)
	in.log.Info("syslog ingest listening", "addr", in.addr, "retention", in.retention)
	go func() {
		<-ctx.Done()
		in.ready.Store(false)
		_ = pc.Close()
	}()
	go in.pruneLoop(ctx)
	buf := make([]byte, 64*1024)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		ip := ""
		if u, ok := from.(*net.UDPAddr); ok {
			ip = u.IP.String()
		} else {
			ip, _, _ = net.SplitHostPort(from.String())
		}
		in.HandleLine(ctx, ip, string(buf[:n]))
	}
}

func (in *Ingest) pruneLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		in.Prune(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Prune deletes lines older than the retention.
func (in *Ingest) Prune(ctx context.Context) {
	n, err := in.logs.PruneDeviceLogs(ctx, time.Now().Add(-in.retention))
	if err != nil {
		in.log.Warn("device log prune", "err", err)
	} else if n > 0 {
		in.log.Info("device logs pruned", "deleted", n)
	}
}

// accept applies the per-source rate limit (sliding minute), logging at most
// one drop per source per minute.
func (in *Ingest) accept(ip string) bool {
	now := time.Now()
	in.mu.Lock()
	defer in.mu.Unlock()
	keep := in.seen[ip][:0]
	for _, t := range in.seen[ip] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= in.rate {
		in.seen[ip] = keep
		in.dropped.Add(1)
		if t, ok := in.denied[ip]; !ok || time.Since(t) >= time.Minute {
			in.denied[ip] = now
			in.log.Warn("syslog line dropped", "source", ip, "reason", "rate limit exceeded", "perMinute", in.rate)
		}
		return false
	}
	in.seen[ip] = append(keep, now)
	return true
}

// deviceFor resolves a source address to a device id (cached for a minute).
func (in *Ingest) deviceFor(ctx context.Context, ip string) string {
	in.mu.Lock()
	if c, ok := in.cache[ip]; ok && time.Since(c.at) < time.Minute {
		in.mu.Unlock()
		return c.id
	}
	in.mu.Unlock()
	id := ""
	devs, err := in.store.ListDevices(ctx, store.DeviceFilter{Q: ip, IncludeDecommissioned: true})
	if err == nil {
		for _, d := range devs {
			if d.Address == ip {
				id = d.ID
				break
			}
		}
	}
	in.mu.Lock()
	in.cache[ip] = cacheEntry{id: id, at: time.Now()}
	in.mu.Unlock()
	return id
}

// HandleLine processes one datagram from ip (exported for tests).
func (in *Ingest) HandleLine(ctx context.Context, ip, raw string) {
	in.received.Add(1)
	if !in.accept(ip) {
		return
	}
	id := in.deviceFor(ctx, ip)
	if id == "" {
		in.unknown.Add(1)
		in.mu.Lock()
		if t, ok := in.denied["unknown:"+ip]; !ok || time.Since(t) >= time.Minute {
			in.denied["unknown:"+ip] = time.Now()
			in.log.Info("syslog from unknown device", "source", ip)
		}
		in.mu.Unlock()
		return
	}
	line := ParseSyslog(raw)
	l := &model.DeviceLog{DeviceID: id, At: time.Now().UTC(), Facility: line.Facility, Severity: line.Severity, Message: line.Message}
	if err := in.logs.AddDeviceLog(ctx, l); err != nil {
		in.log.Error("store device log", "device", id, "err", err)
		return
	}
	in.stored.Add(1)
	in.lastAt.Store(l.At.UnixNano())
}

// Line is a parsed syslog datagram.
type Line struct {
	Facility int
	Severity int
	Host     string
	App      string
	Message  string
}

const maxMessage = 2048

// ParseSyslog decodes an RFC 5424 or BSD syslog line. Anything it cannot
// parse is kept verbatim as the message with facility user(1), severity
// notice(5), so no line is lost.
func ParseSyslog(raw string) Line {
	l := Line{Facility: 1, Severity: 5}
	s := strings.TrimRight(raw, "\r\n\x00")
	if strings.HasPrefix(s, "<") {
		if end := strings.IndexByte(s, '>'); end > 1 && end <= 4 {
			if pri, err := strconv.Atoi(s[1:end]); err == nil && pri >= 0 && pri <= 191 {
				l.Facility, l.Severity = pri/8, pri%8
				s = s[end+1:]
			}
		}
	}
	switch {
	case strings.HasPrefix(s, "1 "):
		parse5424(&l, s[2:])
	default:
		parse3164(&l, s)
	}
	l.Message = clean(l.Message)
	return l
}

// parse5424 reads "TIMESTAMP HOSTNAME APP-NAME PROCID MSGID SD MSG".
func parse5424(l *Line, s string) {
	fields := strings.SplitN(s, " ", 6)
	if len(fields) < 6 {
		l.Message = s
		return
	}
	l.Host, l.App = nil2(fields[1]), nil2(fields[2])
	rest := fields[5]
	// Structured data: "-" or one or more [ ... ] elements (']' inside values is escaped).
	if strings.HasPrefix(rest, "-") {
		rest = strings.TrimPrefix(rest[1:], " ")
	} else {
		for strings.HasPrefix(rest, "[") {
			i := 1
			for i < len(rest) {
				if rest[i] == '\\' {
					i += 2
					continue
				}
				if rest[i] == ']' {
					break
				}
				i++
			}
			if i >= len(rest) {
				rest = ""
				break
			}
			rest = rest[i+1:]
		}
		rest = strings.TrimPrefix(rest, " ")
	}
	rest = strings.TrimPrefix(rest, "\xef\xbb\xbf") // optional BOM before a UTF-8 MSG
	l.Message = rest
	if l.App != "" {
		l.Message = l.App + ": " + l.Message
	}
}

// parse3164 reads "Mmm dd hh:mm:ss HOST MSG" leniently.
func parse3164(l *Line, s string) {
	fields := strings.Fields(s)
	if len(fields) >= 5 && len(fields[0]) == 3 && strings.Contains(fields[2], ":") {
		if _, err := time.Parse("Jan 2 15:04:05", strings.Join(fields[:3], " ")); err == nil {
			l.Host = fields[3]
			l.Message = strings.Join(fields[4:], " ")
			return
		}
	}
	l.Message = strings.TrimSpace(s)
}

func nil2(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

// clean bounds the message and strips control characters.
func clean(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "�")
	}
	s = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' {
			return ' '
		}
		return r
	}, s)
	if len(s) > maxMessage {
		s = s[:maxMessage]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}
