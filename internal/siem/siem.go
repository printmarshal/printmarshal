// Package siem exports PrintMarshal's security events to enterprise log
// collectors and receives syslog from printers (audit A12, docs/export.md).
//
// The Exporter is an events.Bus subscriber. Every catalogued event whose
// Security flag is set, every alert.* event and notification.dead is queued
// for a single worker that writes it to the configured sinks: RFC 5424
// syslog (UDP, TCP or TLS; syslog, CEF or JSON payloads), a JSON-lines file
// with size rotation, and an HTTP collector that receives JSON-lines batches.
// The queue is bounded: when it is full the event is dropped, counted and
// logged, so the bus never blocks on a slow collector.
package siem

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/events"
	"printmarshal/internal/store"
)

// Sink receives formatted events. Write must not block for long; a sink that
// talks to the network keeps its own deadlines and reconnects itself.
type Sink interface {
	Name() string
	Write(env events.Envelope) error
	// Flush sends anything buffered (batching sinks); called periodically and at shutdown.
	Flush() error
	Close() error
	Status() SinkStatus
}

// SinkStatus is what GET /api/export/status reports per sink.
type SinkStatus struct {
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Target    string     `json:"target"`
	Format    string     `json:"format,omitempty"`
	OK        bool       `json:"ok"` // last write succeeded (or nothing has been written yet)
	Sent      int64      `json:"sent"`
	Failed    int64      `json:"failed"`
	LastError string     `json:"lastError,omitempty"`
	LastAt    *time.Time `json:"lastAt,omitempty"`
	Pending   int        `json:"pending,omitempty"` // buffered, not yet delivered (batching sinks)
}

// Status is the whole exporter's state.
type Status struct {
	Enabled    bool         `json:"enabled"`
	Sinks      []SinkStatus `json:"sinks"`
	Queued     int          `json:"queued"`
	QueueSize  int          `json:"queueSize"`
	Received   int64        `json:"received"` // events accepted into the queue
	Dropped    int64        `json:"dropped"`  // events dropped because the queue was full
	Events     []string     `json:"events"`   // event types that are exported
	Ingest     IngestStatus `json:"ingest"`
	Collection []string     `json:"syncCollections"`
}

// counters is shared bookkeeping for sinks.
type counters struct {
	sent, failed atomic.Int64
	mu           sync.Mutex
	lastErr      string
	lastAt       time.Time
	ok           bool
}

func (c *counters) good() {
	c.sent.Add(1)
	c.mu.Lock()
	c.lastErr, c.lastAt, c.ok = "", time.Now().UTC(), true
	c.mu.Unlock()
}

func (c *counters) bad(err error) {
	c.failed.Add(1)
	c.mu.Lock()
	c.lastErr, c.lastAt, c.ok = err.Error(), time.Now().UTC(), false
	c.mu.Unlock()
}

func (c *counters) fill(s *SinkStatus) {
	s.Sent, s.Failed = c.sent.Load(), c.failed.Load()
	c.mu.Lock()
	s.LastError = c.lastErr
	s.OK = c.ok || (c.sent.Load() == 0 && c.failed.Load() == 0)
	if !c.lastAt.IsZero() {
		t := c.lastAt
		s.LastAt = &t
	}
	c.mu.Unlock()
}

// Exporter fans security events out to sinks from a single worker.
type Exporter struct {
	cfg     config.Export
	log     *slog.Logger
	version string
	sinks   []Sink
	queue   chan events.Envelope
	ingest  *Ingest

	received, dropped atomic.Int64
	lastDropLog       atomic.Int64 // unix seconds
	stopped           atomic.Bool  // worker has exited: Handle writes synchronously

	// Flush interval for batching sinks and the worker's tick (tests shorten it).
	Tick time.Duration
}

// New builds an exporter (and its ingest listener, when configured) from
// config. Sinks that fail to initialise are logged and skipped; a mistyped
// collector address must never stop the server. st is used by the ingest
// listener; it needs store.LogStore (the SQLite store has it).
func New(cfg config.Export, st store.Store, log *slog.Logger, version string) *Exporter {
	if cfg.Buffer <= 0 {
		cfg.Buffer = 4096
	}
	e := &Exporter{cfg: cfg, log: log, version: version, queue: make(chan events.Envelope, cfg.Buffer), Tick: time.Second}
	if cfg.Syslog.Address != "" {
		s, err := newSyslogSink(cfg.Syslog, version)
		if err != nil {
			log.Error("export: syslog sink disabled", "address", cfg.Syslog.Address, "err", err)
		} else {
			e.sinks = append(e.sinks, s)
		}
	}
	if cfg.File.Path != "" {
		s, err := newFileSink(cfg.File)
		if err != nil {
			log.Error("export: file sink disabled", "path", cfg.File.Path, "err", err)
		} else {
			e.sinks = append(e.sinks, s)
		}
	}
	if cfg.HTTP.URL != "" {
		s, err := newHTTPSink(cfg.HTTP, log)
		if err != nil {
			log.Error("export: http sink disabled", "url", cfg.HTTP.URL, "err", err)
		} else {
			e.sinks = append(e.sinks, s)
		}
	}
	if cfg.IngestListen != "" {
		if ls, ok := st.(store.LogStore); ok {
			e.ingest = NewIngest(cfg, st, ls, log)
		} else {
			log.Error("export: syslog ingest disabled: the store does not implement store.LogStore")
		}
	}
	return e
}

