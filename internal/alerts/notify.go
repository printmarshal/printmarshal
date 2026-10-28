// Package alerts turns device state into alerts and delivers notifications
// (spec §8.3): webhooks (Slack / Teams / Discord / generic JSON) and SMTP email.
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
	"os"
	"strings"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Notifier delivers events to configured destinations.
type Notifier struct {
	cfg   []config.Notifier
	store store.Store
	log   *slog.Logger
	http  *http.Client
}

// NewNotifier builds a notifier from config.
func NewNotifier(cfg []config.Notifier, st store.Store, log *slog.Logger) *Notifier {
	return &Notifier{cfg: cfg, store: st, log: log, http: &http.Client{Timeout: 10 * time.Second}}
}

func wants(n config.Notifier, event string) bool {
	if len(n.Events) == 0 {
		return strings.HasPrefix(event, "alert.")
	}
	for _, e := range n.Events {
		if e == event || e == "*" {
			return true
		}
	}
	return false
}

// Notify delivers an event to every notifier subscribed to it. Delivery is
// asynchronous and logged to the notification log.
func (n *Notifier) Notify(event string, subject string, body string, data any) {
	for _, dest := range n.cfg {
		if !wants(dest, event) {
			continue
		}
		dest := dest
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			err := n.deliver(ctx, dest, event, subject, body, data)
			l := &model.NotificationLog{Notifier: dest.Name, Event: event, Target: subject, OK: err == nil}
			if err != nil {
				l.Error = err.Error()
				n.log.Warn("notification failed", "notifier", dest.Name, "event", event, "err", err)
			}
			_ = n.store.AddNotificationLog(ctx, l)
		}()
	}
}

func (n *Notifier) deliver(ctx context.Context, dest config.Notifier, event, subject, body string, data any) error {
	switch dest.Type {
	case "smtp":
		return n.sendMail(dest, subject, body)
	case "slack":
		return n.post(ctx, dest.URL, map[string]any{"text": fmt.Sprintf("*%s*\n%s", subject, body)})
	case "discord":
		return n.post(ctx, dest.URL, map[string]any{"content": fmt.Sprintf("**%s**\n%s", subject, body)})
	case "teams":
		return n.post(ctx, dest.URL, map[string]any{"@type": "MessageCard", "title": subject, "text": body, "summary": subject})
	default: // generic webhook
		return n.post(ctx, dest.URL, map[string]any{"event": event, "subject": subject, "text": body, "data": data, "at": time.Now().UTC()})
	}
}

func (n *Notifier) post(ctx context.Context, url string, payload any) error {
	if url == "" {
		return fmt.Errorf("notifier has no url")
	}
	b, _ := json.Marshal(payload)
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

func (n *Notifier) sendMail(dest config.Notifier, subject, body string) error {
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
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if dest.StartTLS {
		if err := c.StartTLS(&tls.Config{ServerName: dest.Host}); err != nil {
			return err
		}
	}
	if dest.Username != "" {
		pw := os.Getenv(dest.PasswordEnv)
		if err := c.Auth(smtp.PlainAuth("", dest.Username, pw, dest.Host)); err != nil {
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
