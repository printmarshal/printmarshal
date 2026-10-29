package fleet

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"

	"printmarshal/internal/model"
)

// Web-page probe (issue #7). Most printers serve their administration UI on
// 443 or 80 but advertise nothing about it, so after a device is identified,
// and again on a poll while nothing is known, PrintMarshal asks https:// then
// http:// on the device's host for "/" and keeps the first that answers with
// any HTTP response. The probe is a plain GET of the root page: it reads
// nothing from the device and changes nothing on it. Self-signed
// certificates are expected and not verified here; the operator's browser
// warns when the link is opened.

// webProbeTimeout bounds each of the two attempts.
const webProbeTimeout = 3 * time.Second

// newWebClient builds the client the probe uses: no redirects (a 30x already
// proves a web server), no TLS verification, short dial and header timeouts.
func newWebClient() *http.Client {
	return &http.Client{
		Timeout: webProbeTimeout,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: webProbeTimeout}).DialContext,
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // device certificates are self-signed; see package comment
			TLSHandshakeTimeout:   webProbeTimeout,
			ResponseHeaderTimeout: webProbeTimeout,
			DisableKeepAlives:     true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// probeWebURL returns the first of https://host/ and http://host/ that
// answers, or "" when neither does. ctx bounds the whole probe.
func probeWebURL(ctx context.Context, client *http.Client, host string) string {
	if host == "" || client == nil {
		return ""
	}
	for _, scheme := range []string{"https", "http"} {
		u := scheme + "://" + host + "/"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "PrintMarshal web-page probe")
		resp, err := client.Do(req)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ""
			}
			continue
		}
		resp.Body.Close()
		return u
	}
	return ""
}

// ensureWebURL probes a device that has no web page yet and stores the
// result. Devices with an operator override, an advertised web UI, or an
// earlier probe result are left alone; force re-probes a device whose last
// probe found nothing (identification does this, polls do not).
func (s *Service) ensureWebURL(ctx context.Context, dev *model.Device, force bool) {
	if dev == nil || dev.WebURLOverride != "" || model.IsWebUI(dev.AdminURL) {
		return
	}
	if dev.WebURLProbed != "" && (dev.WebURLProbed != model.WebURLNone || !force) {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 2*webProbeTimeout)
	defer cancel()
	found := probeWebURL(pctx, s.webClient(), model.WebHost(dev))
	if found == "" {
		found = model.WebURLNone
	}
	if found == dev.WebURLProbed {
		return
	}
	if err := s.Store.SetDeviceWebURLProbed(ctx, dev.ID, found); err != nil {
		s.Log.Debug("web page probe not stored", "device", dev.ID, "err", err)
		return
	}
	dev.WebURLProbed = found
	dev.WebURL = model.ResolveWebURL(dev)
}

func (s *Service) webClient() *http.Client {
	if s.WebClient == nil {
		s.WebClient = newWebClient()
	}
	return s.WebClient
}
