// Package authz resolves Pluris Policy grants: pure combinators over the
// Grants map (this file) plus a DB-backed Service (service.go) that reads
// and writes the roles.permissions JSON column. See
// docs/history/specs/2026-07-08-pluris-policy-authz-design.md
// section "2. Storage & resolution".
package authz

import (
	"encoding/json"
)

// Grants maps a canonical "domain.action" key to its stored value:
// "none"|"own"|"all" for scoped actions, "no"|"yes" for unscoped actions.
// A missing key means deny.
type Grants map[string]string

// BypassKey is a marker entry the auth middleware sets for super_admin
// sessions ({BypassKey: "yes"}); when present, every Can/CanScoped call
// returns true regardless of the requested key.
const BypassKey = "__super_admin__"

// rank orders both scope values and yes/no values on a single scale so
// Union can compare them with one lookup: all > own > none, yes > no.
// Unknown values (defensive) rank lowest.
var rank = map[string]int{
	"all":  4,
	"yes":  3,
	"own":  2,
	"no":   1,
	"none": 0,
}

// Can reports whether the grant for key permits unscoped access: "yes",
// "own", or "all" all count as allowed (any non-deny scope grants at least
// some access; callers that need scope-aware checks should use CanScoped).
func (g Grants) Can(key string) bool {
	if g.bypass() {
		return true
	}
	switch g[key] {
	case "yes", "own", "all":
		return true
	default:
		return false
	}
}

// CanScoped reports whether the grant for key permits access to a
// resource owned by ownerID, from the perspective of identity selfID.
// "all" always permits; "own" permits only when ownerID == selfID;
// anything else (including missing keys) denies.
func (g Grants) CanScoped(key string, ownerID, selfID int64) bool {
	if g.bypass() {
		return true
	}
	switch g[key] {
	case "all":
		return true
	case "own":
		return ownerID == selfID
	default:
		return false
	}
}

// ScopeOf returns the raw stored value for key, or "" when the key is
// absent. Used by menus/UI that need the scope value itself, not just a
// boolean.
func (g Grants) ScopeOf(key string) string {
	return g[key]
}

// bypass reports whether the super_admin bypass marker is present.
func (g Grants) bypass() bool {
	switch g[BypassKey] {
	case "yes", "all", "own":
		return true
	default:
		return false
	}
}

// Union merges any number of Grants, keeping for each key the
// highest-ranked value across all inputs (all > own > none, yes > no).
func Union(gs ...Grants) Grants {
	out := make(Grants)
	for _, g := range gs {
		for key, value := range g {
			existing, ok := out[key]
			if !ok || rank[value] > rank[existing] {
				out[key] = value
			}
		}
	}
	return out
}

// Parse decodes a roles.permissions JSON string into Grants. Invalid JSON
// yields an empty (deny-all) map rather than an error, since a corrupt
// stored value should never crash a request -- it should just deny.
// Unknown keys (actions removed from the registry, or forward-compat
// additions) are kept as-is -- Parse does not filter against the
// permissions registry.
func Parse(permissionsJSON string) Grants {
	out := make(Grants)
	var raw map[string]string
	if err := json.Unmarshal([]byte(permissionsJSON), &raw); err != nil {
		return out
	}
	for k, v := range raw {
		out[k] = v
	}
	return out
}
