package authz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
)

// createCustomRole creates a bare custom (non-builtin) role in tenantID with
// empty permissions, for inheritance test fixtures.
func createCustomRole(t *testing.T, dbase *database.Database, tenantID int64, name string) db.Role {
	t.Helper()
	role, err := dbase.Queries.CreateRole(context.Background(), db.CreateRoleParams{
		TenantID:  tenantID,
		Slug:      name,
		Name:      name,
		IsBuiltin: false,
	})
	if err != nil {
		t.Fatalf("CreateRole(%s) failed: %v", name, err)
	}
	return role
}

// setParentDirect bypasses SetRoleParent's guards to wire up a parent for
// fixture setup (tests that exercise the guards call SetRoleParent itself).
func setParentDirect(t *testing.T, dbase *database.Database, roleID, parentID int64) {
	t.Helper()
	if err := dbase.Queries.UpdateRoleParent(context.Background(), db.UpdateRoleParentParams{
		ID:           roleID,
		ParentRoleID: sql.NullInt64{Int64: parentID, Valid: true},
	}); err != nil {
		t.Fatalf("UpdateRoleParent failed: %v", err)
	}
}

func setupAuthzTestDB(t *testing.T) (*database.Database, int64) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "t.db")

	dbase, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { dbase.Close() })

	tenant, err := dbase.Queries.CreateTenant(context.Background(), db.CreateTenantParams{
		Name: "Test Org", Slug: "test-org-authz",
	})
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	return dbase, tenant.ID
}

func createTestIdentity(t *testing.T, dbase *database.Database, tenantID int64, username string) int64 {
	t.Helper()
	idSvc := services.NewIdentityService(dbase)
	ident, err := idSvc.Create(context.Background(), tenantID, identities.Identity{
		Username:    username,
		Email:       username + "@example.com",
		DisplayName: username,
		Role:        identities.RoleUser,
	})
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}
	return ident.ID
}

func TestEnsureBuiltinGrantsIdempotent(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}

	authzSvc := NewService(dbase)
	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants (1st) failed: %v", err)
	}

	// Manually override a value on the "user" role to something non-default.
	userRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}
	parsed := Parse(userRole.Permissions)
	parsed["identity.view"] = "all" // template default is "own"; this is a manual override
	if err := authzSvc.SaveRolePermissions(ctx, userRole.ID, parsed); err != nil {
		t.Fatalf("SaveRolePermissions failed: %v", err)
	}

	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants (2nd) failed: %v", err)
	}

	roles, err := dbase.Queries.ListRolesByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRolesByTenant failed: %v", err)
	}
	if len(roles) != 4 {
		t.Fatalf("expected 4 builtin roles, got %d", len(roles))
	}
	for _, r := range roles {
		if r.Permissions == "" || r.Permissions == "{}" {
			t.Errorf("role %s has empty permissions after EnsureBuiltinGrants", r.Slug)
		}
	}

	userRole2, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("GetRoleBySlug (2nd) failed: %v", err)
	}
	parsed2 := Parse(userRole2.Permissions)
	if parsed2["identity.view"] != "all" {
		t.Errorf("manual override did not survive merge: got %q, want all", parsed2["identity.view"])
	}
}

func TestEffectiveGrantsUnionsRoles(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	authzSvc := NewService(dbase)
	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants failed: %v", err)
	}

	identityID := createTestIdentity(t, dbase, tenantID, "combouser")

	userRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("GetRoleBySlug(user) failed: %v", err)
	}
	techRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("GetRoleBySlug(technician) failed: %v", err)
	}

	if err := roleSvc.Assign(ctx, identityID, userRole.ID, 0); err != nil {
		t.Fatalf("Assign user role failed: %v", err)
	}
	if err := roleSvc.Assign(ctx, identityID, techRole.ID, 0); err != nil {
		t.Fatalf("Assign technician role failed: %v", err)
	}

	grants, err := authzSvc.EffectiveGrants(ctx, identityID)
	if err != nil {
		t.Fatalf("EffectiveGrants failed: %v", err)
	}
	// user template: identity.view=own; technician template: identity.view=all.
	// Union must pick "all".
	if got := grants.ScopeOf("identity.view"); got != "all" {
		t.Errorf("EffectiveGrants identity.view = %q, want all (technician wins over user)", got)
	}
}

