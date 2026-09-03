package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// PolicyModuleService is the DB-backed persistence layer for Policy
// Modules (Task 4.2). It follows the DependencyGroupService pattern:
// a thin wrapper over *database.Database plus mapping between sqlc rows
// and the catalog/policymodules domain model. catalog/policymodules
// itself stays pure (no DB import) -- resolver.go and defaults.go
// operate on []policymodules.Module slices supplied by callers, and
// this service is how those slices get populated from real data.
type PolicyModuleService struct {
	db *database.Database
}

func NewPolicyModuleService(db *database.Database) *PolicyModuleService {
	return &PolicyModuleService{db: db}
}

// Typed errors -- mirrors the ErrBuiltinProtected-style pattern in
// dependencygroups.go.
var (
	// ErrVersionNotDraft is returned by UpdateDraft when the version's
	// state isn't 'draft'. Published/superseded/revoked versions are
	// immutable per ADR-007.
	ErrVersionNotDraft = errors.New("only draft module versions can be modified")

	// ErrPublishRequiresApplyScript is returned by Publish when the
	// version has no 'apply' phase script. INV-M3: every deployable
	// module version must supply an apply script.
	ErrPublishRequiresApplyScript = errors.New("publishing requires an apply-phase script (INV-M3)")

	// ErrModuleReferenced is returned by DeleteModule when the module
	// is still referenced by a dependency-group link, a configuration
	// group binding, or a module installation.
	ErrModuleReferenced = errors.New("module is referenced and cannot be deleted")

	// ErrInvalidLifecyclePhase is returned by SetScript when phase is
	// outside the Go enum (apply/disable/uninstall/validate/report).
	ErrInvalidLifecyclePhase = errors.New("invalid lifecycle phase")

	// ErrModuleNotFound is a thin wrapper so callers can distinguish
	// "module row missing" from other sql.ErrNoRows situations without
	// depending on database/sql directly.
	ErrModuleNotFound = errors.New("policy module not found")

	// ErrRevokeInvalidState is returned by Revoke when the version isn't
	// published or superseded. Drafts should be deleted, not revoked;
	// already-revoked versions stay revoked.
	ErrRevokeInvalidState = errors.New("only published or superseded module versions can be revoked")

	// ErrNoExportableVersion is returned by ExportVersionIDs when the
	// module has no version matching the export selection.
	ErrNoExportableVersion = errors.New("no exportable version for this module")
)

func validLifecyclePhase(phase policymodules.LifecyclePhase) bool {
	for _, p := range policymodules.AllLifecyclePhases {
		if p == phase {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------
// Module CRUD
// ----------------------------------------------------------------------

// CreateModule creates a new policy module row. tenantID == nil means
// bundled (is_bundled=true, tenant_id NULL); ownerIdentityID is the
// creating identity (caller supplies it from the session -- see
// db/schema/007's header on why ownership is caller-supplied, not
// inferred here). ownerIdentityID is nil for bundled modules.
func (s *PolicyModuleService) CreateModule(ctx context.Context, tenantID *int64, ownerIdentityID *int64, urn, title, description string) (db.PolicyModule, error) {
	var tid sql.NullInt64
	if tenantID != nil {
		tid = sql.NullInt64{Int64: *tenantID, Valid: true}
	}
	row, err := s.db.Queries.CreatePolicyModule(ctx, db.CreatePolicyModuleParams{
		ModuleUrn:   urn,
		TenantID:    tid,
		Title:       title,
		Description: sql.NullString{String: description, Valid: description != ""},
		IsBundled:   tenantID == nil,
	})
	if err != nil {
		return db.PolicyModule{}, err
	}
	if ownerIdentityID != nil {
		oid := sql.NullInt64{Int64: *ownerIdentityID, Valid: true}
		if err := s.db.Queries.SetModuleOwner(ctx, db.SetModuleOwnerParams{ID: row.ID, OwnerIdentityID: oid}); err != nil {
			return db.PolicyModule{}, err
		}
		row.OwnerIdentityID = oid
	}
	return row, nil
}

// UpdateModule mutates a module's metadata (title/description). Owner
// and tenant are set once at creation and changed via dedicated calls
// (SetModuleOwner), not through this generic update.
func (s *PolicyModuleService) UpdateModule(ctx context.Context, id int64, title, description string) (db.PolicyModule, error) {
	return s.db.Queries.UpdatePolicyModule(ctx, db.UpdatePolicyModuleParams{
		ID: id, Title: title, Description: sql.NullString{String: description, Valid: description != ""},
	})
}

// DeleteModule follows the policy-module retention setting. Soft mode marks
// the module deleted without breaking references; immediate mode applies the
// reference guards and permanently removes it.
func (s *PolicyModuleService) DeleteModule(ctx context.Context, tenantID, id int64, urn string, actorID int64) error {
	setting, err := s.db.Queries.GetRetentionSetting(ctx, EntityKindPolicyModule)
	if err != nil {
		return err
	}
	if setting.Mode == RetentionModeImmediate {
		return hardDeletePolicyModule(ctx, s.db.Queries, tenantID, id, urn)
	}
	_, err = s.db.Queries.SoftDeletePolicyModule(ctx, db.SoftDeletePolicyModuleParams{ID: id, DeletedBy: actorID})
	return err
}

// RestoreModule makes a soft-deleted module active again.
func (s *PolicyModuleService) RestoreModule(ctx context.Context, id int64) error {
	_, err := s.db.Queries.RestorePolicyModule(ctx, id)
	return err
}

// PermanentlyDeleteModule applies reference guards at the irreversible boundary.
func (s *PolicyModuleService) PermanentlyDeleteModule(ctx context.Context, tenantID, id int64, urn string) error {
	return hardDeletePolicyModule(ctx, s.db.Queries, tenantID, id, urn)
}

func hardDeletePolicyModule(ctx context.Context, q *db.Queries, tenantID, id int64, urn string) error {
	if n, err := q.CountLinksForModule(ctx, db.CountLinksForModuleParams{TenantID: tenantID, ModuleID: urn}); err != nil {
		return err
	} else if n > 0 {
		return ErrModuleReferenced
	}
	if n, err := q.CountBindingsByModule(ctx, sql.NullInt64{Int64: id, Valid: true}); err != nil {
		return err
	} else if n > 0 {
		return ErrModuleReferenced
	}
	if n, err := q.CountInstallationsByModule(ctx, id); err != nil {
		return err
	} else if n > 0 {
		return ErrModuleReferenced
	}
	return q.DeletePolicyModule(ctx, id)
}

// RevokeVersions revokes every deployable version of a module. Revoked
// versions must not run on agents; drafts remain intact so an editor can
// prepare and publish a replacement later. Repeating the operation is
// intentionally harmless when no published or superseded versions remain.
func (s *PolicyModuleService) RevokeVersions(ctx context.Context, moduleID int64) error {
	versions, err := s.db.Queries.ListVersionsByModule(ctx, moduleID)
	if err != nil {
		return err
	}
	for _, version := range versions {
		if version.State != "published" && version.State != "superseded" {
			continue
		}
		if err := s.Revoke(ctx, version.ID); err != nil {
			return err
		}
	}
	return nil
}

// GetModule loads one module row plus every version and its scripts,
// mapped onto the catalog/policymodules domain model.
func (s *PolicyModuleService) GetModule(ctx context.Context, id int64) (policymodules.Module, error) {
	row, err := s.db.Queries.GetPolicyModule(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return policymodules.Module{}, ErrModuleNotFound
		}
		return policymodules.Module{}, err
	}
	return s.hydrateModule(ctx, row)
}

// GetModuleByURN mirrors GetModule but looks up by the stable dotted-
// slug URN (the id callers outside this package actually know).
func (s *PolicyModuleService) GetModuleByURN(ctx context.Context, urn string) (policymodules.Module, error) {
	row, err := s.db.Queries.GetPolicyModuleByURN(ctx, urn)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return policymodules.Module{}, ErrModuleNotFound
		}
		return policymodules.Module{}, err
	}
	return s.hydrateModule(ctx, row)
}

