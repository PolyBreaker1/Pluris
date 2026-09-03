package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// Task 5.2 — the real backend for Configuration Groups. Replaces
// catalog/configgroups' retired in-memory mock: CRUD for groups,
// assignments (who a group targets), and bindings (which policies +
// values a group carries), plus the parameter_values validation rules
// against a bound module's parameters_schema.
//
// console/handlers/policy_picker.go's "add policy" quick-assign flow
// (the only pre-existing live writer to these tables) is reconciled
// onto AssignPolicyDirect below so there is exactly one write path.

var (
	// ErrConfigGroupNotFound covers both "no such id" and "id belongs
	// to a different tenant" -- cross-tenant reads fail closed exactly
	// like resolveTenantDepGroup's pattern for dependency groups.
	ErrConfigGroupNotFound = errors.New("configuration group not found")
	// ErrInvalidTargetType is returned when a caller names a target
	// kind the assignments table's CHECK constraint doesn't allow, or
	// (from the TargetKind mapping) a kind that has no target_type
	// equivalent at all (configuration_group, regex).
	ErrInvalidTargetType = errors.New("invalid or unsupported target type")
	// ErrTargetNotFound is returned when AddAssignment's cheap
	// existence lookup can't find the named asset/identity/group.
	ErrTargetNotFound = errors.New("assignment target not found")
	// ErrInvalidState is returned when a binding's state isn't one of
	// the configuration_group_bindings CHECK's three values.
	ErrInvalidState = errors.New("invalid binding state")
)

// bindingStates mirrors the configuration_group_bindings.state CHECK
// constraint (db/schema/001_initial.sql).
var bindingStates = map[string]bool{
	"enabled":        true,
	"disabled":       true,
	"not_configured": true,
}

// assignmentTargetTypes mirrors the configuration_group_assignments.
// target_type CHECK constraint.
var assignmentTargetTypes = map[string]bool{
	"asset":    true,
	"identity": true,
	"group":    true,
	"site":     true,
	"tenant":   true,
}

// ConfigGroupService owns every read/write against configuration_groups,
// configuration_group_assignments, and configuration_group_bindings.
type ConfigGroupService struct {
	db        *database.Database
	moduleSvc *PolicyModuleService
}

// NewConfigGroupService constructs the service. moduleSvc resolves a
// binding's effective module (override, else the policy's tenant-visible
// default) so parameter_values can be checked against that module's
// parameters_schema -- see resolveParamSchema.
func NewConfigGroupService(db *database.Database, moduleSvc *PolicyModuleService) *ConfigGroupService {
	return &ConfigGroupService{db: db, moduleSvc: moduleSvc}
}

// isUniqueErr (SQLite UNIQUE-violation matcher, so idempotent create
// paths don't bubble up as 500s) is already defined package-wide in
// dependencygroups.go -- reused here rather than redeclared.

// ---------------------------------------------------------------------
// Groups (General tab / list / create / delete)
// ---------------------------------------------------------------------

// Create makes a new, tenant-scoped Configuration Group. Real groups no
// longer carry a single TargetKind/TargetRef (that was the mock's
// simplification) -- targeting is now the many-to-many assignments
// table, so Create only takes name/description/enabled. scope stays
// "both": the configuration_groups.scope column predates the
// assignments table and nothing reads it for a real group's targeting
// decision anymore (assignments + each binding's own policy scope
// govern that); "both" is the least restrictive value satisfying the
// NOT NULL CHECK.
func (s *ConfigGroupService) Create(ctx context.Context, tenantID int64, name, description string, enabled bool) (db.ConfigurationGroup, error) {
	if strings.TrimSpace(name) == "" {
		return db.ConfigurationGroup{}, fmt.Errorf("%w: name is required", ErrFieldValidation)
	}
	return s.db.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{
		TenantID:    tenantID,
		Name:        name,
		Description: sql.NullString{String: description, Valid: description != ""},
		Scope:       "both",
		Disabled:    !enabled,
	})
}

// Get fetches one group, verifying tenant ownership (cross-tenant reads
// return ErrConfigGroupNotFound, mirroring resolveTenantDepGroup's
// fail-closed behavior for dependency groups).
func (s *ConfigGroupService) Get(ctx context.Context, tenantID, id int64) (db.ConfigurationGroup, error) {
	row, err := s.db.Queries.GetConfigurationGroup(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.ConfigurationGroup{}, ErrConfigGroupNotFound
		}
		return db.ConfigurationGroup{}, err
	}
	if row.TenantID != tenantID {
		return db.ConfigurationGroup{}, ErrConfigGroupNotFound
	}
	return row, nil
}

