package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"printmarshal/internal/model"
)

func scanCredentialPG(r rowScanner) (*model.Credential, error) {
	var c model.Credential
	var purpose string
	var cr, up time.Time
	if err := r.Scan(&c.ID, &c.Name, &c.Type, &c.Scope, &c.Username, &c.Priority, &purpose, &cr, &up); err != nil {
		return nil, err
	}
	c.Purpose = model.CredentialPurpose(purpose)
	if c.Purpose == "" {
		c.Purpose = model.PurposeRead
	}
	c.CreatedAt, c.UpdatedAt = cr.UTC(), up.UTC()
	return &c, nil
}

// ResolveWriteCreds implements WriteCredentialStore: only credentials with
// purpose write or both, or ErrNoWriteCredential when none is bound.
func (s *Postgres) ResolveWriteCreds(ctx context.Context, deviceID string, groupIDs []string) (model.Creds, error) {
	creds, n, err := s.resolveCreds(ctx, deviceID, groupIDs, model.CredentialPurpose.CanWrite)
	if err != nil {
		return creds, err
	}
	if n == 0 {
		return model.Creds{}, ErrNoWriteCredential
	}
	return creds, nil
}

// SetCredentialPurpose implements WriteCredentialStore.
func (s *Postgres) SetCredentialPurpose(ctx context.Context, id string, purpose model.CredentialPurpose) (*model.Credential, error) {
	if !purpose.Valid() {
		return nil, fmt.Errorf("credential purpose %q is not read, write or both", purpose)
	}
	if err := affected(pgExec(ctx, s.db, `UPDATE credentials SET purpose=?, updated_at=? WHERE id=?`, string(purpose), nowUTC(), id)); err != nil {
		return nil, err
	}
	c, err := scanCredentialPG(pgRow(ctx, s.db, `SELECT `+credentialCols+` FROM credentials WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return c, err
}

// resolveCreds merges the credentials in scope whose purpose passes want into
// one Creds, best first (device scope, then group, then global, then
// priority), and reports how many contributed. Same rules as SQLite.
func (s *Postgres) resolveCreds(ctx context.Context, deviceID string, groupIDs []string, want func(model.CredentialPurpose) bool) (model.Creds, int, error) {
	var creds model.Creds
	if s.box == nil {
		return creds, 0, nil
	}
	scopes := []string{""}
	for _, g := range groupIDs {
		scopes = append(scopes, "group:"+g)
	}
	if deviceID != "" {
		scopes = append(scopes, "device:"+deviceID)
	}
	rows, err := pgQuery(ctx, s.db, `SELECT type,scope,username,priority,purpose,secret FROM credentials WHERE scope IN (`+placeholders(len(scopes))+`)`, toAny(scopes)...)
	if err != nil {
		return creds, 0, err
	}
	defer rows.Close()
	type entry struct {
		typ      model.CredentialType
		rank     int
		username string
		sec      model.Secret
	}
	var entries []entry
	for rows.Next() {
		var typ, scope, user, purpose string
		var prio int
		var enc []byte
		if err := rows.Scan(&typ, &scope, &user, &prio, &purpose, &enc); err != nil {
			return creds, 0, err
		}
		if !want(model.CredentialPurpose(purpose)) {
			continue
		}
		plain, err := s.box.Open(enc)
		if err != nil {
			return creds, 0, fmt.Errorf("decrypt credential: %w", err)
		}
		var sec model.Secret
		if err := json.Unmarshal(plain, &sec); err != nil {
			return creds, 0, err
		}
		rank := prio
		switch {
		case strings.HasPrefix(scope, "device:"):
			rank += 2000
		case strings.HasPrefix(scope, "group:"):
			rank += 1000
		}
		entries = append(entries, entry{model.CredentialType(typ), rank, user, sec})
	}
	if err := rows.Err(); err != nil {
		return creds, 0, err
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].rank > entries[j].rank })
	for _, e := range entries {
		switch e.typ {
		case model.CredSNMPCommunity:
			if e.sec.Community != "" {
				creds.Communities = append(creds.Communities, e.sec.Community)
			}
		case model.CredSNMPv3:
			if creds.V3 == nil {
				creds.V3 = &model.V3Creds{Username: e.username, AuthProtocol: e.sec.AuthProtocol, AuthPassword: e.sec.AuthPassword,
					PrivProtocol: e.sec.PrivProtocol, PrivPassword: e.sec.PrivPassword}
			}
		case model.CredAdminPassword:
			if creds.AdminPass == "" {
				creds.AdminUser, creds.AdminPass = e.username, e.sec.Password
			}
		}
	}
	return creds, len(entries), nil
}

var _ WriteCredentialStore = (*Postgres)(nil)