// AddSink registers an extra sink (tests, plugins).
func (e *Exporter) AddSink(s Sink) { e.sinks = append(e.sinks, s) }

// Ingest returns the printer syslog receiver, or nil when it is off.
func (e *Exporter) Ingest() *Ingest { return e.ingest }

// Enabled reports whether any sink is configured.
func (e *Exporter) Enabled() bool { return len(e.sinks) > 0 }

// Attach subscribes the exporter to the bus.
func (e *Exporter) Attach(bus *events.Bus) { bus.Subscribe(e) }

// Exported reports whether an event type is forwarded to the sinks: every
// catalogued security event, every alert.* event and notification.dead.
func Exported(t string) bool {
	if strings.HasPrefix(t, "alert.") || t == "notification.dead" {
		return true
	}
	d, ok := events.Lookup(t)
	return ok && d.Security
}

// ExportedTypes lists the exported event types in catalogue order.
func ExportedTypes() []string {
	var out []string
	for _, d := range events.Catalog {
		if Exported(d.Type) {
			out = append(out, d.Type)
		}
	}
	return out
}

// Handle implements events.Subscriber: queue without blocking. Once the
// worker has stopped (shutdown), events such as server.stopping are written
// synchronously in the publisher's goroutine, bounded by the sinks' own
// deadlines, so the last events are not lost with the process.
func (e *Exporter) Handle(env events.Envelope) {
	if len(e.sinks) == 0 || !Exported(env.Type) {
		return
	}
	if e.stopped.Load() {
		e.received.Add(1)
		e.write(env)
		for _, s := range e.sinks {
			_ = s.Close() // flushes batching sinks and releases connections again
		}
		return
	}
	select {
	case e.queue <- env:
		e.received.Add(1)
	default:
		n := e.dropped.Add(1)
		now := time.Now().Unix()
		if last := e.lastDropLog.Load(); now-last >= 10 && e.lastDropLog.CompareAndSwap(last, now) {
			e.log.Error("export: queue full, dropping events", "dropped", n, "queueSize", cap(e.queue), "type", env.Type)
		}
	}
}

// Run drives the worker (and the ingest listener) until ctx ends, then
// flushes and closes every sink.
func (e *Exporter) Run(ctx context.Context) {
	if e.ingest != nil {
		go func() {
			if err := e.ingest.Run(ctx); err != nil {
				e.log.Error("syslog ingest stopped", "err", err)
			}
		}()
	}
	if len(e.sinks) == 0 {
		<-ctx.Done()
		return
	}
	e.log.Info("export: started", "sinks", len(e.sinks), "queueSize", cap(e.queue))
	t := time.NewTicker(e.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			e.stopped.Store(true)
			e.drain()
			for _, s := range e.sinks {
				_ = s.Flush()
				_ = s.Close()
			}
			return
		case env := <-e.queue:
			e.write(env)
		case <-t.C:
			for _, s := range e.sinks {
				if err := s.Flush(); err != nil {
					e.log.Warn("export: flush failed", "sink", s.Name(), "err", err)
				}
			}
		}
	}
}

// drain writes whatever is still queued at shutdown (bounded by the queue size).
func (e *Exporter) drain() {
	for {
		select {
		case env := <-e.queue:
			e.write(env)
		default:
			return
		}
	}
}

func (e *Exporter) write(env events.Envelope) {
	for _, s := range e.sinks {
		if err := s.Write(env); err != nil {
			e.log.Warn("export: write failed", "sink", s.Name(), "type", env.Type, "err", err)
		}
	}
}

// Status reports the exporter and sink states.
func (e *Exporter) Status() Status {
	st := Status{Enabled: len(e.sinks) > 0, Sinks: []SinkStatus{}, Queued: len(e.queue), QueueSize: cap(e.queue),
		Received: e.received.Load(), Dropped: e.dropped.Load(), Events: ExportedTypes(), Collection: SyncCollections}
	for _, s := range e.sinks {
		st.Sinks = append(st.Sinks, s.Status())
	}
	if e.ingest != nil {
		st.Ingest = e.ingest.Status()
	} else {
		st.Ingest = IngestStatus{Retention: e.cfg.IngestRetention.D().String()}
	}
	return st
}

// SyncCollections are the collections GET /api/sync/{collection} serves.
var SyncCollections = []string{"devices", "alerts", "audit", "jobs", "config-writes"}
