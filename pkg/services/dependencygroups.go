package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// ErrBuiltinProtected is returned when a caller tries to delete a builtin
// dependency group. Builtins are editable but not deletable.
var ErrBuiltinProtected = errors.New("builtin dependency group cannot be deleted")

var ErrDependencyGroupReferenced = errors.New("dependency group is referenced by policy modules")

var ErrDependencyGroupNotFound = errors.New("dependency group not found")

// ErrBuiltinMatchModeProtected mirrors ErrBuiltinProtected for
// SetMatchMode: builtins must keep match_mode="all" (the semantics their
// seed conditions were authored against), so changing it is refused the
// same way deletion is.
var ErrBuiltinMatchModeProtected = errors.New("builtin dependency group match mode cannot be changed")

// ErrInvalidConditionKind is returned when AddCondition receives a kind
// outside "param"/"script" (empty defaults to "param").
var ErrInvalidConditionKind = errors.New("invalid condition kind")

// ErrInvalidOperator is returned when a param-kind condition names an
// operator outside dependencygroups.AllOperators(), or omits a required
// param path.
var ErrInvalidOperator = errors.New("invalid or unsupported operator")

// ErrParamPathRequired is returned when a param-kind condition has an
// empty param path.
var ErrParamPathRequired = errors.New("param path is required")

// ErrScriptSourceRequired is returned when a script-kind condition has an
// empty script source.
var ErrScriptSourceRequired = errors.New("script source is required")

// ErrInvalidScriptExpect is retained for callers that still map it to
// HTTP errors; since migration 011 the standardized operator/value
// expectation replaced script_expect, so only ErrScriptSourceAmbiguous
// class failures reference the script payload now.
var ErrInvalidScriptExpect = errors.New("invalid script_expect: superseded by operator/value expectations")

// ErrScriptSourceAmbiguous is returned when a script-kind condition
// supplies both an inline source and a library script reference, or
// neither.
var ErrScriptSourceAmbiguous = errors.New("script condition requires exactly one of inline source or script reference")

// ErrInvalidMatchMode is returned when SetMatchMode receives a mode
// outside "all"/"any".
var ErrInvalidMatchMode = errors.New("invalid match mode")

// validConditionOperator reports whether op is one the eval engine
// (catalog/dependencygroups.AllOperators) actually implements.
func validConditionOperator(op string) bool {
	for _, allowed := range dependencygroups.AllOperators() {
		if string(allowed) == op {
			return true
		}
	}
	return false
}

// conditionPayload is the kind-agnostic input every condition writer
// (dependency groups, dynamic group rules, module version tests)
// validates through validateConditionPayload — one validation path for
// the one condition model (INV-CB / INV-TEST).
type conditionPayload struct {
	Kind         string
	ParamPath    string
	Operator     string
	ScriptSource string
	ScriptRef    string
}

// validateConditionPayload enforces the standardized test shape
// (subject, operator, expected value) across all kinds. It returns the
// canonical kind (empty defaults to param). Every kind requires a valid
// operator; param requires a param path; command requires an inline
// command line; script requires exactly one of inline source or a
// library script reference.
func validateConditionPayload(p conditionPayload) (string, error) {
	kind := p.Kind
	if kind == "" {
		kind = string(dependencygroups.KindParam)
	}
	if !validConditionOperator(p.Operator) {
		return "", ErrInvalidOperator
	}
	switch dependencygroups.ConditionKind(kind) {
	case dependencygroups.KindParam:
		if p.ParamPath == "" {
			return "", ErrParamPathRequired
		}
	case dependencygroups.KindCommand:
		if strings.TrimSpace(p.ScriptSource) == "" {
			return "", ErrScriptSourceRequired
		}
	case dependencygroups.KindScript:
		hasSource := strings.TrimSpace(p.ScriptSource) != ""
		hasRef := strings.TrimSpace(p.ScriptRef) != ""
		if hasSource == hasRef {
			if !hasSource {
				return "", ErrScriptSourceRequired
			}
			return "", ErrScriptSourceAmbiguous
		}
	default:
		return "", ErrInvalidConditionKind
	}
	return kind, nil
}

// isUniqueErr matches SQLite UNIQUE violations so concurrent first
// requests seeding the same builtin slug are tolerated rather than
// bubbling up as a 500 (mirrors console/handlers/policy_picker.go's
// isUniqueErr).
func isUniqueErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

type DependencyGroupService struct {
	db *database.Database
}

func NewDependencyGroupService(db *database.Database) *DependencyGroupService {
	return &DependencyGroupService{db: db}
}

// builtinGroup is one seed template.
type builtinCond struct {
	Path string
	Op   string
	Vals []string
}
type builtinGroup struct {
	Slug, Name, Desc string
	Conds            []builtinCond
}

