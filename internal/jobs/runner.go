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

// Context is handed to a Handler.
type Context struct {
	Job    *model.Job
	runner *Runner
	mu     sync.Mutex
}

// SetTotal sets the number of units of work.
func (c *Context) SetTotal(n int) {
	c.mu.Lock()
	c.Job.Total = n
	c.mu.Unlock()
	c.runner.persist(c.Job)
}

// Result records one per-device outcome and advances progress.
func (c *Context) Result(deviceID, address, status, message string, data map[string]any) {
	r := &model.JobResult{JobID: c.Job.ID, DeviceID: deviceID, Address: address, Status: status, Message: message, Data: data, At: time.Now().UTC()}
	if err := c.runner.store.AddJobResult(context.Background(), r); err != nil {
		c.runner.log.Error("job result", "job", c.Job.ID, "err", err)
	}
	c.mu.Lock()
	c.Job.Done++
	if status == "error" {
		c.Job.Failed++
	}
	c.mu.Unlock()
	c.runner.persist(c.Job)
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

// Register binds a job type to a handler.
func (r *Runner) Register(jobType string, h Handler) { r.handlers[jobType] = h }

// Submit persists and enqueues a job.
func (r *Runner) Submit(ctx context.Context, j *model.Job) (*model.Job, error) {
	if _, ok := r.handlers[j.Type]; !ok {
		return nil, fmt.Errorf("no handler for job type %q", j.Type)
	}
	j.Status = model.JobQueued
	if err := r.store.CreateJob(ctx, j); err != nil {
		return nil, err
	}
	r.emit(j)
	select {
	case r.queue <- j:
	default:
		j.Status = model.JobFailed
		j.Error = "job queue is full"
		_ = r.store.UpdateJob(ctx, j)
		return nil, fmt.Errorf("job queue is full")
	}
	return j, nil
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
	h := r.handlers[j.Type]
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
	r.persistNow(j)
	if r.events != nil {
		cp := *j
		r.events(model.Event{Type: "job.finished", At: fin, Data: &cp})
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

// persist throttles progress writes to a few per second per job.
func (r *Runner) persist(j *model.Job) {
	r.mu.Lock()
	last := r.lastSave[j.ID]
	if time.Since(last) < 250*time.Millisecond {
		r.mu.Unlock()
		return
	}
	r.lastSave[j.ID] = time.Now()
	r.mu.Unlock()
	r.persistNow(j)
}

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
