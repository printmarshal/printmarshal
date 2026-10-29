package fleet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProbeWebURL(t *testing.T) {
	client := newWebClient()
	ctx := context.Background()

	// A plain HTTP server: https fails to handshake, http answers.
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/" {
			t.Errorf("unexpected probe %s %s", r.Method, r.URL.Path)
		}
		http.Redirect(w, r, "/login", http.StatusFound) // a redirect still counts as answering
	}))
	defer plain.Close()
	host := strings.TrimPrefix(plain.URL, "http://")
	if got := probeWebURL(ctx, client, host); got != "http://"+host+"/" {
		t.Errorf("plain: got %q", got)
	}

	// A TLS server with a self-signed certificate is preferred over nothing.
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer secure.Close()
	host = strings.TrimPrefix(secure.URL, "https://")
	if got := probeWebURL(ctx, client, host); got != "https://"+host+"/" {
		t.Errorf("tls: got %q", got)
	}

	// Nothing listening: both attempts fail quickly and the probe returns "".
	closed := httptest.NewServer(http.NotFoundHandler())
	host = strings.TrimPrefix(closed.URL, "http://")
	closed.Close()
	start := time.Now()
	if got := probeWebURL(ctx, client, host); got != "" {
		t.Errorf("closed port: got %q", got)
	}
	if d := time.Since(start); d > 2*webProbeTimeout {
		t.Errorf("closed port took %v", d)
	}

	// A cancelled context stops the probe without a result.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if got := probeWebURL(cctx, client, strings.TrimPrefix(plain.URL, "http://")); got != "" {
		t.Errorf("cancelled: got %q", got)
	}
}
