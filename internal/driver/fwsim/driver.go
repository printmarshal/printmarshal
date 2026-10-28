// Package fwsim is the standard firmware-delivery driver for simulated and
// explicitly allow-listed devices. There is no universal firmware transport
// across printer vendors, so this driver serves only firmware_update and only
// for devices that identified through the simulator ("sim" identity method) or
// whose address is on a configured allow-list. It uploads the package as a
// multipart POST (field "firmware") to http://<addr>:<port>/firmware and
// expects {"accepted":true,"version":"..."} back, which is exactly what
// cmd/pm-sim serves. Vendor plugins implement engine.FirmwareUpdater against
// the same interface for real devices.
package fwsim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/firmware/fwversion"
	"printmarshal/internal/model"
)

// Name is the driver name recorded in capability profiles and results.
const Name = "fwsim"

// DefaultPort is the simulator's HTTP (IPP) port.
const DefaultPort = 6631

// Options configure the driver.
type Options struct {
	// Port is used for targets listed without one.
	Port int
	// Timeout bounds the upload request (default 2 minutes).
	Timeout time.Duration
	// Client overrides the HTTP client (tests).
	Client *http.Client
}

// Driver implements engine.Driver and engine.FirmwareUpdater.
type Driver struct {
	allow  map[string]int // host -> port (0 = Options.Port)
	opts   Options
	client *http.Client
}

// New returns a driver that may deliver to the given targets ("host" or
// "host:port"). Devices identified through the simulator are always eligible.
func New(targets []string, opts Options) *Driver {
	if opts.Port <= 0 {
		opts.Port = DefaultPort
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	d := &Driver{allow: map[string]int{}, opts: opts, client: opts.Client}
	if d.client == nil {
		d.client = &http.Client{Timeout: opts.Timeout}
	}
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		host, port := splitTarget(t)
		d.allow[strings.ToLower(host)] = port
	}
	return d
}

func splitTarget(t string) (string, int) {
	if h, p, err := net.SplitHostPort(t); err == nil {
		n, _ := strconv.Atoi(p)
		return h, n
	}
	return t, 0
}

func (d *Driver) Name() string { return Name }
func (d *Driver) Capabilities() []model.Capability {
	return []model.Capability{model.CapFirmwareUpdate}
}

// Priority is low so a vendor plugin that also serves the device wins.
func (d *Driver) Priority(model.Capability) int { return 5 }

// Supports serves firmware_update for devices identified via the simulator or
// on the allow-list. It never does network I/O.
func (d *Driver) Supports(dev *model.Device, cap model.Capability) bool {
	if dev == nil || cap != model.CapFirmwareUpdate {
		return false
	}
	for _, m := range dev.IdentifyMethods {
		if m == "sim" {
			return true
		}
	}
	return d.allowed(dev.Address)
}

func (d *Driver) allowed(addr string) bool {
	host, _ := splitTarget(addr)
	_, ok := d.allow[strings.ToLower(host)]
	return ok
}

// endpoint builds the device's firmware URL.
func (d *Driver) endpoint(dev *model.Device, path string) string {
	host, port := splitTarget(dev.Address)
	if p, ok := d.allow[strings.ToLower(host)]; ok && p > 0 {
		port = p
	}
	if port == 0 {
		port = d.opts.Port
	}
	return fmt.Sprintf("http://%s/%s", net.JoinHostPort(host, strconv.Itoa(port)), strings.TrimPrefix(path, "/"))
}

// acceptResponse is what the simulator answers.
type acceptResponse struct {
	Accepted bool   `json:"accepted"`
	Version  string `json:"version"`
	Message  string `json:"message"`
}

// UpdateFirmware validates compatibility, then (unless opts.DryRun) streams the
// verified package to the device. A dry run only checks that the device's
// firmware endpoint answers.
func (d *Driver) UpdateFirmware(ctx context.Context, dev *model.Device, creds model.Creds, pkg *model.FirmwarePackage, opts model.WriteOptions) (*model.FirmwareResult, error) {
	res := &model.FirmwareResult{DeviceID: dev.ID, DryRun: opts.DryRun, At: time.Now().UTC()}
	if !d.Supports(dev, model.CapFirmwareUpdate) {
		return nil, fmt.Errorf("%s is not a simulated or allow-listed device", dev.Address)
	}
	if v := fwversion.Check(pkg, dev, true); !v.Compatible {
		res.Message = "incompatible: " + v.Reason
		return res, nil
	}
	data, err := readVerified(pkg)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.endpoint(dev, "/firmware/status"), nil)
		if err != nil {
			return nil, err
		}
		resp, err := d.client.Do(req)
		if err != nil {
			res.Message = "dry run: firmware endpoint not reachable: " + err.Error()
			return res, nil
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			res.Message = fmt.Sprintf("dry run: firmware endpoint answered HTTP %d", resp.StatusCode)
			return res, nil
		}
		res.Accepted = true
		res.Message = fmt.Sprintf("dry run: package %s (%d bytes, sha256 %s) validated; endpoint %s answers; nothing was sent", pkg.FileName, len(data), pkg.SHA256[:12], d.endpoint(dev, "/firmware"))
		return res, nil
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("version", pkg.Version)
	_ = mw.WriteField("sha256", pkg.SHA256)
	part, err := mw.CreateFormFile("firmware", pkg.FileName)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint(dev, "/firmware"), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var ar acceptResponse
	if jerr := json.Unmarshal(raw, &ar); jerr != nil {
		return nil, fmt.Errorf("upload: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	res.Accepted, res.Version, res.Message = ar.Accepted, ar.Version, ar.Message
	if resp.StatusCode != http.StatusOK && !ar.Accepted && res.Message == "" {
		res.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	if res.Accepted && res.Message == "" {
		res.Message = "image accepted; device reports " + ar.Version
	}
	return res, nil
}

// readVerified loads the package file and checks it against the registered digest.
func readVerified(pkg *model.FirmwarePackage) ([]byte, error) {
	if pkg == nil || pkg.StoragePath == "" {
		return nil, fmt.Errorf("package has no stored file")
	}
	data, err := os.ReadFile(pkg.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("package file: %w", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, pkg.SHA256) {
		return nil, fmt.Errorf("package %s failed integrity check (sha256 mismatch)", pkg.FileName)
	}
	return data, nil
}

var (
	_ engine.Driver          = (*Driver)(nil)
	_ engine.FirmwareUpdater = (*Driver)(nil)
)
