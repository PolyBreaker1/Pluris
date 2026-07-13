package policymodules

// Dependency resolver — server-side closure computation for INV-M2/M3.
// The same algorithm runs on the agent (offline-verified) before
// applying a bundle, per ADR-007. Implementation is intentionally
// straightforward — readability over micro-optimisation. v1 ignores
// version constraints (the Dependency.VersionConstraint field is opaque
// to the mock); Phase 1 wires a real semver matcher.

// ResolveError — every failure mode the resolver can surface. The UI's
// CG binding save path turns these into user-facing messages.
type ResolveError struct {
	Code    string // "missing-dep" | "cycle" | "conflict"
	Message string
	// Path — module IDs forming the offending chain. For "missing-dep"
	// it's [requestor, missing]; for "cycle" the ring; for "conflict"
	// the two conflicting modules + the chain that brought them in.
	Path []string
}

func (e *ResolveError) Error() string { return e.Message }

// ResolutionPlan — what `Resolve` returns on success. The Order is the
// topological order in which the agent must apply modules; the server
// publishes them in this order over NATS.
type ResolutionPlan struct {
	// Order — module IDs in topological order (deps first). Each entry
	// resolves to ModuleVersion via Picks.
	Order []string
	// Picks — the chosen ModuleVersion per module ID. v1: always the
	// LatestVersion of the published module; Phase 1 adds version pinning.
	Picks map[string]string
	// AddedTransitively — module IDs that the user did NOT explicitly
	// pick but that came in via dependencies. The CG dialog surfaces
	// this as "this binding will also install N modules".
	AddedTransitively []string
}

// Resolve — compute the full closure for a set of explicitly-selected
// module IDs. Fails closed on cycles, missing deps, or active conflicts
// in the catalog. INV-M2 & INV-M3.
//
// `selected` is the set the user picked (typically one module per
// binding row in the CG dialog). The catalog parameter lets callers
// resolve against either the live catalog (see catalog.go's Catalog())
// or a tenant-scoped subset.
func Resolve(selected []string, catalog []Module) (*ResolutionPlan, *ResolveError) {
	// Index for O(1) lookup.
	byID := make(map[string]*Module, len(catalog))
	for i := range catalog {
		byID[catalog[i].ID] = &catalog[i]
	}

	plan := &ResolutionPlan{Picks: make(map[string]string)}
	explicit := make(map[string]struct{}, len(selected))
	for _, id := range selected {
		explicit[id] = struct{}{}
	}

	// DFS with three-state colouring detects cycles. Visit emits onto
	// `Order` post-order so `Order` ends up in topo order (deps first).
	const (
		white = 0
		grey  = 1
		black = 2
	)
	colour := make(map[string]int)
	var visit func(id string, stack []string) *ResolveError
	visit = func(id string, stack []string) *ResolveError {
		switch colour[id] {
		case grey:
			ring := append([]string(nil), stack...)
			ring = append(ring, id)
			return &ResolveError{Code: "cycle", Message: "module dependency cycle: " + chain(ring), Path: ring}
		case black:
			return nil
		}
		mod, ok := byID[id]
		if !ok {
			return &ResolveError{Code: "missing-dep", Message: "module not in catalog: " + id, Path: append(stack, id)}
		}
		ver := mod.LatestVersion()
		if ver == nil {
			return &ResolveError{Code: "missing-dep", Message: "module has no published version: " + id, Path: append(stack, id)}
		}
		colour[id] = grey
		stack = append(stack, id)
		for _, dep := range ver.Dependencies {
			if err := visit(dep.ModuleID, stack); err != nil {
				return err
			}
		}
		colour[id] = black
		plan.Order = append(plan.Order, id)
		plan.Picks[id] = ver.Version
		if _, isExplicit := explicit[id]; !isExplicit {
			plan.AddedTransitively = append(plan.AddedTransitively, id)
		}
		return nil
	}

	for _, id := range selected {
		if err := visit(id, nil); err != nil {
			return nil, err
		}
	}

	// Conflict pass — once the closure is built, ensure no two modules
	// in it are mutually exclusive. v1: any module in the closure
	// listed in another's `Conflicts` is a hard reject.
	inClosure := make(map[string]struct{}, len(plan.Order))
	for _, id := range plan.Order {
		inClosure[id] = struct{}{}
	}
	for _, id := range plan.Order {
		mod := byID[id]
		ver := mod.LatestVersion()
		for _, foe := range ver.Conflicts {
			if _, present := inClosure[foe]; present {
				return nil, &ResolveError{
					Code:    "conflict",
					Message: "modules conflict: " + id + " ↔ " + foe,
					Path:    []string{id, foe},
				}
			}
		}
	}

	return plan, nil
}

// chain — small helper that renders a [] of module IDs as "a → b → c".
// Used in error messages and (later) in the UI's path visualization.
func chain(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	out := ids[0]
	for _, id := range ids[1:] {
		out += " → " + id
	}
	return out
}
