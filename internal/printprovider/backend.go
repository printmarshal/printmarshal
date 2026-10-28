package printprovider

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Backend is the local print system the module configures. Nothing behind
// this interface ever talks to a printer; it only edits this host's queues.
type Backend interface {
	// Availability describes what the backend can do on this host.
	Availability() Availability
	// ListQueues returns the queues the local print system currently has.
	ListQueues(ctx context.Context) ([]Queue, error)
	// CreateOrUpdate creates the queue, or re-points an existing one, then
	// enables it and makes it accept jobs.
	CreateOrUpdate(ctx context.Context, spec QueueSpec) error
	// Remove deletes a queue.
	Remove(ctx context.Context, name string) error
}

// Availability reports whether real queues can be created on this host.
type Availability struct {
	Backend   string            `json:"backend"` // cups | dry
	OS        string            `json:"os"`
	Linux     bool              `json:"linux"`
	CUPSFound bool              `json:"cupsFound"`
	Tools     map[string]string `json:"tools,omitempty"` // tool -> path
	Reason    string            `json:"reason,omitempty"`
}

// Queue is one local print queue.
type Queue struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location,omitempty"`
	// Managed is true when the name carries the module's prefix, i.e. the
	// queue is one this module created and may remove.
	Managed bool `json:"managed"`
	// DeviceID is set when the planner matched the queue to a device.
	DeviceID string `json:"deviceId,omitempty"`
}

// QueueSpec is the desired state of one queue.
type QueueSpec struct {
	Name        string      `json:"name"`
	URI         string      `json:"uri"`
	Description string      `json:"description,omitempty"`
	Location    string      `json:"location,omitempty"`
	Driverless  bool        `json:"driverless"`
	Options     [][2]string `json:"options,omitempty"`
}

// cupsTools are the command-line tools a working CUPS host provides.
var cupsTools = []string{"lpadmin", "lpstat", "cupsenable", "cupsaccept"}

// CreateCommands renders the commands CreateOrUpdate runs for a spec. The dry
// backend records exactly these, and the plan preview shows them.
func CreateCommands(spec QueueSpec) [][]string {
	args := []string{"lpadmin", "-p", spec.Name, "-E", "-v", spec.URI}
	if spec.Driverless {
		args = append(args, "-m", "everywhere")
	} else {
		args = append(args, "-m", "raw")
	}
	if spec.Description != "" {
		args = append(args, "-D", spec.Description)
	}
	if spec.Location != "" {
		args = append(args, "-L", spec.Location)
	}
	for _, kv := range spec.Options {
		args = append(args, "-o", kv[0]+"="+kv[1])
	}
	return [][]string{args, {"cupsenable", spec.Name}, {"cupsaccept", spec.Name}}
}

// RemoveCommands renders the commands Remove runs.
func RemoveCommands(name string) [][]string {
	return [][]string{{"lpadmin", "-x", name}}
}

// RenderCommands formats argv lists for display, quoting arguments with spaces.
func RenderCommands(cmds [][]string) []string {
	out := make([]string, 0, len(cmds))
	for _, c := range cmds {
		parts := make([]string, len(c))
		for i, a := range c {
			if strings.ContainsAny(a, " \t\"'") {
				a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
			}
			parts[i] = a
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

// --- dry backend --------------------------------------------------------------

// DryBackend records the commands it would run instead of running them. It is
// the backend on non-Linux hosts, on Linux hosts without CUPS, in CI, and for
// dry-run previews. Its queue inventory is whatever was seeded plus what has
// been "created" through it.
type DryBackend struct {
	mu       sync.Mutex
	avail    Availability
	queues   map[string]Queue
	commands []string
}

// NewDryBackend builds a recording backend; reason says why the real one is
// not in use (shown in the UI).
func NewDryBackend(reason string) *DryBackend {
	return &DryBackend{avail: Availability{Backend: "dry", OS: runtime.GOOS, Linux: runtime.GOOS == "linux", Reason: reason}, queues: map[string]Queue{}}
}

// Seed sets the inventory the dry backend reports (tests, previews).
func (b *DryBackend) Seed(qs ...Queue) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, q := range qs {
		b.queues[strings.ToLower(q.Name)] = q
	}
}

// Commands returns everything recorded so far, in order.
func (b *DryBackend) Commands() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.commands...)
}

// Availability implements Backend.
func (b *DryBackend) Availability() Availability { return b.avail }

