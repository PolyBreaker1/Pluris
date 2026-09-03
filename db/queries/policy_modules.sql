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
SELECT * FROM policy_modules WHERE id = @id AND deleted_at IS NULL LIMIT 1;

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
  AND deleted_at IS NULL
LIMIT 1;

-- name: GetPolicyModuleByURNIncludingDeleted :one
SELECT * FROM policy_modules
WHERE module_urn = @module_urn
LIMIT 1;

-- name: ListPolicyModulesByTenant :many
SELECT * FROM policy_modules 
WHERE tenant_id = @tenant_id 
  AND deleted_at IS NULL
ORDER BY title
LIMIT @limit OFFSET @offset;

-- name: ListBundledModules :many
SELECT * FROM policy_modules 
WHERE is_bundled = TRUE
  AND deleted_at IS NULL
ORDER BY title;

-- name: CountPolicyModulesByTenant :one
SELECT COUNT(*) FROM policy_modules WHERE tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: UpdatePolicyModule :one
UPDATE policy_modules SET
    title = @title,
    description = @description
WHERE id = @id
RETURNING *;

-- name: DeletePolicyModule :exec
DELETE FROM policy_modules WHERE id = @id;

-- name: SoftDeletePolicyModule :execrows
UPDATE policy_modules
SET deleted_at = CURRENT_TIMESTAMP, deleted_by = @deleted_by
WHERE id = @id AND deleted_at IS NULL;

-- name: RestorePolicyModule :execrows
UPDATE policy_modules
SET deleted_at = NULL, deleted_by = NULL
WHERE id = @id AND deleted_at IS NOT NULL;

-- name: ListDeletedVisibleModules :many
SELECT * FROM policy_modules
WHERE (tenant_id = @tenant_id OR is_bundled = TRUE)
  AND deleted_at IS NOT NULL
ORDER BY title;

-- name: ListExpiredPolicyModules :many
SELECT * FROM policy_modules
WHERE deleted_at IS NOT NULL
  AND deleted_at <= @cutoff
ORDER BY deleted_at, id;

-- name: ListVisibleModules :many
-- Every module a tenant can see: its own tenant-authored modules plus
-- every bundled module. Used by the service's ListModules (read path
-- for the Modules Library/Defaults/Sources pages).
SELECT * FROM policy_modules
WHERE (tenant_id = @tenant_id OR is_bundled = TRUE)
  AND deleted_at IS NULL
ORDER BY title;

-- name: SearchPolicyModules :many
SELECT * FROM policy_modules
WHERE (tenant_id = @tenant_id OR is_bundled = TRUE)
  AND deleted_at IS NULL
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

-- name: DeleteDraftModuleVersion :execrows
-- State-guarded in the statement itself (same pattern as
-- PublishModuleVersion): only a draft can be deleted, so a concurrent
-- publish landing between a caller's read and this delete can never
-- destroy a published version.
DELETE FROM policy_module_versions WHERE id = @id AND state = 'draft';

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
-- Policy Module Scripts + Actions (migration 012, replaces migration 008)
--
-- Scripts are now first-class named rows (name/language/origin) instead
-- of phase-keyed. Actions are the enforcement wiring: a separate table
-- that references a script by name (kind='script') or holds an inline
-- command (kind='command'). origin distinguishes seeded defaults
-- (never mutated in place; the Go service layer forks an edited
-- default into a custom row) from tenant-authored rows.
-- ============================================================================

-- name: ListScriptsForVersion :many
SELECT * FROM policy_module_scripts WHERE version_id = @version_id ORDER BY seq, name;

-- name: GetScriptByName :one
SELECT * FROM policy_module_scripts WHERE version_id = @version_id AND name = @name;

-- name: UpsertModuleScriptGuarded :one
-- Insert-or-replace the script identified by (version_id, name), only
-- when the target version's state = 'draft'. The WHERE EXISTS subquery
-- is evaluated as part of the same atomic INSERT statement (no separate
-- read-then-write), so the guard holds even if a Publish races between
-- the service's pre-check and this call. When the version isn't a
-- draft, the SELECT yields zero rows, nothing is inserted, and
-- RETURNING produces no row -- the :one code path surfaces that as
-- sql.ErrNoRows, which the service maps to the typed ErrVersionNotDraft.
INSERT INTO policy_module_scripts (version_id, name, language, source, origin, seq)
SELECT @version_id, @name, @language, @source, @origin, @seq
WHERE EXISTS (
    SELECT 1 FROM policy_module_versions
    WHERE id = @version_id AND state = 'draft'
)
ON CONFLICT(version_id, name) DO UPDATE SET
    language = excluded.language,
    source = excluded.source,
    origin = excluded.origin
RETURNING *;

