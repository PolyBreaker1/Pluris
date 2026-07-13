package authz

// Permission registry keys consumed by the module access decisions
// below. Kept as named constants (rather than repeating the string
// literals) so a registry rename is a one-place edit; see
// catalog/permissions/registry.go for the canonical source (endpoint_policy
// domain) and pkg/auth/rbac.go for the existing route->key mapping these
// mirror.
const (
	// ActionManageModules is the "manage everything about policy
	// modules" grant: view+edit+admin on tenant-owned modules, and
	// view (never edit/admin -- see the matrix below) on bundled ones.
	ActionManageModules = "endpoint_policy.manage_modules"
	// ActionViewPolicy is the general "can see endpoint policy content"
	// grant. For MODULE access specifically it only unlocks viewing
	// bundled modules (see matrix below); it never grants view of a
	// tenant-authored module by itself, since tenant modules may embed
	// sensitive logic and default-deny applies.
	ActionViewPolicy = "endpoint_policy.view"
)

// moduleLevelRank orders the three module_grants levels so a higher
// level implies every lower one (admin ⊇ edit ⊇ view), mirroring the
// scope ranking in grants.go's rank map.
var moduleLevelRank = map[string]int{
	"view":  1,
	"edit":  2,
	"admin": 3,
}

// ModuleGrant is one row of the module_grants table (db.ModuleGrant),
// reduced to the fields the decision helpers need. subject_type is one
// of "identity" | "group" | "role"; level is one of "view" | "edit" |
// "admin".
type ModuleGrant struct {
	SubjectType string
	SubjectID   int64
	Level       string
}

// ModuleAccessInput bundles everything ModuleCanView/Edit/Admin need to
// decide MODULE-level access for one identity against one module. It is
// caller-supplied data only -- these helpers never touch the database,
// so callers (the future module service/handlers) are responsible for
// loading Grants (authz.Service.EffectiveGrants), GroupIDs/RoleIDs
// (the session's group and role memberships), and ModuleGrants (
// db.Queries.ListGrantsForModule) up front.
//
// This governs module-level access only. Filtering the parameter tree
// shown in the module editor for a session's grants is a separate
// concern handled by catalog/params' FilterByGrants/VisibleDefs.
type ModuleAccessInput struct {
	Grants       Grants        // session's effective Pluris Policy grants
	IdentityID   int64         // session identity
	GroupIDs     []int64       // session's group memberships (caller supplies)
	RoleIDs      []int64       // session's role ids (caller supplies)
	OwnerID      *int64        // module's owner_identity_id; nil = bundled/unowned
	IsBundled    bool          // true for policy_modules.is_bundled rows
	ModuleGrants []ModuleGrant // this module's module_grants rows
}

// Decision matrix (default-deny: anything not listed below is false).
// Columns are the three module_grants levels; "own module" means
// OwnerID != nil && *OwnerID == IdentityID.
//
//	condition                                    | view | edit | admin
//	----------------------------------------------+------+------+------
//	super_admin bypass (Grants.bypass())          | yes  | yes  | yes    (bundled included)
//	endpoint_policy.manage_modules, tenant module | yes  | yes  | yes
//	endpoint_policy.manage_modules, bundled module| yes  | no   | no     (clone instead -- never editable)
//	owner (own module)                            | yes  | yes  | yes    (never true for bundled: OwnerID is nil)
//	explicit module_grants row, level >= view     | yes  | no   | no     (unless module is bundled: see below)
//	explicit module_grants row, level >= edit     | yes  | yes  | no     (bundled: capped at view)
//	explicit module_grants row, level == admin    | yes  | yes  | yes    (bundled: capped at view)
//	endpoint_policy.view, bundled module ONLY     | yes  | no   | no
//	endpoint_policy.view, tenant module           | no   | no   | no     (default-deny: tenant modules may hold sensitive logic)
//	stranger (none of the above)                  | no   | no   | no
//
// Bundled modules are never editable/admin-able through manage_modules,
// ownership, or explicit module_grants -- only the super_admin bypass
// can edit a bundled module (used for emergency/break-glass operations,
// never the normal path). The intended path for changing bundled
// behavior is cloning it into a tenant-owned module, not editing it in
// place.
func ModuleCanView(in ModuleAccessInput) bool {
	if in.Grants.bypass() {
		return true
	}
	if in.Grants.Can(ActionManageModules) {
		return true
	}
	if in.isOwner() {
		return true
	}
	if in.hasExplicitLevel("view") {
		return true
	}
	if in.IsBundled && in.Grants.Can(ActionViewPolicy) {
		return true
	}
	return false
}

// ModuleCanEdit reports whether the session may edit the module's
// content (manifest, scripts, parameters). Bundled modules are never
// editable except via the super_admin bypass -- see the matrix above.
func ModuleCanEdit(in ModuleAccessInput) bool {
	if in.Grants.bypass() {
		return true
	}
	if in.IsBundled {
		return false
	}
	if in.Grants.Can(ActionManageModules) {
		return true
	}
	if in.isOwner() {
		return true
	}
	return in.hasExplicitLevel("edit")
}

// ModuleCanAdmin reports whether the session may manage the module's
// own grants (module_grants rows) or delete it. Bundled modules are
// never admin-able except via the super_admin bypass -- see the matrix
// above.
func ModuleCanAdmin(in ModuleAccessInput) bool {
	if in.Grants.bypass() {
		return true
	}
	if in.IsBundled {
		return false
	}
	if in.Grants.Can(ActionManageModules) {
		return true
	}
	if in.isOwner() {
		return true
	}
	return in.hasExplicitLevel("admin")
}

// isOwner reports whether the session identity owns the module. Always
// false for bundled modules, since OwnerID is nil for those by
// construction (migration 007).
func (in ModuleAccessInput) isOwner() bool {
	return in.OwnerID != nil && *in.OwnerID == in.IdentityID
}

// hasExplicitLevel reports whether any of the module's module_grants
// rows names this session as a subject (directly by identity, or via
// GroupIDs/RoleIDs membership) at level >= required, using the
// hierarchical ranking (admin ⊇ edit ⊇ view).
func (in ModuleAccessInput) hasExplicitLevel(required string) bool {
	need := moduleLevelRank[required]
	for _, g := range in.ModuleGrants {
		if moduleLevelRank[g.Level] < need {
			continue
		}
		switch g.SubjectType {
		case "identity":
			if g.SubjectID == in.IdentityID {
				return true
			}
		case "group":
			if containsID(in.GroupIDs, g.SubjectID) {
				return true
			}
		case "role":
			if containsID(in.RoleIDs, g.SubjectID) {
				return true
			}
		}
	}
	return false
}

// containsID reports whether id is present in ids.
func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
