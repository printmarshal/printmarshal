package radius

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/auth/radius/radiustest"
)

func TestPAPExchange(t *testing.T) {
	s := radiustest.New(t, "s3cret")
	s.Users["ann"] = "annpass"
	s.Users["long"] = strings.Repeat("x", 100)
	s.Reply["ann"] = []radiustest.Attr{{Type: AttrFilterID, Value: "printer-admins"}, {Type: AttrClass, Value: "ou=staff"}, {Type: AttrFilterID, Value: "staff"}}
	cfg := Config{Servers: []string{s.Addr}, Secret: "s3cret", Timeout: 500 * time.Millisecond, Retries: 1}

	res, err := Authenticate(context.Background(), cfg, "ann", "annpass")
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Values("Filter-Id"); len(got) != 2 || got[0] != "printer-admins" || got[1] != "staff" {
		t.Fatalf("filter-id %v", got)
	}
	if got := res.Values("Class"); len(got) != 1 || got[0] != "ou=staff" {
		t.Fatalf("class %v", got)
	}
	if res.Values("bogus") != nil || res.Values("18") != nil {
		t.Fatal("unexpected values")
	}
	if s.LastUser != "ann" || s.LastPassword != "annpass" || s.BadMAC != 0 {
		t.Fatalf("server saw %q/%q badmac=%d", s.LastUser, s.LastPassword, s.BadMAC)
	}
	// Passwords longer than 16 bytes span several XOR blocks.
	if _, err := Authenticate(context.Background(), cfg, "long", strings.Repeat("x", 100)); err != nil || s.LastPassword != strings.Repeat("x", 100) {
		t.Fatalf("long password: %v", err)
	}
	// Wrong password and unknown user are both ErrRejected; nothing else leaks.
	for _, c := range [][2]string{{"ann", "nope"}, {"zed", "annpass"}, {"ann", ""}} {
		if _, err := Authenticate(context.Background(), cfg, c[0], c[1]); !errors.Is(err, ErrRejected) {
			t.Fatalf("%q: %v", c[0], err)
		}
	}
	if cfg.Issuer() != "radius:"+s.Addr || Subject(" Ann ") != "ann" {
		t.Fatal("issuer/subject")
	}
}

func TestRetriesAndFailover(t *testing.T) {
	s := radiustest.New(t, "s3cret")
	s.Users["ann"] = "annpass"
	s.Drop = 1
	cfg := Config{Servers: []string{"127.0.0.1:1", s.Addr}, Secret: "s3cret", Timeout: 300 * time.Millisecond, Retries: 1}
	// Port 1 never answers; the second server drops the first packet and
	// answers the retransmission.
	if _, err := Authenticate(context.Background(), cfg, "ann", "annpass"); err != nil {
		t.Fatal(err)
	}
	if s.Requests != 2 {
		t.Fatalf("requests %d", s.Requests)
	}
	// Nothing answers: a transport error, not a rejection.
	cfg.Servers = []string{"127.0.0.1:1"}
	cfg.Retries = 0
	start := time.Now()
	if _, err := Authenticate(context.Background(), cfg, "ann", "annpass"); err == nil || errors.Is(err, ErrRejected) {
		t.Fatalf("dead server: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout not honoured")
	}
	// Context cancellation wins over retries.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	cfg.Timeout, cfg.Retries = 5*time.Second, 3
	if _, err := Authenticate(ctx, cfg, "ann", "annpass"); err == nil {
		t.Fatal("context ignored")
	}
}

func TestForgedReplies(t *testing.T) {
	s := radiustest.New(t, "s3cret")
	s.Users["ann"] = "annpass"
	// A reply signed with the wrong secret fails the Response Authenticator
	// check and is treated as no reply.
	s.ReplySecret = "other"
	cfg := Config{Servers: []string{s.Addr}, Secret: "s3cret", Timeout: 200 * time.Millisecond, Retries: 0}
	if _, err := Authenticate(context.Background(), cfg, "ann", "annpass"); err == nil || errors.Is(err, ErrRejected) {
		t.Fatalf("forged accept: %v", err)
	}
	// Direct checks on parseReply: accept with a bad Message-Authenticator.
	req, auth, _ := buildRequest("s3cret", "ann", "annpass", "nas")
	good := radiustest.Build(CodeAccessAccept, req[1], auth, nil, "s3cret")
	if _, err := parseReply(good, req, auth, "s3cret"); err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), good...)
	bad[len(bad)-1] ^= 1 // a flipped byte also breaks the Response Authenticator
	if _, err := parseReply(bad, req, auth, "s3cret"); err == nil {
		t.Fatal("tampered reply accepted")
	}
	// A valid Response Authenticator around a wrong Message-Authenticator
	// (a buggy or hostile relay) is refused on the second check.
	resp := radiustest.Build(CodeAccessAccept, req[1], auth, nil, "s3cret")
	resp[len(resp)-1] ^= 1
	resp = radiustest.Resign(resp, auth, "s3cret")
	if _, err := parseReply(resp, req, auth, "s3cret"); err == nil || !strings.Contains(err.Error(), "message authenticator") {
		t.Fatalf("bad message authenticator: %v", err)
	}
	wrongID := append([]byte(nil), good...)
	wrongID[1]++
	if _, err := parseReply(wrongID, req, auth, "s3cret"); err == nil {
		t.Fatal("identifier mismatch accepted")
	}
	if _, err := parseReply(good[:10], req, auth, "s3cret"); err == nil {
		t.Fatal("short reply accepted")
	}
	if _, err := Authenticate(context.Background(), Config{}, "ann", "x"); err == nil {
		t.Fatal("empty config accepted")
	}
	if _, err := Authenticate(context.Background(), cfg, "ann", strings.Repeat("x", 129)); err == nil {
		t.Fatal("129-byte password accepted")
	}
}

func TestPasswordHiding(t *testing.T) {
	auth := []byte("0123456789abcdef")
	for _, pw := range []string{"a", "exactly16bytes!!", "seventeen bytes!!", strings.Repeat("z", 128)} {
		if got := string(RevealPassword("s", auth, hidePassword("s", auth, []byte(pw)))); got != pw {
			t.Fatalf("%q -> %q", pw, got)
		}
	}
	if RevealPassword("s", auth, []byte{1, 2, 3}) != nil {
		t.Fatal("odd length accepted")
	}
}
