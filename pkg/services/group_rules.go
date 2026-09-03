package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/db"
)

// Member-kind and membership-mode enum values (Go-enforced — see
// db/schema/009_group_kinds_rules.sql's header comment for why these
// aren't CHECK constraints).
const (
	MemberKindAsset    = "asset"
	MemberKindIdentity = "identity"
	MemberKindMixed    = "mixed"

	MembershipStatic  = "static"
	MembershipDynamic = "dynamic"

	membershipSourceDirect = "direct"
	membershipSourceRule   = "rule"
)

var (
	// ErrInvalidMemberKind is returned when a group's member_kind isn't
	// "asset"/"identity"/"mixed".
	ErrInvalidMemberKind = errors.New("invalid member kind")
	// ErrInvalidMembershipMode is returned when a group's membership
	// isn't "static"/"dynamic".
	ErrInvalidMembershipMode = errors.New("invalid membership mode")
	// ErrGroupNotDynamic is returned by rule CRUD and
	// EvaluateDynamicMembership when the target group's membership is
	// "static" — rules only make sense on a dynamic group.
	ErrGroupNotDynamic = errors.New("group membership rules require a dynamic group")
	// ErrMemberKindConflict is returned by SetGroupMeta when changing
	// member_kind would strand existing members of the excluded kind
	// (e.g. switching to "identity" while the group still has asset
	// members, direct or rule-sourced). The caller (Task 6.2's group
	// settings UI) must remove those members first.
	ErrMemberKindConflict = errors.New("cannot change member kind: group has existing members of the excluded kind")
)

func validMemberKind(k string) bool {
	switch k {
	case MemberKindAsset, MemberKindIdentity, MemberKindMixed:
		return true
	}
	return false
}

func validMembershipMode(m string) bool {
	switch m {
	case MembershipStatic, MembershipDynamic:
		return true
	}
	return false
}

// SetGroupMeta updates a group's description/member_kind/membership/
// rules_match_mode. member_kind and membership are validated against
// their Go-enforced enums; rules_match_mode reuses
// dependencygroups.MatchMode's validation (ErrInvalidMatchMode, defined
// in dependencygroups.go — the same error dependency groups' own
// SetMatchMode returns, since it's the identical "all"/"any" enum).
//
// Changing member_kind away from "mixed" is rejected with
// ErrMemberKindConflict when the group still has members (direct or
// rule-sourced) of the kind being excluded — switching a group to
// "identity"-only while it has asset members would silently orphan
// those rows from the UI's perspective.
func (s *GroupService) SetGroupMeta(ctx context.Context, groupID int64, description, memberKind, membership, rulesMatchMode string) error {
	if !validMemberKind(memberKind) {
		return ErrInvalidMemberKind
	}
	if !validMembershipMode(membership) {
		return ErrInvalidMembershipMode
	}
	switch dependencygroups.MatchMode(rulesMatchMode) {
	case dependencygroups.MatchAll, dependencygroups.MatchAny:
	default:
		return ErrInvalidMatchMode
	}

	if memberKind != MemberKindMixed {
		assetCount, err := s.db.Queries.CountAssetsInGroup(ctx, groupID)
		if err != nil {
			return err
		}
		identityCount, err := s.db.Queries.CountIdentitiesInGroup(ctx, groupID)
		if err != nil {
			return err
		}
		if memberKind == MemberKindIdentity && assetCount > 0 {
			return ErrMemberKindConflict
		}
		if memberKind == MemberKindAsset && identityCount > 0 {
			return ErrMemberKindConflict
		}
	}

	return s.db.Queries.UpdateGroupMeta(ctx, db.UpdateGroupMetaParams{
		ID: groupID, Description: description, MemberKind: memberKind,
		Membership: membership, RulesMatchMode: rulesMatchMode,
	})
}