func TestEffectiveGrantsNoRolesIsEmpty(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()

	identityID := createTestIdentity(t, dbase, tenantID, "norole")

	authzSvc := NewService(dbase)
	grants, err := authzSvc.EffectiveGrants(ctx, identityID)
	if err != nil {
		t.Fatalf("EffectiveGrants failed: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("expected empty (deny-all) grants for identity with no roles, got %+v", grants)
	}
	if grants.Can("identity.create") {
		t.Error("identity with no roles should not be able to do anything")
	}
}

func TestSaveRolePermissionsRoundTrips(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	role, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}

	authzSvc := NewService(dbase)
	want := Grants{"identity.view": "all", "asset.view": "own"}
	if err := authzSvc.SaveRolePermissions(ctx, role.ID, want); err != nil {
		t.Fatalf("SaveRolePermissions failed: %v", err)
	}

	updated, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("GetRoleBySlug (after save) failed: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(updated.Permissions), &got); err != nil {
		t.Fatalf("failed to unmarshal saved permissions: %v", err)
	}
	if got["identity.view"] != "all" || got["asset.view"] != "own" {
		t.Errorf("SaveRolePermissions round trip mismatch: got %+v", got)
	}
}

func TestCloneRole(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	authzSvc := NewService(dbase)
	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants failed: %v", err)
	}

	source, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}

	clone, err := authzSvc.CloneRole(ctx, tenantID, source.ID, "My Custom Tech")
	if err != nil {
		t.Fatalf("CloneRole failed: %v", err)
	}
	if clone.IsBuiltin {
		t.Error("cloned role should not be builtin")
	}
	if !clone.TemplateSlug.Valid || clone.TemplateSlug.String != "technician" {
		t.Errorf("cloned role template_slug = %+v, want technician", clone.TemplateSlug)
	}
	if clone.Permissions != source.Permissions {
		t.Errorf("cloned role permissions %q != source permissions %q", clone.Permissions, source.Permissions)
	}
}

func TestResolveRoleMatrixParentlessReturnsStoredMatrix(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	authzSvc := NewService(dbase)
	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants failed: %v", err)
	}

	techRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}

	resolved, err := authzSvc.ResolveRoleMatrix(ctx, techRole)
	if err != nil {
		t.Fatalf("ResolveRoleMatrix failed: %v", err)
	}
	want := Parse(techRole.Permissions)
	if len(resolved) != len(want) {
		t.Fatalf("resolved matrix length %d, want %d", len(resolved), len(want))
	}
	for k, v := range want {
		if resolved[k] != v {
			t.Errorf("resolved[%q] = %q, want %q", k, resolved[k], v)
		}
	}
}

func TestResolveRoleMatrixChildOverridesParent(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	authzSvc := NewService(dbase)
	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants failed: %v", err)
	}

	techRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}

	child := createCustomRole(t, dbase, tenantID, "child-of-technician")
	setParentDirect(t, dbase, child.ID, techRole.ID)
	// technician template: identity.delete = "no"; override to "yes".
	if err := authzSvc.SaveRolePermissions(ctx, child.ID, Grants{"identity.delete": "yes"}); err != nil {
		t.Fatalf("SaveRolePermissions failed: %v", err)
	}
	child, err = dbase.Queries.GetRole(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}

	resolved, err := authzSvc.ResolveRoleMatrix(ctx, child)
	if err != nil {
		t.Fatalf("ResolveRoleMatrix failed: %v", err)
	}
	if resolved["identity.delete"] != "yes" {
		t.Errorf("resolved identity.delete = %q, want yes (child override)", resolved["identity.delete"])
	}
	techEffective := Parse(techRole.Permissions)
	for k, v := range techEffective {
		if k == "identity.delete" {
			continue
		}
		if resolved[k] != v {
			t.Errorf("resolved[%q] = %q, want inherited %q from technician", k, resolved[k], v)
		}
	}
}

