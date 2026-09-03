-- Group rows for the detail page Groups tabs (Task 9). Same joins as
-- ListGroupsForAsset and ListGroupsForIdentity but each row also carries
-- the membership creation time so the UI can show when the entity was
-- added to the group.

-- name: ListGroupsForAssetDetail :many
-- gm.source is selected (Task 6.2) so GroupService.ListForAsset can show
-- the membership's real source (direct/rule) instead of a hardcoded
-- "Direct" label.
SELECT g.*, gm.created_at AS added_at, gm.source AS source FROM groups g
INNER JOIN group_memberships gm ON g.id = gm.group_id
WHERE gm.asset_id = @asset_id AND g.deleted_at IS NULL
ORDER BY g.name;

-- name: ListGroupsForIdentityDetail :many
-- gm.source selected for the same reason as ListGroupsForAssetDetail
-- above.
SELECT g.*, gm.created_at AS added_at, gm.source AS source FROM groups g
INNER JOIN group_memberships gm ON g.id = gm.group_id
WHERE gm.identity_id = @identity_id AND g.deleted_at IS NULL
ORDER BY g.name;

-- Role rows for the user detail Roles tab (Task 11). Same join as
-- ListRolesForIdentity but carries the assignment time.

-- name: ListRolesForIdentityDetail :many
SELECT r.*, ir.assigned_at FROM roles r
JOIN identity_roles ir ON ir.role_id = r.id
WHERE ir.identity_id = @identity_id
ORDER BY r.name;

-- Assignments reaching a given catalog policy, for the policy detail
-- page Assignments tab (Task 15).

-- name: ListAssignmentsByPolicy :many
SELECT g.name AS group_name, g.scope AS group_scope, g.disabled AS group_disabled,
       a.target_type, a.target_id
FROM configuration_group_bindings b
JOIN configuration_groups g ON g.id = b.configuration_group_id
JOIN configuration_group_assignments a ON a.configuration_group_id = g.id
WHERE b.policy_urn = @policy_urn AND g.tenant_id = @tenant_id AND g.deleted_at IS NULL
ORDER BY g.name;