// AddRule appends a dynamic-membership rule to a group, reusing the
// EXACT same validation DependencyGroupService.AddCondition applies
// (validateConditionPayload, dependencygroups.go — same package, so
// called directly rather than duplicated). Only a dynamic group
// ("membership"="dynamic") may hold rules — a static group's AddRule
// call fails closed with ErrGroupNotDynamic rather than silently
// accepting a rule with no effect. On success,
// EvaluateDynamicMembership runs immediately (rule save is a trigger
// point per the design), reconciling rule-sourced members against the
// now-current rule set.
func (s *GroupService) AddRule(ctx context.Context, groupID int64, kind, paramPath, operator string, values []string, scriptSource, scriptRef string) (db.GroupMembershipRule, error) {
	group, err := s.db.Queries.GetGroup(ctx, groupID)
	if err != nil {
		return db.GroupMembershipRule{}, err
	}
	if group.Membership != MembershipDynamic {
		return db.GroupMembershipRule{}, ErrGroupNotDynamic
	}

	kind, err = validateConditionPayload(conditionPayload{
		Kind: kind, ParamPath: paramPath, Operator: operator,
		ScriptSource: scriptSource, ScriptRef: scriptRef,
	})
	if err != nil {
		return db.GroupMembershipRule{}, err
	}

	existing, err := s.db.Queries.ListRulesForGroup(ctx, groupID)
	if err != nil {
		return db.GroupMembershipRule{}, err
	}
	vals, _ := json.Marshal(values)
	rule, err := s.db.Queries.CreateGroupMembershipRule(ctx, db.CreateGroupMembershipRuleParams{
		GroupID: groupID, Kind: kind, ParamPath: paramPath, Operator: operator, ValueJson: string(vals),
		ScriptSource: scriptSource, ScriptRef: scriptRef, ScriptExpect: "", Seq: int64(len(existing)),
	})
	if err != nil {
		return db.GroupMembershipRule{}, err
	}
	if _, err := s.EvaluateDynamicMembership(ctx, groupID); err != nil {
		return rule, err
	}
	return rule, nil
}

// ListRules returns a group's dynamic-membership rules in seq order.
func (s *GroupService) ListRules(ctx context.Context, groupID int64) ([]db.GroupMembershipRule, error) {
	return s.db.Queries.ListRulesForGroup(ctx, groupID)
}

// RemoveRule deletes one rule and re-runs EvaluateDynamicMembership
// (rule delete is the other trigger point per the design) so members
// that only matched via the removed rule are reconciled away
// immediately.
func (s *GroupService) RemoveRule(ctx context.Context, groupID, ruleID int64) error {
	if err := s.db.Queries.DeleteGroupMembershipRule(ctx, db.DeleteGroupMembershipRuleParams{ID: ruleID, GroupID: groupID}); err != nil {
		return err
	}
	_, err := s.EvaluateDynamicMembership(ctx, groupID)
	return err
}

// DynamicMembershipResult is EvaluateDynamicMembership's verdict:
// counts of rows added/removed by this reconciliation pass, and the
// group's total member count (direct + rule-sourced) afterward.
type DynamicMembershipResult struct {
	Added   int
	Removed int
	Total   int
}

