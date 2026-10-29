// Package alerts turns device state into alerts and delivers notifications
// (spec §8.3): webhooks (Slack / Teams / Discord / generic JSON) and SMTP email.
//
// The Notifier is an events.Bus subscriber: every catalogued event reaches it
// once, it formats a subject/body per event type and enqueues one durable
// outbox row per subscribed destination. A worker drains the outbox with
// retries and exponential backoff; rows that exhaust their attempts are marked
// dead and a notification.dead event is published so operators can see it.
package alerts

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/events"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Notifier delivers events to configured destinations through a durable outbox.
type Notifier struct {
	cfg   []config.Notifier
	store store.Store
	log   *slog.Logger
	http  *http.Client
	bus   *events.Bus

	// MaxAttempts before a delivery is declared dead.
	MaxAttempts int
	// Backoff for attempt n is Backoff * 2^(n-1), capped at MaxBackoff.
	Backoff, MaxBackoff time.Duration
	// Timeout per delivery attempt (HTTP and SMTP).
	Timeout time.Duration

	mu     sync.Mutex
	wake   chan struct{}
	tokens *tokenSource
}

// NewNotifier builds a notifier from config. It validates that every
// configured event type exists in the event catalogue.
func NewNotifier(cfg []config.Notifier, st store.Store, log *slog.Logger) *Notifier {
	for _, d := range cfg {
		for _, e := range d.Events {
			if !events.Known(e) {
				log.Warn("notifier subscribes to an unknown event type; it will never fire", "notifier", d.Name, "event", e)
			}
		}
	}
	return &Notifier{cfg: cfg, store: st, log: log,
		http:        &http.Client{Timeout: 10 * time.Second},
		tokens:      newTokenSource(&http.Client{Timeout: 10 * time.Second}),
		MaxAttempts: 6, Backoff: 30 * time.Second, MaxBackoff: 30 * time.Minute, Timeout: 20 * time.Second,
		wake: make(chan struct{}, 1)}
}

// Attach subscribes the notifier to the bus (and lets it publish notification.dead).
func (n *Notifier) Attach(bus *events.Bus) {
	n.bus = bus
	bus.Subscribe(n)
}

func wants(d config.Notifier, event string) bool {
	if len(d.Events) == 0 {
		return strings.HasPrefix(event, "alert.")
	}
	for _, e := range d.Events {
		if e == event {
			return true
		}
		if e == "*" {
			def, ok := events.Lookup(event)
			return ok && def.Notifiable
		}
	}
	return false
}

// Handle implements events.Subscriber: enqueue one outbox row per subscribed destination.
func (n *Notifier) Handle(env events.Envelope) {
	subject, body := n.format(env)
	if subject == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	queued := 0
	only := ""
	if env.Type == "notification.test" {
		if m, ok := env.Data.(map[string]any); ok {
			only, _ = m["notifier"].(string)
		}
	}
	for _, d := range n.cfg {
		if only != "" {
			if d.Name != only {
				continue
			}
		} else if !wants(d, env.Type) {
			continue
		}
		if env.Type == "notification.dead" {
			// never let a destination be told about its own failures forever
			if m, ok := env.Data.(map[string]any); ok && m["notifier"] == d.Name {
				continue
			}
		}
		o := &model.Outbox{EventID: env.ID, Event: env.Type, Notifier: d.Name, Subject: subject, Body: body, Payload: env.Data}
		if err := n.store.EnqueueNotification(ctx, o); err != nil {
			n.log.Error("enqueue notification", "notifier", d.Name, "event", env.Type, "err", err)
			continue
		}
		queued++
	}
	if queued > 0 {
		select {
		case n.wake <- struct{}{}:
		default:
		}
	}
}

