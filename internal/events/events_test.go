package events

import (
	"testing"

	"printmarshal/internal/model"
)

func TestBusFanOutAndCatalog(t *testing.T) {
	b := NewBus()
	var got []Envelope
	b.Subscribe(SubscriberFunc(func(e Envelope) { got = append(got, e) }))
	unknown := ""
	b.Unknown = func(s string) { unknown = s }
	b.PublishFrom(model.Event{Type: "alert.raised", Data: map[string]any{"x": 1}}, Origin{Source: "operator", Actor: "kim"})
	b.Publish(model.Event{Type: "made.up"})
	if len(got) != 2 || got[0].ID == "" || got[0].Version != 1 || got[0].Origin.Actor != "kim" || got[0].At.IsZero() {
		t.Fatalf("envelopes = %+v", got)
	}
	if unknown != "made.up" {
		t.Errorf("unknown type should be reported, got %q", unknown)
	}
	if !Known("job.finished") || !Known("*") || Known("nope") {
		t.Error("Known")
	}
	seen := map[string]bool{}
	for _, d := range Catalog {
		if seen[d.Type] {
			t.Errorf("duplicate catalogue type %s", d.Type)
		}
		seen[d.Type] = true
	}
}
