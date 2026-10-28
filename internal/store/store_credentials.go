package store

import (
	"context"
	"errors"
	"fmt"

	"printmarshal/internal/model"
)

// ErrNoWriteCredential is returned when a write is requested for a device that
// has no write-capable credential (purpose write or both) bound to it, its
// groups or globally (audit A19). Callers fail the per-device result with it
// instead of falling back to a read credential.
var ErrNoWriteCredential = errors.New(`no write credential bound to this device or its groups; bind a credential with purpose "write" or "both" to the device, one of its groups or globally`)

// WriteCredentialStore is the purpose-bound side of the credential store
// (audit A19). It is defined here (own file), implemented on *SQLite in
// sqlite_credentials.go, and embedded in Store.
type WriteCredentialStore interface {
	// ResolveWriteCreds returns the credentials whose purpose is write or both,
	// device-scoped first, then group-scoped, then global, or
	// ErrNoWriteCredential when none is bound. ResolveCreds keeps returning the
	// read-capable ones (read or both).
	ResolveWriteCreds(ctx context.Context, deviceID string, groupIDs []string) (model.Creds, error)
	// SetCredentialPurpose changes a credential's purpose and returns the
	// updated metadata.
	SetCredentialPurpose(ctx context.Context, id string, purpose model.CredentialPurpose) (*model.Credential, error)
}

// GroupScopesFor lists the groups a device belongs to for credential
// resolution: static membership plus every rule group it matches.
func GroupScopesFor(ctx context.Context, st Store, dev *model.Device) []string {
	if dev == nil || dev.ID == "" || st == nil {
		return nil
	}
	groups, _ := st.GroupsForDevice(ctx, dev.ID)
	if all, err := st.ListGroups(ctx); err == nil {
		for _, g := range all {
			if g.Kind == "rule" && MatchRule(g.Rule, dev) {
				groups = append(groups, g.ID)
			}
		}
	}
	return groups
}

// CredsFor resolves a device's credentials for a purpose: PurposeRead (or
// empty) through ResolveCreds, PurposeWrite through ResolveWriteCreds. It is
// the single place the write-side services go through, so a store that does
// not implement WriteCredentialStore can never hand out write credentials:
// the request fails closed with ErrNoWriteCredential.
func CredsFor(ctx context.Context, st Store, dev *model.Device, purpose model.CredentialPurpose) (model.Creds, error) {
	id := ""
	if dev != nil {
		id = dev.ID
	}
	groups := GroupScopesFor(ctx, st, dev)
	switch purpose {
	case model.PurposeWrite, model.PurposeBoth:
		ws, ok := st.(WriteCredentialStore)
		if !ok {
			return model.Creds{}, fmt.Errorf("%w: the store has no write-credential support", ErrNoWriteCredential)
		}
		return ws.ResolveWriteCreds(ctx, id, groups)
	default:
		return st.ResolveCreds(ctx, id, groups)
	}
}
