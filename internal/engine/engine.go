// Package engine is the device engine (spec §4, §6): a capability model, a
// driver registry keyed by capability, and the identify/poll operations that
// everything above it (jobs, poller, API) issues.
//
// The engine never asks "is this a model-X printer?". It asks "which drivers
// can serve capability C for this device?" and dispatches to the best one.
package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/model"
)

// Driver is implemented by every protocol driver and vendor plugin.
// Each capability is a separate typed interface (Identifier, StatusReader, …);
// a driver implements the subset it declares in Capabilities().
type Driver interface {
	Name() string
	Capabilities() []model.Capability
	// Supports reports whether this driver can serve cap for the given device,
	// based on the device's resolved profile (which protocols answered at
	// identification). It must not do network I/O.
	Supports(d *model.Device, cap model.Capability) bool
	// Priority orders drivers that support the same capability; higher wins.
	Priority(cap model.Capability) int
}

// Identifier can establish identity for a bare address, before any record exists.
type Identifier interface {
	Identify(ctx context.Context, address string, creds model.Creds) (*model.Identity, error)
}

// StatusReader serves read_status.
type StatusReader interface {
	ReadStatus(ctx context.Context, d *model.Device, creds model.Creds) (*model.Status, error)
}

// SupplyReader serves read_supplies.
type SupplyReader interface {
	ReadSupplies(ctx context.Context, d *model.Device, creds model.Creds) ([]model.Supply, error)
}

// TrayReader serves read_trays.
type TrayReader interface {
	ReadTrays(ctx context.Context, d *model.Device, creds model.Creds) ([]model.Tray, error)
}

// CounterReader serves read_counters.
type CounterReader interface {
	ReadCounters(ctx context.Context, d *model.Device, creds model.Creds) (*model.Counters, error)
}

// FirmwareReader serves firmware_read.
type FirmwareReader interface {
	ReadFirmware(ctx context.Context, d *model.Device, creds model.Creds) (string, error)
}

// ErrUnsupported is returned when no driver serves a capability for a device.
var ErrUnsupported = errors.New("no driver supports this capability for this device")

// ErrNotIdentified is returned by Identify when no driver could establish an
// identity for an address (it answered discovery but is not a manageable
// printer, or nothing answered). Callers treat it as "skip", not failure.
var ErrNotIdentified = errors.New("not identified as a manageable printer")

// Registry holds drivers and resolves them per capability (spec §6 "driver registry").
type Registry struct {
	mu      sync.RWMutex
	drivers []Driver
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register adds a driver. Plugins call this at load time; the core has no
// compile-time list of vendors.
func (r *Registry) Register(d Driver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers = append(r.drivers, d)
}

// Drivers returns all registered drivers.
func (r *Registry) Drivers() []Driver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Driver, len(r.drivers))
	copy(out, r.drivers)
	return out
}

// For returns the drivers that declare and support cap for device d, best first.
func (r *Registry) For(d *model.Device, cap model.Capability) []Driver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Driver
	for _, drv := range r.drivers {
		if declares(drv, cap) && drv.Supports(d, cap) {
			out = append(out, drv)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Priority(cap) > out[j].Priority(cap) })
	return out
}

// ForWithFallback returns For(d, cap) followed by every other registered
// driver that declares cap and identified the device, so a failing pinned
// driver can fall back to a sibling (e.g. IPP when SNMP times out).
func (r *Registry) ForWithFallback(d *model.Device, cap model.Capability) []Driver {
	primary := r.For(d, cap)
	seen := map[string]bool{}
	for _, p := range primary {
		seen[p.Name()] = true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var extra []Driver
	for _, drv := range r.drivers {
		if seen[drv.Name()] || !declares(drv, cap) {
			continue
		}
		for _, m := range d.IdentifyMethods {
			if m == drv.Name() {
				extra = append(extra, drv)
				break
			}
		}
	}
	sort.SliceStable(extra, func(i, j int) bool { return extra[i].Priority(cap) > extra[j].Priority(cap) })
	return append(primary, extra...)
}

// Named returns the driver with the given name, if registered.
func (r *Registry) Named(name string) Driver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, d := range r.drivers {
		if d.Name() == name {
			return d
		}
	}
	return nil
}