// ListQueues implements Backend.
func (b *DryBackend) ListQueues(context.Context) ([]Queue, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.commands = append(b.commands, "lpstat -v")
	out := make([]Queue, 0, len(b.queues))
	for _, q := range b.queues {
		out = append(out, q)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// CreateOrUpdate implements Backend.
func (b *DryBackend) CreateOrUpdate(_ context.Context, spec QueueSpec) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.commands = append(b.commands, RenderCommands(CreateCommands(spec))...)
	b.queues[strings.ToLower(spec.Name)] = Queue{Name: spec.Name, URI: spec.URI, Description: spec.Description, Location: spec.Location}
	return nil
}

// Remove implements Backend.
func (b *DryBackend) Remove(_ context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.commands = append(b.commands, RenderCommands(RemoveCommands(name))...)
	delete(b.queues, strings.ToLower(name))
	return nil
}

// --- CUPS backend -------------------------------------------------------------

// runFunc executes a command and returns its combined output.
type runFunc func(ctx context.Context, name string, args ...string) (string, error)

// cupsBackend shells out to the CUPS command-line tools. It is only
// constructed on Linux when every tool resolves.
type cupsBackend struct {
	tools map[string]string
	run   runFunc
}

// detectBackend picks the real CUPS backend when this is a Linux host with the
// CUPS tools on PATH, otherwise a dry backend explaining why.
func detectBackend() Backend {
	tools := map[string]string{}
	missing := []string{}
	for _, t := range cupsTools {
		if p, err := exec.LookPath(t); err == nil {
			tools[t] = p
		} else {
			missing = append(missing, t)
		}
	}
	found := len(missing) == 0
	if runtime.GOOS != "linux" {
		d := NewDryBackend(fmt.Sprintf("not a Linux host (%s): the dry backend records commands only and never changes this machine", runtime.GOOS))
		d.avail.CUPSFound = found
		d.avail.Tools = tools
		return d
	}
	if !found {
		d := NewDryBackend("CUPS tools not found on PATH (" + strings.Join(missing, ", ") + "): install cups to create real queues")
		d.avail.Tools = tools
		return d
	}
	return &cupsBackend{tools: tools, run: execRun}
}

func execRun(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return string(out), fmt.Errorf("%s: %s", name, msg)
	}
	return string(out), nil
}

func (b *cupsBackend) Availability() Availability {
	return Availability{Backend: "cups", OS: runtime.GOOS, Linux: true, CUPSFound: true, Tools: b.tools}
}

func (b *cupsBackend) ListQueues(ctx context.Context) ([]Queue, error) {
	out, err := b.run(ctx, b.tools["lpstat"], "-v")
	if err != nil {
		if strings.Contains(strings.ToLower(out+err.Error()), "no destinations") {
			return nil, nil
		}
		return nil, err
	}
	qs := parseLpstatV(out)
	if detail, err := b.run(ctx, b.tools["lpstat"], "-l", "-p"); err == nil {
		meta := parseLpstatLP(detail)
		for i := range qs {
			if m, ok := meta[strings.ToLower(qs[i].Name)]; ok {
				qs[i].Description, qs[i].Location = m[0], m[1]
			}
		}
	}
	return qs, nil
}

func (b *cupsBackend) CreateOrUpdate(ctx context.Context, spec QueueSpec) error {
	for _, c := range CreateCommands(spec) {
		if _, err := b.run(ctx, b.tools[c[0]], c[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (b *cupsBackend) Remove(ctx context.Context, name string) error {
	for _, c := range RemoveCommands(name) {
		if _, err := b.run(ctx, b.tools[c[0]], c[1:]...); err != nil {
			return err
		}
	}
	return nil
}

// parseLpstatV reads "device for NAME: URI" lines.
func parseLpstatV(out string) []Queue {
	var qs []Queue
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "device for ")
		if !ok {
			continue
		}
		name, uri, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		qs = append(qs, Queue{Name: strings.TrimSpace(name), URI: strings.TrimSpace(uri)})
	}
	return qs
}

// parseLpstatLP reads "printer NAME ..." blocks with indented Description /
// Location lines, keyed by lower-cased name.
func parseLpstatLP(out string) map[string][2]string {
	meta := map[string][2]string{}
	cur := ""
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "printer "); ok {
			cur = strings.ToLower(strings.Fields(rest + " ")[0])
			continue
		}
		t := strings.TrimSpace(line)
		if cur == "" {
			continue
		}
		m := meta[cur]
		if v, ok := strings.CutPrefix(t, "Description:"); ok {
			m[0] = strings.TrimSpace(v)
		} else if v, ok := strings.CutPrefix(t, "Location:"); ok {
			m[1] = strings.TrimSpace(v)
		}
		meta[cur] = m
	}
	return meta
}
