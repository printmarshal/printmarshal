// Package escl is the standards eSCL (AirScan / Mopria Scan) driver (spec §7).
// It reads a device's ScannerCapabilities and ScannerStatus documents over
// HTTP(S) to identify multifunction devices and report scanner state. It is
// read-only and vendor-neutral: the XML schema is the PWG/Mopria eSCL one.
package escl

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// Name is the driver's registry name.
const Name = "escl"

// Transport fetches a URL and returns the body.
type Transport interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// Options tune the driver.
type Options struct {
	Timeout     time.Duration
	InsecureTLS bool
	// Bases are URL templates tried in order; %s is the host.
	Bases []string
}

// DefaultOptions suit LAN devices.
func DefaultOptions() Options { return Options{Timeout: 4 * time.Second, InsecureTLS: true} }

var defaultBases = []string{"http://%s/eSCL", "http://%s:8080/eSCL", "https://%s/eSCL", "https://%s:443/eSCL"}

type httpTransport struct{ c *http.Client }

func (t *httpTransport) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/xml, application/xml")
	resp, err := t.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// NewTransport returns the production HTTP transport.
func NewTransport(opts Options) Transport {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.InsecureTLS}, //nolint:gosec // printers use self-signed certs; reads only
		DisableKeepAlives: true, DialContext: (&net.Dialer{Timeout: opts.Timeout}).DialContext}
	return &httpTransport{c: &http.Client{Transport: tr, Timeout: opts.Timeout}}
}

// Driver implements engine.Driver + engine.Identifier.
type Driver struct {
	tr    Transport
	bases []string
}

// New returns a driver.
func New(tr Transport, opts Options) *Driver {
	b := opts.Bases
	if len(b) == 0 {
		b = defaultBases
	}
	return &Driver{tr: tr, bases: b}
}

func (d *Driver) Name() string                     { return Name }
func (d *Driver) Capabilities() []model.Capability { return []model.Capability{model.CapIdentify} }
func (d *Driver) Supports(dev *model.Device, cap model.Capability) bool {
	return engine.ProfileSupports(dev, Name, cap)
}

// Priority is below SNMP/IPP: eSCL confirms a scanner and fills gaps only.
func (d *Driver) Priority(model.Capability) int { return 20 }

// Capabilities is the ScannerCapabilities document (eSCL 2.x, PWG SM namespace).
type Capabilities struct {
	XMLName      xml.Name  `xml:"ScannerCapabilities"`
	Version      string    `xml:"Version"`
	MakeAndModel string    `xml:"MakeAndModel"`
	SerialNumber string    `xml:"SerialNumber"`
	Manufacturer string    `xml:"Manufacturer"`
	UUID         string    `xml:"UUID"`
	AdminURI     string    `xml:"AdminURI"`
	Platen       *struct{} `xml:"Platen"`
	ADF          *struct{} `xml:"Adf"`
}

// Status is the ScannerStatus document.
type Status struct {
	XMLName  xml.Name `xml:"ScannerStatus"`
	State    string   `xml:"State"`    // Idle | Processing | Testing | Stopped | Down
	AdfState string   `xml:"AdfState"` // ScannerAdfEmpty | ScannerAdfLoaded | ScannerAdfJam ...
}

// ParseCapabilities decodes a ScannerCapabilities document.
func ParseCapabilities(b []byte) (*Capabilities, error) {
	var c Capabilities
	if err := xml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.XMLName.Local != "ScannerCapabilities" {
		return nil, errors.New("not an eSCL ScannerCapabilities document")
	}
	return &c, nil
}

// ParseStatus decodes a ScannerStatus document.
func ParseStatus(b []byte) (*Status, error) {
	var s Status
	if err := xml.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.XMLName.Local != "ScannerStatus" {
		return nil, errors.New("not an eSCL ScannerStatus document")
	}
	return &s, nil
}

func (d *Driver) fetch(ctx context.Context, address, doc string) ([]byte, string, error) {
	var errs []error
	for _, b := range d.bases {
		base := fmt.Sprintf(b, address)
		body, err := d.tr.Get(ctx, base+"/"+doc)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", base, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		return body, base, nil
	}
	return nil, "", errors.Join(errs...)
}

// Identify implements engine.Identifier: a device answering eSCL is a multifunction device.
func (d *Driver) Identify(ctx context.Context, address string, _ model.Creds) (*model.Identity, error) {
	body, _, err := d.fetch(ctx, address, "ScannerCapabilities")
	if err != nil {
		return nil, err
	}
	c, err := ParseCapabilities(body)
	if err != nil {
		return nil, err
	}
	id := &model.Identity{Address: address, Method: Name, DeviceType: "mfp"}
	id.Model = strings.TrimSpace(c.MakeAndModel)
	id.Make = strings.TrimSpace(c.Manufacturer)
	if id.Make == "" && id.Model != "" {
		id.Make = strings.Fields(id.Model)[0]
	}
	id.Serial = strings.TrimSpace(c.SerialNumber)
	id.UUID = strings.TrimPrefix(strings.TrimSpace(c.UUID), "urn:uuid:")
	id.AdminURL = strings.TrimSpace(c.AdminURI)
	id.Capabilities = []model.Capability{model.CapIdentify}
	return id, nil
}

// ScannerStatus reads the scanner state for an identified MFP.
func (d *Driver) ScannerStatus(ctx context.Context, address string) (*Status, error) {
	body, _, err := d.fetch(ctx, address, "ScannerStatus")
	if err != nil {
		return nil, err
	}
	return ParseStatus(body)
}

var _ engine.Identifier = (*Driver)(nil)
