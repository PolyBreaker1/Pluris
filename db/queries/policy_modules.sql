-- Policy module queries
-- Policy modules are versioned, signed policy definitions

-- name: CreatePolicyModule :one
INSERT INTO policy_modules (
    module_urn,
    tenant_id,
    title,
    description,
    is_bundled
) VALUES (
    @module_urn,
    @tenant_id,
    @title,
    @description,
    @is_bundled
) RETURNING *;

-- name: GetPolicyModule :one
SELECT * FROM policy_modules WHERE id = @id LIMIT 1;

-- name: SetModuleOwner :exec
-- Sets (or clears, with a NULL @owner_identity_id) the owning identity of
-- a module. A module's owner is the identity whose Pluris permissions it
-- inherits (see pkg/authz/modules.go). Bundled modules should never have
-- an owner set.
UPDATE policy_modules SET owner_identity_id = @owner_identity_id WHERE id = @id;

-- name: GetModuleOwner :one
-- Returns just the owner_identity_id column; GetPolicyModule's SELECT *
-- already includes it, but this narrow query is convenient for authz
-- checks that only need the owner, not the whole row.
SELECT owner_identity_id FROM policy_modules WHERE id = @id LIMIT 1;

-- name: GetPolicyModuleByURN :one
SELECT * FROM policy_modules 
WHERE module_urn = @module_urn 
LIMIT 1;

-- name: ListPolicyModulesByTenant :many
SELECT * FROM policy_modules 
WHERE tenant_id = @tenant_id 
ORDER BY title
LIMIT @limit OFFSET @offset;

-- name: ListBundledModules :many
SELECT * FROM policy_modules 
WHERE is_bundled = TRUE
ORDER BY title;

-- name: CountPolicyModulesByTenant :one
SELECT COUNT(*) FROM policy_modules WHERE tenant_id = @tenant_id;

-- name: UpdatePolicyModule :one
UPDATE policy_modules SET
    title = @title,
    description = @description
WHERE id = @id
RETURNING *;

-- name: DeletePolicyModule :exec
DELETE FROM policy_modules WHERE id = @id;

-- name: ListVisibleModules :many
-- Every module a tenant can see: its own tenant-authored modules plus
-- every bundled module. Used by the service's ListModules (read path
-- for the Modules Library/Defaults/Sources pages).
SELECT * FROM policy_modules
WHERE tenant_id = @tenant_id OR is_bundled = TRUE
ORDER BY title;

-- name: SearchPolicyModules :many
SELECT * FROM policy_modules
WHERE (tenant_id = @tenant_id OR is_bundled = TRUE)
  AND (title LIKE '%' || @search || '%' OR module_urn LIKE '%' || @search || '%')
ORDER BY title
LIMIT @limit;

