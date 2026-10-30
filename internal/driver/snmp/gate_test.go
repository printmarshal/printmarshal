package snmp

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubSession struct{ closed int }

func (s *stubSession) Get(context.Context, []string) ([]PDU, error) { return nil, nil }
func (s *stubSession) Walk(context.Context, string) ([]PDU, error)  { return nil, nil }
func (s *stubSession) Set(context.Context, []PDU) error             { return nil }
func (s *stubSession) Close() error                                 { s.closed++; return nil }

func TestGateLimitsSessionsPerAddress(t *testing.T) {
	g := NewGate(2)
	ctx := context.Background()
	r1, err := g.acquire(ctx, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := g.acquire(ctx, "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if n := g.InFlight("10.0.0.1"); n != 2 {
		t.Fatalf("in flight: %d", n)
	}
	// A third session to the same address waits (here until its context ends).
	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := g.acquire(short, "10.0.0.1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third session should wait, got %v", err)
	}
	// Other addresses are independent.
	r3, err := g.acquire(ctx, "10.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	r1()
	r1() // releasing twice is harmless
	r4, err := g.acquire(ctx, "10.0.0.1")
	if err != nil {
		t.Fatalf("after a release the slot should be free: %v", err)
	}
	r2()
	r3()
	r4()
	if n := g.InFlight("10.0.0.1"); n != 0 {
		t.Fatalf("in flight after releases: %d", n)
	}
	// No gate, or a zero limit, never blocks.
	var none *Gate
	if rel, err := none.acquire(short, "x"); err != nil {
		t.Fatal(err)
	} else {
		rel()
	}
	if rel, err := NewGate(0).acquire(short, "x"); err != nil {
		t.Fatal(err)
	} else {
		rel()
	}
}

func TestGatedSessionReleasesOnClose(t *testing.T) {
	g := NewGate(1)
	ctx := context.Background()
	rel, err := g.acquire(ctx, "10.0.0.9")
	if err != nil {
		t.Fatal(err)
	}
	inner := &stubSession{}
	s := &gatedSession{Session: inner, release: rel}
	if err := s.Close(); err != nil || inner.closed != 1 {
		t.Fatalf("close: err=%v closed=%d", err, inner.closed)
	}
	if n := g.InFlight("10.0.0.9"); n != 0 {
		t.Fatalf("slot still held after Close: %d", n)
	}
}
