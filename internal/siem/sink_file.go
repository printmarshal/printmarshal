package siem

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"printmarshal/internal/events"
)

// fileSink appends one JSON line per event and rotates by size:
// path -> path.1 -> path.2 ... up to keep files; the oldest is removed.
type fileSink struct {
	path    string
	maxSize int64
	keep    int

	mu   sync.Mutex
	f    *os.File
	size int64
	counters
}

func newFileSink(cfg struct {
	Path      string `json:"path"`
	MaxSizeMB int    `json:"maxSizeMb,omitempty"`
	Keep      int    `json:"keep,omitempty"`
}) (*fileSink, error) {
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 50
	}
	if cfg.Keep <= 0 {
		cfg.Keep = 5
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o750); err != nil {
		return nil, err
	}
	s := &fileSink{path: cfg.Path, maxSize: int64(cfg.MaxSizeMB) << 20, keep: cfg.Keep}
	if err := s.open(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *fileSink) Name() string { return "file" }

func (s *fileSink) open() error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	s.f, s.size = f, st.Size()
	return nil
}

// rotate shifts path.N-1 -> path.N and reopens path. Rename failures for
// missing generations are ignored.
func (s *fileSink) rotate() error {
	if s.f != nil {
		_ = s.f.Close()
		s.f = nil
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", s.path, s.keep))
	for i := s.keep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", s.path, i), fmt.Sprintf("%s.%d", s.path, i+1))
	}
	if err := os.Rename(s.path, s.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return s.open()
}

func (s *fileSink) Write(env events.Envelope) error {
	line := append(jsonLine(env), '\n')
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		if err := s.open(); err != nil {
			s.bad(err)
			return err
		}
	}
	if s.size > 0 && s.size+int64(len(line)) > s.maxSize {
		if err := s.rotate(); err != nil {
			s.bad(err)
			return err
		}
	}
	n, err := s.f.Write(line)
	s.size += int64(n)
	if err != nil {
		s.bad(err)
		return err
	}
	s.good()
	return nil
}

func (s *fileSink) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		return s.f.Sync()
	}
	return nil
}

func (s *fileSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f != nil {
		err := s.f.Close()
		s.f = nil
		return err
	}
	return nil
}

func (s *fileSink) Status() SinkStatus {
	st := SinkStatus{Name: s.Name(), Type: "file", Target: s.path, Format: "json"}
	s.counters.fill(&st)
	return st
}
