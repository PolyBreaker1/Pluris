package services

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
)

// Migration 012 replaced the phase-keyed policy_module_scripts row
// (008) with first-class named scripts (name/language/origin) plus a
// separate policy_module_actions table for enforcement wiring. This
// file is the service-layer half of that redesign: CRUD over both
// tables, the default-vs-custom fork-on-edit rule, and the pure
// ReferencedParams parser used by the security allow-list.
//
// Fork design (default vs custom coexistence): the schema's
// UNIQUE(version_id, name) means a default-origin row and a custom row
// can never share the exact same name. When UpsertScript is asked to
// edit a script that currently has origin='default', it first makes
// sure the pristine default survives under a reserved name --
// name+" (default)" (defaultReservedSuffix below), still origin=
// 'default' -- creating that reserved copy on first edit if it doesn't
// already exist. It then upserts the plain name as the origin='custom'
// working copy with the caller's new content. This means:
//   - Right after seeding, only the plain name exists (origin=
//     'default'); nothing is forked yet.
//   - After the first edit, both the plain name (origin='custom',
//     edited) and "<name> (default)" (origin='default', pristine)
//     exist. Enforcement actions keep referencing the plain name, so
//     they automatically pick up the edited script with no rewiring.
//   - RestoreDefaults re-points actions at the reserved pristine name
//     without touching the custom row (spec: "keeps customs").
//   - FullReset deletes the custom row, then rebuilds the plain name
//     and its action from the reserved pristine copy (or reseeds from
//     scratch if defaults were never forked in the first place).
const defaultReservedSuffix = " (default)"

func reservedDefaultName(name string) string { return name + defaultReservedSuffix }

func scriptFromRow(r db.PolicyModuleScript) policymodules.Script {
	return policymodules.Script{Name: r.Name, Language: r.Language, Source: r.Source, Origin: r.Origin, Seq: int(r.Seq)}
}

func actionFromRow(r db.PolicyModuleAction) policymodules.ModuleAction {
	return policymodules.ModuleAction{Key: r.ActionKey, Label: r.Label, Kind: r.Kind, Value: r.Value, Origin: r.Origin, Seq: int(r.Seq)}
}

// mapVersionNotDraft turns the sql.ErrNoRows a guarded write's zero-rows
// result surfaces as into the typed ErrVersionNotDraft when the version
// exists but isn't a draft, mirroring UpdateDraft/SetScript's existing
// not-found-vs-frozen split. Non-ErrNoRows errors pass through
// unchanged.
func (s *PolicyModuleService) mapVersionNotDraft(ctx context.Context, versionID int64, err error) error {
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, gerr := s.db.Queries.GetPolicyModuleVersion(ctx, versionID); gerr == nil {
		return ErrVersionNotDraft
	}
	return sql.ErrNoRows
}

// ListScripts returns every script row for versionID, ordered seq/name.
func (s *PolicyModuleService) ListScripts(ctx context.Context, versionID int64) ([]policymodules.Script, error) {
	rows, err := s.db.Queries.ListScriptsForVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	out := make([]policymodules.Script, 0, len(rows))
	for _, r := range rows {
		out = append(out, scriptFromRow(r))
	}
	return out, nil
}

// UpsertScript writes sc under versionID, draft-guarded. See the
// package-level fork-design comment above for the default-vs-custom
// coexistence rule this implements.
func (s *PolicyModuleService) UpsertScript(ctx context.Context, versionID int64, sc policymodules.Script) (policymodules.Script, error) {
	existing, err := s.db.Queries.GetScriptByName(ctx, db.GetScriptByNameParams{VersionID: versionID, Name: sc.Name})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return policymodules.Script{}, err
	}
	if err == nil && existing.Origin == "default" {
		reserved := reservedDefaultName(sc.Name)
		if _, rerr := s.db.Queries.GetScriptByName(ctx, db.GetScriptByNameParams{VersionID: versionID, Name: reserved}); errors.Is(rerr, sql.ErrNoRows) {
			if _, werr := s.db.Queries.UpsertModuleScriptGuarded(ctx, db.UpsertModuleScriptGuardedParams{
				VersionID: versionID, Name: reserved, Language: existing.Language, Source: existing.Source, Origin: "default", Seq: existing.Seq,
			}); werr != nil {
				return policymodules.Script{}, s.mapVersionNotDraft(ctx, versionID, werr)
			}
		} else if rerr != nil {
			return policymodules.Script{}, rerr
		}
	}
	lang := sc.Language
	if lang == "" {
		lang = string(policymodules.LangSh)
	}
	row, err := s.db.Queries.UpsertModuleScriptGuarded(ctx, db.UpsertModuleScriptGuardedParams{
		VersionID: versionID, Name: sc.Name, Language: lang, Source: sc.Source, Origin: "custom", Seq: int64(sc.Seq),
	})
	if err != nil {
		return policymodules.Script{}, s.mapVersionNotDraft(ctx, versionID, err)
	}
	return scriptFromRow(row), nil
}

