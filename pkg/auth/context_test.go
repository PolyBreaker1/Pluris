package auth

import (
	"context"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
)

func TestSessionRoundTripsThroughContext(t *testing.T) {
	sess := &UserSession{
		IdentityID:  7,
		Email:       "jdoe@example.com",
		DisplayName: "Jane Doe",
		Role:        identities.RoleAdmin,
		TenantID:    3,
	}
	ctx := WithSession(context.Background(), sess)
	got := FromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil session from context")
	}
	if got.Email != "jdoe@example.com" {
		t.Fatalf("expected email jdoe@example.com, got %s", got.Email)
	}
	if got.Role != identities.RoleAdmin {
		t.Fatalf("expected role %q, got %q", identities.RoleAdmin, got.Role)
	}
	if got.IdentityID != 7 {
		t.Fatalf("expected identity id 7, got %d", got.IdentityID)
	}
	if got.DisplayName != "Jane Doe" {
		t.Fatalf("expected display name 'Jane Doe', got %q", got.DisplayName)
	}
	if got.TenantID != 3 {
		t.Fatalf("expected tenant id 3, got %d", got.TenantID)
	}
}

func TestFromContextReturnsNilWhenAbsent(t *testing.T) {
	if got := FromContext(context.Background()); got != nil {
		t.Fatalf("expected nil session, got %+v", got)
	}
}
