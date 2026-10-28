package store

import (
	"context"
	"errors"
	"testing"

	"printmarshal/internal/model"
)

// Audit A19: ResolveCreds hands out read-capable credentials only,
// ResolveWriteCreds write-capable ones only, and a device with nothing
// write-capable in scope gets ErrNoWriteCredential, never a read credential.
func TestCredentialPurposeResolution(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	mk := func(name, scope string, purpose model.CredentialPurpose, prio int, community string) *model.Credential {
		t.Helper()
		c := &model.Credential{Name: name, Type: model.CredSNMPCommunity, Scope: scope, Priority: prio, Purpose: purpose}
		if err := s.CreateCredential(ctx, c, &model.Secret{Community: community}); err != nil {
			t.Fatal(err)
		}
		return c
	}
	mk("global-read", "", model.PurposeRead, 0, "readComm")
	mk("global-both", "", model.PurposeBoth, 1, "bothComm")
	mk("group-write", "group:g1", model.PurposeWrite, 0, "groupWriteComm")
	mk("dev-write", "device:dev1", model.PurposeWrite, 0, "devWriteComm")
	legacy := &model.Credential{Name: "legacy", Type: model.CredSNMPCommunity} // no purpose: defaults to read
	if err := s.CreateCredential(ctx, legacy, &model.Secret{Community: "legacyComm"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCredential(ctx, &model.Credential{Name: "bad", Type: model.CredSNMPCommunity, Purpose: "admin"}, &model.Secret{Community: "x"}); err == nil {
		t.Error("an unknown purpose must be rejected")
	}

	list, err := s.ListCredentials(ctx)
	if err != nil {
		t.Fatal(err)
	}
	purposes := map[string]model.CredentialPurpose{}
	for _, c := range list {
		purposes[c.Name] = c.Purpose
	}
	if purposes["legacy"] != model.PurposeRead || purposes["group-write"] != model.PurposeWrite || purposes["global-both"] != model.PurposeBoth {
		t.Errorf("listed purposes = %v", purposes)
	}

	// Reads: read + both, write-only never.
	read, err := s.ResolveCreds(ctx, "dev1", []string{"g1"})
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(read.Communities, []string{"bothComm", "readComm", "legacyComm"}) {
		t.Errorf("read communities = %v", read.Communities)
	}
	// Writes: write + both, device scope first, read-only never.
	write, err := s.ResolveWriteCreds(ctx, "dev1", []string{"g1"})
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(write.Communities, []string{"devWriteComm", "groupWriteComm", "bothComm"}) {
		t.Errorf("write communities = %v", write.Communities)
	}
	// Global "both" counts for every device.
	if w, err := s.ResolveWriteCreds(ctx, "other", nil); err != nil || !equalStrings(w.Communities, []string{"bothComm"}) {
		t.Errorf("global both = %v %v", w.Communities, err)
	}

	// Demote the global "both" to read: a device with only read credentials in
	// scope must get ErrNoWriteCredential.
	var both *model.Credential
	for _, c := range list {
		if c.Name == "global-both" {
			both = c
		}
	}
	upd, err := s.SetCredentialPurpose(ctx, both.ID, model.PurposeRead)
	if err != nil || upd.Purpose != model.PurposeRead || upd.ID != both.ID {
		t.Fatalf("set purpose = %+v %v", upd, err)
	}
	if _, err := s.ResolveWriteCreds(ctx, "other", nil); !errors.Is(err, ErrNoWriteCredential) {
		t.Errorf("expected ErrNoWriteCredential, got %v", err)
	}
	if w, err := s.ResolveWriteCreds(ctx, "dev1", nil); err != nil || !equalStrings(w.Communities, []string{"devWriteComm"}) {
		t.Errorf("device write creds after demotion = %v %v", w.Communities, err)
	}
	if _, err := s.SetCredentialPurpose(ctx, "nope", model.PurposeWrite); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id = %v", err)
	}
	if _, err := s.SetCredentialPurpose(ctx, both.ID, "x"); err == nil {
		t.Error("invalid purpose accepted")
	}

	// CredsFor is the services' single entry point: write fails closed.
	dev := &model.Device{ID: "other"}
	if _, err := CredsFor(ctx, s, dev, model.PurposeWrite); !errors.Is(err, ErrNoWriteCredential) {
		t.Errorf("CredsFor write = %v", err)
	}
	if c, err := CredsFor(ctx, s, dev, model.PurposeRead); err != nil || len(c.Communities) == 0 {
		t.Errorf("CredsFor read = %v %v", c, err)
	}
}

// The purpose column is added by a guarded ALTER TABLE; a row written without
// it (a pre-migration database) reads back as a read credential.
func TestCredentialPurposeMigrationDefault(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	enc, err := s.box.Seal([]byte(`{"community":"oldComm"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO credentials(id,name,type,scope,username,priority,secret,created_at,updated_at) VALUES('old','old','snmp_community','','',0,?,?,?)`, enc, now(), now()); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateCredentialPurpose(); err != nil {
		t.Fatalf("migration must be idempotent: %v", err)
	}
	list, _ := s.ListCredentials(ctx)
	if len(list) != 1 || list[0].Purpose != model.PurposeRead {
		t.Fatalf("legacy row = %+v", list)
	}
	if _, err := s.ResolveWriteCreds(ctx, "", nil); !errors.Is(err, ErrNoWriteCredential) {
		t.Errorf("legacy credential must not be write-capable: %v", err)
	}
	if c, _ := s.ResolveCreds(ctx, "", nil); !equalStrings(c.Communities, []string{"oldComm"}) {
		t.Errorf("legacy credential must still read: %v", c.Communities)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