// EvaluateDynamicMembership recomputes a dynamic group's rule-sourced
// members. It loads the group's rules into a dependencygroups.Group
// shape (Conditions + MatchMode, the exact same struct
// DependencyGroupService.Evaluate builds from dependency_group_conditions
// rows), then evaluates it against every tenant asset and/or identity
// permitted by the group's member_kind:
//
//   - member_kind "asset"  -> assets only
//   - member_kind "identity" -> identities only
//   - member_kind "mixed"  -> both populations
//
// A candidate is a member iff dependencygroups.EvalGroup returns "pass"
// for its facts (built via FactsForAsset / FactsForIdentity, facts.go —
// the same helpers EvaluateForAsset uses, so asset facts are derived
// identically whether the caller is checking module eligibility or
// dynamic group membership). "unknown" (e.g. every script-kind rule,
// whose result an agent hasn't reported) is NOT a pass — a script rule
// alone therefore yields zero members until the agent reports a result,
// never a false-positive membership.
//
// Reconciliation only ever touches group_memberships rows with
// source='rule': passing candidates not yet members are inserted with
// source='rule' (a no-op if the row already exists as a direct member —
// the (group_id, asset_id)/(group_id, identity_id) UNIQUE constraints
// make that insert conflict harmlessly, and direct rows are never
// "upgraded" to rule or vice versa); rule-sourced members that no
// longer pass are deleted. source='direct' rows are never read for
// eligibility and never written or deleted here — an admin's explicit
// add/remove always wins over what the rules would otherwise compute.
//
// Static groups (membership != "dynamic") reject evaluation with
// ErrGroupNotDynamic, mirroring AddRule's guard: recalculating rules
// that (by construction) cannot exist on a static group is a caller
// bug, not a silent no-op.
//
// No scheduler triggers this automatically today — it runs on rule
// save/delete (AddRule/RemoveRule above) and is exposed here for an
// explicit "Recalculate" action the next task's UI wires up. Periodic
// re-evaluation (picking up facts that changed without any rule
// change, e.g. an asset's os_family being updated) is deferred; documented
// here rather than silently assumed.
func (s *GroupService) EvaluateDynamicMembership(ctx context.Context, groupID int64) (DynamicMembershipResult, error) {
	group, err := s.db.Queries.GetGroup(ctx, groupID)
	if err != nil {
		return DynamicMembershipResult{}, err
	}
	if group.Membership != MembershipDynamic {
		return DynamicMembershipResult{}, ErrGroupNotDynamic
	}

	ruleRows, err := s.db.Queries.ListRulesForGroup(ctx, groupID)
	if err != nil {
		return DynamicMembershipResult{}, err
	}

	// Zero-rules choke point: a dynamic group with NO rules matches
	// NOBODY, not everybody. The shared eval engine
	// (catalog/dependencygroups/eval.go) treats an empty condition set
	// under the default match_mode='all' as a vacuous AND that passes for
	// every candidate — correct for dependency groups (an unconstrained
	// filter is universally satisfied), catastrophic for group membership
	// (a single "remove last rule" click would mass-add every tenant
	// identity/asset as a rule-sourced member, and dynamic groups can
	// carry group_roles). We do NOT change the engine's semantics —
	// dependency groups depend on them — this is a group-membership policy
	// decision, so it lives here at the single reconciliation choke point:
	// with zero rules the target rule-membership set is empty, i.e. remove
	// ALL rule-sourced members (direct members are never touched). Every
	// caller (RemoveRule, and any future scheduler) inherits this.
	if len(ruleRows) == 0 {
		return s.clearRuleSourcedMembers(ctx, group)
	}

	depGroup := dependencygroups.Group{
		ID:        group.ID,
		Name:      group.Name,
		MatchMode: dependencygroups.MatchMode(group.RulesMatchMode),
	}
	for _, r := range ruleRows {
		var vals []string
		_ = json.Unmarshal([]byte(r.ValueJson), &vals)
		depGroup.Conditions = append(depGroup.Conditions, dependencygroups.Condition{
			ID: r.ID, ParamPath: r.ParamPath, Operator: dependencygroups.Operator(r.Operator), Values: vals,
			Kind: dependencygroups.ConditionKind(r.Kind), ScriptSource: r.ScriptSource, ScriptRef: r.ScriptRef, ScriptExpect: r.ScriptExpect,
		})
	}

	var result DynamicMembershipResult

	if group.MemberKind == MemberKindAsset || group.MemberKind == MemberKindMixed {
		added, removed, err := s.reconcileAssetRuleMembers(ctx, group, depGroup)
		if err != nil {
			return DynamicMembershipResult{}, err
		}
		result.Added += added
		result.Removed += removed
	}
	if group.MemberKind == MemberKindIdentity || group.MemberKind == MemberKindMixed {
		added, removed, err := s.reconcileIdentityRuleMembers(ctx, group, depGroup)
		if err != nil {
			return DynamicMembershipResult{}, err
		}
		result.Added += added
		result.Removed += removed
	}

	assetTotal, err := s.db.Queries.CountAssetsInGroup(ctx, groupID)
	if err != nil {
		return DynamicMembershipResult{}, err
	}
	identityTotal, err := s.db.Queries.CountIdentitiesInGroup(ctx, groupID)
	if err != nil {
		return DynamicMembershipResult{}, err
	}
	result.Total = int(assetTotal + identityTotal)
	return result, nil
}

// clearRuleSourcedMembers is EvaluateDynamicMembership's zero-rules path:
// the target rule-membership set is empty, so every source='rule' row is
// removed (source='direct' rows are never touched). Returns the same
// Added/Removed/Total shape as a normal reconciliation, with Removed
// counting the rule-sourced rows cleared and Total the group's remaining
// (direct-only) member count.
func (s *GroupService) clearRuleSourcedMembers(ctx context.Context, group db.Group) (DynamicMembershipResult, error) {
	assetRuleIDs, err := s.db.Queries.ListRuleSourcedAssetIDsForGroup(ctx, group.ID)
	if err != nil {
		return DynamicMembershipResult{}, err
	}
	identityRuleIDs, err := s.db.Queries.ListRuleSourcedIdentityIDsForGroup(ctx, group.ID)
	if err != nil {
		return DynamicMembershipResult{}, err
	}
	removed := len(assetRuleIDs) + len(identityRuleIDs)
	if removed > 0 {
		if err := s.db.Queries.DeleteAllRuleSourcedMembersForGroup(ctx, group.ID); err != nil {
			return DynamicMembershipResult{}, err
		}
	}
	assetTotal, err := s.db.Queries.CountAssetsInGroup(ctx, group.ID)
	if err != nil {
		return DynamicMembershipResult{}, err
	}
	identityTotal, err := s.db.Queries.CountIdentitiesInGroup(ctx, group.ID)
	if err != nil {
		return DynamicMembershipResult{}, err
	}
	return DynamicMembershipResult{Added: 0, Removed: removed, Total: int(assetTotal + identityTotal)}, nil
}

