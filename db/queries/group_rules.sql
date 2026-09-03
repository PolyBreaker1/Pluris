-- Group member-kind/membership metadata and dynamic membership rules.
-- Matches db/schema/009_group_kinds_rules.sql. group_membership_rules
-- mirrors dependency_group_conditions column-for-column (see that
-- migration's header comment) so both are driven by the same eval
-- engine (catalog/dependencygroups/eval.go).

-- ============================================================================
-- Group metadata (member_kind / membership / rules_match_mode / description)
-- ============================================================================

-- name: UpdateGroupMeta :exec
UPDATE groups SET
    description = @description,
    member_kind = @member_kind,
    membership = @membership,
    rules_match_mode = @rules_match_mode
WHERE id = @id;

-- ============================================================================
-- Membership rules (group_membership_rules)
-- ============================================================================

-- name: CreateGroupMembershipRule :one
INSERT INTO group_membership_rules (group_id, kind, param_path, operator, value_json, script_source, script_ref, script_expect, seq)
VALUES (@group_id, @kind, @param_path, @operator, @value_json, @script_source, @script_ref, @script_expect, @seq)
RETURNING *;

-- name: ListRulesForGroup :many
SELECT * FROM group_membership_rules
WHERE group_id = @group_id
ORDER BY seq, id;

-- name: DeleteGroupMembershipRule :exec
DELETE FROM group_membership_rules WHERE id = @id AND group_id = @group_id;

-- name: DeleteRulesForGroup :exec
DELETE FROM group_membership_rules WHERE group_id = @group_id;

-- ============================================================================
-- Rule-sourced membership reconciliation
-- ============================================================================

-- name: AddAssetToGroupWithSource :exec
INSERT INTO group_memberships (group_id, asset_id, source)
VALUES (@group_id, @asset_id, @source)
ON CONFLICT DO NOTHING;

-- name: AddIdentityToGroupWithSource :exec
INSERT INTO group_memberships (group_id, identity_id, source)
VALUES (@group_id, @identity_id, @source)
ON CONFLICT DO NOTHING;

-- name: DeleteRuleSourcedAssetFromGroup :exec
DELETE FROM group_memberships
WHERE group_id = @group_id AND asset_id = @asset_id AND source = 'rule';

-- name: DeleteRuleSourcedIdentityFromGroup :exec
DELETE FROM group_memberships
WHERE group_id = @group_id AND identity_id = @identity_id AND source = 'rule';

-- name: DeleteAllRuleSourcedMembersForGroup :exec
DELETE FROM group_memberships
WHERE group_id = @group_id AND source = 'rule';

-- name: ListRuleSourcedAssetIDsForGroup :many
SELECT asset_id FROM group_memberships
WHERE group_id = @group_id AND source = 'rule' AND asset_id IS NOT NULL;

-- name: ListRuleSourcedIdentityIDsForGroup :many
SELECT identity_id FROM group_memberships
WHERE group_id = @group_id AND source = 'rule' AND identity_id IS NOT NULL;

-- name: ListDirectAssetIDsForGroup :many
SELECT asset_id FROM group_memberships
WHERE group_id = @group_id AND source = 'direct' AND asset_id IS NOT NULL;

-- name: ListDirectIdentityIDsForGroup :many
SELECT identity_id FROM group_memberships
WHERE group_id = @group_id AND source = 'direct' AND identity_id IS NOT NULL;

-- name: CountDirectAssetsInGroup :one
SELECT COUNT(*) FROM group_memberships
WHERE group_id = @group_id AND asset_id IS NOT NULL AND source = 'direct';

-- name: CountDirectIdentitiesInGroup :one
SELECT COUNT(*) FROM group_memberships
WHERE group_id = @group_id AND identity_id IS NOT NULL AND source = 'direct';