func declares(d Driver, cap model.Capability) bool {
	for _, c := range d.Capabilities() {
		if c == cap {
			return true
		}
	}
	return false
}

// Engine dispatches capability requests to drivers.
type Engine struct {
	Registry *Registry
	Log      *slog.Logger
	// Limiter bounds concurrent device I/O (spec §11 rate limiting). nil = unbounded.
	Limiter chan struct{}
}

// New returns an engine over a registry.
func New(reg *Registry, log *slog.Logger, maxConcurrent int) *Engine {
	e := &Engine{Registry: reg, Log: log}
	if maxConcurrent > 0 {
		e.Limiter = make(chan struct{}, maxConcurrent)
	}
	return e
}

func (e *Engine) acquire(ctx context.Context) (release func(), err error) {
	if e.Limiter == nil {
		return func() {}, nil
	}
	select {
	case e.Limiter <- struct{}{}:
		return func() { <-e.Limiter }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Identify probes an address with every Identifier driver and merges what they
// report into one Identity plus the resolved capability profile (spec §6
// "capability resolution happens at identification").
func (e *Engine) Identify(ctx context.Context, address string, creds model.Creds) (*model.Identity, map[model.Capability]string, error) {
	release, err := e.acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	type res struct {
		id  *model.Identity
		drv Driver
		err error
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []res
	)
	for _, drv := range e.Registry.Drivers() {
		ident, ok := drv.(Identifier)
		if !ok || !declares(drv, model.CapIdentify) {
			continue
		}
		wg.Add(1)
		go func(drv Driver, ident Identifier) {
			defer wg.Done()
			id, err := ident.Identify(ctx, address, creds)
			mu.Lock()
			results = append(results, res{id, drv, err})
			mu.Unlock()
		}(drv, ident)
	}
	wg.Wait()

	// Deterministic merge order: by identify priority, highest first.
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].drv.Priority(model.CapIdentify) > results[j].drv.Priority(model.CapIdentify)
	})

	merged := &model.Identity{Address: address}
	caps := map[model.Capability]string{}
	capPrio := map[model.Capability]int{}
	var errs []error
	found := false
	for _, r := range results {
		if r.err != nil || r.id == nil {
			if r.err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", r.drv.Name(), r.err))
			}
			continue
		}
		found = true
		mergeIdentity(merged, r.id)
		merged.Method = appendMethod(merged.Method, r.drv.Name())
		for _, c := range r.id.Capabilities {
			p := r.drv.Priority(c)
			if cur, ok := capPrio[c]; !ok || p > cur {
				caps[c] = r.drv.Name()
				capPrio[c] = p
			}
		}
	}
	if !found {
		if len(errs) == 0 {
			return nil, nil, fmt.Errorf("%s: %w", address, ErrNotIdentified)
		}
		return nil, nil, fmt.Errorf("%s: %w: %w", address, ErrNotIdentified, errors.Join(errs...))
	}
	return merged, caps, nil
}

func appendMethod(cur, name string) string {
	if cur == "" {
		return name
	}
	return cur + "," + name
}

// mergeIdentity fills empty fields of dst from src (first driver wins, by priority).
func mergeIdentity(dst, src *model.Identity) {
	set := func(d *string, s string) {
		if *d == "" && s != "" {
			*d = strings.TrimSpace(s)
		}
	}
	if dst.Hostname == "" && strings.TrimSpace(src.Hostname) != "" {
		dst.Hostname = strings.TrimSpace(src.Hostname)
		dst.HostnameSource = strings.TrimSpace(src.HostnameSource)
	}
	set(&dst.MAC, src.MAC)
	set(&dst.Serial, src.Serial)
	set(&dst.UUID, src.UUID)
	set(&dst.Make, src.Make)
	set(&dst.Model, src.Model)
	set(&dst.Firmware, src.Firmware)
	set(&dst.SysDescr, src.SysDescr)
	set(&dst.SysName, src.SysName)
	set(&dst.SysLocation, src.SysLocation)
	set(&dst.AdminURL, src.AdminURL)
	if dst.EnterpriseID == 0 {
		dst.EnterpriseID = src.EnterpriseID
	}
	// "mfp" beats "printer" beats "unknown".
	if rankType(src.DeviceType) > rankType(dst.DeviceType) {
		dst.DeviceType = src.DeviceType
	}
}