-- ============================================================================
-- Policy Module Versions
--
-- Migration 008 reconciled this table with the catalog/policymodules
-- domain model: runtime moved to per-phase (derived in Go from
-- LifecyclePhase, never stored -- see 008's header), and the fixed
-- enforce_script/validate_script/rollback_script columns were replaced
-- by the policy_module_scripts child table below.
-- ============================================================================

-- name: CreatePolicyModuleVersion :one
INSERT INTO policy_module_versions (
    module_id,
    version,
    state,
    manifest_yaml,
    target_os,
    scope,
    satisfies,
    parameters_schema,
    depends_on,
    conflicts,
    sandbox_profile,
    report_schema
) VALUES (
    @module_id,
    @version,
    @state,
    @manifest_yaml,
    @target_os,
    @scope,
    @satisfies,
    @parameters_schema,
    @depends_on,
    @conflicts,
    @sandbox_profile,
    @report_schema
) RETURNING *;

-- name: GetPolicyModuleVersion :one
SELECT * FROM policy_module_versions WHERE id = @id LIMIT 1;

-- name: GetPolicyModuleVersionByNumber :one
SELECT * FROM policy_module_versions
WHERE module_id = @module_id AND version = @version
LIMIT 1;

-- name: GetLatestPublishedVersion :one
SELECT * FROM policy_module_versions
WHERE module_id = @module_id AND state = 'published'
ORDER BY published_at DESC
LIMIT 1;

-- name: ListVersionsByModule :many
SELECT v.*, i.display_name as publisher_name
FROM policy_module_versions v
LEFT JOIN identities i ON i.id = v.published_by
WHERE v.module_id = @module_id
ORDER BY v.created_at DESC;

-- name: CountVersionsByModule :one
SELECT COUNT(*) FROM policy_module_versions WHERE module_id = @module_id;

-- name: UpdatePolicyModuleVersionDraft :one
-- Mutates a version's fields. The WHERE state = 'draft' guard makes the
-- immutability rule (ADR-007: published/superseded/revoked versions are
-- frozen) hold atomically even if a Publish races between the service's
-- pre-read and this UPDATE: a non-draft row matches nothing and sqlite's
-- RETURNING yields no row (sql.ErrNoRows), which the service maps to
-- ErrVersionNotDraft.
UPDATE policy_module_versions SET
    version = @version,
    target_os = @target_os,
    scope = @scope,
    satisfies = @satisfies,
    parameters_schema = @parameters_schema,
    depends_on = @depends_on,
    conflicts = @conflicts,
    sandbox_profile = @sandbox_profile,
    report_schema = @report_schema,
    manifest_yaml = @manifest_yaml
WHERE id = @id AND state = 'draft'
RETURNING *;

-- name: PublishModuleVersion :execrows
-- State-guarded: only a draft can transition to published. Returns rows
-- affected so the service detects a lost race (two concurrent Publishes
-- of the same draft: exactly one sees 1 row). Runs inside Publish's
-- transaction together with SupersedeCurrentPublishedVersion.
UPDATE policy_module_versions SET
    state = 'published',
    published_at = CURRENT_TIMESTAMP,
    published_by = @published_by
WHERE id = @id AND state = 'draft';

-- name: SupersedeCurrentPublishedVersion :execrows
-- Marks whatever is currently published for @module_id (excluding the
-- version being published, @exclude_id) superseded by
-- @superseded_by_version. One state-guarded statement instead of a
-- read-then-update so the "at most one published version per module"
-- invariant can't be broken by a writer racing between the read and the
-- write. Runs inside Publish's transaction.
UPDATE policy_module_versions SET
    state = 'superseded',
    superseded_by_version = @superseded_by_version
WHERE module_id = @module_id AND state = 'published' AND id != @exclude_id;

-- name: RevokeModuleVersion :execrows
-- State-guarded: only published/superseded versions can be revoked;
-- drafts should be deleted instead (the service returns a typed error
-- when 0 rows change).
UPDATE policy_module_versions SET
    state = 'revoked'
WHERE id = @id AND state IN ('published', 'superseded');

-- name: DeletePolicyModuleVersion :exec
DELETE FROM policy_module_versions WHERE id = @id;

-- name: GetModuleWithLatestVersion :one
SELECT
    m.*,
    v.id as latest_version_id,
    v.version as latest_version,
    v.published_at as latest_published_at
FROM policy_modules m
LEFT JOIN policy_module_versions v ON v.module_id = m.id AND v.state = 'published'
WHERE m.id = @id
ORDER BY v.published_at DESC
LIMIT 1;

-- ============================================================================
-- Policy Module Scripts (migration 008)
-- ============================================================================

-- name: UpsertModuleScript :one
-- Insert-or-replace the script for (version_id, phase, seq). v1 always
-- writes seq=0 (one script per phase); the seq column exists for a
-- future multi-file phase.
--
-- UNGUARDED -- only safe to call when the caller has already established
-- the version is a draft (e.g. ForkLatestPublished copying a brand-new
-- draft's scripts forward). Editor-facing script writes MUST go through
-- UpsertModuleScriptGuarded below instead.
INSERT INTO policy_module_scripts (version_id, phase, filename, source, seq)
VALUES (@version_id, @phase, @filename, @source, @seq)
ON CONFLICT(version_id, phase, seq) DO UPDATE SET
    filename = excluded.filename,
    source = excluded.source
RETURNING *;

-- name: UpsertModuleScriptGuarded :one
-- Task 4.3 mandatory fix: the plain UpsertModuleScript above has no
-- draft guard, so a published/superseded/revoked version's scripts could
-- be silently overwritten -- breaking ADR-007 immutability the same way
-- an unguarded version-field UPDATE would. This variant only inserts (or
-- upserts) a row when the target version's state = 'draft'; the WHERE
-- EXISTS subquery is evaluated as part of the same atomic INSERT
-- statement (no separate read-then-write), so it holds even if a Publish
-- races between the service's pre-check and this call. When the version
-- isn't a draft, the SELECT yields zero rows, nothing is inserted, and
-- RETURNING produces no row -- the :one code path surfaces that as
-- sql.ErrNoRows, which the service maps to the typed ErrVersionNotDraft.
INSERT INTO policy_module_scripts (version_id, phase, filename, source, seq)
SELECT @version_id, @phase, @filename, @source, @seq
WHERE EXISTS (
    SELECT 1 FROM policy_module_versions
    WHERE id = @version_id AND state = 'draft'
)
ON CONFLICT(version_id, phase, seq) DO UPDATE SET
    filename = excluded.filename,
    source = excluded.source
RETURNING *;

-- name: ListScriptsForVersion :many
SELECT * FROM policy_module_scripts
WHERE version_id = @version_id
ORDER BY phase, seq;

-- name: DeleteModuleScript :exec
DELETE FROM policy_module_scripts
WHERE version_id = @version_id AND phase = @phase AND seq = @seq;

-- name: DeleteScriptsForVersion :exec
DELETE FROM policy_module_scripts WHERE version_id = @version_id;
