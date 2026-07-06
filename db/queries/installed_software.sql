-- Installed software queries - per asset software inventory backed by
-- the installed_software table in db/schema/003_roles_software_logs.sql.

-- name: CreateInstalledSoftware :one
INSERT INTO installed_software (asset_id, name, version, publisher, pkg_type, installed_at, size_mb)
VALUES (@asset_id, @name, @version, @publisher, @pkg_type, @installed_at, @size_mb)
RETURNING *;

-- name: ListSoftwareForAsset :many
SELECT * FROM installed_software
WHERE asset_id = @asset_id
ORDER BY name;
