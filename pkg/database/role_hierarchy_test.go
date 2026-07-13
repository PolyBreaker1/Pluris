package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/db"
)

// TestRoleHierarchyAndGroupRoles covers migration 005 on a fresh database:
// role parent inheritance (UpdateRoleParent, ListRoleChildren) and
// group-role assignment (AssignRoleToGroup, RemoveRoleFromGroup,
// ListRolesForGroup, ListGroupsForRole, ListGroupRolesForIdentity).
func TestRoleHierarchyAndGroupRoles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "role_hierarchy.db")
	ctx := context.Background()

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	tenant, err := database.Queries.CreateTenant(ctx, db.CreateTenantParams{
		Name: "Hierarchy Org", Slug: "hierarchy-org",
	})
	if err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	parentRole, err := database.Queries.CreateRole(ctx, db.CreateRoleParams{
		TenantID:  tenant.ID,
		Slug:      "helpdesk-l1",
		Name:      "Helpdesk L1",
		IsBuiltin: false,
	})
	if err != nil {
		t.Fatalf("CreateRole (parent) failed: %v", err)
	}

	childRole, err := database.Queries.CreateRole(ctx, db.CreateRoleParams{
		TenantID:  tenant.ID,
		Slug:      "helpdesk-l2",
		Name:      "Helpdesk L2",
		IsBuiltin: false,
	})
	if err != nil {
		t.Fatalf("CreateRole (child) failed: %v", err)
	}

	group, err := database.Queries.CreateGroup(ctx, db.CreateGroupParams{
		TenantID: tenant.ID,
		Name:     "Support Team",
		Slug:     "support-team",
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	identity, err := database.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID: tenant.ID, Username: "member1", Email: "member1@example.com",
		DisplayName: "Member One", Role: "user",
	})
	if err != nil {
		t.Fatalf("CreateIdentity failed: %v", err)
	}

	if err := database.Queries.AddIdentityToGroup(ctx, db.AddIdentityToGroupParams{
		GroupID:    group.ID,
		IdentityID: sql.NullInt64{Int64: identity.ID, Valid: true},
	}); err != nil {
		t.Fatalf("AddIdentityToGroup failed: %v", err)
	}

	// --- Role parent inheritance ---
	if err := database.Queries.UpdateRoleParent(ctx, db.UpdateRoleParentParams{
		ID:           childRole.ID,
		ParentRoleID: sql.NullInt64{Int64: parentRole.ID, Valid: true},
	}); err != nil {
		t.Fatalf("UpdateRoleParent failed: %v", err)
	}

	gotChild, err := database.Queries.GetRole(ctx, childRole.ID)
	if err != nil {
		t.Fatalf("GetRole failed: %v", err)
	}
	if !gotChild.ParentRoleID.Valid || gotChild.ParentRoleID.Int64 != parentRole.ID {
		t.Fatalf("expected ParentRoleID=%d, got %+v", parentRole.ID, gotChild.ParentRoleID)
	}

	children, err := database.Queries.ListRoleChildren(ctx, sql.NullInt64{Int64: parentRole.ID, Valid: true})
	if err != nil {
		t.Fatalf("ListRoleChildren failed: %v", err)
	}
	if len(children) != 1 || children[0].ID != childRole.ID {
		t.Fatalf("expected exactly childRole as a child of parentRole, got %+v", children)
	}

	// --- Group role assignment ---
	if err := database.Queries.AssignRoleToGroup(ctx, db.AssignRoleToGroupParams{
		GroupID: group.ID,
		RoleID:  parentRole.ID,
	}); err != nil {
		t.Fatalf("AssignRoleToGroup failed: %v", err)
	}
	// INSERT OR IGNORE: re-assigning must not error.
	if err := database.Queries.AssignRoleToGroup(ctx, db.AssignRoleToGroupParams{
		GroupID: group.ID,
		RoleID:  parentRole.ID,
	}); err != nil {
		t.Fatalf("re-assigning the same group role should be a no-op, got: %v", err)
	}

	rolesForGroup, err := database.Queries.ListRolesForGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("ListRolesForGroup failed: %v", err)
	}
	if len(rolesForGroup) != 1 || rolesForGroup[0].ID != parentRole.ID {
		t.Fatalf("expected exactly parentRole assigned to group, got %+v", rolesForGroup)
	}

	groupsForRole, err := database.Queries.ListGroupsForRole(ctx, parentRole.ID)
	if err != nil {
		t.Fatalf("ListGroupsForRole failed: %v", err)
	}
	if len(groupsForRole) != 1 || groupsForRole[0].ID != group.ID {
		t.Fatalf("expected exactly the support group holding parentRole, got %+v", groupsForRole)
	}

	// Identity is a member of the group, so it inherits the group's role.
	groupRolesForIdentity, err := database.Queries.ListGroupRolesForIdentity(ctx, sql.NullInt64{Int64: identity.ID, Valid: true})
	if err != nil {
		t.Fatalf("ListGroupRolesForIdentity failed: %v", err)
	}
	if len(groupRolesForIdentity) != 1 || groupRolesForIdentity[0].ID != parentRole.ID {
		t.Fatalf("expected identity to inherit parentRole via group membership, got %+v", groupRolesForIdentity)
	}

	// --- Removal ---
	if err := database.Queries.RemoveRoleFromGroup(ctx, db.RemoveRoleFromGroupParams{
		GroupID: group.ID,
		RoleID:  parentRole.ID,
	}); err != nil {
		t.Fatalf("RemoveRoleFromGroup failed: %v", err)
	}

	rolesForGroupAfter, err := database.Queries.ListRolesForGroup(ctx, group.ID)
	if err != nil {
		t.Fatalf("ListRolesForGroup (after removal) failed: %v", err)
	}
	if len(rolesForGroupAfter) != 0 {
		t.Fatalf("expected no roles assigned to group after removal, got %+v", rolesForGroupAfter)
	}
}
