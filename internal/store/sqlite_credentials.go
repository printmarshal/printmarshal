package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"printmarshal/internal/model"
)

const credentialCols = `id,name,type,scope,username,priority,purpose,created_at,updated_at`

func scanCredential(r rowScanner) (*model.Credential, error) {
	var c model.Credential
	var purpose, cr, up string
	if err := r.Scan(&c.ID, &c.Name, &c.Type, &c.Scope, &c.Username, &c.Priority, &purpose, &cr, &up); err != nil {
		return nil, err
	}
	c.Purpose = model.CredentialPurpose(purpose)
	if c.Purpose == "" {
		c.Purpose = model.PurposeRead
	}
	c.CreatedAt, c.UpdatedAt = parseTime(cr), parseTime(up)
	return &c, nil
}

// migrateCredentialPurpose adds credentials.purpose (default 'read') when the
// column does not exist yet. Guarded by PRAGMA table_info so it is idempotent.
func (s *SQLite) migrateCredentialPurpose() error {
	rows, err := s.db.Query(`PRAGMA table_info(credentials)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "purpose" {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE credentials ADD COLUMN purpose TEXT NOT NULL DEFAULT 'read'`)
	return err
}

// ResolveWriteCreds implements WriteCredentialStore: only credentials with
// purpose write or both, or ErrNoWriteCredential when none is bound.
func (s *SQLite) ResolveWriteCreds(ctx context.Context, deviceID string, groupIDs []string) (model.Creds, error) {
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
func (s *SQLite) SetCredentialPurpose(ctx context.Context, id string, purpose model.CredentialPurpose) (*model.Credential, error) {
	if !purpose.Valid() {
		return nil, fmt.Errorf("credential purpose %q is not read, write or both", purpose)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE credentials SET purpose=?, updated_at=? WHERE id=?`, string(purpose), now(), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	c, err := scanCredential(s.db.QueryRowContext(ctx, `SELECT `+credentialCols+` FROM credentials WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return c, err
}

// resolveCreds merges the credentials in scope (global, the device's groups,
// the device) whose purpose passes want into one Creds, best first: device
// scope outranks group scope outranks global, then priority. It returns how
// many credentials contributed so callers can tell "nothing bound" apart from
// "bound but empty".
func (s *SQLite) resolveCreds(ctx context.Context, deviceID string, groupIDs []string, want func(model.CredentialPurpose) bool) (model.Creds, int, error) {
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
	rows, err := s.db.QueryContext(ctx, `SELECT type,scope,username,priority,purpose,secret FROM credentials WHERE scope IN (`+placeholders(len(scopes))+`)`, toAny(scopes)...)
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

var _ WriteCredentialStore = (*SQLite)(nil)