// Notify enqueues an ad-hoc notification for event with an explicit subject
// and body (used by tests and the "send test notification" endpoint). It
// bypasses the bus but still goes through the outbox.
func (n *Notifier) Notify(event, subject, body string, data any) {
	n.Handle(events.Envelope{ID: fmt.Sprintf("adhoc-%d", time.Now().UnixNano()), Version: 1, Type: event, At: time.Now().UTC(), Origin: events.System, Data: data})
	if subject != "" { // override the catalogue formatting with the caller's words
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rows, _ := n.store.ListOutbox(ctx, "pending", 50)
		for _, o := range rows {
			if o.Event == event && strings.HasPrefix(o.EventID, "adhoc-") && o.Subject != subject {
				o.Subject, o.Body = subject, body
				_ = n.store.UpdateNotificationText(ctx, o)
			}
		}
	}
}

// Run drains the outbox until ctx ends. Deliveries are attempted in order of
// due time; failures are retried with exponential backoff.
func (n *Notifier) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		n.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-n.wake:
		}
	}
}

// Drain processes due rows once (exported for tests and the admin endpoint).
func (n *Notifier) Drain(ctx context.Context) { n.drain(ctx) }

func (n *Notifier) drain(ctx context.Context) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for {
		rows, err := n.store.DueNotifications(ctx, 20)
		if err != nil || len(rows) == 0 {
			return
		}
		for _, o := range rows {
			n.attempt(ctx, o)
		}
		if len(rows) < 20 {
			return
		}
	}
}

func (n *Notifier) dest(name string) (config.Notifier, bool) {
	for _, d := range n.cfg {
		if d.Name == name {
			return d, true
		}
	}
	return config.Notifier{}, false
}

func (n *Notifier) attempt(ctx context.Context, o *model.Outbox) {
	d, ok := n.dest(o.Notifier)
	var err error
	if !ok {
		err = fmt.Errorf("notifier %q is no longer configured", o.Notifier)
	} else {
		actx, cancel := context.WithTimeout(ctx, n.Timeout)
		err = n.deliver(actx, d, o.Event, o.Subject, o.Body, o.Payload)
		cancel()
	}
	o.Attempts++
	l := &model.NotificationLog{Notifier: o.Notifier, Event: o.Event, Target: o.Subject, OK: err == nil}
	now := time.Now().UTC()
	if err == nil {
		o.State, o.DoneAt, o.LastError = "delivered", &now, ""
	} else {
		l.Error = err.Error()
		o.LastError = err.Error()
		if !ok || o.Attempts >= n.MaxAttempts {
			o.State, o.DoneAt = "dead", &now
			n.log.Error("notification dead", "notifier", o.Notifier, "event", o.Event, "attempts", o.Attempts, "err", err)
			// A failure to deliver a notification.dead is logged but never
			// published as another notification.dead: with two failing
			// destinations that would ping-pong forever (code review H-014).
			if n.bus != nil && o.Event != "notification.dead" {
				n.bus.Publish(model.Event{Type: "notification.dead", Data: map[string]any{"notifier": o.Notifier, "event": o.Event, "subject": o.Subject, "attempts": o.Attempts, "error": err.Error(), "outboxId": o.ID}})
			}
		} else {
			back := n.Backoff << (o.Attempts - 1)
			if back > n.MaxBackoff || back <= 0 {
				back = n.MaxBackoff
			}
			o.NextAt = now.Add(back)
			n.log.Warn("notification failed; will retry", "notifier", o.Notifier, "event", o.Event, "attempt", o.Attempts, "retryIn", back, "err", err)
		}
	}
	if uerr := n.store.UpdateNotification(ctx, o); uerr != nil {
		n.log.Error("update outbox", "id", o.ID, "err", uerr)
	}
	if lerr := n.store.AddNotificationLog(ctx, l); lerr != nil {
		n.log.Error("notification log", "err", lerr)
	}
}

