package services

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// AssignmentService resolves which policies effectively apply to a
// target (Task 12). Resolution walks every assignment scope the target
// participates in: the target itself, each of its groups, its site
// (assets only) and the tenant, then dedupes by binding so one policy
// bound in one configuration group shows once even when assigned at
// several scopes.
type AssignmentService struct {
	db *database.Database
}

// NewAssignmentService creates a new assignment service.
func NewAssignmentService(db *database.Database) *AssignmentService {
	return &AssignmentService{db: db}
}

// AppliedPolicy is one row on an Applied Policies tab.
type AppliedPolicy struct {
	BindingID    int64
	PolicyID     string
	PolicyName   string
	SourceGroup  string
	Scope        string
	ValueSummary string
	Status       string
}

// policyNameByID resolves a catalog policy's display name; unknown IDs
// fall back to the raw ID so stale bindings never error.
func policyNameByID(id string) string {
	for _, p := range policies.Catalog() {
		if p.ID == id {
			return p.Name
		}
	}
	return id
}

// valueSummary renders a binding's parameter_values JSON as "k=v" pairs,
// keys upgraded to canonical parameter paths when the registry
// recognizes them (INV-CPP).
func valueSummary(raw string) string {
	if raw == "" || raw == "{}" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		key := k
		if _, _, _, err := params.ResolvePath(k); err == nil {
			key = k // already a canonical path
		}
		pairs = append(pairs, key+"="+jsonValueString(v))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ", ")
}

func jsonValueString(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.Trim(string(b), `"`)
}

// scopeTarget is one (target_type, target_id) pair resolution walks.
type scopeTarget struct {
	Type string
	ID   int64
}

// ResolveForTarget returns the effective policies for a target.
// targetType is "asset" or "identity"; groupIDs are the target's group
// memberships; siteID is 0 when the target has no site.
func (s *AssignmentService) ResolveForTarget(ctx context.Context, tenantID int64, targetType string, targetID int64, groupIDs []int64, siteID int64) ([]AppliedPolicy, error) {
	scopes := []scopeTarget{{targetType, targetID}}
	for _, g := range groupIDs {
		scopes = append(scopes, scopeTarget{"group", g})
	}
	if siteID != 0 {
		scopes = append(scopes, scopeTarget{"site", siteID})
	}
	scopes = append(scopes, scopeTarget{"tenant", tenantID})

	seenBinding := map[int64]bool{}
	var out []AppliedPolicy
	for _, sc := range scopes {
		assignments, err := s.db.Queries.ListAssignmentsByTarget(ctx, db.ListAssignmentsByTargetParams{
			TargetType: sc.Type,
			TargetID:   sc.ID,
		})
		if err != nil {
			return nil, err
		}
		for _, a := range assignments {
			group, err := s.db.Queries.GetConfigurationGroup(ctx, a.ConfigurationGroupID)
			if err != nil {
				return nil, err
			}
			if group.TenantID != tenantID {
				continue // cross-tenant assignments never apply
			}
			bindings, err := s.db.Queries.ListBindingsByGroup(ctx, a.ConfigurationGroupID)
			if err != nil {
				return nil, err
			}
			for _, b := range bindings {
				if seenBinding[b.ID] {
					continue
				}
				seenBinding[b.ID] = true
				status := "Assigned"
				if b.Disabled || group.Disabled {
					status = "Disabled"
				}
				values := ""
				if b.ParameterValues.Valid {
					values = valueSummary(b.ParameterValues.String)
				}
				out = append(out, AppliedPolicy{
					BindingID:    b.ID,
					PolicyID:     b.PolicyUrn,
					PolicyName:   policyNameByID(b.PolicyUrn),
					SourceGroup:  group.Name,
					Scope:        group.Scope,
					ValueSummary: values,
					Status:       status,
				})
			}
		}
	}
	return out, nil
}