func TestResolveRoleMatrixGrandchildLeafWins(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	authzSvc := NewService(dbase)
	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants failed: %v", err)
	}

	techRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}

	mid := createCustomRole(t, dbase, tenantID, "mid-role")
	setParentDirect(t, dbase, mid.ID, techRole.ID)
	if err := authzSvc.SaveRolePermissions(ctx, mid.ID, Grants{
		"identity.delete": "yes",
		"asset.delete":    "yes",
	}); err != nil {
		t.Fatalf("SaveRolePermissions(mid) failed: %v", err)
	}

	leaf := createCustomRole(t, dbase, tenantID, "leaf-role")
	setParentDirect(t, dbase, leaf.ID, mid.ID)
	// leaf overrides identity.delete back to "no"; leaf must win over mid.
	if err := authzSvc.SaveRolePermissions(ctx, leaf.ID, Grants{"identity.delete": "no"}); err != nil {
		t.Fatalf("SaveRolePermissions(leaf) failed: %v", err)
	}
	leaf, err = dbase.Queries.GetRole(ctx, leaf.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}

	resolved, err := authzSvc.ResolveRoleMatrix(ctx, leaf)
	if err != nil {
		t.Fatalf("ResolveRoleMatrix failed: %v", err)
	}
	if resolved["identity.delete"] != "no" {
		t.Errorf("resolved identity.delete = %q, want no (leaf wins over mid)", resolved["identity.delete"])
	}
	if resolved["asset.delete"] != "yes" {
		t.Errorf("resolved asset.delete = %q, want yes (inherited from mid)", resolved["asset.delete"])
	}
	techEffective := Parse(techRole.Permissions)
	if resolved["identity.view"] != techEffective["identity.view"] {
		t.Errorf("resolved identity.view = %q, want inherited %q from technician (root)", resolved["identity.view"], techEffective["identity.view"])
	}
}

func TestResolveRoleMatrixMissingParentTreatedAsParentless(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	authzSvc := NewService(dbase)

	parent := createCustomRole(t, dbase, tenantID, "doomed-parent")
	child := createCustomRole(t, dbase, tenantID, "orphan-child")
	setParentDirect(t, dbase, child.ID, parent.ID)
	if err := authzSvc.SaveRolePermissions(ctx, child.ID, Grants{"identity.delete": "yes"}); err != nil {
		t.Fatalf("SaveRolePermissions failed: %v", err)
	}

	// Delete the parent row out from under the child. The parent_role_id
	// FK is ON DELETE SET NULL, so the child's row now has a NULL
	// parent_role_id -- ResolveRoleMatrix must treat it as parentless
	// (this exercises the same "parent is gone" contract the defensive
	// sql.ErrNoRows branch in ResolveRoleMatrix/SetRoleParent covers for
	// any lower-level path that manages to leave a stale reference).
	if err := dbase.Queries.DeleteRole(ctx, parent.ID); err != nil {
		t.Fatalf("DeleteRole(parent) failed: %v", err)
	}

	child, err := dbase.Queries.GetRole(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}
	if child.ParentRoleID.Valid {
		t.Fatalf("child.ParentRoleID = %+v, want cleared by ON DELETE SET NULL", child.ParentRoleID)
	}

	resolved, err := authzSvc.ResolveRoleMatrix(ctx, child)
	if err != nil {
		t.Fatalf("ResolveRoleMatrix failed: %v", err)
	}
	want := Parse(child.Permissions)
	if len(resolved) != len(want) || resolved["identity.delete"] != "yes" {
		t.Errorf("resolved = %+v, want own stored matrix %+v (missing parent treated as parentless)", resolved, want)
	}
}