-- name: RenameModuleScriptGuarded :execrows
UPDATE policy_module_scripts
SET name = @new_name
WHERE version_id = @version_id AND name = @old_name
  AND EXISTS (
    SELECT 1 FROM policy_module_versions
    WHERE id = @version_id AND state = 'draft'
  );

-- name: DeleteModuleScriptGuarded :execrows
DELETE FROM policy_module_scripts
WHERE version_id = @version_id AND name = @name
  AND EXISTS (
    SELECT 1 FROM policy_module_versions
    WHERE id = @version_id AND state = 'draft'
  );

-- name: DeleteCustomScriptsForVersion :exec
DELETE FROM policy_module_scripts WHERE version_id = @version_id AND origin = 'custom';

-- name: ListActionsForVersion :many
SELECT * FROM policy_module_actions WHERE version_id = @version_id ORDER BY seq, action_key;

-- name: UpsertModuleActionGuarded :one
-- Insert-or-replace the action identified by (version_id, action_key),
-- draft-guarded the same way UpsertModuleScriptGuarded is.
INSERT INTO policy_module_actions (version_id, action_key, label, kind, value, origin, seq)
SELECT @version_id, @action_key, @label, @kind, @value, @origin, @seq
WHERE EXISTS (
    SELECT 1 FROM policy_module_versions
    WHERE id = @version_id AND state = 'draft'
)
ON CONFLICT(version_id, action_key) DO UPDATE SET
    label = excluded.label,
    kind = excluded.kind,
    value = excluded.value,
    origin = excluded.origin
RETURNING *;

-- name: DeleteModuleActionGuarded :execrows
DELETE FROM policy_module_actions
WHERE version_id = @version_id AND action_key = @action_key
  AND EXISTS (
    SELECT 1 FROM policy_module_versions
    WHERE id = @version_id AND state = 'draft'
  );

-- name: DeleteCustomActionsForVersion :exec
DELETE FROM policy_module_actions WHERE version_id = @version_id AND origin = 'custom';

-- ============================================================================
-- Module Version Conditions (migration 011)
--
-- Per-version applicability tests, column parity with
-- dependency_group_conditions so the same eval engine and condition
-- builder drive both (INV-CB). All writes are draft-guarded via a
-- WHERE EXISTS subquery on the parent version state, mirroring
-- UpsertModuleScriptGuarded: zero rows affected on a non-draft version
-- surfaces as sql.ErrNoRows / 0 execrows, which the service maps to
-- ErrVersionNotDraft. script_expect exists for parity only and is
-- never written (legacy dead column, see 011 header).
-- ============================================================================

-- name: ListVersionConditions :many
SELECT * FROM module_version_conditions
WHERE version_id = @version_id
ORDER BY seq, id;

-- name: CreateVersionConditionGuarded :one
INSERT INTO module_version_conditions (version_id, kind, param_path, operator, value_json, script_source, script_ref, seq)
SELECT @version_id, @kind, @param_path, @operator, @value_json, @script_source, @script_ref, @seq
WHERE EXISTS (
    SELECT 1 FROM policy_module_versions
    WHERE id = @version_id AND state = 'draft'
)
RETURNING *;

-- name: UpdateVersionConditionGuarded :execrows
UPDATE module_version_conditions
SET kind = @kind,
    param_path = @param_path,
    operator = @operator,
    value_json = @value_json,
    script_source = @script_source,
    script_ref = @script_ref
WHERE module_version_conditions.id = @id AND version_id = @version_id
  AND EXISTS (
    SELECT 1 FROM policy_module_versions v
    WHERE v.id = @version_id AND v.state = 'draft'
  );

-- name: DeleteVersionConditionGuarded :execrows
DELETE FROM module_version_conditions
WHERE module_version_conditions.id = @id AND version_id = @version_id
  AND EXISTS (
    SELECT 1 FROM policy_module_versions v
    WHERE v.id = @version_id AND v.state = 'draft'
  );

-- name: MaxVersionConditionSeq :one
SELECT COALESCE(MAX(seq), -1) FROM module_version_conditions
WHERE version_id = @version_id;

-- name: UpdateVersionConditionsMatchMode :execrows
-- Draft-guarded in the statement itself, same pattern as the other
-- version mutators.
UPDATE policy_module_versions
SET conditions_match_mode = @conditions_match_mode
WHERE id = @id AND state = 'draft';

-- name: SetModuleOrigin :exec
UPDATE policy_modules SET origin = @origin WHERE id = @id;

-- name: CacheVersionManifest :exec
-- manifest_yaml is a derived export artifact (008 header); this cache
-- write happens at .pmdl export time and is never read back as input.
UPDATE policy_module_versions SET manifest_yaml = @manifest_yaml WHERE id = @id;
