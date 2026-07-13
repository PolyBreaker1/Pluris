package authz

import "testing"

// ptr is a small helper for constructing *int64 owner IDs inline.
func ptr(v int64) *int64 { return &v }

// TestModuleAccessMatrix is the exhaustive decision-matrix test required
// for this security-critical authz surface: every combination of
// bypass/manage_modules/owner/explicit-grant x bundled/tenant module x
// view/edit/admin, plus the level hierarchy and stranger-denied cases.
func TestModuleAccessMatrix(t *testing.T) {
	const (
		me      int64 = 100
		other   int64 = 200
		myGroup int64 = 10
		myRole  int64 = 20
	)

	cases := []struct {
		name      string
		in        ModuleAccessInput
		wantView  bool
		wantEdit  bool
		wantAdmin bool
	}{
		{
			name: "super_admin bypass grants everything on a tenant module",
			in: ModuleAccessInput{
				Grants:     Grants{BypassKey: "yes"},
				IdentityID: me,
				OwnerID:    ptr(other),
			},
			wantView: true, wantEdit: true, wantAdmin: true,
		},
		{
			name: "super_admin bypass grants everything even on a bundled module",
			in: ModuleAccessInput{
				Grants:     Grants{BypassKey: "yes"},
				IdentityID: me,
				IsBundled:  true,
			},
			wantView: true, wantEdit: true, wantAdmin: true,
		},
		{
			name: "manage_modules grants view+edit+admin on a tenant module owned by someone else",
			in: ModuleAccessInput{
				Grants:     Grants{"endpoint_policy.manage_modules": "yes"},
				IdentityID: me,
				OwnerID:    ptr(other),
			},
			wantView: true, wantEdit: true, wantAdmin: true,
		},
		{
			name: "manage_modules on a bundled module is view-only -- bundled is never editable",
			in: ModuleAccessInput{
				Grants:     Grants{"endpoint_policy.manage_modules": "yes"},
				IdentityID: me,
				IsBundled:  true,
			},
			wantView: true, wantEdit: false, wantAdmin: false,
		},
		{
			name: "owner has full access to own module",
			in: ModuleAccessInput{
				Grants:     Grants{},
				IdentityID: me,
				OwnerID:    ptr(me),
			},
			wantView: true, wantEdit: true, wantAdmin: true,
		},
		{
			name: "non-owner with no grants gets nothing on a tenant module",
			in: ModuleAccessInput{
				Grants:     Grants{},
				IdentityID: me,
				OwnerID:    ptr(other),
			},
			wantView: false, wantEdit: false, wantAdmin: false,
		},
		{
			name: "stranger denied on a bundled module with no view key",
			in: ModuleAccessInput{
				Grants:     Grants{},
				IdentityID: me,
				IsBundled:  true,
			},
			wantView: false, wantEdit: false, wantAdmin: false,
		},
		{
			name: "endpoint_policy.view grants view of a bundled module only",
			in: ModuleAccessInput{
				Grants:     Grants{"endpoint_policy.view": "yes"},
				IdentityID: me,
				IsBundled:  true,
			},
			wantView: true, wantEdit: false, wantAdmin: false,
		},
		{
			name: "endpoint_policy.view does NOT grant view of a tenant module (default-deny)",
			in: ModuleAccessInput{
				Grants:     Grants{"endpoint_policy.view": "yes"},
				IdentityID: me,
				OwnerID:    ptr(other),
			},
			wantView: false, wantEdit: false, wantAdmin: false,
		},
		{
			name: "explicit identity grant at view level",
			in: ModuleAccessInput{
				Grants:       Grants{},
				IdentityID:   me,
				OwnerID:      ptr(other),
				ModuleGrants: []ModuleGrant{{SubjectType: "identity", SubjectID: me, Level: "view"}},
			},
			wantView: true, wantEdit: false, wantAdmin: false,
		},
		{
			name: "explicit identity grant at edit level implies view",
			in: ModuleAccessInput{
				Grants:       Grants{},
				IdentityID:   me,
				OwnerID:      ptr(other),
				ModuleGrants: []ModuleGrant{{SubjectType: "identity", SubjectID: me, Level: "edit"}},
			},
			wantView: true, wantEdit: true, wantAdmin: false,
		},
		{
			name: "explicit identity grant at admin level implies view+edit",
			in: ModuleAccessInput{
				Grants:       Grants{},
				IdentityID:   me,
				OwnerID:      ptr(other),
				ModuleGrants: []ModuleGrant{{SubjectType: "identity", SubjectID: me, Level: "admin"}},
			},
			wantView: true, wantEdit: true, wantAdmin: true,
		},
		{
			name: "explicit group grant at edit level via GroupIDs membership",
			in: ModuleAccessInput{
				Grants:       Grants{},
				IdentityID:   me,
				OwnerID:      ptr(other),
				GroupIDs:     []int64{myGroup},
				ModuleGrants: []ModuleGrant{{SubjectType: "group", SubjectID: myGroup, Level: "edit"}},
			},
			wantView: true, wantEdit: true, wantAdmin: false,
		},
		{
			name: "group grant does not apply when identity is not in that group",
			in: ModuleAccessInput{
				Grants:       Grants{},
				IdentityID:   me,
				OwnerID:      ptr(other),
				GroupIDs:     []int64{myGroup},
				ModuleGrants: []ModuleGrant{{SubjectType: "group", SubjectID: 999, Level: "admin"}},
			},
			wantView: false, wantEdit: false, wantAdmin: false,
		},
		{
			name: "explicit role grant at admin level via RoleIDs membership",
			in: ModuleAccessInput{
				Grants:       Grants{},
				IdentityID:   me,
				OwnerID:      ptr(other),
				RoleIDs:      []int64{myRole},
				ModuleGrants: []ModuleGrant{{SubjectType: "role", SubjectID: myRole, Level: "admin"}},
			},
			wantView: true, wantEdit: true, wantAdmin: true,
		},
		{
			name: "role grant does not apply when identity does not hold that role",
			in: ModuleAccessInput{
				Grants:       Grants{},
				IdentityID:   me,
				OwnerID:      ptr(other),
				RoleIDs:      []int64{myRole},
				ModuleGrants: []ModuleGrant{{SubjectType: "role", SubjectID: 888, Level: "view"}},
			},
			wantView: false, wantEdit: false, wantAdmin: false,
		},
		{
			name: "explicit grants on a bundled module never unlock edit/admin",
			in: ModuleAccessInput{
				Grants:       Grants{},
				IdentityID:   me,
				IsBundled:    true,
				ModuleGrants: []ModuleGrant{{SubjectType: "identity", SubjectID: me, Level: "admin"}},
			},
			wantView: true, wantEdit: false, wantAdmin: false,
		},
		{
			name: "unowned, non-bundled module (nil OwnerID) with no grants denies view",
			in: ModuleAccessInput{
				Grants:     Grants{},
				IdentityID: me,
				OwnerID:    nil,
			},
			wantView: false, wantEdit: false, wantAdmin: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModuleCanView(c.in); got != c.wantView {
				t.Errorf("ModuleCanView = %v, want %v", got, c.wantView)
			}
			if got := ModuleCanEdit(c.in); got != c.wantEdit {
				t.Errorf("ModuleCanEdit = %v, want %v", got, c.wantEdit)
			}
			if got := ModuleCanAdmin(c.in); got != c.wantAdmin {
				t.Errorf("ModuleCanAdmin = %v, want %v", got, c.wantAdmin)
			}
		})
	}
}