func TestSaveRoleOverridesParentlessStoresFullMatrix(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	authzSvc := NewService(dbase)

	role := createCustomRole(t, dbase, tenantID, "standalone-role")
	full := Grants{"identity.view": "own", "asset.view": "all"}
	if err := authzSvc.SaveRoleOverrides(ctx, role.ID, full); err != nil {
		t.Fatalf("SaveRoleOverrides failed: %v", err)
	}

	stored, err := dbase.Queries.GetRole(ctx, role.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(stored.Permissions), &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(raw) != 2 || raw["identity.view"] != "own" || raw["asset.view"] != "all" {
		t.Errorf("stored permissions = %+v, want full matrix %+v (parentless)", raw, full)
	}
}

func TestSaveRoleOverridesChildStoresOnlyDiff(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	authzSvc := NewService(dbase)
	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants failed: %v", err)
	}

	techRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}
	child := createCustomRole(t, dbase, tenantID, "diff-child")
	setParentDirect(t, dbase, child.ID, techRole.ID)

	// Submit the FULL matrix (as the form would), with exactly one key
	// changed relative to technician's effective matrix.
	full := Parse(techRole.Permissions)
	full["identity.delete"] = "yes" // technician default is "no"
	if err := authzSvc.SaveRoleOverrides(ctx, child.ID, full); err != nil {
		t.Fatalf("SaveRoleOverrides failed: %v", err)
	}

	stored, err := dbase.Queries.GetRole(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(stored.Permissions), &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("stored permissions = %+v, want exactly 1 diff key", raw)
	}
	if raw["identity.delete"] != "yes" {
		t.Errorf("stored identity.delete = %q, want yes", raw["identity.delete"])
	}

	// Resolve should still reflect the full effective matrix.
	child, err = dbase.Queries.GetRole(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}
	resolved, err := authzSvc.ResolveRoleMatrix(ctx, child)
	if err != nil {
		t.Fatalf("ResolveRoleMatrix failed: %v", err)
	}
	if resolved["identity.delete"] != "yes" {
		t.Errorf("resolved identity.delete = %q, want yes", resolved["identity.delete"])
	}
	if resolved["identity.view"] != full["identity.view"] {
		t.Errorf("resolved identity.view = %q, want %q (unchanged, inherited)", resolved["identity.view"], full["identity.view"])
	}
}

func TestSetRoleParentRejectsSelf(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	authzSvc := NewService(dbase)

	role := createCustomRole(t, dbase, tenantID, "self-role")
	err := authzSvc.SetRoleParent(ctx, role.ID, role.ID)
	if !errors.Is(err, ErrRoleCycle) {
		t.Errorf("SetRoleParent(self) err = %v, want ErrRoleCycle", err)
	}
}

func TestSetRoleParentRejectsCycle(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	authzSvc := NewService(dbase)

	a := createCustomRole(t, dbase, tenantID, "role-a")
	b := createCustomRole(t, dbase, tenantID, "role-b")

	if err := authzSvc.SetRoleParent(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("SetRoleParent(a->b) failed: %v", err)
	}
	// Now try b -> a, which would create a cycle a->b->a.
	err := authzSvc.SetRoleParent(ctx, b.ID, a.ID)
	if !errors.Is(err, ErrRoleCycle) {
		t.Errorf("SetRoleParent(b->a) err = %v, want ErrRoleCycle", err)
	}
}

func TestSetRoleParentRejectsBuiltinChild(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	roleSvc := services.NewRoleService(dbase)
	ctx := context.Background()

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	authzSvc := NewService(dbase)

	admin, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "admin"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}
	superAdmin, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "super_admin"})
	if err != nil {
		t.Fatalf("GetRoleBySlug failed: %v", err)
	}

	err = authzSvc.SetRoleParent(ctx, admin.ID, superAdmin.ID)
	if !errors.Is(err, ErrBuiltinParent) {
		t.Errorf("SetRoleParent(builtin child) err = %v, want ErrBuiltinParent", err)
	}
}

func TestSetRoleParentRejectsCrossTenantParent(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	authzSvc := NewService(dbase)

	otherTenant, err := dbase.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Other Org", Slug: "other-org-authz"})
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	role := createCustomRole(t, dbase, tenantID, "local-role")
	foreignParent := createCustomRole(t, dbase, otherTenant.ID, "foreign-role")

	err = authzSvc.SetRoleParent(ctx, role.ID, foreignParent.ID)
	if err == nil {
		t.Fatal("SetRoleParent(cross-tenant parent) succeeded, want error")
	}
	if errors.Is(err, ErrRoleCycle) || errors.Is(err, ErrBuiltinParent) {
		t.Errorf("SetRoleParent(cross-tenant parent) err = %v, want a not-found style error", err)
	}
}

