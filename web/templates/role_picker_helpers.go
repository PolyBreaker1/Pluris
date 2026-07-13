package templates

import (
	"sort"
	"strings"

	"github.com/pluris/pluris/db"
)

// RolePickerOption is one flattened, display-ready entry for a
// hierarchical role <select> (Task 5's shared helper, reused by the
// create-role parent picker here and by Task 7's user/group role
// pickers). Family groups roles into <optgroup> buckets; Depth drives
// the indent prefix within a family.
type RolePickerOption struct {
	ID     int64
	Label  string
	Family string
	Depth  int
}

// maxRolePickerDepth caps the indent walk so a corrupt/cyclic
// ParentRoleID chain (shouldn't happen -- authz.Service rejects cycles
// on write) can never loop forever when rendering.
const maxRolePickerDepth = 5

// rolePickerOptions groups roles by template family and computes each
// role's indent depth by walking ParentRoleID within the passed slice.
// Builtin roles are their own family root (family = the role's own
// slug); custom roles belong to their TemplateSlug family, falling back
// to "custom" when TemplateSlug is unset (e.g. a parentless custom role
// created standalone, not via clone). Options are returned grouped by
// family (families in first-seen order) and depth-then-name ordered
// within a family so indentation reads as a tree.
func rolePickerOptions(roles []db.Role) []RolePickerOption {
	byID := make(map[int64]db.Role, len(roles))
	for _, r := range roles {
		byID[r.ID] = r
	}

	depthOf := func(r db.Role) int {
		depth := 0
		current := r
		for depth < maxRolePickerDepth {
			if !current.ParentRoleID.Valid {
				break
			}
			parent, ok := byID[current.ParentRoleID.Int64]
			if !ok {
				break
			}
			depth++
			current = parent
		}
		return depth
	}

	familyOf := func(r db.Role) string {
		if r.IsBuiltin {
			return r.Slug
		}
		if r.TemplateSlug.Valid && r.TemplateSlug.String != "" {
			return r.TemplateSlug.String
		}
		return "custom"
	}

	familyOrder := make([]string, 0)
	seenFamily := make(map[string]bool)
	byFamily := make(map[string][]db.Role)
	for _, r := range roles {
		f := familyOf(r)
		if !seenFamily[f] {
			seenFamily[f] = true
			familyOrder = append(familyOrder, f)
		}
		byFamily[f] = append(byFamily[f], r)
	}

	out := make([]RolePickerOption, 0, len(roles))
	for _, family := range familyOrder {
		members := byFamily[family]
		sort.SliceStable(members, func(i, j int) bool {
			di, dj := depthOf(members[i]), depthOf(members[j])
			if di != dj {
				return di < dj
			}
			return members[i].Name < members[j].Name
		})
		for _, r := range members {
			depth := depthOf(r)
			out = append(out, RolePickerOption{
				ID:     r.ID,
				Label:  strings.Repeat("— ", depth) + r.Name,
				Family: family,
				Depth:  depth,
			})
		}
	}
	return out
}

// rolePickerGroup is one <optgroup>'s worth of options, in family order.
type rolePickerGroup struct {
	Family  string
	Options []RolePickerOption
}

// groupRolePickerOptions buckets an already family-ordered option slice
// (as produced by rolePickerOptions) into per-family groups for
// RolePickerSelect's <optgroup> rendering.
func groupRolePickerOptions(opts []RolePickerOption) []rolePickerGroup {
	groups := make([]rolePickerGroup, 0)
	index := make(map[string]int)
	for _, o := range opts {
		i, ok := index[o.Family]
		if !ok {
			i = len(groups)
			index[o.Family] = i
			groups = append(groups, rolePickerGroup{Family: o.Family})
		}
		groups[i].Options = append(groups[i].Options, o)
	}
	return groups
}
