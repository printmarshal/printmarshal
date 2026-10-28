package vendorkit

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
)

// --- production transports ----------------------------------------------

// HTTPOptions tune the vendor web-service client.
type HTTPOptions struct {
	Timeout     time.Duration
	InsecureTLS bool
}

// DefaultHTTPOptions suit LAN printers, including ones that must wake from
// sleep before their embedded web server answers (observed: ~15 s on an HP
// LaserJet Pro).
func DefaultHTTPOptions() HTTPOptions {
	return HTTPOptions{Timeout: 25 * time.Second, InsecureTLS: true}
}

type httpDoer struct{ c *http.Client }

// NewHTTP returns a read-only HTTP GET client. It never sends anything but GET.
func NewHTTP(opts HTTPOptions) HTTPDoer {
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: opts.InsecureTLS}, //nolint:gosec // printers use self-signed certs; reads only
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Timeout: opts.Timeout}).DialContext,
	}
	return &httpDoer{c: &http.Client{Transport: tr, Timeout: opts.Timeout}}
}

func (h *httpDoer) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml, text/xml, */*")
	resp, err := h.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20))
}

// --- test / simulator transports ------------------------------------------

// FixtureDialer returns a Dialer that always hands out the fixture.
func FixtureDialer(fx *snmp.Fixture) snmp.Dialer {
	return func(ctx context.Context, address string, creds model.Creds) (snmp.Session, string, error) {
		return fx, "v2c", nil
	}
}

// FailingDialer returns a Dialer that never answers.
func FailingDialer(err error) snmp.Dialer {
	if err == nil {
		err = errors.New("snmp timeout")
	}
	return func(ctx context.Context, address string, creds model.Creds) (snmp.Session, string, error) {
		return nil, "", err
	}
}

// StaticIPP is an ipp.Transport that returns one recorded response.
type StaticIPP struct{ Resp []byte }

// Do implements ipp.Transport.
func (s StaticIPP) Do(ctx context.Context, url string, body []byte) ([]byte, error) {
	if len(s.Resp) == 0 {
		return nil, errors.New("ipp: connection refused")
	}
	return s.Resp, nil
}

var _ ipp.Transport = StaticIPP{}

// StaticHTTP is an HTTPDoer that serves recorded pages by URL path.
type StaticHTTP struct{ Pages map[string][]byte }

// Get implements HTTPDoer.
func (s StaticHTTP) Get(ctx context.Context, url string) ([]byte, error) {
	path := url
	if i := strings.Index(url, "://"); i >= 0 {
		rest := url[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			path = rest[j:]
		} else {
			path = "/"
		}
	}
	if b, ok := s.Pages[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("http 404 for %s", path)
}

// NoHTTP is an HTTPDoer for devices without a web service.
type NoHTTP struct{}

// Get implements HTTPDoer.
func (NoHTTP) Get(ctx context.Context, url string) ([]byte, error) {
	return nil, errors.New("http: connection refused")
}
