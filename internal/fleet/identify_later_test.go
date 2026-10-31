package fleet

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/discovery"
	"printmarshal/internal/engine"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

// switchedDriver is a printer a test switches on: identify and status only,
// no network and no write path.
type switchedDriver struct {
	on         atomic.Bool
	identifies atomic.Int64
	community  atomic.Value // the credential the last identification used
}

func (d *switchedDriver) Name() string { return "switched" }
func (d *switchedDriver) Capabilities() []model.Capability {
	return []model.Capability{model.CapIdentify, model.CapReadStatus}
}
func (d *switchedDriver) Supports(dev *model.Device, c model.Capability) bool {
	return engine.ProfileSupports(dev, d.Name(), c)
}
func (d *switchedDriver) Priority(model.Capability) int { return 10 }
func (d *switchedDriver) Identify(_ context.Context, address string, creds model.Creds) (*model.Identity, error) {
	d.identifies.Add(1)
	d.community.Store(strings.Join(creds.Communities, ","))
	if !d.on.Load() {
		return nil, errors.New("timeout")
	}
	return &model.Identity{Address: address, Serial: "SW-1", Make: "Acme", Model: "Acme 100", DeviceType: "printer", SysLocation: "from the agent",
		Capabilities: []model.Capability{model.CapReadStatus}}, nil
}
func (d *switchedDriver) ReadStatus(context.Context, *model.Device, model.Creds) (*model.Status, error) {
	if !d.on.Load() {
		return nil, errors.New("timeout")
	}
	return &model.Status{Online: true, State: "idle", Severity: model.SeverityOK, At: time.Now().UTC()}, nil
}

func newSwitchedService(t *testing.T) (*Service, store.Store, *switchedDriver) {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	st, err := store.OpenSQLite(t.TempDir()+"/t.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	drv := &switchedDriver{}
	reg := engine.NewRegistry()
	reg.Register(drv)
	runner := jobs.New(st, log, 2, func(model.Event) {})
	t.Cleanup(runner.Stop)
	svc := New(st, engine.New(reg, log, 4), runner, config.Default(), log, func(model.Event) {}, nil)
	svc.Resolver = &fakeResolver{}
	svc.WebClient = &http.Client{Transport: noWebProbe{}}
	return svc, st, drv
}

// addressOnly records a printer the way the CSV import does when it does not
// answer, with one imported field.
func addressOnly(t *testing.T, st store.Store, address string) *model.Device {
	t.Helper()
	ctx := context.Background()
	d, created, err := st.UpsertDevice(ctx, &model.Identity{Address: address, Method: "import"}, map[model.Capability]string{}, []string{"import"})
	if err != nil || !created {
		t.Fatalf("address-only record: %v created=%v", err, created)
	}
	loc := "Floor 7"
	d, err = st.PatchDevice(ctx, d.ID, store.DevicePatch{Location: &loc, FieldSources: map[string]model.FieldSource{"location": {Source: model.SourceImport, At: time.Now().UTC()}}})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func runScheduledPoll(t *testing.T, svc *Service) {
	t.Helper()
	ctx := context.Background()
	j, err := svc.SubmitPoll(ctx, nil, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if done, err := svc.Jobs.Wait(wctx, j.ID); err != nil || done.FinishedAt == nil {
		t.Fatalf("poll job: %+v %v", done, err)
	}
}

// A record with no make, model or serial is identified by the scheduled poll
// when it first answers, through the read-only identify path, in place.
func TestScheduledPollIdentifiesAddressOnlyRecord(t *testing.T) {
	svc, st, drv := newSwitchedService(t)
	ctx := context.Background()
	d := addressOnly(t, st, "10.0.0.50")

	runScheduledPoll(t, svc)
	if drv.identifies.Load() != 1 {
		t.Errorf("identifications while off: %d, want 1 per poll", drv.identifies.Load())
	}
	got, _ := st.GetDevice(ctx, d.ID)
	if got.Serial != "" || got.Location != "Floor 7" {
		t.Fatalf("while off: %+v", got)
	}

	drv.on.Store(true)
	runScheduledPoll(t, svc)
	got, _ = st.GetDevice(ctx, d.ID)
	if got.Serial != "SW-1" || got.Make != "Acme" || got.Capabilities[model.CapReadStatus] != drv.Name() {
		t.Fatalf("not identified: %+v", got)
	}
	if got.Location != "Floor 7" || got.FieldSources["location"].Source != model.SourceImport {
		t.Errorf("imported field lost: %q %v", got.Location, got.FieldSources)
	}
	if got.Status == nil || got.Status.State != "idle" {
		t.Errorf("the same poll should read it once identified: %+v", got.Status)
	}
	if devs, _ := st.ListDevices(ctx, store.DeviceFilter{}); len(devs) != 1 {
		t.Errorf("%d records", len(devs))
	}

	// Identified now: later polls do not identify again.
	before := drv.identifies.Load()
	runScheduledPoll(t, svc)
	if drv.identifies.Load() != before {
		t.Errorf("an identified device was identified again")
	}
}

// Identification uses the record's own credentials, so a printer imported
// into a group with a group credential can answer it.
func TestPollIdentifiesWithTheRecordsCredentials(t *testing.T) {
	svc, st, drv := newSwitchedService(t)
	ctx := context.Background()
	d := addressOnly(t, st, "10.0.0.51")
	if err := st.CreateCredential(ctx, &model.Credential{Name: "here", Type: model.CredSNMPCommunity, Scope: "device:" + d.ID, Purpose: model.PurposeRead}, &model.Secret{Community: "site-a"}); err != nil {
		t.Fatal(err)
	}
	runScheduledPoll(t, svc)
	if got, _ := drv.community.Load().(string); !strings.Contains(got, "site-a") {
		t.Errorf("identified with communities %q", got)
	}
}

// A later discovery of the address merges into the address-only record, and
// keeps the imported fields over what the agent reports.
func TestDiscoveryMergesIntoAddressOnlyRecord(t *testing.T) {
	svc, st, drv := newSwitchedService(t)
	ctx := context.Background()
	d := addressOnly(t, st, "10.0.0.52")
	if _, _, err := svc.IdentifyAndUpsert(ctx, "10.0.0.52", []string{"scan"}, discovery.Candidate{Address: "10.0.0.52"}); !errors.Is(err, engine.ErrNotIdentified) {
		t.Fatalf("while off: %v", err)
	}
	drv.on.Store(true)
	got, created, err := svc.IdentifyAndUpsert(ctx, "10.0.0.52", []string{"scan"}, discovery.Candidate{Address: "10.0.0.52"})
	if err != nil || created || got.ID != d.ID {
		t.Fatalf("merge: %+v created=%v %v", got, created, err)
	}
	if got.Serial != "SW-1" || got.Location != "Floor 7" || got.FieldSources["location"].Source != model.SourceImport {
		t.Errorf("merged record: %+v", got)
	}
	if devs, _ := st.ListDevices(ctx, store.DeviceFilter{}); len(devs) != 1 {
		t.Errorf("%d records", len(devs))
	}
}
