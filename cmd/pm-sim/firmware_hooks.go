package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Firmware simulation: the simulator accepts a firmware image over HTTP, the
// way internal/driver/fwsim delivers it, and reports the new version over IPP
// (printer-firmware-string-version reads SimState.Firmware) once the optional
// flash delay has elapsed.
//
//	POST /firmware          multipart; field "firmware" (the image), optional
//	                        "version" (else parsed from the file name) and
//	                        "sha256" (checked when present)
//	                        -> {"accepted":true,"version":"…","message":"…"}
//	GET  /firmware/status   -> current, pending and last-upload details
var firmwareDelay = flag.Duration("firmware-delay", 0, "delay before an uploaded firmware version takes effect (simulates flashing and reboot)")

func init() {
	HTTPHooks = append(HTTPHooks, func(mux *http.ServeMux, state *SimState) {
		mountFirmware(mux, state, *firmwareDelay)
	})
}

var versionInName = regexp.MustCompile(`\d{8}|\d+(?:\.\d+)+`)

// firmwareSim is the hook's own bookkeeping (SimState is shared and fixed).
type firmwareSim struct {
	mu         sync.Mutex
	state      *SimState
	delay      time.Duration
	Pending    string     `json:"pending,omitempty"`
	Uploads    int        `json:"uploads"`
	LastFile   string     `json:"lastFile,omitempty"`
	LastSize   int64      `json:"lastSize,omitempty"`
	LastSHA256 string     `json:"lastSha256,omitempty"`
	LastUpload *time.Time `json:"lastUpload,omitempty"`
	AppliesAt  *time.Time `json:"appliesAt,omitempty"`
}

type firmwareReply struct {
	Accepted bool   `json:"accepted"`
	Version  string `json:"version,omitempty"`
	Message  string `json:"message,omitempty"`
}

// mountFirmware registers the firmware routes on mux and returns the simulator
// so tests can inspect it.
func mountFirmware(mux *http.ServeMux, state *SimState, delay time.Duration) *firmwareSim {
	f := &firmwareSim{state: state, delay: delay}
	mux.HandleFunc("POST /firmware", f.upload)
	mux.HandleFunc("GET /firmware/status", f.status)
	return f
}

func reply(w http.ResponseWriter, code int, r firmwareReply) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(r)
}

func (f *firmwareSim) upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		reply(w, http.StatusBadRequest, firmwareReply{Message: "expected a multipart upload: " + err.Error()})
		return
	}
	file, hdr, err := r.FormFile("firmware")
	if err != nil {
		reply(w, http.StatusBadRequest, firmwareReply{Message: `missing multipart field "firmware"`})
		return
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, file)
	if err != nil {
		reply(w, http.StatusBadRequest, firmwareReply{Message: "read image: " + err.Error()})
		return
	}
	if n == 0 {
		reply(w, http.StatusBadRequest, firmwareReply{Message: "image is empty"})
		return
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if want := strings.TrimSpace(r.FormValue("sha256")); want != "" && !strings.EqualFold(want, sum) {
		reply(w, http.StatusBadRequest, firmwareReply{Message: "sha256 mismatch: image is corrupt"})
		return
	}
	version := strings.TrimSpace(r.FormValue("version"))
	if version == "" {
		version = versionInName.FindString(hdr.Filename)
	}
	if version == "" {
		reply(w, http.StatusBadRequest, firmwareReply{Message: "no version: send a \"version\" field or a file name containing one"})
		return
	}

	now := time.Now().UTC()
	f.mu.Lock()
	f.Uploads++
	f.LastFile, f.LastSize, f.LastSHA256, f.LastUpload = hdr.Filename, n, sum, &now
	f.Pending = version
	f.AppliesAt = nil
	if f.delay > 0 {
		at := now.Add(f.delay)
		f.AppliesAt = &at
	}
	f.mu.Unlock()

	msg := "image accepted; version " + version + " active"
	if f.delay > 0 {
		msg = "image accepted; flashing, version " + version + " active in " + f.delay.String()
		time.AfterFunc(f.delay, func() { f.apply(version) })
	} else {
		f.apply(version)
	}
	log.Printf("pm-sim: firmware upload %s (%d bytes, sha256 %s) -> version %s", hdr.Filename, n, sum[:12], version)
	reply(w, http.StatusOK, firmwareReply{Accepted: true, Version: version, Message: msg})
}

func (f *firmwareSim) apply(version string) {
	f.state.Lock()
	f.state.Firmware = version
	f.state.Unlock()
	f.mu.Lock()
	if f.Pending == version {
		f.Pending = ""
		f.AppliesAt = nil
	}
	f.mu.Unlock()
}

func (f *firmwareSim) status(w http.ResponseWriter, r *http.Request) {
	f.state.Lock()
	current := f.state.Firmware
	f.state.Unlock()
	f.mu.Lock()
	out := map[string]any{"version": current, "pending": f.Pending, "uploads": f.Uploads, "delay": f.delay.String(),
		"lastFile": f.LastFile, "lastSize": f.LastSize, "lastSha256": f.LastSHA256, "lastUpload": f.LastUpload, "appliesAt": f.AppliesAt}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