func TestSetRoleParentAcceptsZeroClears(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	authzSvc := NewService(dbase)

	a := createCustomRole(t, dbase, tenantID, "clear-a")
	b := createCustomRole(t, dbase, tenantID, "clear-b")

	if err := authzSvc.SetRoleParent(ctx, a.ID, b.ID); err != nil {
		t.Fatalf("SetRoleParent(set) failed: %v", err)
	}
	if err := authzSvc.SetRoleParent(ctx, a.ID, 0); err != nil {
		t.Fatalf("SetRoleParent(clear) failed: %v", err)
	}
	got, err := dbase.Queries.GetRole(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}
	if got.ParentRoleID.Valid {
		t.Errorf("ParentRoleID = %+v, want cleared (invalid)", got.ParentRoleID)
	}
}

func TestSetRoleParentDepthCap(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	authzSvc := NewService(dbase)

	// Build a chain of MaxRoleDepth roles: r0 (root, parentless) -> r1 -> ... -> r(MaxRoleDepth-1).
	roles := make([]db.Role, MaxRoleDepth)
	roles[0] = createCustomRole(t, dbase, tenantID, "chain-0")
	for i := 1; i < MaxRoleDepth; i++ {
		roles[i] = createCustomRole(t, dbase, tenantID, "chain-"+string(rune('0'+i)))
	}
	for i := 1; i < MaxRoleDepth; i++ {
		if err := authzSvc.SetRoleParent(ctx, roles[i].ID, roles[i-1].ID); err != nil {
			t.Fatalf("SetRoleParent(chain depth %d) failed: %v", i, err)
		}
	}
	// Chain is now roles[0] (depth 1) .. roles[MaxRoleDepth-1] (depth MaxRoleDepth). That's exactly at the cap and must be allowed.

	// A 6th level (one more than MaxRoleDepth) must be rejected.
	extra := createCustomRole(t, dbase, tenantID, "chain-extra")
	err := authzSvc.SetRoleParent(ctx, extra.ID, roles[MaxRoleDepth-1].ID)
	if !errors.Is(err, ErrRoleCycle) {
		t.Errorf("SetRoleParent(exceeds depth) err = %v, want ErrRoleCycle (depth)", err)
	}
}

func TestResolveRoleMatrixDepthCapped(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	authzSvc := NewService(dbase)

	// Build a chain deeper than MaxRoleDepth directly via setParentDirect,
	// bypassing SetRoleParent's guard, to exercise the defensive read-time
	// depth cap. Root sets identity.view=all; every level below overrides
	// it to "own" except we track what the capped resolve should yield.
	depth := MaxRoleDepth + 3
	rolesChain := make([]db.Role, depth)
	rolesChain[0] = createCustomRole(t, dbase, tenantID, "deep-0")
	if err := authzSvc.SaveRolePermissions(ctx, rolesChain[0].ID, Grants{"identity.view": "all"}); err != nil {
		t.Fatalf("SaveRolePermissions failed: %v", err)
	}
	for i := 1; i < depth; i++ {
		rolesChain[i] = createCustomRole(t, dbase, tenantID, "deep-"+string(rune('a'+i)))
		setParentDirect(t, dbase, rolesChain[i].ID, rolesChain[i-1].ID)
	}
	// Leaf overrides a distinct key so we can tell it was reached (or not).
	leaf := rolesChain[depth-1]
	if err := authzSvc.SaveRolePermissions(ctx, leaf.ID, Grants{"asset.view": "all"}); err != nil {
		t.Fatalf("SaveRolePermissions failed: %v", err)
	}
	leaf, err := dbase.Queries.GetRole(ctx, leaf.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}

	// Must not error or hang despite exceeding MaxRoleDepth.
	resolved, err := authzSvc.ResolveRoleMatrix(ctx, leaf)
	if err != nil {
		t.Fatalf("ResolveRoleMatrix (over-depth chain) failed: %v", err)
	}
	if resolved["asset.view"] != "all" {
		t.Errorf("resolved asset.view = %q, want all (leaf's own value always applies)", resolved["asset.view"])
	}
}

