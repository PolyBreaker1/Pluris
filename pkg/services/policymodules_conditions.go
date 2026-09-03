package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/db"
)

// Module version conditions are per-version applicability tests sharing
// the one condition model (INV-CB / INV-TEST): validated through
// validateConditionPayload, evaluated through dependencygroups.EvalGroup
// via VersionConditionsGroup, authored through the shared condition
// builder dialog. Every mutator is draft-guarded in the SQL statement
// itself (same pattern as SetScript), surfacing ErrVersionNotDraft when
// the version exists but is frozen.

func (s *PolicyModuleService) ListVersionConditions(ctx context.Context, versionID int64) ([]db.ModuleVersionCondition, error) {
	return s.db.Queries.ListVersionConditions(ctx, versionID)
}

func (s *PolicyModuleService) AddVersionCondition(ctx context.Context, versionID int64, kind, paramPath, operator string, values []string, scriptSource, scriptRef string) (db.ModuleVersionCondition, error) {
	kind, err := validateConditionPayload(conditionPayload{
		Kind: kind, ParamPath: paramPath, Operator: operator,
		ScriptSource: scriptSource, ScriptRef: scriptRef,
	})
	if err != nil {
		return db.ModuleVersionCondition{}, err
	}
	maxSeq, err := s.db.Queries.MaxVersionConditionSeq(ctx, versionID)
	if err != nil {
		return db.ModuleVersionCondition{}, err
	}
	seq, _ := maxSeq.(int64)
	vals, _ := json.Marshal(values)
	cond, err := s.db.Queries.CreateVersionConditionGuarded(ctx, db.CreateVersionConditionGuardedParams{
		VersionID: versionID, Kind: kind, ParamPath: paramPath, Operator: operator,
		ValueJson: string(vals), ScriptSource: scriptSource, ScriptRef: scriptRef, Seq: seq + 1,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return db.ModuleVersionCondition{}, s.versionNotDraftOrMissing(ctx, versionID)
		}
		return db.ModuleVersionCondition{}, err
	}
	return cond, nil
}

func (s *PolicyModuleService) UpdateVersionCondition(ctx context.Context, versionID, conditionID int64, kind, paramPath, operator string, values []string, scriptSource, scriptRef string) error {
	kind, err := validateConditionPayload(conditionPayload{
		Kind: kind, ParamPath: paramPath, Operator: operator,
		ScriptSource: scriptSource, ScriptRef: scriptRef,
	})
	if err != nil {
		return err
	}
	vals, _ := json.Marshal(values)
	rows, err := s.db.Queries.UpdateVersionConditionGuarded(ctx, db.UpdateVersionConditionGuardedParams{
		ID: conditionID, VersionID: versionID, Kind: kind, ParamPath: paramPath,
		Operator: operator, ValueJson: string(vals), ScriptSource: scriptSource, ScriptRef: scriptRef,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return s.versionNotDraftOrMissing(ctx, versionID)
	}
	return nil
}

func (s *PolicyModuleService) RemoveVersionCondition(ctx context.Context, versionID, conditionID int64) error {
	rows, err := s.db.Queries.DeleteVersionConditionGuarded(ctx, db.DeleteVersionConditionGuardedParams{
		ID: conditionID, VersionID: versionID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return s.versionNotDraftOrMissing(ctx, versionID)
	}
	return nil
}

func (s *PolicyModuleService) SetConditionsMatchMode(ctx context.Context, versionID int64, mode string) error {
	switch dependencygroups.MatchMode(mode) {
	case dependencygroups.MatchAll, dependencygroups.MatchAny:
	default:
		return ErrInvalidMatchMode
	}
	rows, err := s.db.Queries.UpdateVersionConditionsMatchMode(ctx, db.UpdateVersionConditionsMatchModeParams{
		ID: versionID, ConditionsMatchMode: mode,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return s.versionNotDraftOrMissing(ctx, versionID)
	}
	return nil
}

// versionNotDraftOrMissing distinguishes "exists but frozen"
// (ErrVersionNotDraft) from "no such version or condition"
// (sql.ErrNoRows), matching SetScript's contract.
func (s *PolicyModuleService) versionNotDraftOrMissing(ctx context.Context, versionID int64) error {
	if v, err := s.db.Queries.GetPolicyModuleVersion(ctx, versionID); err == nil && v.State != "draft" {
		return ErrVersionNotDraft
	}
	return sql.ErrNoRows
}

// VersionConditionsGroup marshals a version's tests into a
// dependencygroups.Group so dependencygroups.EvalGroup remains the one
// eval engine for every condition consumer.
func VersionConditionsGroup(matchMode string, conds []db.ModuleVersionCondition) dependencygroups.Group {
	g := dependencygroups.Group{MatchMode: dependencygroups.MatchMode(matchMode)}
	for _, c := range conds {
		var vals []string
		_ = json.Unmarshal([]byte(c.ValueJson), &vals)
		g.Conditions = append(g.Conditions, dependencygroups.Condition{
			ID: c.ID, ParamPath: c.ParamPath, Operator: dependencygroups.Operator(c.Operator),
			Values: vals, Kind: dependencygroups.ConditionKind(c.Kind),
			ScriptSource: c.ScriptSource, ScriptRef: c.ScriptRef,
		})
	}
	return g
}

// DeleteDraftVersion deletes a draft version (scripts and conditions
// cascade). Published/superseded/revoked versions are immutable history
// and cannot be deleted -- revoke instead. State-guarded in the DELETE
// statement itself, so a racing publish can never lose a published
// version.
func (s *PolicyModuleService) DeleteDraftVersion(ctx context.Context, versionID int64) error {
	rows, err := s.db.Queries.DeleteDraftModuleVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if rows == 0 {
		return s.versionNotDraftOrMissing(ctx, versionID)
	}
	return nil
}

// DeleteScript removes one named script from a draft version
// (draft-guarded in the statement). Deleting an absent script on a
// draft is a no-op success. Migration 012 replaced the old phase-keyed
// policy_module_scripts row with a name-keyed one; name is the script's
// name column (for migrated rows this is the old phase string, e.g.
// "apply").
func (s *PolicyModuleService) DeleteScript(ctx context.Context, versionID int64, name string) error {
	rows, err := s.db.Queries.DeleteModuleScriptGuarded(ctx, db.DeleteModuleScriptGuardedParams{
		VersionID: versionID, Name: name,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		if v, gerr := s.db.Queries.GetPolicyModuleVersion(ctx, versionID); gerr == nil && v.State != "draft" {
			return ErrVersionNotDraft
		}
	}
	return nil
}