// reconcileAssetRuleMembers evaluates depGroup against every asset in
// the group's tenant and reconciles source='rule' group_memberships
// rows to match. See EvaluateDynamicMembership's doc comment for the
// reconciliation contract.
func (s *GroupService) reconcileAssetRuleMembers(ctx context.Context, group db.Group, depGroup dependencygroups.Group) (added, removed int, err error) {
	assets, err := s.db.Queries.ListAssets(ctx, db.ListAssetsParams{TenantID: group.TenantID, Limit: targetCatalogLimit, Offset: 0})
	if err != nil {
		return 0, 0, err
	}
	directIDs, err := s.db.Queries.ListDirectAssetIDsForGroup(ctx, group.ID)
	if err != nil {
		return 0, 0, err
	}
	direct := make(map[int64]bool, len(directIDs))
	for _, id := range directIDs {
		if id.Valid {
			direct[id.Int64] = true
		}
	}
	ruleIDs, err := s.db.Queries.ListRuleSourcedAssetIDsForGroup(ctx, group.ID)
	if err != nil {
		return 0, 0, err
	}
	currentRule := make(map[int64]bool, len(ruleIDs))
	for _, id := range ruleIDs {
		if id.Valid {
			currentRule[id.Int64] = true
		}
	}

	passing := make(map[int64]bool)
	for _, a := range assets {
		if dependencygroups.EvalGroup(depGroup, FactsForAsset(a)) == "pass" {
			passing[a.ID] = true
		}
	}

	for id := range passing {
		if direct[id] || currentRule[id] {
			continue
		}
		if err := s.db.Queries.AddAssetToGroupWithSource(ctx, db.AddAssetToGroupWithSourceParams{
			GroupID: group.ID, AssetID: sql.NullInt64{Int64: id, Valid: true}, Source: membershipSourceRule,
		}); err != nil {
			return added, removed, err
		}
		added++
	}
	for id := range currentRule {
		if passing[id] {
			continue
		}
		if err := s.db.Queries.DeleteRuleSourcedAssetFromGroup(ctx, db.DeleteRuleSourcedAssetFromGroupParams{
			GroupID: group.ID, AssetID: sql.NullInt64{Int64: id, Valid: true},
		}); err != nil {
			return added, removed, err
		}
		removed++
	}
	return added, removed, nil
}

// reconcileIdentityRuleMembers mirrors reconcileAssetRuleMembers for
// identities.
func (s *GroupService) reconcileIdentityRuleMembers(ctx context.Context, group db.Group, depGroup dependencygroups.Group) (added, removed int, err error) {
	identities, err := s.db.Queries.ListIdentitiesByTenant(ctx, db.ListIdentitiesByTenantParams{TenantID: group.TenantID, Limit: targetCatalogLimit, Offset: 0})
	if err != nil {
		return 0, 0, err
	}
	directIDs, err := s.db.Queries.ListDirectIdentityIDsForGroup(ctx, group.ID)
	if err != nil {
		return 0, 0, err
	}
	direct := make(map[int64]bool, len(directIDs))
	for _, id := range directIDs {
		if id.Valid {
			direct[id.Int64] = true
		}
	}
	ruleIDs, err := s.db.Queries.ListRuleSourcedIdentityIDsForGroup(ctx, group.ID)
	if err != nil {
		return 0, 0, err
	}
	currentRule := make(map[int64]bool, len(ruleIDs))
	for _, id := range ruleIDs {
		if id.Valid {
			currentRule[id.Int64] = true
		}
	}

	passing := make(map[int64]bool)
	for _, ident := range identities {
		if dependencygroups.EvalGroup(depGroup, FactsForIdentity(ident)) == "pass" {
			passing[ident.ID] = true
		}
	}

	for id := range passing {
		if direct[id] || currentRule[id] {
			continue
		}
		if err := s.db.Queries.AddIdentityToGroupWithSource(ctx, db.AddIdentityToGroupWithSourceParams{
			GroupID: group.ID, IdentityID: sql.NullInt64{Int64: id, Valid: true}, Source: membershipSourceRule,
		}); err != nil {
			return added, removed, err
		}
		added++
	}
	for id := range currentRule {
		if passing[id] {
			continue
		}
		if err := s.db.Queries.DeleteRuleSourcedIdentityFromGroup(ctx, db.DeleteRuleSourcedIdentityFromGroupParams{
			GroupID: group.ID, IdentityID: sql.NullInt64{Int64: id, Valid: true},
		}); err != nil {
			return added, removed, err
		}
		removed++
	}
	return added, removed, nil
}
