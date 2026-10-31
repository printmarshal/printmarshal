package firmware

import (
	"context"
	"strings"
	"testing"

	"printmarshal/internal/model"
)

// Decommissioned devices are filtered when a campaign is planned, but a
// campaign can sit at its canary gate for days. A printer retired meanwhile
// must not get a delivery, or even the pre-flight poll.
func TestDeviceDecommissionedDuringCampaignIsSkipped(t *testing.T) {
	h := newHarness(t)
	pkg := h.pkg("Acme", []string{"4200"}, "2.15.0", "")
	canary := h.device("p0", "Acme", "Acme LaserPro 4200", "2.14.7")
	later := h.device("p1", "Acme", "Acme LaserPro 4200", "2.14.7")
	c := h.campaign(pkg, []*model.Device{canary, later}, func(c *model.Campaign) { c.CanaryCount = 1; c.Concurrency = 1 })
	h.start(c, false)
	c, _ = h.waitFor(func(c *model.Campaign, _ []*model.CampaignDevice) bool {
		return c.Status == model.CampaignPaused && c.PauseReason == model.PauseCanary
	})

	if _, err := h.st.SetDeviceState(context.Background(), later.ID, model.StateDecommissioned); err != nil {
		t.Fatal(err)
	}
	polled := false
	h.dev.onPoll = func(string) { polled = true }
	if _, err := h.svc.Confirm(context.Background(), c.ID, "op"); err != nil {
		t.Fatal(err)
	}
	_, rows := h.waitFor(status(model.CampaignCompleted))
	row := byDevice(rows)[later.ID]
	if row.Status != model.CampaignDeviceSkipped || !strings.Contains(row.Message, "decommissioned") {
		t.Errorf("row = %+v, want skipped with the reason", row)
	}
	if polled {
		t.Error("the pre-flight poll reached a decommissioned device")
	}
	if calls := h.upd.Calls(); len(calls) != 1 || calls[0].DeviceID != canary.ID {
		t.Errorf("only the canary may be delivered to: %+v", calls)
	}
}
