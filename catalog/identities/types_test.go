package identities

import "testing"

func TestIdentityDisplayNameFallsBackToGivenSurname(t *testing.T) {
	id := Identity{GivenName: "Jane", Surname: "Doe"}
	if got := id.ResolvedDisplayName(); got != "Jane Doe" {
		t.Fatalf("expected 'Jane Doe', got %q", got)
	}

	id2 := Identity{DisplayName: "J. Doe", GivenName: "Jane", Surname: "Doe"}
	if got := id2.ResolvedDisplayName(); got != "J. Doe" {
		t.Fatalf("expected explicit DisplayName to win, got %q", got)
	}

	id3 := Identity{Username: "jdoe"}
	if got := id3.ResolvedDisplayName(); got != "jdoe" {
		t.Fatalf("expected fallback to username, got %q", got)
	}

	id4 := Identity{GivenName: "Jane"}
	if got := id4.ResolvedDisplayName(); got != "Jane" {
		t.Fatalf("expected 'Jane' for GivenName-only, got %q", got)
	}

	id5 := Identity{Surname: "Doe"}
	if got := id5.ResolvedDisplayName(); got != "Doe" {
		t.Fatalf("expected 'Doe' for Surname-only, got %q", got)
	}
}

func TestRoleIsValid(t *testing.T) {
	valid := []Role{RoleSuperAdmin, RoleAdmin, RoleTechnician, RoleUser}
	for _, r := range valid {
		if !r.IsValid() {
			t.Fatalf("expected %q to be valid", r)
		}
	}
	if Role("bogus").IsValid() {
		t.Fatal("expected 'bogus' role to be invalid")
	}
}