var builtinGroups = []builtinGroup{
	{"rpm-based", "RPM-based OS", "Fedora, RHEL, openSUSE and other RPM package systems.", []builtinCond{{"computer/hardware/os_package_family", "in", []string{"rpm"}}}},
	{"debian-based", "Debian-based OS", "Debian, Ubuntu and other deb package systems.", []builtinCond{{"computer/hardware/os_package_family", "in", []string{"deb"}}}},
	{"arch-based", "Arch-based OS", "Arch and derivatives using pacman.", []builtinCond{{"computer/hardware/os_package_family", "in", []string{"arch"}}}},
	{"any-linux", "Any Linux", "Any Linux operating system.", []builtinCond{{"computer/hardware/os_family", "in", []string{"linux"}}}},
	{"windows", "Windows", "Any Windows operating system.", []builtinCond{{"computer/hardware/os_family", "in", []string{"windows"}}}},
	{"disk-encryption-active", "Disk encryption active", "Primary disk uses any encryption mechanism.", []builtinCond{{"computer/hardware/disk_encryption", "not_in", []string{"none"}}}},
	{"bitlocker", "BitLocker enabled", "Primary disk encrypted with BitLocker.", []builtinCond{{"computer/hardware/disk_encryption", "in", []string{"bitlocker"}}}},
	{"luks", "LUKS enabled", "Primary disk encrypted with LUKS.", []builtinCond{{"computer/hardware/disk_encryption", "in", []string{"luks"}}}},
}

// builtinModuleLinks seeds default module to group links for bundled
// modules. moduleID must match a catalog/policymodules mock slug.
var builtinModuleLinks = []struct {
	ModuleID, Slug, Role string
}{
	{"pluris.sshd.password-auth-disable", "any-linux", "platform"},
}

func (s *DependencyGroupService) EnsureBuiltins(ctx context.Context, tenantID int64) error {
	for _, b := range builtinGroups {
		if _, err := s.db.Queries.GetDependencyGroupBySlug(ctx, db.GetDependencyGroupBySlugParams{TenantID: tenantID, Slug: b.Slug}); err == nil {
			// Already seeded; leave user edits intact.
			continue
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		g, err := s.db.Queries.CreateDependencyGroup(ctx, db.CreateDependencyGroupParams{
			TenantID: tenantID, Slug: b.Slug, Name: b.Name,
			Description: sql.NullString{String: b.Desc, Valid: true}, IsBuiltin: true,
		})
		if err != nil {
			if !isUniqueErr(err) {
				return err
			}
			// Lost the race to a concurrent first request seeding the
			// same slug: re-fetch what the other writer created and
			// skip condition seeding — it already did that too.
			if _, ferr := s.db.Queries.GetDependencyGroupBySlug(ctx, db.GetDependencyGroupBySlugParams{TenantID: tenantID, Slug: b.Slug}); ferr != nil {
				return ferr
			}
			continue
		}
		for i, c := range b.Conds {
			vals, _ := json.Marshal(c.Vals)
			if _, err := s.db.Queries.CreateDependencyGroupCondition(ctx, db.CreateDependencyGroupConditionParams{
				GroupID: g.ID, ParamPath: c.Path, Operator: c.Op, ValueJson: string(vals), Seq: int64(i),
				Kind: string(dependencygroups.KindParam),
			}); err != nil {
				return err
			}
		}
	}
	// Default module links (idempotent via INSERT OR IGNORE).
	for _, l := range builtinModuleLinks {
		g, err := s.db.Queries.GetDependencyGroupBySlug(ctx, db.GetDependencyGroupBySlugParams{TenantID: tenantID, Slug: l.Slug})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}
		_ = s.db.Queries.CreateModuleDependencyLink(ctx, db.CreateModuleDependencyLinkParams{
			TenantID: tenantID, ModuleID: l.ModuleID, GroupID: g.ID, Role: l.Role,
		})
	}
	return nil
}

func (s *DependencyGroupService) toGroup(ctx context.Context, row db.DependencyGroup) (dependencygroups.Group, error) {
	conds, err := s.db.Queries.ListConditionsForGroup(ctx, row.ID)
	if err != nil {
		return dependencygroups.Group{}, err
	}
	g := dependencygroups.Group{ID: row.ID, Slug: row.Slug, Name: row.Name, Builtin: row.IsBuiltin, MatchMode: dependencygroups.MatchMode(row.MatchMode)}
	if row.Description.Valid {
		g.Description = row.Description.String
	}
	for _, c := range conds {
		var vals []string
		if err := json.Unmarshal([]byte(c.ValueJson), &vals); err != nil {
			return dependencygroups.Group{}, err
		}
		g.Conditions = append(g.Conditions, dependencygroups.Condition{
			ID: c.ID, ParamPath: c.ParamPath, Operator: dependencygroups.Operator(c.Operator), Values: vals,
			Kind: dependencygroups.ConditionKind(c.Kind), ScriptSource: c.ScriptSource, ScriptRef: c.ScriptRef, ScriptExpect: c.ScriptExpect,
		})
	}
	return g, nil
}

