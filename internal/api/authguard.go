package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginLimiter throttles failed sign-ins per source address (audit A05): after
// `limit` failures within a minute the address must wait before trying again.
type loginLimiter struct {
	mu    sync.Mutex
	limit int
	fails map[string][]time.Time
}

func newLoginLimiter(limit int) *loginLimiter {
	if limit <= 0 {
		limit = 10
	}
	return &loginLimiter{limit: limit, fails: map[string][]time.Time{}}
}

func (l *loginLimiter) prune(ip string, now time.Time) []time.Time {
	ts := l.fails[ip]
	keep := ts[:0]
	for _, t := range ts {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(l.fails, ip)
		return nil
	}
	l.fails[ip] = keep
	return keep
}

// Blocked reports whether ip has exceeded the failure budget.
func (l *loginLimiter) Blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(ip, time.Now())) >= l.limit
}

// Fail records a failure.
func (l *loginLimiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.fails[ip] = append(l.prune(ip, now), now)
}

// Reset clears failures after a successful sign-in.
func (l *loginLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}

// clientIP returns the peer address without the port. Proxy headers are
// deliberately not trusted here; terminate TLS and rate-limit at the proxy
// when one is in front of PrintMarshal (docs/security.md).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
