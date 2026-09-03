package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

const (
	EntityKindIdentity           = "identity"
	EntityKindAsset              = "asset"
	EntityKindGroup              = "group"
	EntityKindConfigurationGroup = "configuration_group"
	EntityKindDependencyGroup    = "dependency_group"
	EntityKindPolicyModule       = "policy_module"

	RetentionModeSoft      = "soft"
	RetentionModeImmediate = "immediate"
)

var (
	ErrInvalidEntityKind    = errors.New("invalid retention entity kind")
	ErrInvalidRetentionMode = errors.New("invalid retention mode")
	ErrInvalidRetentionDays = errors.New("retention days must be non-negative")
)

var retentionEntityKinds = map[string]bool{
	EntityKindIdentity: true, EntityKindAsset: true, EntityKindGroup: true,
	EntityKindConfigurationGroup: true, EntityKindDependencyGroup: true,
	EntityKindPolicyModule: true,
}

type RetentionService struct {
	db  *database.Database
	now func() time.Time
}

func NewRetentionService(database *database.Database) *RetentionService {
	return &RetentionService{db: database, now: time.Now}
}

func (s *RetentionService) ListSettings(ctx context.Context) ([]db.RetentionSetting, error) {
	return s.db.Queries.ListRetentionSettings(ctx)
}

func (s *RetentionService) GetSetting(ctx context.Context, kind string) (db.RetentionSetting, error) {
	return s.db.Queries.GetRetentionSetting(ctx, kind)
}

func RetentionDeleteCopy(setting db.RetentionSetting, noun string) string {
	if setting.Mode == RetentionModeImmediate {
		return "This permanently deletes the selected " + noun + " and cannot be undone."
	}
	days, ok, _ := nullableInt64(setting.PurgeAfterDays)
	if !ok {
		return "Selected " + noun + " move to Deleted and are kept until deleted permanently."
	}
	return fmt.Sprintf("Selected %s move to Deleted and are kept for %d days.", noun, days)
}

func (s *RetentionService) UpdateSetting(ctx context.Context, kind, mode string, days *int64, actorID int64) (db.RetentionSetting, error) {
	if !retentionEntityKinds[kind] {
		return db.RetentionSetting{}, ErrInvalidEntityKind
	}
	if mode != RetentionModeSoft && mode != RetentionModeImmediate {
		return db.RetentionSetting{}, ErrInvalidRetentionMode
	}
	if days != nil && *days < 0 {
		return db.RetentionSetting{}, ErrInvalidRetentionDays
	}
	var purgeAfterDays any
	if days != nil {
		purgeAfterDays = *days
	}
	return s.db.Queries.UpdateRetentionSetting(ctx, db.UpdateRetentionSettingParams{
		EntityKind: kind, Mode: mode, PurgeAfterDays: purgeAfterDays, UpdatedBy: actorID,
	})
}

type PurgeResult struct {
	EntityKind string
	EntityID   int64
	TenantID   int64
	Purged     bool
	Skipped    bool
	Err        error
}

// PurgeExpired permanently removes expired soft-deleted rows. Each item is
// processed in its own transaction. Reference-blocked modules are skipped so
// a later cycle can retry them.
func (s *RetentionService) PurgeExpired(ctx context.Context) ([]PurgeResult, error) {
	settings, err := s.db.Queries.ListRetentionSettings(ctx)
	if err != nil {
		return nil, err
	}
	byKind := make(map[string]db.RetentionSetting, len(settings))
	for _, setting := range settings {
		byKind[setting.EntityKind] = setting
	}
	results := []PurgeResult{}
	moduleResults, err := s.purgeExpiredPolicyModules(ctx, byKind[EntityKindPolicyModule])
	if err != nil {
		return nil, err
	}
	results = append(results, moduleResults...)
	assetResults, err := s.purgeExpiredAssets(ctx, byKind[EntityKindAsset])
	if err != nil {
		return nil, err
	}
	results = append(results, assetResults...)
	identityResults, err := s.purgeExpiredIdentities(ctx, byKind[EntityKindIdentity])
	if err != nil {
		return nil, err
	}
	results = append(results, identityResults...)
	groupResults, err := s.purgeExpiredGroups(ctx, byKind[EntityKindGroup])
	if err != nil {
		return nil, err
	}
	results = append(results, groupResults...)
	configGroupResults, err := s.purgeExpiredConfigurationGroups(ctx, byKind[EntityKindConfigurationGroup])
	if err != nil {
		return nil, err
	}
	results = append(results, configGroupResults...)
	dependencyGroupResults, err := s.purgeExpiredDependencyGroups(ctx, byKind[EntityKindDependencyGroup])
	if err != nil {
		return nil, err
	}
	return append(results, dependencyGroupResults...), nil
}

