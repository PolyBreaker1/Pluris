package params

// Canonical Parameter Paths (INV-CPP, see docs/UX_INVARIANTS.md and
// docs/superpowers/specs/2026-07-05-standardized-detail-pages-design.md §0).
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

// AllPaths returns every registered canonical path, sorted.
func AllPaths() []string {
	pathIndexOnce.Do(buildPathIndex)
	out := make([]string, 0, len(targetByPath))
	for p := range targetByPath {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
