package siem

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/events"
)

// httpSink POSTs JSON-lines batches (Content-Type application/x-ndjson) to a
// collector, with an optional bearer token read from the environment and a
// small retry with backoff per batch. A batch that still fails is dropped
// and counted; the worker never waits longer than the retry budget.
type httpSink struct {
	url      string
	tokenEnv string
	batch    int
	every    time.Duration
	client   *http.Client
	log      *slog.Logger

	mu       sync.Mutex
	pending  [][]byte
	lastSend time.Time
	counters

	// Attempts per batch and the first backoff (doubles each time).
	Attempts int
	Backoff  time.Duration
}

func newHTTPSink(cfg struct {
	URL           string          `json:"url"`
	TokenEnv      string          `json:"tokenEnv,omitempty"`
	BatchSize     int             `json:"batchSize,omitempty"`
	FlushInterval config.Duration `json:"flushInterval,omitempty"`
	Timeout       config.Duration `json:"timeout,omitempty"`
}, log *slog.Logger) (*httpSink, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("http export url must be http(s)://host/path, got %q", cfg.URL)
	}
	if cfg.TokenEnv != "" && os.Getenv(cfg.TokenEnv) == "" {
		log.Warn("export: http tokenEnv is set but the variable is empty; posting without a bearer token", "tokenEnv", cfg.TokenEnv)
	}
	batch := cfg.BatchSize
	if batch <= 0 {
		batch = 100
	}
	every := cfg.FlushInterval.D()
	if every <= 0 {
		every = 5 * time.Second
	}
	timeout := cfg.Timeout.D()
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &httpSink{url: cfg.URL, tokenEnv: cfg.TokenEnv, batch: batch, every: every, log: log,
		client: &http.Client{Timeout: timeout}, lastSend: time.Now(), Attempts: 3, Backoff: 500 * time.Millisecond}, nil
}

func (s *httpSink) Name() string { return "http" }

func (s *httpSink) Write(env events.Envelope) error {
	s.mu.Lock()
	s.pending = append(s.pending, jsonLine(env))
	full := len(s.pending) >= s.batch
	s.mu.Unlock()
	if full {
		return s.Flush()
	}
	return nil
}

// Flush posts the pending lines when the batch is full or the interval has
// passed (always at shutdown via Close).
func (s *httpSink) Flush() error {
	s.mu.Lock()
	if len(s.pending) == 0 || (len(s.pending) < s.batch && time.Since(s.lastSend) < s.every) {
		s.mu.Unlock()
		return nil
	}
	lines := s.pending
	s.pending = nil
	s.lastSend = time.Now()
	s.mu.Unlock()
	return s.post(lines)
}

func (s *httpSink) post(lines [][]byte) error {
	body := bytes.Join(lines, []byte("\n"))
	body = append(body, '\n')
	var err error
	back := s.Backoff
	for attempt := 1; attempt <= s.Attempts; attempt++ {
		err = s.once(body)
		if err == nil {
			s.sent.Add(int64(len(lines)))
			s.mu.Lock()
			s.lastErr, s.lastAt, s.ok = "", time.Now().UTC(), true
			s.mu.Unlock()
			return nil
		}
		if attempt < s.Attempts {
			time.Sleep(back)
			back *= 2
		}
	}
	s.failed.Add(int64(len(lines)))
	s.mu.Lock()
	s.lastErr, s.lastAt, s.ok = err.Error(), time.Now().UTC(), false
	s.mu.Unlock()
	s.log.Error("export: http batch dropped after retries", "url", s.url, "events", len(lines), "err", err)
	return err
}

func (s *httpSink) once(body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.client.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("User-Agent", "printmarshal-export")
	if s.tokenEnv != "" {
		if tok := os.Getenv(s.tokenEnv); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("collector returned %d", resp.StatusCode)
	}
	return nil
}

// Close sends whatever is pending regardless of the interval.
func (s *httpSink) Close() error {
	s.mu.Lock()
	lines := s.pending
	s.pending = nil
	s.mu.Unlock()
	if len(lines) == 0 {
		return nil
	}
	return s.post(lines)
}

func (s *httpSink) Status() SinkStatus {
	st := SinkStatus{Name: s.Name(), Type: "http", Target: s.url, Format: "json"}
	s.counters.fill(&st)
	s.mu.Lock()
	st.Pending = len(s.pending)
	s.mu.Unlock()
	return st
}
