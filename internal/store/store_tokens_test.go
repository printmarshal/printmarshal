package store

import (
	"context"
	"testing"
	"time"

	"printmarshal/internal/model"
)

func testAPITokens(t *testing.T, s testStore) {
	ctx := context.Background()
	exp := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	tok := &APIToken{Name: "cmdb", Role: model.RoleViewer, Scope: []string{"g1"}, ExpiresAt: &exp, CreatedBy: "admin"}
	if err := s.CreateToken(ctx, tok, "hash-a"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateToken(ctx, &APIToken{Name: "dup", Role: model.RoleViewer}, "hash-a"); err == nil {
		t.Error("duplicate secret hash accepted")
	}
	if err := s.CreateToken(ctx, &APIToken{Name: "ops", Role: model.RoleOperator}, "hash-b"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTokenByHash(ctx, "hash-a")
	if err != nil || got.ID != tok.ID || got.Role != model.RoleViewer || len(got.Scope) != 1 || got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) || got.CreatedAt.IsZero() {
		t.Fatalf("by hash: %+v %v", got, err)
	}
	if _, err := s.GetTokenByHash(ctx, "nope"); err != ErrNotFound {
		t.Errorf("missing hash: %v", err)
	}
	if list, _ := s.ListTokens(ctx); len(list) != 2 || list[0].Name != "ops" || len(list[1].Scope) != 1 {
		t.Errorf("list (newest first) = %+v", list)
	}
	used := time.Now().UTC().Truncate(time.Second)
	if err := s.TouchToken(ctx, tok.ID, used); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(ctx, tok.ID, used); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeToken(ctx, tok.ID, used); err != ErrNotFound {
		t.Errorf("revoke twice: %v", err)
	}
	got, _ = s.GetToken(ctx, tok.ID)
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(used) || got.RevokedAt == nil || got.Active(time.Now()) {
		t.Errorf("after revoke: %+v", got)
	}
	if _, err := s.GetToken(ctx, "nope"); err != ErrNotFound {
		t.Errorf("missing token: %v", err)
	}

	u := &model.User{Username: "sso", Role: model.RoleViewer}
	if err := s.CreateUser(ctx, u, "h"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserRole(ctx, u.ID, model.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetUser(ctx, u.ID); got.Role != model.RoleAdmin {
		t.Errorf("role = %s", got.Role)
	}
	if err := s.SetUserRole(ctx, "nope", model.RoleAdmin); err != ErrNotFound {
		t.Errorf("role of missing user: %v", err)
	}
}
