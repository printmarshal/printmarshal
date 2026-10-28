package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/driver/fwsim"
	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
)

func firmwareAttr(attrs []ipp.Attribute) string {
	for _, a := range attrs {
		if a.Name == "printer-firmware-string-version" && len(a.Values) > 0 {
			s, _ := a.Values[0].(string)
			return s
		}
	}
	return ""
}

// Integration (spec §14): the fwsim driver delivers a package to the
// simulator's firmware hook in-process; the version it then reports over IPP
// is the package version. Nothing here touches a real printer.
func TestFirmwareHookEndToEnd(t *testing.T) {
	fx, err := snmp.LoadFixture("../../testdata/fixtures/generic-laser.snmp")
	if err != nil {
		t.Fatal(err)
	}
	state := &SimState{Fixture: fx, Firmware: "2.14.7", Settings: map[string]any{}, Overrides: map[string]ipp.Attribute{}}
	mux := http.NewServeMux()
	sim := mountFirmware(mux, state, 30*time.Millisecond)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "http://"))

	image := []byte("simulated firmware image 2.15.0")
	sum := sha256.Sum256(image)
	dir := t.TempDir()
	path := filepath.Join(dir, "acme-2.15.0.bin")
	if err := os.WriteFile(path, image, 0o640); err != nil {
		t.Fatal(err)
	}
	pkg := &model.FirmwarePackage{ID: "p1", Vendor: "Acme", Models: []string{"4200"}, Version: "2.15.0", FileName: "acme-2.15.0.bin",
		Size: int64(len(image)), SHA256: hex.EncodeToString(sum[:]), StoragePath: path}
	dev := &model.Device{ID: "d1", Address: host, Make: "Acme", Model: "Acme LaserPro 4200", Firmware: "2.14.7", IdentifyMethods: []string{"snmp", "ipp"}}
	ctx := context.Background()

	// Not allow-listed and not a "sim" identity: refused without any I/O.
	stranger := fwsim.New([]string{"198.51.100.5"}, fwsim.Options{})
	if stranger.Supports(dev, model.CapFirmwareUpdate) {
		t.Fatal("device must not be supported")
	}
	if _, err := stranger.UpdateFirmware(ctx, dev, model.Creds{}, pkg, model.WriteOptions{}); err == nil {
		t.Fatal("delivery to a non-allow-listed device must fail")
	}

	drv := fwsim.New([]string{host + ":" + port}, fwsim.Options{})
	if !drv.Supports(dev, model.CapFirmwareUpdate) || drv.Supports(dev, model.CapReadStatus) {
		t.Fatal("supports")
	}
	simIdent := &model.Device{ID: "d2", Address: "192.0.2.9", IdentifyMethods: []string{"sim"}}
	if !drv.Supports(simIdent, model.CapFirmwareUpdate) {
		t.Fatal("sim identity method must be supported")
	}

	// Dry run: validates and probes the endpoint, sends nothing.
	res, err := drv.UpdateFirmware(ctx, dev, model.Creds{}, pkg, model.WriteOptions{DryRun: true})
	if err != nil || !res.DryRun || !res.Accepted || !strings.Contains(res.Message, "nothing was sent") {
		t.Fatalf("dry run: %+v err=%v", res, err)
	}
	if sim.Uploads != 0 || state.Firmware != "2.14.7" {
		t.Fatalf("dry run changed the device: uploads=%d firmware=%s", sim.Uploads, state.Firmware)
	}

	// Incompatible package: reported, not delivered.
	res, err = drv.UpdateFirmware(ctx, dev, model.Creds{}, &model.FirmwarePackage{Vendor: "Globex", Version: "9", StoragePath: path, SHA256: pkg.SHA256}, model.WriteOptions{})
	if err != nil || res.Accepted || !strings.Contains(res.Message, "incompatible") || sim.Uploads != 0 {
		t.Fatalf("incompatible: %+v err=%v uploads=%d", res, err, sim.Uploads)
	}

	// Tampered package: integrity failure before any upload.
	bad := *pkg
	bad.SHA256 = strings.Repeat("0", 64)
	if _, err := drv.UpdateFirmware(ctx, dev, model.Creds{}, &bad, model.WriteOptions{}); err == nil || !strings.Contains(err.Error(), "integrity") {
		t.Fatalf("tampered package: %v", err)
	}

	// Real delivery.
	res, err = drv.UpdateFirmware(ctx, dev, model.Creds{}, pkg, model.WriteOptions{})
	if err != nil || !res.Accepted || res.Version != "2.15.0" || res.DryRun {
		t.Fatalf("deliver: %+v err=%v", res, err)
	}
	if sim.Uploads != 1 || sim.LastSHA256 != pkg.SHA256 || sim.LastSize != int64(len(image)) {
		t.Errorf("sim bookkeeping %+v", sim)
	}
	// The flash delay has not elapsed: IPP still reports the old version.
	if got := firmwareAttr(currentAttributes(ctx, state)); got != "2.14.7" {
		t.Errorf("before delay: %s", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if got := firmwareAttr(currentAttributes(ctx, state)); got == "2.15.0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("IPP never reported the new version; state=%q", state.Firmware)
		}
		time.Sleep(5 * time.Millisecond)
	}
	resp, err := http.Get(ts.URL + "/firmware/status")
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&st)
	resp.Body.Close()
	if st["version"] != "2.15.0" || st["pending"] != "" || st["uploads"].(float64) != 1 {
		t.Errorf("status %+v", st)
	}
}

