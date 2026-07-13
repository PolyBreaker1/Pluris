package params

// Canonical Parameter Paths (INV-CPP, see docs/endpoint-management/ui/invariants.md and
// docs/history/specs/2026-07-05-standardized-detail-pages-design.md §0).
//
// Every parameter mounted by a schema is addressable as
// "<entity>/<section>/<param>" — lowercase snake, forward slashes.
// Paths are DERIVED from the registry (never stored in parallel):
// entity = SubtypeSchema.PathEntity, section = SchemaSection.Key,
// param = ParamDef.Key. Shared ParamDefs (e.g. ram_mb on computer and
// server) get one path per mounting entity, all resolving to the same
// definition.
//
// The Schemas registry is immutable after package init: the path index
// is built lazily on first use and never rebuilt, so mutating Schemas
// afterwards would silently desynchronize it.

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type pathTarget struct {
	schema  *SubtypeSchema
	section *SchemaSection
	def     *ParamDef
}

var (
	pathIndexOnce sync.Once
	pathByEntity  map[string]map[string]string // entity -> param key -> path
	targetByPath  map[string]pathTarget
	entityBySlug  map[string]*SubtypeSchema
)

func buildPathIndex() {
	pathByEntity = map[string]map[string]string{}
	targetByPath = map[string]pathTarget{}
	entityBySlug = map[string]*SubtypeSchema{}
	for _, s := range Schemas {
		if s.PathEntity == "" {
			panic("params: schema " + s.Subtype + " has no PathEntity (INV-CPP)")
		}
		if _, dup := entityBySlug[s.PathEntity]; dup {
			panic("params: duplicate PathEntity " + s.PathEntity)
		}
		entityBySlug[s.PathEntity] = s
		pathByEntity[s.PathEntity] = map[string]string{}
		for i := range s.Sections {
			sec := &s.Sections[i]
			for _, key := range sec.Params {
				def := DefByKey(key)
				if def == nil {
					continue // registry tests already guard this
				}
				p := s.PathEntity + "/" + sec.Key + "/" + key
				if _, dup := targetByPath[p]; dup {
					panic("params: duplicate canonical path " + p)
				}
				if prev, mounted := pathByEntity[s.PathEntity][key]; mounted {
					panic("params: param " + key + " mounted twice for entity " + s.PathEntity + " (" + prev + " vs " + p + ")")
				}
				pathByEntity[s.PathEntity][key] = p
				targetByPath[p] = pathTarget{schema: s, section: sec, def: def}
			}
		}
	}
}

// PathFor returns the canonical path for a param mounted by the given
// entity ("user/identity/email"), or "" when the entity does not mount
// that key.
func PathFor(entity, key string) string {
	pathIndexOnce.Do(buildPathIndex)
	return pathByEntity[entity][key]
}

// SchemaByPathEntity returns the schema registered under a canonical
// entity slug ("user" -> SchemaIdentity), or nil.
func SchemaByPathEntity(entity string) *SubtypeSchema {
	pathIndexOnce.Do(buildPathIndex)
	return entityBySlug[entity]
}

// ResolvePath parses "entity/section/param" and fails closed on any
// unknown segment or malformed shape.
func ResolvePath(path string) (*SubtypeSchema, *SchemaSection, *ParamDef, error) {
	pathIndexOnce.Do(buildPathIndex)
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, nil, nil, fmt.Errorf("params: malformed canonical path %q (want entity/section/param)", path)
	}
	t, ok := targetByPath[path]
	if !ok {
		return nil, nil, nil, fmt.Errorf("params: unknown canonical path %q", path)
	}
	return t.schema, t.section, t.def, nil
}

// AllPaths returns every canonical path known to any registered Source
// (see sources.go), sorted. Today only the built-in entity source is
// registered, so behavior is identical to walking Schemas directly; future
// tenant/module sources are merged in here once they register.
func AllPaths() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(targetByPath))
	for _, s := range sources {
		for _, p := range s.Paths() {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

// entityAllPaths returns every canonical path registered by the built-in
// entity registry (computer/server/printer/desk/identity schemas). This is
// the pre-Source-abstraction body of AllPaths, preserved verbatim as the
// entitySource's Paths() implementation.
func entityAllPaths() []string {
	pathIndexOnce.Do(buildPathIndex)
	out := make([]string, 0, len(targetByPath))
	for p := range targetByPath {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// EffectivePermission returns the canonical "domain.action" permission key
// (see catalog/permissions) that gates visibility of the parameter mounted
// at the given canonical path ("entity/section/param").
//
// Resolution rule: the mounting ParamDef's own Permission if non-empty,
// else the owning schema's DefaultPermission, else "" (visible to any
// authenticated user). Unknown/malformed paths (including reserved-but-
// unimplemented namespaces like "tenant/..." and "module/...") return "".
//
// Entity paths (the built-in registry) resolve exactly as before via
// ResolvePath, including the schema-DefaultPermission fallback that only
// makes sense for schema-mounted params. Paths owned by any OTHER
// registered Source (see sources.go) fall through to that source's
// Resolve, using its def's own Permission with no schema-default
// fallback (non-entity sources have no SubtypeSchema to default from).
func EffectivePermission(path string) string {
	if schema, _, def, err := ResolvePath(path); err == nil {
		if def.Permission != "" {
			return def.Permission
		}
		return schema.DefaultPermission
	}
	for _, s := range sources {
		if _, isEntity := s.(entitySource); isEntity {
			continue // already tried above via ResolvePath
		}
		if def, ok := s.Resolve(path); ok {
			return def.Permission
		}
	}
	return ""
}

// ResolveDef resolves path via ANY registered Source, entity or
// otherwise — the asymmetry-closing counterpart to AllPaths() (see
// sources.go's Source.Resolve doc). Entity paths are tried first via
// ResolvePath (preserving its existing error surface for that common
// case); paths owned by other registered sources are tried next, in
// registration order.
func ResolveDef(path string) (ParamDef, bool) {
	if _, _, def, err := ResolvePath(path); err == nil {
		return *def, true
	}
	for _, s := range sources {
		if _, isEntity := s.(entitySource); isEntity {
			continue
		}
		if def, ok := s.Resolve(path); ok {
			return def, true
		}
	}
	return ParamDef{}, false
}
