// Package jobs runs fleet work as persisted, resumable jobs on an in-process
// worker pool (spec §5 "job execution", §3 principle 7).
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Handler executes one job. It reports per-device results through the Context
// and returns an error only for whole-job failure.
type Handler func(ctx context.Context, jc *Context) error

// Context is handed to a Handler. Handlers call Result from many goroutines
// at once, so the job is only ever read for persistence or events as a
// snapshot taken under mu, and saves of one job are serialised by saveMu so
// an older snapshot cannot overwrite a newer one (code review H-011).
type Context struct {
	Job    *model.Job
	runner *Runner
	mu     sync.Mutex
	saveMu sync.Mutex
}

// snapshot copies the job under the lock.
func (c *Context) snapshot() *model.Job {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := *c.Job
	return &cp
}

// persist writes a throttled progress snapshot.
func (c *Context) persist() {
	if !c.runner.throttle(c.Job.ID) {
		return
	}
	c.persistNow()
}

// persistNow writes the current snapshot; saves of one job never overlap.
func (c *Context) persistNow() {
	c.saveMu.Lock()
	defer c.saveMu.Unlock()
	c.runner.persistNow(c.snapshot())
}

// SetTotal sets the number of units of work.
func (c *Context) SetTotal(n int) {
	c.mu.Lock()
	c.Job.Total = n
	c.mu.Unlock()
	c.persist()
}

// Result records one per-device outcome and advances progress. A status of
// "error" or "failed" counts as a failure.
func (c *Context) Result(deviceID, address, status, message string, data map[string]any) {
	r := &model.JobResult{JobID: c.Job.ID, DeviceID: deviceID, Address: address, Status: status, Message: message, Data: data, At: time.Now().UTC()}
	if err := c.runner.store.AddJobResult(context.Background(), r); err != nil {
		c.runner.log.Error("job result", "job", c.Job.ID, "err", err)
	}
	c.mu.Lock()
	c.Job.Done++
	if status == "error" || status == "failed" {
		c.Job.Failed++
	}
	c.mu.Unlock()
	c.persist()
}

// Runner owns the worker pool and the job table.
type Runner struct {
	store    store.Store
	log      *slog.Logger
	handlers map[string]Handler
	queue    chan *model.Job
	events   func(model.Event)
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	mu       sync.Mutex
	waiters  map[string][]chan *model.Job
	lastSave map[string]time.Time
}

// New creates a runner with n workers. events receives job.updated events (may be nil).
func New(st store.Store, log *slog.Logger, workers int, events func(model.Event)) *Runner {
	if workers <= 0 {
		workers = 4
	}
	r := &Runner{store: st, log: log, handlers: map[string]Handler{}, queue: make(chan *model.Job, 1024), events: events,
		waiters: map[string][]chan *model.Job{}, lastSave: map[string]time.Time{}}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	for i := 0; i < workers; i++ {
		r.wg.Add(1)
		go r.worker(ctx)
	}
	return r
}

// Register binds a job type to a handler. It may be called while workers
// run jobs (the API registers the import job when it builds its routes).
func (r *Runner) Register(jobType string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[jobType] = h
}

func (r *Runner) handler(jobType string) (Handler, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.handlers[jobType]
	return h, ok
}

// Submit persists and enqueues a job.
func (r *Runner) Submit(ctx context.Context, j *model.Job) (*model.Job, error) {
	if _, ok := r.handler(j.Type); !ok {
		return nil, fmt.Errorf("no handler for job type %q", j.Type)
	}
	j.Status = model.JobQueued
	if err := r.store.CreateJob(ctx, j); err != nil {
		return nil, err
	}
	r.emit(j)
	// The worker owns j once it is queued; callers get a copy taken before
	// that so serialising the response cannot race with the worker marking
	// the job running.
	cp := *j
	select {
	case r.queue <- j:
	default:
		cp.Status, j.Status = model.JobFailed, model.JobFailed
		cp.Error, j.Error = "job queue is full", "job queue is full"
		_ = r.store.UpdateJob(ctx, &cp)
		return nil, fmt.Errorf("job queue is full")
	}
	return &cp, nil
}

// Wait blocks until the job finishes or ctx ends; returns the latest job state.
func (r *Runner) Wait(ctx context.Context, jobID string) (*model.Job, error) {
	ch := make(chan *model.Job, 1)
	r.mu.Lock()
	r.waiters[jobID] = append(r.waiters[jobID], ch)
	r.mu.Unlock()
	// The job may already be done.
	if j, err := r.store.GetJob(ctx, jobID); err == nil && j.FinishedAt != nil {
		return j, nil
	}
	select {
	case j := <-ch:
		return j, nil
	case <-ctx.Done():
		return r.store.GetJob(context.Background(), jobID)
	}
}

// Stop drains workers.
func (r *Runner) Stop() {
	r.cancel()
	r.wg.Wait()
}

func (r *Runner) worker(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-r.queue:
			r.run(ctx, j)
		}
	}
}

func (r *Runner) run(ctx context.Context, j *model.Job) {
	h, _ := r.handler(j.Type)
	now := time.Now().UTC()
	j.Status, j.StartedAt = model.JobRunning, &now
	r.persistNow(j)
	jc := &Context{Job: j, runner: r}
	var err error
	func() {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("panic: %v", p)
				r.log.Error("job panic", "job", j.ID, "type", j.Type, "panic", p, "stack", string(debug.Stack()))
			}
		}()
		err = h(ctx, jc)
	}()
	fin := time.Now().UTC()
	jc.mu.Lock()
	j.FinishedAt = &fin
	switch {
	case err != nil:
		j.Status, j.Error = model.JobFailed, err.Error()
	case j.Failed > 0 && j.Failed >= j.Done:
		j.Status = model.JobFailed
	case j.Failed > 0:
		j.Status = model.JobPartial
	default:
		j.Status = model.JobSucceeded
	}
	jc.mu.Unlock()
	jc.persistNow()
	if r.events != nil {
		r.events(model.Event{Type: "job.finished", At: fin, Data: jc.snapshot()})
	}
	r.mu.Lock()
	ws := r.waiters[j.ID]
	delete(r.waiters, j.ID)
	delete(r.lastSave, j.ID)
	r.mu.Unlock()
	for _, ch := range ws {
		ch <- j
	}
}

// throttle allows a few progress writes per second per job.
func (r *Runner) throttle(jobID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.lastSave[jobID]) < 250*time.Millisecond {
		return false
	}
	r.lastSave[jobID] = time.Now()
	return true
}

// persistNow saves and announces j, which must be a snapshot no goroutine
// still mutates.
func (r *Runner) persistNow(j *model.Job) {
	if err := r.store.UpdateJob(context.Background(), j); err != nil {
		r.log.Error("job update", "job", j.ID, "err", err)
	}
	r.emit(j)
}

func (r *Runner) emit(j *model.Job) {
	if r.events != nil {
		cp := *j
		r.events(model.Event{Type: "job.updated", At: time.Now().UTC(), Data: &cp})
	}
}