// GetModuleRow loads the raw policy_modules row (not the hydrated
// catalog/policymodules.Module) by its dotted-slug URN. The module
// editor (Task 4.3) needs the raw row's numeric ID/TenantID/
// OwnerIdentityID/IsBundled for authz decisions (pkg/authz/modules.go's
// ModuleAccessInput) and for route params, which the hydrated Module
// type doesn't carry.
func (s *PolicyModuleService) GetModuleRow(ctx context.Context, urn string) (db.PolicyModule, error) {
	row, err := s.db.Queries.GetPolicyModuleByURN(ctx, urn)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.PolicyModule{}, ErrModuleNotFound
		}
		return db.PolicyModule{}, err
	}
	return row, nil
}

func (s *PolicyModuleService) GetModuleRowIncludingDeleted(ctx context.Context, urn string) (db.PolicyModule, error) {
	row, err := s.db.Queries.GetPolicyModuleByURNIncludingDeleted(ctx, urn)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.PolicyModule{}, ErrModuleNotFound
		}
		return db.PolicyModule{}, err
	}
	return row, nil
}

// ListModules returns every module visible to tenantID (its own
// tenant-authored modules plus every bundled module), each hydrated
// with versions + scripts. This is the read path for the Modules
// Library/Defaults/Sources pages -- it replaces the old hardcoded
// mock.go catalog slice.
func (s *PolicyModuleService) ListModules(ctx context.Context, tenantID int64) ([]policymodules.Module, error) {
	rows, err := s.db.Queries.ListVisibleModules(ctx, sql.NullInt64{Int64: tenantID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]policymodules.Module, 0, len(rows))
	for _, row := range rows {
		m, err := s.hydrateModule(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *PolicyModuleService) ListDeletedModules(ctx context.Context, tenantID int64) ([]policymodules.Module, error) {
	rows, err := s.db.Queries.ListDeletedVisibleModules(ctx, sql.NullInt64{Int64: tenantID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]policymodules.Module, 0, len(rows))
	for _, row := range rows {
		module, err := s.hydrateModule(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, module)
	}
	return out, nil
}

// ListBundled returns every bundled module (tenant_id NULL), hydrated
// with versions + scripts. Used to back catalog.SetCatalogProvider at
// server startup: the pure helpers in catalog/policymodules (
// CandidatesForPolicy, the extension-framework loader, the Policy
// Catalog table's candidate-module column) run outside any request's
// tenant context, so they see the tenant-agnostic bundled set rather
// than a specific tenant's full visible list (mirrors the old mock's
// single hardcoded catalog, minus the one tenant-authored fixture
// module it used to include).
func (s *PolicyModuleService) ListBundled(ctx context.Context) ([]policymodules.Module, error) {
	rows, err := s.db.Queries.ListBundledModules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]policymodules.Module, 0, len(rows))
	for _, row := range rows {
		m, err := s.hydrateModule(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *PolicyModuleService) hydrateModule(ctx context.Context, row db.PolicyModule) (policymodules.Module, error) {
	versionRows, err := s.db.Queries.ListVersionsByModule(ctx, row.ID)
	if err != nil {
		return policymodules.Module{}, err
	}
	m := policymodules.Module{
		ID:          row.ModuleUrn,
		Title:       row.Title,
		Description: row.Description.String,
		Scope:       "", // set from the newest version below, mirrors the mock's per-module Scope
		Origin:      moduleOrigin(row),
	}
	for _, v := range versionRows {
		mv, err := s.hydrateVersion(ctx, v.ID, mapVersionRow(v))
		if err != nil {
			return policymodules.Module{}, err
		}
		m.Versions = append(m.Versions, mv.ModuleVersion)
		if m.Scope == "" {
			m.Scope = mv.scope
		}
		if len(m.TargetOS) == 0 {
			m.TargetOS = mv.targetOS
		}
		if len(m.Satisfies) == 0 {
			m.Satisfies = mv.satisfies
		}
	}
	return m, nil
}

// moduleOrigin reads the origin column (migration 011: bundled |
// tenant | imported, backfilled from is_bundled), falling back to the
// legacy is_bundled mapping for any row with an empty origin.
func moduleOrigin(row db.PolicyModule) string {
	if row.Origin != "" {
		return row.Origin
	}
	if row.IsBundled {
		return "bundled"
	}
	return "tenant"
}

// versionRowShape is the common subset of GetPolicyModuleVersion's and
// ListVersionsByModule's row shapes (the latter adds publisher_name).
// mapVersionRow below normalizes both call sites onto it.
type versionRowShape struct {
	db.PolicyModuleVersion
	PublisherName sql.NullString
}

func mapVersionRow(v db.ListVersionsByModuleRow) versionRowShape {
	return versionRowShape{
		PolicyModuleVersion: db.PolicyModuleVersion{
			ID: v.ID, ModuleID: v.ModuleID, Version: v.Version, State: v.State,
			TargetOs: v.TargetOs, Scope: v.Scope, Satisfies: v.Satisfies,
			ParametersSchema: v.ParametersSchema, DependsOn: v.DependsOn, Conflicts: v.Conflicts,
			SandboxProfile: v.SandboxProfile, ReportSchema: v.ReportSchema, ManifestYaml: v.ManifestYaml,
			SignatureAlgo: v.SignatureAlgo, SignatureData: v.SignatureData, SignedBy: v.SignedBy,
			PublishedAt: v.PublishedAt, PublishedBy: v.PublishedBy, SupersededByVersion: v.SupersededByVersion,
			CreatedAt: v.CreatedAt,
		},
		PublisherName: v.PublisherName,
	}
}

// hydratedVersion carries the mapped policymodules.ModuleVersion plus a
// couple of fields the caller (hydrateModule) wants without
// re-parsing JSON.
type hydratedVersion struct {
	policymodules.ModuleVersion
	scope     string
	targetOS  []policymodules.TargetOS
	satisfies []string
}

func (s *PolicyModuleService) hydrateVersion(ctx context.Context, versionID int64, v versionRowShape) (hydratedVersion, error) {
	scriptRows, err := s.db.Queries.ListScriptsForVersion(ctx, versionID)
	if err != nil {
		return hydratedVersion{}, err
	}
	// Migration 012 replaced phase-keyed rows with named ones. For the
	// UI's phase strip / editor to keep working unmodified in this
	// backend-only checkpoint, a script whose name is one of the five
	// legacy phase strings maps back onto LifecycleScript.Phase (true for
	// every migrated row and every SetScript-written row, since both use
	// the phase string as the name). A script under a genuinely new,
	// non-phase name has no phase slot to render into yet and is
	// skipped here -- CP2+ replaces LifecycleScript-based rendering with
	// the named Script list directly.
	scripts := make([]policymodules.LifecycleScript, 0, len(scriptRows))
	for _, sc := range scriptRows {
		if !validLifecyclePhase(policymodules.LifecyclePhase(sc.Name)) {
			continue
		}
		scripts = append(scripts, policymodules.LifecycleScript{
			Phase:    policymodules.LifecyclePhase(sc.Name),
			Filename: sc.Name,
			Source:   sc.Source,
		})
	}

	var targetOS []policymodules.TargetOS
	_ = json.Unmarshal([]byte(v.TargetOs), &targetOS)
	var satisfies []string
	_ = json.Unmarshal([]byte(v.Satisfies), &satisfies)
	var deps []policymodules.Dependency
	if v.DependsOn.Valid {
		_ = json.Unmarshal([]byte(v.DependsOn.String), &deps)
	}
	var conflicts []string
	if v.Conflicts.Valid {
		_ = json.Unmarshal([]byte(v.Conflicts.String), &conflicts)
	}
	var sandbox policymodules.SandboxProfile
	_ = json.Unmarshal([]byte(v.SandboxProfile), &sandbox)

	publishedAt := ""
	if v.PublishedAt.Valid {
		publishedAt = v.PublishedAt.Time.Format("2006-01-02T15:04:05Z")
	}
	publishedBy := v.PublisherName.String
	if publishedBy == "" {
		publishedBy = v.SignedBy.String
	}

	mv := policymodules.ModuleVersion{
		Version:      v.Version,
		PublishedAt:  publishedAt,
		PublishedBy:  publishedBy,
		Sandbox:      sandbox,
		Dependencies: deps,
		Conflicts:    conflicts,
		Scripts:      scripts,
		ReportSchema: v.ReportSchema,
		Signature: policymodules.Signature{
			KeyID:  v.SignedBy.String,
			Algo:   v.SignatureAlgo.String,
			Bytes:  v.SignatureData.String,
			Signer: publishedBy,
		},
		Status: v.State,
	}
	return hydratedVersion{ModuleVersion: mv, scope: v.Scope, targetOS: targetOS, satisfies: satisfies}, nil
}

// ----------------------------------------------------------------------
// Version lifecycle
// ----------------------------------------------------------------------

// ModuleVersionFields is the mutable field set for a draft version --
// everything UpdatePolicyModuleVersionDraft can change. CreateDraft and
// UpdateDraft both take this shape so the module editor (next task) has
// one struct to fill in regardless of whether it's creating or editing.
type ModuleVersionFields struct {
	Version          string
	TargetOS         []policymodules.TargetOS
	Scope            string // "machine" | "user" | "both"
	Satisfies        []string
	ParametersSchema string
	DependsOn        []policymodules.Dependency
	Conflicts        []string
	SandboxProfile   policymodules.SandboxProfile
	ReportSchema     string
	ManifestYAML     string
}

func marshalOr(v any, fallback string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(b)
}

// FieldsFromVersionRow unmarshals a raw db.PolicyModuleVersion row into
// the editable ModuleVersionFields shape, mirroring hydrateVersion's
// JSON parsing. The module editor (Task 4.3) uses this to seed a
// partial-update call: load the current draft, apply just the
// caller's changed keys onto the returned struct, then pass it back
// through UpdateDraft.
func FieldsFromVersionRow(v db.PolicyModuleVersion) ModuleVersionFields {
	var targetOS []policymodules.TargetOS
	_ = json.Unmarshal([]byte(v.TargetOs), &targetOS)
	var satisfies []string
	_ = json.Unmarshal([]byte(v.Satisfies), &satisfies)
	var deps []policymodules.Dependency
	if v.DependsOn.Valid {
		_ = json.Unmarshal([]byte(v.DependsOn.String), &deps)
	}
	var conflicts []string
	if v.Conflicts.Valid {
		_ = json.Unmarshal([]byte(v.Conflicts.String), &conflicts)
	}
	var sandbox policymodules.SandboxProfile
	_ = json.Unmarshal([]byte(v.SandboxProfile), &sandbox)
	return ModuleVersionFields{
		Version:          v.Version,
		TargetOS:         targetOS,
		Scope:            v.Scope,
		Satisfies:        satisfies,
		ParametersSchema: v.ParametersSchema.String,
		DependsOn:        deps,
		Conflicts:        conflicts,
		SandboxProfile:   sandbox,
		ReportSchema:     v.ReportSchema,
		ManifestYAML:     v.ManifestYaml,
	}
}

// CreateDraft creates a new draft version for moduleID from scratch.
func (s *PolicyModuleService) CreateDraft(ctx context.Context, moduleID int64, fields ModuleVersionFields) (db.PolicyModuleVersion, error) {
	return s.db.Queries.CreatePolicyModuleVersion(ctx, db.CreatePolicyModuleVersionParams{
		ModuleID:         moduleID,
		Version:          fields.Version,
		State:            "draft",
		ManifestYaml:     fields.ManifestYAML,
		TargetOs:         marshalOr(fields.TargetOS, "[]"),
		Scope:            fields.Scope,
		Satisfies:        marshalOr(fields.Satisfies, "[]"),
		ParametersSchema: sql.NullString{String: fields.ParametersSchema, Valid: fields.ParametersSchema != ""},
		DependsOn:        sql.NullString{String: marshalOr(fields.DependsOn, "[]"), Valid: true},
		Conflicts:        sql.NullString{String: marshalOr(fields.Conflicts, "[]"), Valid: true},
		SandboxProfile:   marshalOr(fields.SandboxProfile, "{}"),
		ReportSchema:     fields.ReportSchema,
	})
}

// ForkLatestPublished creates a new draft version by copying the
// module's current published version's fields and scripts forward
// under a new version string. Returns ErrModuleNotFound if the module
// has no published version to fork from.
func (s *PolicyModuleService) ForkLatestPublished(ctx context.Context, moduleID int64, newVersion string) (db.PolicyModuleVersion, error) {
	src, err := s.db.Queries.GetLatestPublishedVersion(ctx, moduleID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.PolicyModuleVersion{}, ErrModuleNotFound
		}
		return db.PolicyModuleVersion{}, err
	}
	draft, err := s.db.Queries.CreatePolicyModuleVersion(ctx, db.CreatePolicyModuleVersionParams{
		ModuleID:         moduleID,
		Version:          newVersion,
		State:            "draft",
		ManifestYaml:     src.ManifestYaml,
		TargetOs:         src.TargetOs,
		Scope:            src.Scope,
		Satisfies:        src.Satisfies,
		ParametersSchema: src.ParametersSchema,
		DependsOn:        src.DependsOn,
		Conflicts:        src.Conflicts,
		SandboxProfile:   src.SandboxProfile,
		ReportSchema:     src.ReportSchema,
	})
	if err != nil {
		return db.PolicyModuleVersion{}, err
	}
	scripts, err := s.db.Queries.ListScriptsForVersion(ctx, src.ID)
	if err != nil {
		return db.PolicyModuleVersion{}, err
	}
	for _, sc := range scripts {
		// draft.ID was just created above, so its state is 'draft' --
		// the guard on UpsertModuleScriptGuarded cannot reject this.
		if _, err := s.db.Queries.UpsertModuleScriptGuarded(ctx, db.UpsertModuleScriptGuardedParams{
			VersionID: draft.ID, Name: sc.Name, Language: sc.Language, Source: sc.Source, Origin: sc.Origin, Seq: sc.Seq,
		}); err != nil {
			return db.PolicyModuleVersion{}, err
		}
	}
	conds, err := s.db.Queries.ListVersionConditions(ctx, src.ID)
	if err != nil {
		return db.PolicyModuleVersion{}, err
	}
	for _, cond := range conds {
		if _, err := s.db.Queries.CreateVersionConditionGuarded(ctx, db.CreateVersionConditionGuardedParams{
			VersionID: draft.ID, Kind: cond.Kind, ParamPath: cond.ParamPath,
			Operator: cond.Operator, ValueJson: cond.ValueJson,
			ScriptSource: cond.ScriptSource, ScriptRef: cond.ScriptRef, Seq: cond.Seq,
		}); err != nil {
			return db.PolicyModuleVersion{}, err
		}
	}
	if src.ConditionsMatchMode != "all" && src.ConditionsMatchMode != "" {
		if _, err := s.db.Queries.UpdateVersionConditionsMatchMode(ctx, db.UpdateVersionConditionsMatchModeParams{
			ID: draft.ID, ConditionsMatchMode: src.ConditionsMatchMode,
		}); err != nil {
			return db.PolicyModuleVersion{}, err
		}
	}
	return draft, nil
}

// UpdateDraft mutates a version's fields. Returns ErrVersionNotDraft if
// the version's state isn't 'draft' -- published/superseded/revoked
// versions are immutable (ADR-007). The UPDATE itself carries a
// WHERE state='draft' guard, so the rule holds even if a Publish races
// between this function's pre-read and its write: the guarded UPDATE
// matches nothing and we map the resulting sql.ErrNoRows to the typed
// error.
func (s *PolicyModuleService) UpdateDraft(ctx context.Context, versionID int64, fields ModuleVersionFields) (db.PolicyModuleVersion, error) {
	row, err := s.db.Queries.UpdatePolicyModuleVersionDraft(ctx, db.UpdatePolicyModuleVersionDraftParams{
		ID:               versionID,
		Version:          fields.Version,
		TargetOs:         marshalOr(fields.TargetOS, "[]"),
		Scope:            fields.Scope,
		Satisfies:        marshalOr(fields.Satisfies, "[]"),
		ParametersSchema: sql.NullString{String: fields.ParametersSchema, Valid: fields.ParametersSchema != ""},
		DependsOn:        sql.NullString{String: marshalOr(fields.DependsOn, "[]"), Valid: true},
		Conflicts:        sql.NullString{String: marshalOr(fields.Conflicts, "[]"), Valid: true},
		SandboxProfile:   marshalOr(fields.SandboxProfile, "{}"),
		ReportSchema:     fields.ReportSchema,
		ManifestYaml:     fields.ManifestYAML,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Either the version doesn't exist or it isn't a draft.
			// Distinguish for a precise error: missing id keeps the raw
			// not-found; existing-but-frozen gets the typed error.
			if _, gerr := s.db.Queries.GetPolicyModuleVersion(ctx, versionID); gerr == nil {
				return db.PolicyModuleVersion{}, ErrVersionNotDraft
			}
			return db.PolicyModuleVersion{}, sql.ErrNoRows
		}
		return db.PolicyModuleVersion{}, err
	}
	return row, nil
}

// Publish transitions a draft version to published. Requires an
// apply-phase script to exist (INV-M3; ErrPublishRequiresApplyScript
// otherwise). Any currently-published version of the same module is
// marked superseded (state='superseded', superseded_by_version set to
// the newly-published version string) -- at most one published version
// per module at a time.
//
// The supersede + publish state transitions run inside one transaction
// with state-guarded UPDATEs, so the invariant holds under concurrency
// (two racing Publishes: exactly one wins the WHERE state='draft' guard,
// the loser gets ErrVersionNotDraft) and under partial failure (a crash
// mid-sequence rolls the whole transaction back -- never zero published
// versions where there was one).
func (s *PolicyModuleService) Publish(ctx context.Context, versionID int64, publishedBy int64) error {
	cur, err := s.db.Queries.GetPolicyModuleVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if cur.State != "draft" {
		return ErrVersionNotDraft
	}
	scripts, err := s.db.Queries.ListScriptsForVersion(ctx, versionID)
	if err != nil {
		return err
	}
	hasApply := false
	for _, sc := range scripts {
		if sc.Name == string(policymodules.PhaseApply) {
			hasApply = true
			break
		}
	}
	if !hasApply {
		return ErrPublishRequiresApplyScript
	}

	tx, err := s.db.BeginTx()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit
	q := s.db.Queries.WithTx(tx)

	// Publish first (state-guarded): if another Publish won the race
	// since the pre-read above, this matches 0 rows and we bail with
	// the typed error before touching anything else.
	n, err := q.PublishModuleVersion(ctx, db.PublishModuleVersionParams{
		ID: versionID, PublishedBy: sql.NullInt64{Int64: publishedBy, Valid: true},
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrVersionNotDraft
	}
	// Supersede every other currently-published version of this module
	// (normally zero or one) in a single guarded statement.
	if _, err := q.SupersedeCurrentPublishedVersion(ctx, db.SupersedeCurrentPublishedVersionParams{
		ModuleID:            cur.ModuleID,
		ExcludeID:           versionID,
		SupersededByVersion: sql.NullString{String: cur.Version, Valid: true},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// Revoke marks a published or superseded version revoked. Agents must
// not run a revoked version (ADR-007). Drafts cannot be revoked --
// delete them instead (ErrRevokeInvalidState); revoking an
// already-revoked version is also rejected with the same error (no-op
// disguised as success would hide caller bugs).
func (s *PolicyModuleService) Revoke(ctx context.Context, versionID int64) error {
	n, err := s.db.Queries.RevokeModuleVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if n == 0 {
		// Distinguish "no such version" from "wrong state".
		if _, gerr := s.db.Queries.GetPolicyModuleVersion(ctx, versionID); gerr != nil {
			return gerr
		}
		return ErrRevokeInvalidState
	}
	return nil
}

// ----------------------------------------------------------------------
// Scripts
// ----------------------------------------------------------------------

// Deprecated: SetScript is a thin compatibility wrapper over UpsertScript
// for callers still speaking the old phase-keyed API (migration 008).
// filename is accepted but discarded -- migration 012's
// policy_module_scripts has no filename column; the script's name is the
// phase string itself, matching what migration 012's copy-forward did
// for pre-existing rows. New callers should use UpsertScript directly.
func (s *PolicyModuleService) SetScript(ctx context.Context, versionID int64, phase policymodules.LifecyclePhase, filename, source string) (db.PolicyModuleScript, error) {
	if !validLifecyclePhase(phase) {
		return db.PolicyModuleScript{}, ErrInvalidLifecyclePhase
	}
	lang := string(policymodules.LangSh)
	if phase.Runtime() == policymodules.RuntimeWASM {
		lang = string(policymodules.LangPython)
	}
	sc, err := s.UpsertScript(ctx, versionID, policymodules.Script{
		Name: string(phase), Language: lang, Source: source, Origin: "custom",
	})
	if err != nil {
		return db.PolicyModuleScript{}, err
	}
	return db.PolicyModuleScript{
		VersionID: versionID, Name: sc.Name, Language: sc.Language, Source: sc.Source, Origin: sc.Origin, Seq: int64(sc.Seq),
	}, nil
}

// ----------------------------------------------------------------------
// Bundled-module seeding
// ----------------------------------------------------------------------

// seedScript is one lifecycle script in the bundled seed table below.
type seedScript struct {
	Phase    policymodules.LifecyclePhase
	Filename string
	Source   string
}

// seedVersion is one version of a bundled seed module. Only the fields
// SeedBundled needs are here (bundled modules are always
// state=published, signed by the well-known bundled key).
type seedVersion struct {
	Version      string
	Scope        string
	TargetOS     []policymodules.TargetOS
	Satisfies    []string
	DependsOn    []policymodules.Dependency
	Conflicts    []string
	Sandbox      policymodules.SandboxProfile
	ReportSchema string
	Scripts      []seedScript
}

type seedModule struct {
	URN         string
	Title       string
	Description string
	Versions    []seedVersion // newest first; only Versions[0] is seeded (SeedBundled seeds the current published version)
}

// bundledSeed is the DB-backed replacement for catalog/policymodules/
// mock.go's hardcoded bundled catalog entries. Content is carried
// forward verbatim (module IDs, satisfies URNs, sandbox profiles,
// script bodies) so SeedBundled's output is behaviourally identical to
// what the mock used to serve -- only the storage moved.
var bundledSeed = []seedModule{
	{
		URN:         "pluris.sshd.password-auth-disable",
		Title:       "Disable SSH password authentication",
		Description: "Drops a snippet into /etc/ssh/sshd_config.d/ disabling password auth and reloads sshd. Idempotent; safe to re-apply.",
		Versions: []seedVersion{
			{
				Version:  "1.2.0",
				Scope:    "machine",
				TargetOS: []policymodules.TargetOS{policymodules.OSLinux},
				// sec.opt.null-passwords-console is the closest catalog entry:
				// both are about restricting password-based authentication
				// over network services, and its own LinuxImpl already names
				// sshd explicitly. No catalog entry maps 1:1 onto "SSH
				// password auth" specifically -- OpenSSH Server on modern
				// Windows is configured via sshd_config directly, with no
				// classic-GPMC ADMX template, so there is no exact Windows
				// GP lever to cite for a dedicated entry.
				Satisfies: []string{"sec.opt.null-passwords-console"},
				DependsOn: []policymodules.Dependency{
					{ModuleID: "pluris.sshd.base-config", VersionConstraint: ">=1.0.0 <2.0.0"},
				},
				Conflicts: []string{"pluris.sshd.password-auth-allow"},
				Sandbox: policymodules.SandboxProfile{
					FsRead: []string{"/etc/ssh/", "/proc/version"}, FsWrite: []string{"/etc/ssh/sshd_config.d/"}, User: "root",
				},
				ReportSchema: `{"type":"object","properties":{"sshd_running":{"type":"boolean"},"password_auth":{"type":"boolean"}}}`,
				Scripts: []seedScript{
					{Phase: policymodules.PhaseApply, Filename: "enforce.sh", Source: "" +
						"#!/usr/bin/env bash\nset -euo pipefail\n" +
						"# Disable password auth idempotently.\n" +
						"snippet=/etc/ssh/sshd_config.d/40-pluris-no-password.conf\n" +
						"want='PasswordAuthentication no'\n" +
						"echo \"$want\" > \"$snippet\"\nchmod 0644 \"$snippet\"\nsystemctl reload sshd\n"},
					{Phase: policymodules.PhaseDisable, Filename: "disable.sh", Source: "" +
						"#!/usr/bin/env bash\nset -euo pipefail\n" +
						"snippet=/etc/ssh/sshd_config.d/40-pluris-no-password.conf\n" +
						"sed -i 's/^/# pluris-disabled: /' \"$snippet\" || true\nsystemctl reload sshd\n"},
					{Phase: policymodules.PhaseUninstall, Filename: "rollback.sh", Source: "" +
						"#!/usr/bin/env bash\nset -euo pipefail\n" +
						"rm -f /etc/ssh/sshd_config.d/40-pluris-no-password.conf\nsystemctl reload sshd\n"},
					{Phase: policymodules.PhaseValidate, Filename: "validate.wasm", Source: "(WASM module — read sshd_config, return JSON state)"},
				},
			},
		},
	},
	{
		URN:         "pluris.sshd.base-config",
		Title:       "SSH base configuration",
		Description: "Ensures /etc/ssh/sshd_config.d/ exists and is loaded by sshd. Required by every other SSH-policy module.",
		Versions: []seedVersion{
			{
				Version:  "1.0.3",
				Scope:    "machine",
				TargetOS: []policymodules.TargetOS{policymodules.OSLinux},
				// Pure infrastructure (ensures the config-include mechanism
				// exists for the other SSH modules); not itself an
				// admin-facing policy decision, so it satisfies nothing.
				Satisfies: nil,
				Sandbox: policymodules.SandboxProfile{
					FsWrite: []string{"/etc/ssh/sshd_config.d/", "/etc/ssh/sshd_config"}, User: "root",
				},
				Scripts: []seedScript{
					{Phase: policymodules.PhaseApply, Filename: "enforce.sh", Source: "" +
						"#!/usr/bin/env bash\nset -euo pipefail\n" +
						"mkdir -p /etc/ssh/sshd_config.d\n" +
						"grep -q 'Include /etc/ssh/sshd_config.d/' /etc/ssh/sshd_config || " +
						"echo 'Include /etc/ssh/sshd_config.d/*.conf' >> /etc/ssh/sshd_config\n"},
				},
			},
		},
	},
	{
		URN:         "pluris.sshd.password-auth-allow",
		Title:       "Allow SSH password authentication",
		Description: "Explicitly enables password auth (for migration windows). Conflicts with the disable module.",
		Versions: []seedVersion{
			{
				Version:  "1.0.0",
				Scope:    "machine",
				TargetOS: []policymodules.TargetOS{policymodules.OSLinux},
				// Same catalog entry as the disable module (see its comment
				// above) -- the two are opposite implementations of the
				// same policy, which is exactly what Conflicts encodes.
				Satisfies: []string{"sec.opt.null-passwords-console"},
				DependsOn: []policymodules.Dependency{
					{ModuleID: "pluris.sshd.base-config", VersionConstraint: ">=1.0.0 <2.0.0"},
				},
				Conflicts: []string{"pluris.sshd.password-auth-disable"},
				Sandbox:   policymodules.SandboxProfile{FsWrite: []string{"/etc/ssh/sshd_config.d/"}, User: "root"},
				Scripts: []seedScript{
					{Phase: policymodules.PhaseApply, Filename: "enforce.sh",
						Source: "echo 'PasswordAuthentication yes' > /etc/ssh/sshd_config.d/40-pluris-allow-password.conf"},
				},
			},
		},
	},
	{
		URN:         "pluris.user.screen-lock-timeout",
		Title:       "Set screen lock timeout (user)",
		Description: "Configures GNOME / Plasma idle-lock to a parameterised timeout. User-scope; runs as $target_user.",
		Versions: []seedVersion{
			{
				Version:  "2.0.1",
				Scope:    "user",
				TargetOS: []policymodules.TargetOS{policymodules.OSLinux},
				// u.personalization.screensaver is the closest catalog entry:
				// both concern the same GNOME idle-delay/screensaver-lock
				// dconf keys (org.gnome.desktop.session idle-delay,
				// org.gnome.desktop.screensaver lock-*) -- this module sets
				// the timeout value the catalog policy locks in place.
				Satisfies: []string{"u.personalization.screensaver"},
				Sandbox: policymodules.SandboxProfile{
					FsRead:  []string{"/etc/os-release"},
					FsWrite: []string{"$HOME/.config/dconf/", "$HOME/.config/plasma-locale-settings"},
					User:    "$target_user",
				},
				Scripts: []seedScript{
					{Phase: policymodules.PhaseApply, Filename: "enforce.sh", Source: "# read PLURIS_PARAMS_FD for idle-timeout, write dconf db"},
					{Phase: policymodules.PhaseUninstall, Filename: "rollback.sh", Source: "# remove pluris dconf overrides"},
					{Phase: policymodules.PhaseValidate, Filename: "validate.wasm", Source: "(WASM — return current idle timeout)"},
				},
			},
		},
	},
	{
		URN:         "pluris.cert.client-pin",
		Title:       "Pin corporate CA on workstations",
		Description: "Installs the corporate Root CA into the system trust store; periodically refreshes the CRL.",
		Versions: []seedVersion{
			{
				Version:  "1.0.0",
				Scope:    "machine",
				TargetOS: []policymodules.TargetOS{policymodules.OSLinux},
				// pki.ca.trusted ("Trusted Root Certification Authorities")
				// is the real Windows GP setting this implements -- pinning
				// the corporate CA is deploying it into the trust store via
				// the same update-ca-certificates/update-ca-trust mechanism
				// that catalog entry already documents.
				Satisfies: []string{"pki.ca.trusted"},
				Sandbox: policymodules.SandboxProfile{
					FsRead: []string{"/etc/os-release"}, FsWrite: []string{"/etc/pki/ca-trust/source/anchors/", "/usr/local/share/ca-certificates/"},
					NetEgress: []string{"https://crl.tenant.local/"}, User: "root",
				},
				ReportSchema: `{"type":"object","properties":{"crl_age_seconds":{"type":"integer"},"ca_present":{"type":"boolean"}}}`,
				Scripts: []seedScript{
					{Phase: policymodules.PhaseApply, Filename: "enforce.sh", Source: "# install CA, refresh CRL"},
					{Phase: policymodules.PhaseUninstall, Filename: "rollback.sh", Source: "# remove CA, regenerate trust store"},
					{Phase: policymodules.PhaseReport, Filename: "report.wasm", Source: "(WASM — return CRL freshness JSON)"},
				},
			},
		},
	},
}

// SeedBundled idempotently populates the bundled modules (is_bundled=
// TRUE, owner NULL, tenant NULL, state published), replacing the old
// mock.go hardcoded catalog. Safe to call on every server boot / first
// request: existing rows (matched by module_urn) are left untouched,
// and a UNIQUE-constraint race from a concurrent first request is
// tolerated the same way EnsureBuiltins tolerates it.
func (s *PolicyModuleService) SeedBundled(ctx context.Context) error {
	for _, sm := range bundledSeed {
		if _, err := s.db.Queries.GetPolicyModuleByURN(ctx, sm.URN); err == nil {
			continue // already seeded
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		row, err := s.db.Queries.CreatePolicyModule(ctx, db.CreatePolicyModuleParams{
			ModuleUrn: sm.URN, Title: sm.Title,
			Description: sql.NullString{String: sm.Description, Valid: sm.Description != ""},
			IsBundled:   true,
		})
		if err != nil {
			if isUniqueErr(err) {
				continue // lost the race to a concurrent seeder
			}
			return err
		}
		for _, sv := range sm.Versions {
			ver, err := s.db.Queries.CreatePolicyModuleVersion(ctx, db.CreatePolicyModuleVersionParams{
				ModuleID:       row.ID,
				Version:        sv.Version,
				State:          "draft", // published via PublishModuleVersion below, same as any other version
				ManifestYaml:   "",
				TargetOs:       marshalOr(sv.TargetOS, "[]"),
				Scope:          sv.Scope,
				Satisfies:      marshalOr(sv.Satisfies, "[]"),
				DependsOn:      sql.NullString{String: marshalOr(sv.DependsOn, "[]"), Valid: true},
				Conflicts:      sql.NullString{String: marshalOr(sv.Conflicts, "[]"), Valid: true},
				SandboxProfile: marshalOr(sv.Sandbox, "{}"),
				ReportSchema:   sv.ReportSchema,
			})
			if err != nil {
				return err
			}
			for _, sc := range sv.Scripts {
				lang := string(policymodules.LangSh)
				if sc.Phase.Runtime() == policymodules.RuntimeWASM {
					lang = string(policymodules.LangPython)
				}
				// ver.State is 'draft' at this point (published below),
				// so the guard on UpsertModuleScriptGuarded cannot reject
				// this seed write.
				if _, err := s.db.Queries.UpsertModuleScriptGuarded(ctx, db.UpsertModuleScriptGuardedParams{
					VersionID: ver.ID, Name: string(sc.Phase), Language: lang, Source: sc.Source, Origin: "custom", Seq: 0,
				}); err != nil {
					return err
				}
			}
			if _, err := s.db.Queries.PublishModuleVersion(ctx, db.PublishModuleVersionParams{ID: ver.ID}); err != nil {
				return err
			}
		}
	}
	return nil
}
