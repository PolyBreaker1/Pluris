-- Task 6.2 -- Group detail page Members tab + create/delete-guard support.
-- Mirrors group_detail.sql's "g.*, gm.created_at AS added_at" shape:
-- these two queries add gm.source so the Members tab can render a
-- Direct/Dynamic chip per row (fixing the GroupService.ListForAsset/
-- ListForIdentity Source hardcoding carry-forward at its root: the raw
-- membership row's source column, not a hardcoded "Direct" string).

-- name: ListAssetMembersForGroup :many
SELECT a.*, gm.source AS source, gm.created_at AS added_at FROM assets a
INNER JOIN group_memberships gm ON a.id = gm.asset_id
WHERE gm.group_id = @group_id AND a.deleted_at IS NULL
ORDER BY a.human_id;

-- name: ListIdentityMembersForGroup :many
SELECT i.*, gm.source AS source, gm.created_at AS added_at FROM identities i
INNER JOIN group_memberships gm ON i.id = gm.identity_id
WHERE gm.group_id = @group_id AND i.deleted_at IS NULL
ORDER BY i.display_name;

-- name: GetGroupMembershipSourceForAsset :one
SELECT source FROM group_memberships WHERE group_id = @group_id AND asset_id = @asset_id;

-- name: GetGroupMembershipSourceForIdentity :one
SELECT source FROM group_memberships WHERE group_id = @group_id AND identity_id = @identity_id;

-- name: CreateGroupFull :one
-- Create-page insert: every column the create form + presets need in one
-- round trip, rather than CreateGroup (name/slug only) followed by a
-- second UpdateGroupMeta call.
INSERT INTO groups (tenant_id, site_id, name, slug, group_category, group_scope, description, member_kind, membership, rules_match_mode)
VALUES (@tenant_id, @site_id, @name, @slug, @group_category, @group_scope, @description, @member_kind, @membership, @rules_match_mode)
RETURNING *;

-- name: CountConfigGroupAssignmentsForGroupTarget :one
-- Delete-guard: configuration_group_assignments.target_id is a
-- polymorphic reference (no FK -- see 001_initial.sql's comment on that
-- table) so deleting a group referenced there would leave a dangling
-- assignment row pointing at a now-nonexistent group id. Group deletion
-- is blocked while any assignment still targets it (target_type='group').
SELECT COUNT(*) FROM configuration_group_assignments
WHERE target_type = 'group' AND target_id = @target_id;