// format renders a subject and body for an event; an empty subject means the
// event is not something a human should be told about.
func (n *Notifier) format(env events.Envelope) (string, string) {
	devName := func(id string) string {
		if id == "" || n.store == nil {
			return id
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if d, err := n.store.GetDevice(ctx, id); err == nil {
			return fmt.Sprintf("%s (%s, %s)", d.Name(), d.Model, d.Address)
		}
		return id
	}
	switch v := env.Data.(type) {
	case *model.Alert:
		switch env.Type {
		case "alert.cleared":
			return fmt.Sprintf("[CLEARED] %s: %s", devName(v.DeviceID), v.Description), fmt.Sprintf("%s\n%s is resolved", devName(v.DeviceID), v.Description)
		case "alert.acked":
			return fmt.Sprintf("[ACKED] %s: %s", devName(v.DeviceID), v.Description), fmt.Sprintf("%s\nacknowledged by %s", devName(v.DeviceID), v.AckedBy)
		default:
			return fmt.Sprintf("[%s] %s: %s", strings.ToUpper(string(v.Severity)), devName(v.DeviceID), v.Description), fmt.Sprintf("%s\n%s (source: %s)", devName(v.DeviceID), v.Description, v.Source)
		}
	case *model.Device:
		if env.Type == "device.discovered" {
			return "New device: " + v.Name(), fmt.Sprintf("%s %s at %s (serial %s)", v.Make, v.Model, v.Address, v.Serial)
		}
		return "", ""
	case *model.Job:
		if env.Type != "job.finished" {
			return "", ""
		}
		return fmt.Sprintf("[JOB %s] %s", strings.ToUpper(string(v.Status)), v.Type), fmt.Sprintf("job %s (%s): %d/%d done, %d failed%s", v.ID, v.Type, v.Done, v.Total, v.Failed, errSuffix(v.Error))
	}
	def, ok := events.Lookup(env.Type)
	if !ok || !def.Notifiable {
		return "", ""
	}
	b, _ := json.MarshalIndent(env.Data, "", "  ")
	body := string(b)
	if m, ok := env.Data.(map[string]any); ok {
		if s, _ := m["subject"].(string); s != "" {
			return s, firstString(m, "body", body)
		}
		if s, _ := m["summary"].(string); s != "" {
			return "[" + env.Type + "] " + s, body
		}
	}
	return "[" + env.Type + "] " + def.Description, body
}

func firstString(m map[string]any, key, def string) string {
	if s, _ := m[key].(string); s != "" {
		return s
	}
	return def
}

func errSuffix(e string) string {
	if e == "" {
		return ""
	}
	return "; error: " + e
}

func (n *Notifier) deliver(ctx context.Context, dest config.Notifier, event, subject, body string, data any) error {
	switch dest.Type {
	case "smtp":
		return n.sendMail(ctx, dest, subject, body)
	case "slack":
		return n.post(ctx, dest.URL, map[string]any{"text": fmt.Sprintf("*%s*\n%s", subject, body)})
	case "discord":
		return n.post(ctx, dest.URL, map[string]any{"content": fmt.Sprintf("**%s**\n%s", subject, body)})
	case "teams":
		return n.post(ctx, dest.URL, teamsPayload(dest, subject, body))
	case "webhook", "":
		return n.post(ctx, dest.URL, map[string]any{"event": event, "subject": subject, "body": body, "data": data, "at": time.Now().UTC()})
	default:
		return fmt.Errorf("unknown notifier type %q", dest.Type)
	}
}

func (n *Notifier) post(ctx context.Context, url string, payload any) error {
	if url == "" {
		return fmt.Errorf("notifier has no url")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

// sendMail delivers over SMTP with a connection deadline derived from ctx so a
// stalled mail server cannot pin a worker.
func (n *Notifier) sendMail(ctx context.Context, dest config.Notifier, subject, body string) error {
	if dest.Host == "" || len(dest.To) == 0 {
		return fmt.Errorf("smtp notifier needs host and to")
	}
	port := dest.Port
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(dest.Host, fmt.Sprint(port))
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		dest.From, strings.Join(dest.To, ", "), subject, body)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(n.Timeout))
	}
	c, err := smtp.NewClient(conn, dest.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer c.Close()
	if dest.StartTLS {
		if err := c.StartTLS(&tls.Config{ServerName: dest.Host}); err != nil {
			return err
		}
	}
	if dest.Username != "" {
		a, err := n.smtpAuth(ctx, dest)
		if err != nil {
			return err
		}
		if err := c.Auth(a); err != nil {
			return err
		}
	}
	if err := c.Mail(dest.From); err != nil {
		return err
	}
	for _, to := range dest.To {
		if err := c.Rcpt(to); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
