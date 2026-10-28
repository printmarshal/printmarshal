package store

import (
	"context"
	"errors"
	"testing"

	"printmarshal/internal/catalog"
)

func testFamilyQualifications(t *testing.T, s testStore) {
	ctx := context.Background()
	if _, err := s.GetFamilyQualification(ctx, "HP01"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing override should be ErrNotFound, got %v", err)
	}
	q := &catalog.Qualification{FamilyID: "HP01", Status: catalog.QualLabValidated, Note: "bench E87770z", Actor: "ops"}
	if err := s.SetFamilyQualification(ctx, q); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetFamilyQualification(ctx, "HP01")
	if err != nil || got.Status != catalog.QualLabValidated || got.Note != "bench E87770z" || got.Actor != "ops" || got.UpdatedAt.IsZero() {
		t.Fatalf("get: %+v %v", got, err)
	}
	// Replace keeps one row per family.
	if err := s.SetFamilyQualification(ctx, &catalog.Qualification{FamilyID: "HP01", Status: catalog.QualUnsupported, Actor: "ops2"}); err != nil {
		t.Fatal(err)
	}
	all, err := s.ListFamilyQualifications(ctx)
	if err != nil || len(all) != 1 || all["HP01"].Status != catalog.QualUnsupported || all["HP01"].Actor != "ops2" {
		t.Fatalf("list: %+v %v", all, err)
	}
}