func TestFirmwareHookRejectsBadUploads(t *testing.T) {
	state := &SimState{Firmware: "1.0"}
	mux := http.NewServeMux()
	mountFirmware(mux, state, 0)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	post := func(fields map[string]string, file, name string) (int, firmwareReply) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		for k, v := range fields {
			_ = mw.WriteField(k, v)
		}
		if name != "" {
			p, _ := mw.CreateFormFile("firmware", name)
			_, _ = p.Write([]byte(file))
		}
		_ = mw.Close()
		resp, err := http.Post(ts.URL+"/firmware", mw.FormDataContentType(), &body)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var r firmwareReply
		b, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(b, &r)
		return resp.StatusCode, r
	}
	if code, r := post(nil, "", ""); code != 400 || r.Accepted {
		t.Errorf("no file: %d %+v", code, r)
	}
	if code, r := post(nil, "", "fw.bin"); code != 400 || r.Accepted || !strings.Contains(r.Message, "empty") {
		t.Errorf("empty file: %d %+v", code, r)
	}
	if code, r := post(nil, "data", "fw.bin"); code != 400 || r.Accepted || !strings.Contains(r.Message, "version") {
		t.Errorf("no version: %d %+v", code, r)
	}
	if code, r := post(map[string]string{"sha256": strings.Repeat("a", 64)}, "data", "fw-20240115.bin"); code != 400 || r.Accepted || !strings.Contains(r.Message, "sha256") {
		t.Errorf("bad sha: %d %+v", code, r)
	}
	if state.Firmware != "1.0" {
		t.Fatalf("rejected uploads changed the version: %s", state.Firmware)
	}
	if code, r := post(nil, "data", "fw-20240115.bin"); code != 200 || !r.Accepted || r.Version != "20240115" {
		t.Errorf("version from file name: %d %+v", code, r)
	}
	if state.Firmware != "20240115" {
		t.Errorf("version not applied: %s", state.Firmware)
	}
	if code, r := post(map[string]string{"version": "3.1"}, "data", "whatever.bin"); code != 200 || r.Version != "3.1" || state.Firmware != "3.1" {
		t.Errorf("version field: %d %+v state=%s", code, r, state.Firmware)
	}
	// A plain IPP POST to "/" still belongs to the IPP handler (not mounted here) -> 404.
	resp, _ := http.Get(ts.URL + "/firmware")
	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /firmware = %d", resp.StatusCode)
	}
	resp.Body.Close()
}
