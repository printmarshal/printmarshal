package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
)

// A printer the monitor can't vouch for must never read as fine. With the
// up/down switch off the board says only that its state is unconfirmed, and it
// says nothing at all about a printer that answered its last poll.
func TestPublicBoardUnconfirmedWhenUpDownHidden(t *testing.T) {
	up := &model.Device{ID: "up", State: model.StateActive, Status: &model.Status{Online: true, State: "idle"}}
	down := &model.Device{ID: "down", State: model.StateActive, Status: &model.Status{Online: false, State: "offline"}}
	never := &model.Device{ID: "never", State: model.StateActive}
	retired := &model.Device{ID: "retired", State: model.StateDecommissioned, Status: &model.Status{Online: true, State: "idle"}}
	cfg := publicStatusConfig{Enabled: true, Devices: []publicStatusDevice{
		{ID: "up", Label: "Up"}, {ID: "down", Label: "Down"}, {ID: "never", Label: "Never"}, {ID: "retired", Label: "Retired"},
	}, Show: publicShow{Paper: true}}
	b := buildPublicBoard(cfg, []*model.Device{up, down, never, retired}, nil, time.Now(), 0, 15)
	want := map[string]bool{"Up": false, "Down": true, "Never": true, "Retired": true}
	for _, d := range b.Devices {
		if d.Online != nil {
			t.Errorf("%s: online sent with the switch off", d.Label)
		}
		raw, _ := json.Marshal(d)
		if got := strings.Contains(string(raw), `"unconfirmed":true`); got != want[d.Label] {
			t.Errorf("%s: unconfirmed = %v, want %v: %s", d.Label, got, want[d.Label], raw)
		}
		if !want[d.Label] && strings.Contains(string(raw), "unconfirmed") {
			t.Errorf("%s: the key must be absent when false: %s", d.Label, raw)
		}
	}
}

// A decommissioned printer is never contacted, so its last poll says nothing
// about now, and the board must not repeat it.
func TestPublicBoardDecommissionedIsNotOnline(t *testing.T) {
	retired := &model.Device{ID: "retired", State: model.StateDecommissioned, Status: &model.Status{Online: true, State: "idle"}}
	cfg := publicStatusConfig{Enabled: true, Devices: []publicStatusDevice{{ID: "retired", Label: "Retired"}},
		Show: publicShow{Online: true, State: true}}
	b := buildPublicBoard(cfg, []*model.Device{retired}, nil, time.Now(), 0, 15)
	if len(b.Devices) != 1 {
		t.Fatalf("devices = %d", len(b.Devices))
	}
	d := b.Devices[0]
	if d.Online == nil || *d.Online {
		t.Errorf("online = %v, want false", d.Online)
	}
	if d.State == nil || *d.State != "unknown" {
		t.Errorf("state = %v, want unknown", d.State)
	}
	if raw, _ := json.Marshal(d); strings.Contains(string(raw), "unconfirmed") {
		t.Errorf("online already says it; unconfirmed is for the switch being off: %s", raw)
	}
}

// With the up/down switch on, the board already answers this directly.
func TestPublicBoardOnlineSwitchOnSendsNoUnconfirmed(t *testing.T) {
	down := &model.Device{ID: "down", State: model.StateActive, Status: &model.Status{Online: false, State: "offline"}}
	cfg := publicStatusConfig{Enabled: true, Devices: []publicStatusDevice{{ID: "down", Label: "Down"}}, Show: publicShow{Online: true}}
	b := buildPublicBoard(cfg, []*model.Device{down}, nil, time.Now(), 0, 15)
	if raw, _ := json.Marshal(b.Devices[0]); strings.Contains(string(raw), "unconfirmed") {
		t.Errorf("got %s", raw)
	}
}