func (s *DependencyGroupService) ListByTenant(ctx context.Context, tenantID int64) ([]dependencygroups.Group, error) {
	rows, err := s.db.Queries.ListDependencyGroupsByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]dependencygroups.Group, 0, len(rows))
	for _, r := range rows {
		g, err := s.toGroup(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (s *DependencyGroupService) ListDeletedByTenant(ctx context.Context, tenantID int64) ([]dependencygroups.Group, error) {
	rows, err := s.db.Queries.ListDeletedDependencyGroupsByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]dependencygroups.Group, 0, len(rows))
	for _, row := range rows {
		group, err := s.toGroup(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, group)
	}
	return out, nil
}

func (s *DependencyGroupService) Get(ctx context.Context, id int64) (dependencygroups.Group, error) {
	row, err := s.db.Queries.GetDependencyGroup(ctx, id)
	if err != nil {
		return dependencygroups.Group{}, err
	}
	return s.toGroup(ctx, row)
}

func (s *DependencyGroupService) Create(ctx context.Context, tenantID int64, name, description string) (dependencygroups.Group, error) {
	row, err := s.db.Queries.CreateDependencyGroup(ctx, db.CreateDependencyGroupParams{
		TenantID: tenantID, Slug: slugify(name), Name: name,
		Description: sql.NullString{String: description, Valid: description != ""}, IsBuiltin: false,
	})
	if err != nil {
		return dependencygroups.Group{}, err
	}
	return s.toGroup(ctx, row)
}

func (s *DependencyGroupService) Update(ctx context.Context, id int64, name, description string) error {
	return s.db.Queries.UpdateDependencyGroup(ctx, db.UpdateDependencyGroupParams{
		ID: id, Name: name, Description: sql.NullString{String: description, Valid: description != ""},
	})
}

func (s *DependencyGroupService) Delete(ctx context.Context, tenantID, id, actorID int64) error {
	row, err := s.db.Queries.GetDependencyGroupIncludingDeleted(ctx, id)
	if err != nil || row.TenantID != tenantID {
		return ErrDependencyGroupNotFound
	}
	if row.IsBuiltin {
		return ErrBuiltinProtected
	}
	setting, err := s.db.Queries.GetRetentionSetting(ctx, EntityKindDependencyGroup)
	if err != nil {
		return err
	}
	if setting.Mode == RetentionModeImmediate {
		return hardDeleteDependencyGroup(ctx, s.db.Queries, id)
	}
	_, err = s.db.Queries.SoftDeleteDependencyGroup(ctx, db.SoftDeleteDependencyGroupParams{
		DeletedBy: actorID, ID: id, TenantID: tenantID,
	})
	return err
}

func (s *DependencyGroupService) Restore(ctx context.Context, tenantID, id int64) error {
	row, err := s.db.Queries.GetDependencyGroupIncludingDeleted(ctx, id)
	if err != nil || row.TenantID != tenantID {
		return ErrDependencyGroupNotFound
	}
	_, err = s.db.Queries.RestoreDependencyGroup(ctx, db.RestoreDependencyGroupParams{ID: id, TenantID: tenantID})
	return err
}

func (s *DependencyGroupService) PermanentlyDelete(ctx context.Context, tenantID, id int64) error {
	row, err := s.db.Queries.GetDependencyGroupIncludingDeleted(ctx, id)
	if err != nil || row.TenantID != tenantID {
		return ErrDependencyGroupNotFound
	}
	if row.IsBuiltin {
		return ErrBuiltinProtected
	}
	return hardDeleteDependencyGroup(ctx, s.db.Queries, id)
}

func hardDeleteDependencyGroup(ctx context.Context, queries *db.Queries, id int64) error {
	count, err := queries.CountLinksForGroup(ctx, id)
	if err != nil {
		return err
	}
	if count != 0 {
		return ErrDependencyGroupReferenced
	}
	return queries.DeleteDependencyGroup(ctx, id)
}