// ListByTenant returns every Configuration Group for tenantID, name-
// ordered, for the list page.
func (s *ConfigGroupService) ListByTenant(ctx context.Context, tenantID int64) ([]db.ConfigurationGroup, error) {
	return s.db.Queries.ListConfigurationGroupsByTenant(ctx, db.ListConfigurationGroupsByTenantParams{
		TenantID: tenantID, Limit: 10000, Offset: 0,
	})
}

func (s *ConfigGroupService) ListDeletedByTenant(ctx context.Context, tenantID int64) ([]db.ConfigurationGroup, error) {
	return s.db.Queries.ListDeletedConfigurationGroupsByTenant(ctx, db.ListDeletedConfigurationGroupsByTenantParams{
		TenantID: tenantID, Limit: 10000, Offset: 0,
	})
}

// UpdateFields applies a partial name/description/enabled update (the
// General tab's inline-edit field API -- see
// console/handlers/field_api.go's FieldUpdateRequest/Response shape,
// which console/handlers/config_groups.go's ConfigGroupFieldUpdate
// binds this to). Unknown keys and an empty name both fail with
// ErrFieldValidation so the handler can map them to 400.
func (s *ConfigGroupService) UpdateFields(ctx context.Context, tenantID, id int64, fields map[string]string) ([]string, error) {
	row, err := s.Get(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	name := row.Name
	description := row.Description.String
	disabled := row.Disabled

	updated := make([]string, 0, len(fields))
	for key, raw := range fields {
		switch key {
		case "name":
			if strings.TrimSpace(raw) == "" {
				return nil, fmt.Errorf("%w: name cannot be empty", ErrFieldValidation)
			}
			name = raw
		case "description":
			description = raw
		case "enabled":
			b, perr := strconv.ParseBool(raw)
			if perr != nil {
				return nil, fmt.Errorf("%w: enabled must be a boolean", ErrFieldValidation)
			}
			disabled = !b
		default:
			return nil, fmt.Errorf("%w: unknown field %q", ErrFieldValidation, key)
		}
		updated = append(updated, key)
	}

	if _, err := s.db.Queries.UpdateConfigurationGroup(ctx, db.UpdateConfigurationGroupParams{
		ID:          id,
		Name:        name,
		Description: sql.NullString{String: description, Valid: description != ""},
		Scope:       row.Scope,
		Disabled:    disabled,
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

// Delete removes a group; ON DELETE CASCADE on both child tables
// (db/schema/001_initial.sql) takes its bindings and assignments with
// it, so no explicit child cleanup is needed here.
func (s *ConfigGroupService) Delete(ctx context.Context, tenantID, id, actorID int64) error {
	row, err := s.db.Queries.GetConfigurationGroupIncludingDeleted(ctx, id)
	if err != nil || row.TenantID != tenantID {
		return ErrConfigGroupNotFound
	}
	setting, err := s.db.Queries.GetRetentionSetting(ctx, EntityKindConfigurationGroup)
	if err != nil {
		return err
	}
	if setting.Mode == RetentionModeImmediate {
		return s.db.Queries.DeleteConfigurationGroup(ctx, id)
	}
	_, err = s.db.Queries.SoftDeleteConfigurationGroup(ctx, db.SoftDeleteConfigurationGroupParams{DeletedBy: actorID, ID: id, TenantID: tenantID})
	return err
}

func (s *ConfigGroupService) Restore(ctx context.Context, tenantID, id int64) error {
	_, err := s.db.Queries.RestoreConfigurationGroup(ctx, db.RestoreConfigurationGroupParams{ID: id, TenantID: tenantID})
	return err
}

func (s *ConfigGroupService) PermanentlyDelete(ctx context.Context, tenantID, id int64) error {
	row, err := s.db.Queries.GetConfigurationGroupIncludingDeleted(ctx, id)
	if err != nil || row.TenantID != tenantID {
		return ErrConfigGroupNotFound
	}
	return s.db.Queries.DeleteConfigurationGroup(ctx, id)
}

// FindOrCreateByName returns the tenant's group with the given name,
// creating it if absent. Used by AssignPolicyDirect's "Direct - <entity>"
// find-or-create semantics (the one behavior the old
// console/handlers/policy_picker.go write path depended on).
func (s *ConfigGroupService) FindOrCreateByName(ctx context.Context, tenantID int64, name, description, scope string) (db.ConfigurationGroup, error) {
	cg, err := s.db.Queries.GetConfigurationGroupByName(ctx, db.GetConfigurationGroupByNameParams{
		TenantID: tenantID, Name: name,
	})
	if err == nil {
		return cg, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return db.ConfigurationGroup{}, err
	}
	return s.db.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{
		TenantID:    tenantID,
		Name:        name,
		Description: sql.NullString{String: description, Valid: description != ""},
		Scope:       scope,
		Disabled:    false,
	})
}

// AssignPolicyDirect implements the "Add policy" quick-assign flow
// shared by AssetPolicyAddSubmit/UserPolicyAddSubmit
// (console/handlers/policy_picker.go): find-or-create the
// "Direct - <entity>" group, bind the policy with unconfigured
// ("{}") values, and assign the group to the target. Idempotent on
// repeats via isUniqueErr.
//
// Deliberately skips AddBinding/AddAssignment's stricter validation
// (schema checking, target-existence lookups): the caller has already
// resolved and validated both the policy and the target before calling
// in, this flow always writes empty parameter_values (nothing of
// substance to validate against a schema), and the DB's own UNIQUE
// constraints are what make it idempotent -- exactly the behavior the
// pre-reconciliation handler code implemented directly against
// h.db.Queries. Keeping it a distinct, lenient method (rather than
// routing through AddBinding/AddAssignment) preserves that behavior for
// policy_picker_test.go without a special-case flag threaded through
// the general-purpose methods.
func (s *ConfigGroupService) AssignPolicyDirect(ctx context.Context, tenantID int64, groupName, groupDescription, scope, policyURN, targetType string, targetID int64) (db.ConfigurationGroup, error) {
	cg, err := s.FindOrCreateByName(ctx, tenantID, groupName, groupDescription, scope)
	if err != nil {
		return db.ConfigurationGroup{}, err
	}
	if _, err := s.db.Queries.CreateConfigurationGroupBinding(ctx, db.CreateConfigurationGroupBindingParams{
		ConfigurationGroupID: cg.ID,
		PolicyUrn:            policyURN,
		State:                "enabled",
		ParameterValues:      sql.NullString{String: "{}", Valid: true},
	}); err != nil && !isUniqueErr(err) {
		return db.ConfigurationGroup{}, err
	}
	if _, err := s.db.Queries.CreateConfigurationGroupAssignment(ctx, db.CreateConfigurationGroupAssignmentParams{
		ConfigurationGroupID: cg.ID,
		TargetType:           targetType,
		TargetID:             targetID,
	}); err != nil && !isUniqueErr(err) {
		return db.ConfigurationGroup{}, err
	}
	return cg, nil
}

// ---------------------------------------------------------------------
// Assignments (Assignments tab)
// ---------------------------------------------------------------------

// KindToTargetType maps a TargetPickerDialog kind onto the
// configuration_group_assignments.target_type CHECK vocabulary
// (asset/identity/group/site/tenant). computer -> asset, user ->
// identity, computer_group/user_group -> group (the assignments table
// has no distinct computer-group/user-group split -- see
// pkg/services/targets.go's groupTargets TODO on the same limitation).
// configuration_group and regex have no target_type equivalent: the
// CHECK constraint doesn't list "configuration_group" (nesting one CG
// inside another's assignments isn't implemented at the assignment
// layer yet) and regex targets aren't implemented at all -- both map to
// ErrInvalidTargetType so the detail page's allowedKinds list simply
// excludes them (see web/templates/config_groups.templ).
func KindToTargetType(k configgroups.TargetKind) (string, error) {
	switch k {
	case configgroups.KindComputer:
		return "asset", nil
	case configgroups.KindUser:
		return "identity", nil
	case configgroups.KindComputerGroup, configgroups.KindUserGroup:
		return "group", nil
	}
	return "", ErrInvalidTargetType
}

// AddAssignment assigns groupID to one target. targetType must be in
// assignmentTargetTypes; for asset/identity/group kinds the target's
// existence is verified with a cheap row lookup (defense against a
// hand-crafted POST naming a numeric id that doesn't exist -- the
// TargetPickerDialog only ever emits ids it read from a real catalog
// row, so this is a belt-and-braces check, not the primary UX guard).
// site/tenant aren't checked: sites and tenants aren't targets the
// TargetPickerDialog can pick from in this task, so there is no cheap
// lookup wired up for them yet.
func (s *ConfigGroupService) AddAssignment(ctx context.Context, tenantID, groupID int64, targetType string, targetID int64, priority int64, enforced bool) (db.ConfigurationGroupAssignment, error) {
	if _, err := s.Get(ctx, tenantID, groupID); err != nil {
		return db.ConfigurationGroupAssignment{}, err
	}
	if !assignmentTargetTypes[targetType] {
		return db.ConfigurationGroupAssignment{}, ErrInvalidTargetType
	}
	switch targetType {
	case "asset":
		if _, err := s.db.Queries.GetAsset(ctx, targetID); err != nil {
			return db.ConfigurationGroupAssignment{}, ErrTargetNotFound
		}
	case "identity":
		if _, err := s.db.Queries.GetIdentity(ctx, targetID); err != nil {
			return db.ConfigurationGroupAssignment{}, ErrTargetNotFound
		}
	case "group":
		if _, err := s.db.Queries.GetGroup(ctx, targetID); err != nil {
			return db.ConfigurationGroupAssignment{}, ErrTargetNotFound
		}
	}
	row, err := s.db.Queries.CreateConfigurationGroupAssignment(ctx, db.CreateConfigurationGroupAssignmentParams{
		ConfigurationGroupID: groupID,
		TargetType:           targetType,
		TargetID:             targetID,
		Priority:             priority,
		Enforced:             enforced,
	})
	if err != nil && isUniqueErr(err) {
		// Already assigned -- idempotent no-op, return the existing row.
		existing, lerr := s.db.Queries.ListAssignmentsByGroup(ctx, groupID)
		if lerr != nil {
			return db.ConfigurationGroupAssignment{}, err
		}
		for _, a := range existing {
			if a.TargetType == targetType && a.TargetID == targetID {
				return a, nil
			}
		}
	}
	return row, err
}

// UpdateAssignment changes priority/enforced on an existing assignment.
// target_type/target_id are immutable (remove + re-add to retarget).
func (s *ConfigGroupService) UpdateAssignment(ctx context.Context, tenantID, groupID, assignmentID int64, priority int64, enforced bool) (db.ConfigurationGroupAssignment, error) {
	if _, err := s.Get(ctx, tenantID, groupID); err != nil {
		return db.ConfigurationGroupAssignment{}, err
	}
	row, err := s.db.Queries.GetConfigurationGroupAssignment(ctx, assignmentID)
	if err != nil || row.ConfigurationGroupID != groupID {
		return db.ConfigurationGroupAssignment{}, ErrConfigGroupNotFound
	}
	return s.db.Queries.UpdateConfigurationGroupAssignment(ctx, db.UpdateConfigurationGroupAssignmentParams{
		ID: assignmentID, Priority: priority, Enforced: enforced,
	})
}

// RemoveAssignment deletes one assignment, verifying it belongs to
// groupID (which itself is verified tenant-owned).
func (s *ConfigGroupService) RemoveAssignment(ctx context.Context, tenantID, groupID, assignmentID int64) error {
	if _, err := s.Get(ctx, tenantID, groupID); err != nil {
		return err
	}
	row, err := s.db.Queries.GetConfigurationGroupAssignment(ctx, assignmentID)
	if err != nil || row.ConfigurationGroupID != groupID {
		return ErrConfigGroupNotFound
	}
	return s.db.Queries.DeleteConfigurationGroupAssignment(ctx, assignmentID)
}

// ListAssignments returns every assignment for groupID, tenant-verified.
func (s *ConfigGroupService) ListAssignments(ctx context.Context, tenantID, groupID int64) ([]db.ConfigurationGroupAssignment, error) {
	if _, err := s.Get(ctx, tenantID, groupID); err != nil {
		return nil, err
	}
	return s.db.Queries.ListAssignmentsByGroup(ctx, groupID)
}

// ---------------------------------------------------------------------
// Bindings (Policy Bindings tab)
// ---------------------------------------------------------------------

// ModuleCandidates lists every tenant-visible module (tenant-authored +
// bundled, via PolicyModuleService.ListModules) that satisfies
// policyURN. Backs the binding form's module-override <select>.
func (s *ConfigGroupService) ModuleCandidates(ctx context.Context, tenantID int64, policyURN string) ([]policymodules.Module, error) {
	mods, err := s.moduleSvc.ListModules(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]policymodules.Module, 0, len(mods))
	for _, m := range mods {
		if m.SatisfiesURN(policyURN) {
			out = append(out, m)
		}
	}
	return out, nil
}

// resolveParamSchema finds the parameters_schema JSON for the module
// that would back a binding of policyURN with the given override (""=
// no override, resolve the policy's default module). Returns ok=false
// -- the documented "skip validation" path -- whenever the module or
// its schema can't be resolved: unknown override URN is instead
// reported to the caller as a hard ErrFieldValidation by
// AddBinding/UpdateBinding directly (an override the admin typed in
// should exist); every OTHER failure here (no default module
// satisfies the policy, resolved module has no published version, or
// its schema is empty) is a "nothing to validate against" case, not a
// caller error, so it's silently permissive -- exactly the behavior
// the module editor's own Parameters tab has for a moduleless policy.
func (s *ConfigGroupService) resolveParamSchema(ctx context.Context, tenantID int64, policyURN, moduleOverrideURN string) (string, bool) {
	var moduleRow db.PolicyModule
	var err error
	if moduleOverrideURN != "" {
		// Tenant-isolated resolve: a foreign or unknown override URN
		// resolves to "no schema" (skip validation) rather than leaking
		// another tenant's required-param names / enum values. AddBinding/
		// UpdateBinding already reject such an override with a hard
		// ErrFieldValidation before reaching here, so this is
		// defense-in-depth for any other caller of buildParameterValuesJSON.
		moduleRow, err = s.resolveOverrideModule(ctx, tenantID, moduleOverrideURN)
		if err != nil {
			return "", false
		}
	} else {
		mods, lerr := s.moduleSvc.ListModules(ctx, tenantID)
		if lerr != nil {
			return "", false
		}
		var picked *policymodules.Module
		for i := range mods {
			if !mods[i].SatisfiesURN(policyURN) {
				continue
			}
			if mods[i].Origin == "bundled" {
				picked = &mods[i]
				break
			}
			if picked == nil {
				picked = &mods[i]
			}
		}
		if picked == nil {
			return "", false
		}
		moduleRow, err = s.moduleSvc.GetModuleRow(ctx, picked.ID)
		if err != nil {
			return "", false
		}
	}
	ver, verr := s.db.Queries.GetLatestPublishedVersion(ctx, moduleRow.ID)
	if verr != nil {
		return "", false
	}
	if !ver.ParametersSchema.Valid || strings.TrimSpace(ver.ParametersSchema.String) == "" {
		return "", false
	}
	return ver.ParametersSchema.String, true
}

// paramSchema is the JSON Schema subset the module editor's Parameters
// tab writes (web/static/policy-module-editor.js's buildSchema): a flat
// object with typed properties and a required list. Values never
// define params -- see the binding form's doc comment -- so this is a
// read-only view of that schema, not a new source of truth for it.
type paramSchema struct {
	Properties map[string]paramProp `json:"properties"`
	Required   []string             `json:"required"`
}

type paramProp struct {
	Type string   `json:"type"`
	Enum []string `json:"enum"`
}

// validateParamValues checks a binding's submitted values against
// schemaRaw: unknown keys are rejected, every required key must be
// present, and each present key's value is type/enum-checked and
// coerced to its schema type (string/number/boolean; "enum" is stored
// as type "string" with an Enum list per the editor's buildSchema). The
// coerced map is what gets persisted as parameter_values JSON, so a
// stored "42" for a number-typed param round-trips as JSON 42, not the
// string "42".
func validateParamValues(schemaRaw string, values map[string]string) (map[string]interface{}, error) {
	var schema paramSchema
	if strings.TrimSpace(schemaRaw) != "" {
		if err := json.Unmarshal([]byte(schemaRaw), &schema); err != nil {
			// A malformed schema is the module's bug, not the caller's --
			// don't block the binding save on it.
			schema = paramSchema{}
		}
	}
	out := make(map[string]interface{}, len(values))
	for key, raw := range values {
		prop, ok := schema.Properties[key]
		if !ok {
			return nil, fmt.Errorf("%w: unknown parameter %q", ErrFieldValidation, key)
		}
		switch prop.Type {
		case "number":
			n, perr := strconv.ParseFloat(raw, 64)
			if perr != nil {
				return nil, fmt.Errorf("%w: %q must be a number", ErrFieldValidation, key)
			}
			out[key] = n
		case "boolean":
			b, perr := strconv.ParseBool(raw)
			if perr != nil {
				return nil, fmt.Errorf("%w: %q must be a boolean", ErrFieldValidation, key)
			}
			out[key] = b
		default: // "string", "enum" (stored as "string" + Enum), or unset.
			if len(prop.Enum) > 0 && !containsStr(prop.Enum, raw) {
				return nil, fmt.Errorf("%w: %q must be one of %v", ErrFieldValidation, key, prop.Enum)
			}
			out[key] = raw
		}
	}
	for _, req := range schema.Required {
		if _, present := values[req]; !present {
			return nil, fmt.Errorf("%w: missing required parameter %q", ErrFieldValidation, req)
		}
	}
	return out, nil
}

func containsStr(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// resolveOverrideModule resolves a caller-supplied override module URN
// with tenant isolation. GetModuleRow is a GLOBAL URN lookup (no
// tenant/bundled filter), so a naked call would let an admin in tenant A
// pin tenant B's module on a binding -- disclosing B's title (via
// ListBindingsByGroup's LEFT JOIN) and its parameters_schema
// (required-param names / enum values via validateParamValues). This
// mirrors console/handlers/module_authz.go's resolveTenantModuleByURN
// rule EXACTLY: a module is visible iff it's bundled (legitimately
// cross-tenant) OR owned by tenantID. Anything else -- unknown URN or a
// foreign tenant's module -- is reported as ErrFieldValidation (the
// binding handlers surface that as 400; a foreign module is
// indistinguishable from a nonexistent one to the caller, which is the
// intended non-disclosure). Every override-URN resolution in this file
// (AddBinding, UpdateBinding, resolveParamSchema) goes through here so
// the rule can't drift.
func (s *ConfigGroupService) resolveOverrideModule(ctx context.Context, tenantID int64, moduleOverrideURN string) (db.PolicyModule, error) {
	row, err := s.moduleSvc.GetModuleRow(ctx, moduleOverrideURN)
	if err != nil {
		return db.PolicyModule{}, fmt.Errorf("%w: unknown module %q", ErrFieldValidation, moduleOverrideURN)
	}
	if !row.IsBundled && (!row.TenantID.Valid || row.TenantID.Int64 != tenantID) {
		return db.PolicyModule{}, fmt.Errorf("%w: unknown module %q", ErrFieldValidation, moduleOverrideURN)
	}
	return row, nil
}

// AddBinding creates a policy binding on groupID. moduleOverrideURN=""
// means "inherit the policy's default module" (module_id stays NULL);
// a non-empty value must name a module the caller can resolve, or the
// call fails with ErrFieldValidation. parameter_values are validated
// against the resolved module's parameters_schema when one can be
// found (see resolveParamSchema); when none can be found, values are
// persisted as-typed strings without validation (the documented
// "skip validation" path -- the module editor's own Parameters tab
// applies the same leniency to an unset schema).
func (s *ConfigGroupService) AddBinding(ctx context.Context, tenantID, groupID int64, policyURN, moduleOverrideURN, moduleVersionPinned string, values map[string]string, state string) (db.ConfigurationGroupBinding, error) {
	if _, err := s.Get(ctx, tenantID, groupID); err != nil {
		return db.ConfigurationGroupBinding{}, err
	}
	if strings.TrimSpace(policyURN) == "" {
		return db.ConfigurationGroupBinding{}, fmt.Errorf("%w: policy_urn is required", ErrFieldValidation)
	}
	if !bindingStates[state] {
		return db.ConfigurationGroupBinding{}, ErrInvalidState
	}

	var moduleID sql.NullInt64
	if moduleOverrideURN != "" {
		row, err := s.resolveOverrideModule(ctx, tenantID, moduleOverrideURN)
		if err != nil {
			return db.ConfigurationGroupBinding{}, err
		}
		moduleID = sql.NullInt64{Int64: row.ID, Valid: true}
	}

	pv, err := s.buildParameterValuesJSON(ctx, tenantID, policyURN, moduleOverrideURN, values)
	if err != nil {
		return db.ConfigurationGroupBinding{}, err
	}

	row, err := s.db.Queries.CreateConfigurationGroupBinding(ctx, db.CreateConfigurationGroupBindingParams{
		ConfigurationGroupID: groupID,
		PolicyUrn:            policyURN,
		State:                state,
		ParameterValues:      sql.NullString{String: pv, Valid: true},
		ModuleID:             moduleID,
		ModuleVersionPinned:  sql.NullString{String: moduleVersionPinned, Valid: moduleVersionPinned != ""},
		Disabled:             false,
	})
	if err != nil && isUniqueErr(err) {
		return db.ConfigurationGroupBinding{}, fmt.Errorf("%w: this policy is already bound in this group", ErrFieldValidation)
	}
	return row, err
}

// buildParameterValuesJSON runs validateParamValues when a schema can
// be resolved, else falls through to persisting the raw string values.
func (s *ConfigGroupService) buildParameterValuesJSON(ctx context.Context, tenantID int64, policyURN, moduleOverrideURN string, values map[string]string) (string, error) {
	schema, ok := s.resolveParamSchema(ctx, tenantID, policyURN, moduleOverrideURN)
	if ok {
		coerced, verr := validateParamValues(schema, values)
		if verr != nil {
			return "", verr
		}
		b, err := json.Marshal(coerced)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	b, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// UpdateBinding changes a binding's state/module-override/values.
func (s *ConfigGroupService) UpdateBinding(ctx context.Context, tenantID, groupID, bindingID int64, moduleOverrideURN, moduleVersionPinned string, values map[string]string, state string) (db.ConfigurationGroupBinding, error) {
	if _, err := s.Get(ctx, tenantID, groupID); err != nil {
		return db.ConfigurationGroupBinding{}, err
	}
	binding, err := s.db.Queries.GetConfigurationGroupBinding(ctx, bindingID)
	if err != nil || binding.ConfigurationGroupID != groupID {
		return db.ConfigurationGroupBinding{}, ErrConfigGroupNotFound
	}
	if !bindingStates[state] {
		return db.ConfigurationGroupBinding{}, ErrInvalidState
	}

	var moduleID sql.NullInt64
	if moduleOverrideURN != "" {
		row, merr := s.resolveOverrideModule(ctx, tenantID, moduleOverrideURN)
		if merr != nil {
			return db.ConfigurationGroupBinding{}, merr
		}
		moduleID = sql.NullInt64{Int64: row.ID, Valid: true}
	}

	pv, verr := s.buildParameterValuesJSON(ctx, tenantID, binding.PolicyUrn, moduleOverrideURN, values)
	if verr != nil {
		return db.ConfigurationGroupBinding{}, verr
	}

	return s.db.Queries.UpdateConfigurationGroupBinding(ctx, db.UpdateConfigurationGroupBindingParams{
		State:               state,
		ParameterValues:     sql.NullString{String: pv, Valid: true},
		ModuleID:            moduleID,
		ModuleVersionPinned: sql.NullString{String: moduleVersionPinned, Valid: moduleVersionPinned != ""},
		Disabled:            binding.Disabled,
		ID:                  bindingID,
	})
}

// RemoveBinding deletes one binding, verifying it belongs to groupID.
func (s *ConfigGroupService) RemoveBinding(ctx context.Context, tenantID, groupID, bindingID int64) error {
	if _, err := s.Get(ctx, tenantID, groupID); err != nil {
		return err
	}
	binding, err := s.db.Queries.GetConfigurationGroupBinding(ctx, bindingID)
	if err != nil || binding.ConfigurationGroupID != groupID {
		return ErrConfigGroupNotFound
	}
	return s.db.Queries.DeleteConfigurationGroupBinding(ctx, bindingID)
}

// ListBindings returns every binding for groupID (tenant-verified),
// each row carrying the joined module_title for display.
func (s *ConfigGroupService) ListBindings(ctx context.Context, tenantID, groupID int64) ([]db.ListBindingsByGroupRow, error) {
	if _, err := s.Get(ctx, tenantID, groupID); err != nil {
		return nil, err
	}
	return s.db.Queries.ListBindingsByGroup(ctx, groupID)
}

// CountAssignments/CountBindings back the list page's summary columns.
func (s *ConfigGroupService) CountAssignments(ctx context.Context, groupID int64) (int64, error) {
	return s.db.Queries.CountAssignmentsByGroup(ctx, groupID)
}

func (s *ConfigGroupService) CountBindings(ctx context.Context, groupID int64) (int64, error) {
	rows, err := s.db.Queries.ListBindingsByGroup(ctx, groupID)
	if err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}