// TestEffectiveGrantsIncludesGroupRolesWithInheritance covers RBAC v2
// EffectiveGrants: an identity with a direct "user" role plus membership
// in a group holding a custom role parented to "technician" (with its own
// override) must see both the inherited technician grants and the
// override, and lose them again once the group role is removed.
func TestEffectiveGrantsIncludesGroupRolesWithInheritance(t *testing.T) {
	dbase, tenantID := setupAuthzTestDB(t)
	ctx := context.Background()
	roleSvc := services.NewRoleService(dbase)
	authzSvc := NewService(dbase)

	if err := roleSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltins failed: %v", err)
	}
	if err := authzSvc.EnsureBuiltinGrants(ctx, tenantID); err != nil {
		t.Fatalf("EnsureBuiltinGrants failed: %v", err)
	}

	identityID := createTestIdentity(t, dbase, tenantID, "groupinherit")

	userRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "user"})
	if err != nil {
		t.Fatalf("GetRoleBySlug(user) failed: %v", err)
	}
	if err := roleSvc.Assign(ctx, identityID, userRole.ID, 0); err != nil {
		t.Fatalf("Assign user role failed: %v", err)
	}

	techRole, err := dbase.Queries.GetRoleBySlug(ctx, db.GetRoleBySlugParams{TenantID: tenantID, Slug: "technician"})
	if err != nil {
		t.Fatalf("GetRoleBySlug(technician) failed: %v", err)
	}

	// Custom role parented to technician: technician template has
	// identity.delete=no; this role overrides it to yes.
	custom := createCustomRole(t, dbase, tenantID, "senior-tech")
	setParentDirect(t, dbase, custom.ID, techRole.ID)
	if err := authzSvc.SaveRolePermissions(ctx, custom.ID, Grants{"identity.delete": "yes"}); err != nil {
		t.Fatalf("SaveRolePermissions (override) failed: %v", err)
	}

	group, err := dbase.Queries.CreateGroup(ctx, db.CreateGroupParams{
		TenantID: tenantID, Name: "Senior Techs", Slug: "senior-techs",
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if err := dbase.Queries.AddIdentityToGroup(ctx, db.AddIdentityToGroupParams{
		GroupID: group.ID, IdentityID: sql.NullInt64{Int64: identityID, Valid: true},
	}); err != nil {
		t.Fatalf("AddIdentityToGroup failed: %v", err)
	}
	if err := dbase.Queries.AssignRoleToGroup(ctx, db.AssignRoleToGroupParams{
		GroupID: group.ID, RoleID: custom.ID,
	}); err != nil {
		t.Fatalf("AssignRoleToGroup failed: %v", err)
	}

	grants, err := authzSvc.EffectiveGrants(ctx, identityID)
	if err != nil {
		t.Fatalf("EffectiveGrants failed: %v", err)
	}
	// user template alone has identity.create=no; technician (inherited
	// via the group role, since the custom role has no own override for
	// this key) has identity.create=yes.
	if !grants.Can("identity.create") {
		t.Errorf("EffectiveGrants identity.create = %v, want true (inherited from technician via group role)", grants.Can("identity.create"))
	}
	// The custom role's own override must win over technician's default.
	if got := grants["identity.delete"]; got != "yes" {
		t.Errorf("EffectiveGrants identity.delete = %q, want yes (child override via group role)", got)
	}

	if err := dbase.Queries.RemoveRoleFromGroup(ctx, db.RemoveRoleFromGroupParams{
		GroupID: group.ID, RoleID: custom.ID,
	}); err != nil {
		t.Fatalf("RemoveRoleFromGroup failed: %v", err)
	}

	grants, err = authzSvc.EffectiveGrants(ctx, identityID)
	if err != nil {
		t.Fatalf("EffectiveGrants (after removal) failed: %v", err)
	}
	if grants.Can("identity.create") {
		t.Error("EffectiveGrants identity.create should be false after removing the group role (only direct user role remains)")
	}
	if grants.Can("identity.delete") {
		t.Error("EffectiveGrants identity.delete should be false after removing the group role")
	}
}

func TestBuiltinSlugsOrder(t *testing.T) {
	got := permissions.BuiltinSlugs()
	want := []string{"super_admin", "admin", "technician", "user"}
	if len(got) != len(want) {
		t.Fatalf("BuiltinSlugs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("BuiltinSlugs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