func rankType(t string) int {
	switch t {
	case "mfp":
		return 3
	case "printer", "scanner":
		return 2
	case "unknown":
		return 1
	}
	return 0
}

// PollResult is everything a poll produced, with per-capability outcomes.
type PollResult struct {
	Status   *model.Status
	Supplies []model.Supply
	Trays    []model.Tray
	Counters *model.Counters
	Firmware string
	ServedBy map[model.Capability]string
	Errors   map[model.Capability]string
	At       time.Time
	// Reachable is true if at least one read capability succeeded.
	Reachable bool
}

// Poll reads status, supplies, trays, counters and firmware for a device using the
// best driver per capability, falling back to the next driver on error.
func (e *Engine) Poll(ctx context.Context, d *model.Device, creds model.Creds) *PollResult {
	res := &PollResult{
		ServedBy: map[model.Capability]string{},
		Errors:   map[model.Capability]string{},
		At:       time.Now().UTC(),
	}
	release, err := e.acquire(ctx)
	if err != nil {
		res.Errors[model.CapReadStatus] = err.Error()
		return res
	}
	defer release()

	try := func(cap model.Capability, call func(Driver) error) {
		drivers := e.Registry.ForWithFallback(d, cap)
		if len(drivers) == 0 {
			res.Errors[cap] = ErrUnsupported.Error()
			return
		}
		var last error
		for _, drv := range drivers {
			if err := call(drv); err != nil {
				last = fmt.Errorf("%s: %w", drv.Name(), err)
				continue
			}
			res.ServedBy[cap] = drv.Name()
			res.Reachable = true
			return
		}
		res.Errors[cap] = last.Error()
	}

	try(model.CapReadStatus, func(drv Driver) error {
		r, ok := drv.(StatusReader)
		if !ok {
			return ErrUnsupported
		}
		s, err := r.ReadStatus(ctx, d, creds)
		if err != nil {
			return err
		}
		s.Source = drv.Name()
		res.Status = s
		return nil
	})
	try(model.CapReadSupplies, func(drv Driver) error {
		r, ok := drv.(SupplyReader)
		if !ok {
			return ErrUnsupported
		}
		s, err := r.ReadSupplies(ctx, d, creds)
		if err != nil {
			return err
		}
		res.Supplies = s
		return nil
	})
	try(model.CapReadTrays, func(drv Driver) error {
		r, ok := drv.(TrayReader)
		if !ok {
			return ErrUnsupported
		}
		t, err := r.ReadTrays(ctx, d, creds)
		if err != nil {
			return err
		}
		res.Trays = t
		return nil
	})
	try(model.CapReadCounters, func(drv Driver) error {
		r, ok := drv.(CounterReader)
		if !ok {
			return ErrUnsupported
		}
		c, err := r.ReadCounters(ctx, d, creds)
		if err != nil {
			return err
		}
		res.Counters = c
		return nil
	})
	try(model.CapFirmwareRead, func(drv Driver) error {
		r, ok := drv.(FirmwareReader)
		if !ok {
			return ErrUnsupported
		}
		fw, err := r.ReadFirmware(ctx, d, creds)
		if err != nil {
			return err
		}
		res.Firmware = fw
		return nil
	})

	if !res.Reachable {
		res.Status = &model.Status{
			Online:   false,
			State:    "offline",
			Severity: model.SeverityCritical,
			Message:  "device unreachable",
			At:       res.At,
		}
	}
	return res
}

// HasCap reports whether the device's resolved profile includes cap.
func HasCap(d *model.Device, cap model.Capability) bool {
	if d == nil || d.Capabilities == nil {
		return false
	}
	_, ok := d.Capabilities[cap]
	return ok
}

// ServedBy reports which driver the profile assigns to cap ("" if none).
func ServedBy(d *model.Device, cap model.Capability) string {
	if d == nil || d.Capabilities == nil {
		return ""
	}
	return d.Capabilities[cap]
}
