// Package events is the single event contract: a catalogue of event types, a
// versioned envelope with a stable id and origin, and a bus that fans every
// published event out to the live stream and to durable subscribers (the
// notification outbox, SIEM export). Producers publish once; nothing is
// delivered "only to the hub" or "only to notifiers".
package events

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"printmarshal/internal/model"
)

// Envelope is the versioned event record.
type Envelope struct {
	ID      string    `json:"id"`      // stable, used for de-duplication and replay
	Version int       `json:"version"` // envelope schema version
	Type    string    `json:"type"`
	At      time.Time `json:"at"`
	Origin  Origin    `json:"origin"`
	Data    any       `json:"data"`
}

// Origin says who or what caused the event.
type Origin struct {
	Source    string `json:"source"`              // system | operator | device | scheduler
	Actor     string `json:"actor,omitempty"`     // username / service account
	RequestID string `json:"requestId,omitempty"` // API request correlation id
	IP        string `json:"ip,omitempty"`
	DeviceID  string `json:"deviceId,omitempty"`
}

// Def describes a catalogued event type.
type Def struct {
	Type        string
	Description string
	// Notifiable events may be routed to notification destinations.
	Notifiable bool
	// Security events are included in the security/audit export.
	Security bool
}

// Catalog is the complete list of event types the product can emit. Notifier
// subscriptions are validated against it, so a configured event always has a
// producer.
var Catalog = []Def{
	{Type: "device.discovered", Description: "A new device was added to the inventory", Notifiable: true},
	{Type: "device.updated", Description: "A device record or its polled state changed"},
	{Type: "device.deleted", Description: "A device was removed", Notifiable: true, Security: true},
	{Type: "alert.raised", Description: "An alert was raised", Notifiable: true},
	{Type: "alert.cleared", Description: "An alert was cleared (automatically or by an operator)", Notifiable: true},
	{Type: "alert.acked", Description: "An operator acknowledged an alert", Notifiable: true},
	{Type: "job.updated", Description: "A job's progress changed"},
	{Type: "job.finished", Description: "A job finished (succeeded, partial, failed or cancelled)", Notifiable: true},
	{Type: "config.applied", Description: "A configuration write completed on a device", Notifiable: true, Security: true},
	{Type: "security.assessed", Description: "A security assessment completed for a device", Notifiable: true, Security: true},
	{Type: "security.remediated", Description: "A security remediation wrote to a device", Notifiable: true, Security: true},
	{Type: "firmware.campaign.updated", Description: "A firmware campaign changed state", Notifiable: true, Security: true},
	{Type: "firmware.device.updated", Description: "A device's firmware update progressed", Notifiable: true, Security: true},
	{Type: "consumable.updated", Description: "A stock item changed"},
	{Type: "consumable.low", Description: "A stock item fell to its warning or critical level", Notifiable: true},
	{Type: "consumable.replacement", Description: "A supply replacement was detected or confirmed", Notifiable: true},
	{Type: "auth.login", Description: "A sign-in succeeded", Security: true},
	{Type: "auth.failed", Description: "A sign-in failed", Notifiable: true, Security: true},
	{Type: "auth.denied", Description: "An authenticated request was refused by authorization", Security: true},
	{Type: "auth.logout", Description: "A session ended", Security: true},
	{Type: "auth.session.revoked", Description: "Sessions were revoked", Security: true},
	{Type: "audit.recorded", Description: "An audit entry was written", Security: true},
	{Type: "settings.updated", Description: "Runtime settings changed", Notifiable: true, Security: true},
	{Type: "notification.test", Description: "An operator sent a test notification", Notifiable: true},
	{Type: "notification.dead", Description: "A notification could not be delivered after all retries", Notifiable: true},
	{Type: "stream.connected", Description: "(client-side) live stream connected"},
	{Type: "server.stopping", Description: "The server is shutting down", Security: true},
}

var byType = func() map[string]Def {
	m := map[string]Def{}
	for _, d := range Catalog {
		m[d.Type] = d
	}
	return m
}()

// Lookup returns the catalogue entry for a type.
func Lookup(t string) (Def, bool) { d, ok := byType[t]; return d, ok }

// Known reports whether t is a catalogued event type; "*" is accepted as "all notifiable".
func Known(t string) bool { _, ok := byType[t]; return ok || t == "*" }

// Subscriber receives every published envelope. It must not block for long.
type Subscriber interface {
	Handle(env Envelope)
}

// SubscriberFunc adapts a function.
type SubscriberFunc func(env Envelope)

// Handle implements Subscriber.
func (f SubscriberFunc) Handle(env Envelope) { f(env) }

// Bus fans events out to subscribers.
type Bus struct {
	mu   sync.RWMutex
	subs []Subscriber
	// Unknown receives event types that are not in the catalogue (logged by main).
	Unknown func(t string)
}

// NewBus returns an empty bus.
func NewBus() *Bus { return &Bus{} }

// Subscribe adds a subscriber.
func (b *Bus) Subscribe(s Subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs = append(b.subs, s)
}

// Publish wraps a model.Event in an envelope (system origin) and fans it out.
func (b *Bus) Publish(e model.Event) {
	b.PublishFrom(e, Origin{Source: "system"})
}

// PublishFrom publishes with an explicit origin.
func (b *Bus) PublishFrom(e model.Event, origin Origin) {
	if _, ok := byType[e.Type]; !ok && b.Unknown != nil {
		b.Unknown(e.Type)
	}
	at := e.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	env := Envelope{ID: uuid.NewString(), Version: 1, Type: e.Type, At: at, Origin: origin, Data: e.Data}
	b.mu.RLock()
	subs := append([]Subscriber(nil), b.subs...)
	b.mu.RUnlock()
	for _, s := range subs {
		s.Handle(env)
	}
}

// Publisher returns a func(model.Event) suitable for fleet.Deps.Events.
func (b *Bus) Publisher() func(model.Event) { return b.Publish }

// Origins for common producers.
var (
	System    = Origin{Source: "system"}
	Scheduler = Origin{Source: "scheduler", Actor: "scheduler"}
)
