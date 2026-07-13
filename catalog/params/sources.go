package params

// This file introduces the SOURCE abstraction: the concept that canonical
// parameter paths (INV-CPP, see paths.go) can come from more than one
// place. Today there is exactly one source — the built-in entity registry
// (computer/server/printer/desk/identity schemas) — but the module editor
// parameter tree and dependency-condition builder (later tasks) need to
// blend in parameters from other namespaces without every consumer of
// AllPaths()/ResolvePath() having to know about it.
//
// Namespace convention: a canonical path's first "/"-delimited segment is
// its namespace. Built-in entity paths have no distinct namespace prefix —
// the first segment IS the PathEntity ("computer", "user", ...). Two
// prefixes are reserved for future sources and MUST NOT be used as a
// PathEntity or otherwise collide:
//
//   - "tenant/..." — tenant-scoped custom parameters. Contents (and a real
//     Source implementation) are out of scope for this task.
//   - "module/..." — per-module-instance dynamic inputs, resolved at
//     policy-assignment time rather than from a static registry (Task
//     4.4). Contents are out of scope for this task.
//
// Until a real source registers for "tenant" or "module", paths under
// those prefixes simply match no Source and no PathEntity, so
// ResolvePath("module/input/x") fails closed with a plain "not found"
// error — exactly like any other unknown path — and never panics.
type Source interface {
	// Paths returns every canonical path this source currently knows
	// about. AllPaths() merges the Paths() of every registered source.
	Paths() []string

	// Resolve returns the ParamDef mounted at path by this source, if
	// this source owns that path. This closes the historic asymmetry
	// between Paths() (aggregate enumeration, via AllPaths) and
	// ResolvePath (single-path lookup that only ever consulted the
	// built-in entity registry): a path a Source contributed to
	// AllPaths() could not, before this method existed, be resolved
	// back to a definition through any Source-generic entry point.
	// ResolveDef (paths.go) is the asymmetry-closing entry point that
	// walks every registered source calling Resolve; entitySource's
	// implementation adapts trivially onto the existing ResolvePath.
	Resolve(path string) (ParamDef, bool)
}

// entitySource is the built-in source backing every existing
// computer/server/printer/desk/identity path. Its Paths() is the exact
// pre-Source-abstraction body of AllPaths (see entityAllPaths in
// paths.go) — introducing Source changes nothing about entity path
// resolution.
type entitySource struct{}

func (entitySource) Paths() []string { return entityAllPaths() }

// Resolve adapts the existing entity-specific ResolvePath onto the
// Source-generic contract: same lookup, discarding the schema/section
// context ResolveDef callers don't need.
func (entitySource) Resolve(path string) (ParamDef, bool) {
	_, _, def, err := ResolvePath(path)
	if err != nil {
		return ParamDef{}, false
	}
	return *def, true
}

// sources is the single registration point for parameter namespaces.
// RegisterSource appends to it; AllPaths() iterates it. The built-in
// entity source is registered unconditionally below. Future tasks
// register a "tenant" source and a "module" source here (or via
// RegisterSource from their own package init()).
var sources = []Source{entitySource{}}

// RegisterSource adds a Source to the registry so its paths are included
// by AllPaths(). Intended for future namespace packages (tenant, module)
// to call from their own init(), before any params.AllPaths() consumer
// runs. Not used by the built-in entity source, which is registered
// directly in `sources` above.
func RegisterSource(s Source) {
	sources = append(sources, s)
}
