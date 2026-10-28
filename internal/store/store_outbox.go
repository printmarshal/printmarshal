package store

import (
	"context"

	"printmarshal/internal/model"
)

// OutboxStore persists notification deliveries so they are retried and
// survive restarts (durable outbox).
type OutboxStore interface {
	EnqueueNotification(ctx context.Context, o *model.Outbox) error
	// DueNotifications returns pending rows whose next attempt is due.
	DueNotifications(ctx context.Context, limit int) ([]*model.Outbox, error)
	UpdateNotification(ctx context.Context, o *model.Outbox) error
	// ListOutbox lists rows, newest first; state "" means all.
	ListOutbox(ctx context.Context, state string, limit int) ([]*model.Outbox, error)
	// RequeueDead moves a dead row back to pending.
	RequeueDead(ctx context.Context, id int64) error
	// UpdateNotificationText rewrites a pending row's subject and body.
	UpdateNotificationText(ctx context.Context, o *model.Outbox) error
}