// RenameScript renames oldName to newName for versionID, draft-guarded.
// A no-match (nonexistent oldName) on a real draft is a no-op success,
// mirroring DeleteScript's convention.
func (s *PolicyModuleService) RenameScript(ctx context.Context, versionID int64, oldName, newName string) error {
	rows, err := s.db.Queries.RenameModuleScriptGuarded(ctx, db.RenameModuleScriptGuardedParams{
		VersionID: versionID, OldName: oldName, NewName: newName,
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

// ListActions returns every action row for versionID, ordered
// seq/action_key.
func (s *PolicyModuleService) ListActions(ctx context.Context, versionID int64) ([]policymodules.ModuleAction, error) {
	rows, err := s.db.Queries.ListActionsForVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	out := make([]policymodules.ModuleAction, 0, len(rows))
	for _, r := range rows {
		out = append(out, actionFromRow(r))
	}
	return out, nil
}

// UpsertAction writes a under versionID, draft-guarded.
func (s *PolicyModuleService) UpsertAction(ctx context.Context, versionID int64, a policymodules.ModuleAction) (policymodules.ModuleAction, error) {
	kind := a.Kind
	if kind == "" {
		kind = "script"
	}
	origin := a.Origin
	if origin == "" {
		origin = "custom"
	}
	row, err := s.db.Queries.UpsertModuleActionGuarded(ctx, db.UpsertModuleActionGuardedParams{
		VersionID: versionID, ActionKey: a.Key, Label: a.Label, Kind: kind, Value: a.Value, Origin: origin, Seq: int64(a.Seq),
	})
	if err != nil {
		return policymodules.ModuleAction{}, s.mapVersionNotDraft(ctx, versionID, err)
	}
	return actionFromRow(row), nil
}

// DeleteAction removes the action keyed by key from versionID,
// draft-guarded. A no-match on a real draft is a no-op success.
func (s *PolicyModuleService) DeleteAction(ctx context.Context, versionID int64, key string) error {
	rows, err := s.db.Queries.DeleteModuleActionGuarded(ctx, db.DeleteModuleActionGuardedParams{VersionID: versionID, ActionKey: key})
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

// defaultApplySource is the CP1 minimal-viable seeded apply script
// body. Richer bundled defaults are a later checkpoint's job.
const defaultApplySource = "#!/bin/sh\n# default apply\n"

// SeedModuleDefaults populates the default script + action wiring for a
// version that has neither yet (i.e. brand new). Called explicitly by
// callers that want a version to start from the default set -- it is
// NOT invoked automatically by CreateDraft. A version that already has
// any script or action row is left untouched (idempotent no-op), so
// calling this twice, or calling it after a caller has already added
// content of their own, never clobbers anything.
func (s *PolicyModuleService) SeedModuleDefaults(ctx context.Context, versionID int64) error {
	scripts, err := s.db.Queries.ListScriptsForVersion(ctx, versionID)
	if err != nil {
		return err
	}
	actions, err := s.db.Queries.ListActionsForVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if len(scripts) > 0 || len(actions) > 0 {
		return nil
	}
	name := string(policymodules.PhaseApply)
	if _, err := s.db.Queries.UpsertModuleScriptGuarded(ctx, db.UpsertModuleScriptGuardedParams{
		VersionID: versionID, Name: name, Language: string(policymodules.LangSh), Source: defaultApplySource, Origin: "default", Seq: 0,
	}); err != nil {
		return s.mapVersionNotDraft(ctx, versionID, err)
	}
	if _, err := s.db.Queries.UpsertModuleActionGuarded(ctx, db.UpsertModuleActionGuardedParams{
		VersionID: versionID, ActionKey: name, Label: "", Kind: "script", Value: name, Origin: "default", Seq: 0,
	}); err != nil {
		return s.mapVersionNotDraft(ctx, versionID, err)
	}
	return nil
}

// RestoreDefaults re-points enforcement actions at the default wiring
// without touching custom scripts (spec section 7: after restore, an
// edited custom script AND the pristine default both exist). For every
// reserved-name pristine default script found (i.e. every script that
// was forked by an UpsertScript edit), the corresponding action is
// upserted to point at the reserved name. Scripts that were never
// edited (still plain-named, origin='default') need no action change --
// their action already points at them.
func (s *PolicyModuleService) RestoreDefaults(ctx context.Context, versionID int64) error {
	if v, err := s.db.Queries.GetPolicyModuleVersion(ctx, versionID); err != nil {
		return err
	} else if v.State != "draft" {
		return ErrVersionNotDraft
	}
	scripts, err := s.db.Queries.ListScriptsForVersion(ctx, versionID)
	if err != nil {
		return err
	}
	for _, sc := range scripts {
		if sc.Origin != "default" || !strings.HasSuffix(sc.Name, defaultReservedSuffix) {
			continue
		}
		canonical := strings.TrimSuffix(sc.Name, defaultReservedSuffix)
		if _, err := s.db.Queries.UpsertModuleActionGuarded(ctx, db.UpsertModuleActionGuardedParams{
			VersionID: versionID, ActionKey: canonical, Label: "", Kind: "script", Value: sc.Name, Origin: "default", Seq: sc.Seq,
		}); err != nil {
			return s.mapVersionNotDraft(ctx, versionID, err)
		}
	}
	return nil
}

// FullReset deletes every custom script and action for versionID, then
// rebuilds the default wiring: for each reserved pristine default found
// (a script that had been forked), the plain name and its action are
// restored from the reserved copy's content/origin. If no reserved
// copies exist at all -- either the defaults were never forked, so the
// plain default rows survived DeleteCustomScriptsForVersion untouched,
// or defaults were never seeded in the first place -- SeedModuleDefaults
// is called as a fallback (itself a no-op if the version already has
// content, i.e. the never-forked case).
func (s *PolicyModuleService) FullReset(ctx context.Context, versionID int64) error {
	if v, err := s.db.Queries.GetPolicyModuleVersion(ctx, versionID); err != nil {
		return err
	} else if v.State != "draft" {
		return ErrVersionNotDraft
	}
	if err := s.db.Queries.DeleteCustomScriptsForVersion(ctx, versionID); err != nil {
		return err
	}
	if err := s.db.Queries.DeleteCustomActionsForVersion(ctx, versionID); err != nil {
		return err
	}

	scripts, err := s.db.Queries.ListScriptsForVersion(ctx, versionID)
	if err != nil {
		return err
	}
	restoredAny := false
	for _, sc := range scripts {
		if sc.Origin != "default" || !strings.HasSuffix(sc.Name, defaultReservedSuffix) {
			continue
		}
		canonical := strings.TrimSuffix(sc.Name, defaultReservedSuffix)
		if _, err := s.db.Queries.UpsertModuleScriptGuarded(ctx, db.UpsertModuleScriptGuardedParams{
			VersionID: versionID, Name: canonical, Language: sc.Language, Source: sc.Source, Origin: "default", Seq: sc.Seq,
		}); err != nil {
			return s.mapVersionNotDraft(ctx, versionID, err)
		}
		if _, err := s.db.Queries.UpsertModuleActionGuarded(ctx, db.UpsertModuleActionGuardedParams{
			VersionID: versionID, ActionKey: canonical, Label: "", Kind: "script", Value: canonical, Origin: "default", Seq: sc.Seq,
		}); err != nil {
			return s.mapVersionNotDraft(ctx, versionID, err)
		}
		restoredAny = true
	}
	if !restoredAny {
		return s.SeedModuleDefaults(ctx, versionID)
	}
	return nil
}

// paramTokenRe matches the `{{ param "path" }}` injection token (spec
// section 6/8/9's security allow-list source). Kept intentionally
// narrow -- \w plus /.- covers every catalog/params path shape
// (INV-CPP: entity/section/param).
var paramTokenRe = regexp.MustCompile(`\{\{\s*param\s+"([\w\/.\-]+)"\s*\}\}`)

// ReferencedParams returns the distinct param paths referenced by
// source, in first-seen order. Pure function, no DB access -- this is
// the security allow-list computation: whatever this returns is the
// complete set of params a script may read at execution time.
func (s *PolicyModuleService) ReferencedParams(source string) []string {
	matches := paramTokenRe.FindAllStringSubmatch(source, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		p := m[1]
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