// AddCondition appends a condition to a group. Every kind follows the
// standardized test shape (validateConditionPayload): param requires a
// paramPath, command requires an inline command in scriptSource, script
// requires exactly one of scriptSource/scriptRef; operator and values
// carry the expectation for all kinds.
func (s *DependencyGroupService) AddCondition(ctx context.Context, groupID int64, paramPath, operator string, values []string, kind, scriptSource, scriptRef string) error {
	kind, err := validateConditionPayload(conditionPayload{
		Kind: kind, ParamPath: paramPath, Operator: operator,
		ScriptSource: scriptSource, ScriptRef: scriptRef,
	})
	if err != nil {
		return err
	}

	conds, err := s.db.Queries.ListConditionsForGroup(ctx, groupID)
	if err != nil {
		return err
	}
	vals, _ := json.Marshal(values)
	_, err = s.db.Queries.CreateDependencyGroupCondition(ctx, db.CreateDependencyGroupConditionParams{
		GroupID: groupID, ParamPath: paramPath, Operator: operator, ValueJson: string(vals), Seq: int64(len(conds)),
		Kind: kind, ScriptSource: scriptSource, ScriptRef: scriptRef, ScriptExpect: "",
	})
	return err
}

// SetMatchMode changes a group's match_mode ("all" or "any"). Builtins
// are protected the same way Delete protects them — their seed
// conditions were authored assuming AND semantics, so changing the mode
// out from under them would silently change what modules apply to.
func (s *DependencyGroupService) SetMatchMode(ctx context.Context, groupID int64, mode string) error {
	switch dependencygroups.MatchMode(mode) {
	case dependencygroups.MatchAll, dependencygroups.MatchAny:
	default:
		return ErrInvalidMatchMode
	}
	row, err := s.db.Queries.GetDependencyGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if row.IsBuiltin {
		return ErrBuiltinMatchModeProtected
	}
	return s.db.Queries.UpdateGroupMatchMode(ctx, db.UpdateGroupMatchModeParams{ID: groupID, MatchMode: mode})
}

func (s *DependencyGroupService) RemoveCondition(ctx context.Context, groupID, condID int64) error {
	return s.db.Queries.DeleteDependencyGroupCondition(ctx, db.DeleteDependencyGroupConditionParams{ID: condID, GroupID: groupID})
}

func (s *DependencyGroupService) LinkModule(ctx context.Context, tenantID int64, moduleID string, groupID int64, role string) error {
	return s.db.Queries.CreateModuleDependencyLink(ctx, db.CreateModuleDependencyLinkParams{
		TenantID: tenantID, ModuleID: moduleID, GroupID: groupID, Role: role,
	})
}

func (s *DependencyGroupService) UnlinkModule(ctx context.Context, tenantID int64, moduleID string, groupID int64) error {
	return s.db.Queries.DeleteModuleDependencyLink(ctx, db.DeleteModuleDependencyLinkParams{
		TenantID: tenantID, ModuleID: moduleID, GroupID: groupID,
	})
}

func (s *DependencyGroupService) ListLinksForModule(ctx context.Context, tenantID int64, moduleID string) ([]dependencygroups.ModuleLink, error) {
	rows, err := s.db.Queries.ListLinksForModule(ctx, db.ListLinksForModuleParams{TenantID: tenantID, ModuleID: moduleID})
	if err != nil {
		return nil, err
	}
	out := make([]dependencygroups.ModuleLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, dependencygroups.ModuleLink{GroupID: r.GroupID, Role: dependencygroups.Role(r.Role)})
	}
	return out, nil
}

func (s *DependencyGroupService) CountLinks(ctx context.Context, groupID int64) (int64, error) {
	return s.db.Queries.CountLinksForGroup(ctx, groupID)
}

func (s *DependencyGroupService) Evaluate(ctx context.Context, tenantID int64, moduleID string, facts map[string]string) (dependencygroups.Result, error) {
	links, err := s.ListLinksForModule(ctx, tenantID, moduleID)
	if err != nil {
		return dependencygroups.Result{}, err
	}
	groups, err := s.ListByTenant(ctx, tenantID)
	if err != nil {
		return dependencygroups.Result{}, err
	}
	byID := make(map[int64]dependencygroups.Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}
	return dependencygroups.Eligible(links, byID, facts), nil
}

// EvaluateForAsset is Evaluate for one asset row: it derives the facts
// map via FactsForAsset (facts.go) — the SAME helper
// EvaluateDynamicMembership (groups.go, Task 6.1) uses to build facts
// for dynamic-group rule evaluation — rather than duplicating asset ->
// fact conversion inline. Evaluate itself keeps its existing
// caller-supplied-facts signature unchanged.
func (s *DependencyGroupService) EvaluateForAsset(ctx context.Context, tenantID int64, moduleID string, asset db.Asset) (dependencygroups.Result, error) {
	return s.Evaluate(ctx, tenantID, moduleID, FactsForAsset(asset))
}

// slugify lowercases name and collapses runs of non alphanumeric ASCII
// characters into a single hyphen, trimming leading/trailing hyphens.
func slugify(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
