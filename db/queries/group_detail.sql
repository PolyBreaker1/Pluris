-- Group rows for the detail page Groups tabs (Task 9). Same joins as
-- ListGroupsForAsset and ListGroupsForIdentity but each row also carries
-- the membership creation time so the UI can show when the entity was
-- added to the group.

-- name: ListGroupsForAssetDetail :many
SELECT g.*, gm.created_at AS added_at FROM groups g
INNER JOIN group_memberships gm ON g.id = gm.group_id
WHERE gm.asset_id = @asset_id
ORDER BY g.name;

-- name: ListGroupsForIdentityDetail :many
SELECT g.*, gm.created_at AS added_at FROM groups g
INNER JOIN group_memberships gm ON g.id = gm.group_id
WHERE gm.identity_id = @identity_id
ORDER BY g.name;

-- Role rows for the user detail Roles tab (Task 11). Same join as
-- ListRolesForIdentity but carries the assignment time.

-- name: ListRolesForIdentityDetail :many
SELECT r.*, ir.assigned_at FROM roles r
JOIN identity_roles ir ON ir.role_id = r.id
WHERE ir.identity_id = @identity_id
ORDER BY r.name;
