package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

// countingWebProbe counts web-page probes (they reach the device's address).
type countingWebProbe struct{ n *atomic.Int64 }

func (c countingWebProbe) RoundTrip(*http.Request) (*http.Response, error) {
	c.n.Add(1)
	return nil, errors.New("web probe disabled in tests")
}

// newTripwireService is newTestService with counters on the two ways this
// package reaches a device: the SNMP session factory and the web-page probe.
func newTripwireService(t *testing.T) (svc *Service, st store.Store, sessions, webProbes *atomic.Int64) {
	t.Helper()
	fx, err := snmp.LoadFixture("../../testdata/fixtures/generic-laser.snmp")
	if err != nil {
		t.Fatal(err)
	}
	box, _ := secrets.New(make([]byte, 32))
	st, err = store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessions, webProbes = &atomic.Int64{}, &atomic.Int64{}
	reg := engine.NewRegistry()
	reg.Register(snmp.New(func(ctx context.Context, address string, creds model.Creds) (snmp.Session, string, error) {
		sessions.Add(1)
		return fx, "v2c", nil
	}))
	runner := jobs.New(st, log, 2, func(model.Event) {})
	t.Cleanup(runner.Stop)
	svc = New(st, engine.New(reg, log, 4), runner, config.Default(), log, func(model.Event) {}, func(context.Context, string) bool { return true })
	svc.Resolver = &fakeResolver{}
	svc.WebClient = &http.Client{Transport: countingWebProbe{webProbes}}
	return svc, st, sessions, webProbes
}

// decommissioned identifies a device at address and retires it, then clears
// the counters so what follows counts contact with a decommissioned printer.
func decommissioned(t *testing.T, svc *Service, st store.Store, address string, sessions, webProbes *atomic.Int64) *model.Device {
	t.Helper()
	ctx := context.Background()
	dev, _, err := svc.IdentifyAndUpsert(ctx, address, []string{"scan"}, discoveryCandidate(address))
	if err != nil {
		t.Fatal(err)
	}
	if dev, err = st.SetDeviceState(ctx, dev.ID, model.StateDecommissioned); err != nil {
		t.Fatal(err)
	}
	sessions.Store(0)
	webProbes.Store(0)
	return dev
}

func waitForJobs(t *testing.T, svc *Service, st store.Store) []*model.Job {
	t.Helper()
	js, err := st.ListJobs(context.Background(), store.JobFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range js {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := svc.Jobs.Wait(ctx, j.ID)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
	return js
}

func TestPollDeviceRefusesDecommissioned(t *testing.T) {
	svc, st, sessions, webProbes := newTripwireService(t)
	dev := decommissioned(t, svc, st, "10.0.0.9", sessions, webProbes)
	if _, _, err := svc.PollDevice(context.Background(), dev); err == nil || !strings.Contains(err.Error(), "decommissioned") {
		t.Errorf("PollDevice err = %v, want the decommissioned refusal", err)
	}
	if n := sessions.Load(); n != 0 {
		t.Errorf("PollDevice opened %d SNMP sessions to a decommissioned device", n)
	}
	after, err := st.GetDevice(context.Background(), dev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastPolled != nil {
		t.Errorf("a refused poll must not stamp the record: %v", after.LastPolled)
	}
}

// The scheduled poll has always left decommissioned devices out; pin it.
func TestScheduledPollSkipsDecommissioned(t *testing.T) {
	svc, st, sessions, webProbes := newTripwireService(t)
	decommissioned(t, svc, st, "10.0.0.9", sessions, webProbes)
	svc.scheduledPoll(context.Background())
	waitForJobs(t, svc, st)
	if n := sessions.Load(); n != 0 {
		t.Errorf("the scheduled poll opened %d SNMP sessions to a decommissioned device", n)
	}
}

// A poll job that names a decommissioned device records a skip.
func TestPollJobSkipsNamedDecommissioned(t *testing.T) {
	svc, st, sessions, webProbes := newTripwireService(t)
	ctx := context.Background()
	dev := decommissioned(t, svc, st, "10.0.0.9", sessions, webProbes)
	// Queued directly: the job could have been accepted before the device retired.
	j, err := svc.Jobs.Submit(ctx, &model.Job{Type: "poll", CreatedBy: "test", Target: map[string]any{"deviceIds": []any{dev.ID}}, Params: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	done, err := svc.Jobs.Wait(wctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := sessions.Load(); n != 0 {
		t.Errorf("the poll job opened %d SNMP sessions to a decommissioned device", n)
	}
	results, _ := st.ListJobResults(ctx, j.ID)
	b, _ := json.Marshal(results)
	if len(results) != 1 || results[0].Status != "skipped" || !strings.Contains(results[0].Message, "decommissioned") {
		t.Errorf("results = %s", b)
	}
	if done.Failed != 0 {
		t.Errorf("a skip is not a failure: %+v", done)
	}
}

// A trap from the address of a retired printer may come from whoever has that
// address now: no alert on the retired record, no reconciling poll.
func TestTrapFromDecommissionedIsIgnored(t *testing.T) {
	for _, tc := range []struct {
		name     string
		text     string
		severity model.Severity
	}{{"alert trap", "paper jam", model.SeverityCritical}, {"informational trap", "link up", model.SeverityOK}} {
		t.Run(tc.name, func(t *testing.T) {
			svc, st, sessions, webProbes := newTripwireService(t)
			ctx := context.Background()
			dev := decommissioned(t, svc, st, "10.0.0.9", sessions, webProbes)
			svc.HandleTrap(ctx, dev.Address, tc.text, tc.severity, nil)
			if js := waitForJobs(t, svc, st); len(js) != 0 {
				t.Errorf("a trap queued %d jobs for a decommissioned device", len(js))
			}
			if n := sessions.Load(); n != 0 {
				t.Errorf("a trap led to %d SNMP sessions to a decommissioned device", n)
			}
			as, err := st.ListAlerts(ctx, store.AlertFilter{DeviceID: dev.ID, ActiveOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(as) != 0 {
				t.Errorf("a trap raised alerts on a decommissioned device: %+v", as[0])
			}
		})
	}
}

// A sweep reaches addresses, so it may meet a retired printer: identification
// is how it finds out. Nothing further happens once the record is known.
func TestDiscoveryDoesNotPollRediscoveredDecommissioned(t *testing.T) {
	svc, st, sessions, webProbes := newTripwireService(t)
	ctx := context.Background()
	dev := decommissioned(t, svc, st, "10.0.0.9", sessions, webProbes)
	j, err := svc.SubmitDiscovery(ctx, DiscoveryParams{Targets: []string{"10.0.0.9"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := svc.Jobs.Wait(wctx, j.ID); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetDevice(ctx, dev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != model.StateDecommissioned {
		t.Errorf("discovery changed the state to %s", after.State)
	}
	if after.LastPolled != nil {
		t.Errorf("discovery polled a decommissioned device at %v", after.LastPolled)
	}
	if n := webProbes.Load(); n != 0 {
		t.Errorf("discovery probed the web page of a decommissioned device %d times", n)
	}
	results, _ := st.ListJobResults(ctx, j.ID)
	found := false
	for _, r := range results {
		found = found || (r.DeviceID == dev.ID && strings.Contains(r.Message, "decommissioned"))
	}
	if !found {
		b, _ := json.Marshal(results)
		t.Errorf("the job result must say the device is decommissioned: %s", b)
	}
}
