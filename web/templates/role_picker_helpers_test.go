package templates

import (
	"database/sql"
	"strconv"
	"testing"

	"github.com/pluris/pluris/db"
)

func TestRolePickerOptionsBuiltinFamilyGrouping(t *testing.T) {
	roles := []db.Role{
		{ID: 1, Name: "User", Slug: "user", IsBuiltin: true},
		{ID: 2, Name: "Technician", Slug: "technician", IsBuiltin: true},
		{ID: 3, Name: "Custom User Clone", Slug: "custom-user-clone", IsBuiltin: false,
			TemplateSlug: sql.NullString{String: "user", Valid: true}},
	}
	opts := rolePickerOptions(roles)
	if len(opts) != 3 {
		t.Fatalf("len(opts) = %d, want 3", len(opts))
	}

	byID := make(map[int64]RolePickerOption, len(opts))
	for _, o := range opts {
		byID[o.ID] = o
	}

	if byID[1].Family != "user" {
		t.Errorf("builtin user role family = %q, want %q (own slug as family root)", byID[1].Family, "user")
	}
	if byID[2].Family != "technician" {
		t.Errorf("builtin technician role family = %q, want %q", byID[2].Family, "technician")
	}
	if byID[3].Family != "user" {
		t.Errorf("custom clone family = %q, want %q (from TemplateSlug)", byID[3].Family, "user")
	}
	// Custom clone shares the builtin user's family and sits alongside it,
	// not nested under it (clones aren't ParentRoleID children).
	if byID[3].Depth != 0 {
		t.Errorf("custom clone depth = %d, want 0 (no ParentRoleID set)", byID[3].Depth)
	}
}

func TestRolePickerOptionsCustomFamilyFallback(t *testing.T) {
	roles := []db.Role{
		{ID: 1, Name: "Standalone", Slug: "standalone", IsBuiltin: false},
	}
	opts := rolePickerOptions(roles)
	if len(opts) != 1 {
		t.Fatalf("len(opts) = %d, want 1", len(opts))
	}
	if opts[0].Family != "custom" {
		t.Errorf("family = %q, want %q (fallback for unset TemplateSlug)", opts[0].Family, "custom")
	}
}

func TestRolePickerOptionsChildIndentDepth(t *testing.T) {
	roles := []db.Role{
		{ID: 1, Name: "Root", Slug: "root", IsBuiltin: false},
		{ID: 2, Name: "Child", Slug: "child", IsBuiltin: false,
			ParentRoleID: sql.NullInt64{Int64: 1, Valid: true}},
		{ID: 3, Name: "Grandchild", Slug: "grandchild", IsBuiltin: false,
			ParentRoleID: sql.NullInt64{Int64: 2, Valid: true}},
	}
	opts := rolePickerOptions(roles)
	byID := make(map[int64]RolePickerOption, len(opts))
	for _, o := range opts {
		byID[o.ID] = o
	}

	if byID[1].Depth != 0 {
		t.Errorf("root depth = %d, want 0", byID[1].Depth)
	}
	if byID[2].Depth != 1 {
		t.Errorf("child depth = %d, want 1", byID[2].Depth)
	}
	if byID[3].Depth != 2 {
		t.Errorf("grandchild depth = %d, want 2", byID[3].Depth)
	}
	wantLabel := "— Child"
	if byID[2].Label != wantLabel {
		t.Errorf("child label = %q, want %q", byID[2].Label, wantLabel)
	}
}

func TestRolePickerOptionsDepthCap(t *testing.T) {
	// A chain of 8 roles, each parented to the previous, all sharing the
	// "custom" family fallback. Depth must cap at maxRolePickerDepth (5)
	// so a pathological chain never produces an unbounded indent.
	roles := make([]db.Role, 0, 8)
	for i := int64(1); i <= 8; i++ {
		r := db.Role{ID: i, Name: "R" + strconv.FormatInt(i, 10), Slug: "r" + strconv.FormatInt(i, 10)}
		if i > 1 {
			r.ParentRoleID = sql.NullInt64{Int64: i - 1, Valid: true}
		}
		roles = append(roles, r)
	}
	opts := rolePickerOptions(roles)
	byID := make(map[int64]RolePickerOption, len(opts))
	for _, o := range opts {
		byID[o.ID] = o
	}
	if byID[8].Depth != maxRolePickerDepth {
		t.Errorf("deep chain depth = %d, want cap %d", byID[8].Depth, maxRolePickerDepth)
	}
}
