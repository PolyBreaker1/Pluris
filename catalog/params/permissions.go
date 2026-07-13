package params

// This file makes the parameter registry permission-aware: given a
// predicate that answers "can the caller see this permission key at all",
// it filters ParamDefs/paths down to what should be visible.
//
// The predicate is a plain func(string) bool rather than a concrete grants
// type so catalog/params stays decoupled from pkg/authz. Callers using
// pkg/authz.Grants pass `grants.Can` — Can treats "yes"/"own"/"all" as
// visible (any non-deny scope grants at least some access). That is
// intentional here: this filter governs DEFINITION visibility (should the
// parameter appear in the tree/API/filter builder at all), not row-level
// scope enforcement (which rows of data the caller may see) — scope
// enforcement for "own" grants happens separately, at the data layer, via
// grants.CanScoped.

// FilterByGrants returns the subset of defs visible under the has
// predicate. A ParamDef with an empty Permission field is always visible
// (no permission required); otherwise it is visible only when
// has(def.Permission) is true.
//
// FilterByGrants does not resolve schema-level DefaultPermission itself —
// it is a pure, schema-agnostic filter over whatever Permission each
// ParamDef already carries. Callers that need the schema default resolved
// (i.e. most callers) should pass ParamDefs whose Permission has already
// been set to EffectivePermission(path) for the path each def was
// resolved from, or use VisiblePaths, which does that resolution for you.
func FilterByGrants(has func(permKey string) bool, defs []ParamDef) []ParamDef {
	out := make([]ParamDef, 0, len(defs))
	for _, d := range defs {
		if d.Permission == "" || has(d.Permission) {
			out = append(out, d)
		}
	}
	return out
}

// MountedDef is a ParamDef resolved at one canonical mount point.
// Def is a COPY of the registered definition with Permission stamped to
// EffectivePermission(Path) — the schema default already resolved — so a
// MountedDef is always safe to permission-filter directly. This is the
// safe counterpart to filtering raw schema defs with FilterByGrants
// (which would see every def's empty Permission field and treat the whole
// registry as visible).
type MountedDef struct {
	// Path is the canonical "entity/section/param" mount path.
	Path string

	// SectionKey/SectionLabel identify the mounting SchemaSection, so
	// consumers (e.g. the /api/params tree) can group without re-walking
	// the schema.
	SectionKey   string
	SectionLabel string

	// Def is a copy of the mounted ParamDef with Def.Permission set to
	// EffectivePermission(Path). Never empty for schemas that declare a
	// DefaultPermission.
	Def ParamDef
}

// EffectiveDefs returns every parameter this schema mounts, in section
// order then param order (deterministic), each with its Permission
// stamped to the effective permission of its canonical path.
func (s *SubtypeSchema) EffectiveDefs() []MountedDef {
	out := make([]MountedDef, 0, 32)
	for i := range s.Sections {
		sec := &s.Sections[i]
		for _, key := range sec.Params {
			def := DefByKey(key)
			if def == nil {
				continue // registry tests already guard this
			}
			path := s.PathEntity + "/" + sec.Key + "/" + key
			d := *def
			d.Permission = EffectivePermission(path)
			out = append(out, MountedDef{
				Path:         path,
				SectionKey:   sec.Key,
				SectionLabel: sec.Label,
				Def:          d,
			})
		}
	}
	return out
}

// VisibleDefs returns EffectiveDefs filtered to what the has predicate may
// see — the same visibility rule as FilterByGrants (empty effective
// permission is always visible), applied to defs whose schema default has
// already been resolved. This is the ONLY entry point permission-filtered
// consumers of a schema's defs (the /api/params feed, the module editor
// tree, the condition builder) should use.
func (s *SubtypeSchema) VisibleDefs(has func(permKey string) bool) []MountedDef {
	all := s.EffectiveDefs()
	out := make([]MountedDef, 0, len(all))
	for _, md := range all {
		if md.Def.Permission == "" || has(md.Def.Permission) {
			out = append(out, md)
		}
	}
	return out
}

// VisiblePaths returns every canonical path from AllPaths() whose
// EffectivePermission is visible under the has predicate (empty effective
// permission is always visible).
func VisiblePaths(has func(permKey string) bool) []string {
	all := AllPaths()
	out := make([]string, 0, len(all))
	for _, p := range all {
		perm := EffectivePermission(p)
		if perm == "" || has(perm) {
			out = append(out, p)
		}
	}
	return out
}