func (s *RetentionService) purgeExpiredPolicyModules(ctx context.Context, setting db.RetentionSetting) ([]PurgeResult, error) {
	cutoff, ok, err := s.cutoff(setting)
	if err != nil || !ok {
		return []PurgeResult{}, err
	}
	rows, err := s.db.Queries.ListExpiredPolicyModules(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	results := make([]PurgeResult, 0, len(rows))
	for _, row := range rows {
		result := PurgeResult{EntityKind: EntityKindPolicyModule, EntityID: row.ID, TenantID: row.TenantID.Int64}
		tx, beginErr := s.db.BeginTx()
		if beginErr != nil {
			result.Err = beginErr
			results = append(results, result)
			continue
		}
		err = hardDeletePolicyModule(ctx, s.db.Queries.WithTx(tx), row.TenantID.Int64, row.ID, row.ModuleUrn)
		if errors.Is(err, ErrModuleReferenced) {
			_ = tx.Rollback()
			result.Skipped = true
			result.Err = err
			results = append(results, result)
			continue
		}
		if err != nil {
			_ = tx.Rollback()
			result.Err = err
			results = append(results, result)
			continue
		}
		if err = tx.Commit(); err != nil {
			result.Err = err
		} else {
			result.Purged = true
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *RetentionService) purgeExpiredAssets(ctx context.Context, setting db.RetentionSetting) ([]PurgeResult, error) {
	cutoff, ok, err := s.cutoff(setting)
	if err != nil || !ok {
		return []PurgeResult{}, err
	}
	rows, err := s.db.Queries.ListExpiredAssets(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	results := make([]PurgeResult, 0, len(rows))
	for _, row := range rows {
		result := PurgeResult{EntityKind: EntityKindAsset, EntityID: row.ID, TenantID: row.TenantID}
		tx, beginErr := s.db.BeginTx()
		if beginErr != nil {
			result.Err = beginErr
			results = append(results, result)
			continue
		}
		if err := s.db.Queries.WithTx(tx).DeleteAsset(ctx, row.ID); err != nil {
			_ = tx.Rollback()
			result.Err = err
		} else if err := tx.Commit(); err != nil {
			result.Err = err
		} else {
			result.Purged = true
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *RetentionService) purgeExpiredIdentities(ctx context.Context, setting db.RetentionSetting) ([]PurgeResult, error) {
	cutoff, ok, err := s.cutoff(setting)
	if err != nil || !ok {
		return []PurgeResult{}, err
	}
	rows, err := s.db.Queries.ListExpiredIdentities(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	results := make([]PurgeResult, 0, len(rows))
	for _, row := range rows {
		result := PurgeResult{EntityKind: EntityKindIdentity, EntityID: row.ID, TenantID: row.TenantID}
		tx, beginErr := s.db.BeginTx()
		if beginErr != nil {
			result.Err = beginErr
			results = append(results, result)
			continue
		}
		if err := s.db.Queries.WithTx(tx).DeleteIdentity(ctx, row.ID); err != nil {
			_ = tx.Rollback()
			result.Err = err
		} else if err := tx.Commit(); err != nil {
			result.Err = err
		} else {
			result.Purged = true
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *RetentionService) purgeExpiredGroups(ctx context.Context, setting db.RetentionSetting) ([]PurgeResult, error) {
	cutoff, ok, err := s.cutoff(setting)
	if err != nil || !ok {
		return []PurgeResult{}, err
	}
	rows, err := s.db.Queries.ListExpiredGroups(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	results := make([]PurgeResult, 0, len(rows))
	for _, row := range rows {
		result := PurgeResult{EntityKind: EntityKindGroup, EntityID: row.ID, TenantID: row.TenantID}
		tx, beginErr := s.db.BeginTx()
		if beginErr != nil {
			result.Err = beginErr
			results = append(results, result)
			continue
		}
		err = hardDeleteGroup(ctx, s.db.Queries.WithTx(tx), row.ID)
		if errors.Is(err, ErrGroupReferenced) {
			_ = tx.Rollback()
			result.Skipped = true
			result.Err = err
		} else if err != nil {
			_ = tx.Rollback()
			result.Err = err
		} else if err := tx.Commit(); err != nil {
			result.Err = err
		} else {
			result.Purged = true
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *RetentionService) purgeExpiredConfigurationGroups(ctx context.Context, setting db.RetentionSetting) ([]PurgeResult, error) {
	cutoff, ok, err := s.cutoff(setting)
	if err != nil || !ok {
		return []PurgeResult{}, err
	}
	rows, err := s.db.Queries.ListExpiredConfigurationGroups(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	results := make([]PurgeResult, 0, len(rows))
	for _, row := range rows {
		result := PurgeResult{EntityKind: EntityKindConfigurationGroup, EntityID: row.ID, TenantID: row.TenantID}
		tx, beginErr := s.db.BeginTx()
		if beginErr != nil {
			result.Err = beginErr
			results = append(results, result)
			continue
		}
		if err := s.db.Queries.WithTx(tx).DeleteConfigurationGroup(ctx, row.ID); err != nil {
			_ = tx.Rollback()
			result.Err = err
		} else if err := tx.Commit(); err != nil {
			result.Err = err
		} else {
			result.Purged = true
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *RetentionService) purgeExpiredDependencyGroups(ctx context.Context, setting db.RetentionSetting) ([]PurgeResult, error) {
	cutoff, ok, err := s.cutoff(setting)
	if err != nil || !ok {
		return []PurgeResult{}, err
	}
	rows, err := s.db.Queries.ListExpiredDependencyGroups(ctx, cutoff)
	if err != nil {
		return nil, err
	}
	results := make([]PurgeResult, 0, len(rows))
	for _, row := range rows {
		result := PurgeResult{EntityKind: EntityKindDependencyGroup, EntityID: row.ID, TenantID: row.TenantID}
		tx, beginErr := s.db.BeginTx()
		if beginErr != nil {
			result.Err = beginErr
			results = append(results, result)
			continue
		}
		err = hardDeleteDependencyGroup(ctx, s.db.Queries.WithTx(tx), row.ID)
		if errors.Is(err, ErrDependencyGroupReferenced) {
			_ = tx.Rollback()
			result.Skipped = true
			result.Err = err
		} else if err != nil {
			_ = tx.Rollback()
			result.Err = err
		} else if err := tx.Commit(); err != nil {
			result.Err = err
		} else {
			result.Purged = true
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *RetentionService) cutoff(setting db.RetentionSetting) (time.Time, bool, error) {
	days, ok, err := nullableInt64(setting.PurgeAfterDays)
	if err != nil || !ok {
		return time.Time{}, ok, err
	}
	return s.now().UTC().Add(-time.Duration(days) * 24 * time.Hour), true, nil
}

func nullableInt64(value any) (int64, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	switch v := value.(type) {
	case int64:
		return v, true, nil
	case int:
		return int64(v), true, nil
	case sql.NullInt64:
		return v.Int64, v.Valid, nil
	default:
		return 0, false, fmt.Errorf("unexpected retention day type %T", value)
	}
}
