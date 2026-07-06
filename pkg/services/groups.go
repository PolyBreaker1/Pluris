package services

import (
	"context"
	"database/sql"
	"time"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// GroupService handles group membership for the detail-page Groups tabs
// (Task 9). Add/Remove operate on the group_memberships table; listing
// returns display-ready rows.
type GroupService struct {
	db *database.Database
}

// NewGroupService creates a new group service.
func NewGroupService(db *database.Database) *GroupService {
	return &GroupService{db: db}
}

// GroupRow is one row on a Groups tab. Source is "Direct" for every
// membership today — inheritance labels (site/tenant rules, sync) arrive
// with sites and dynamic groups later.
type GroupRow struct {
	ID       int64
	Name     string
	Category string
	Scope    string
	Source   string
	AddedAt  time.Time
}

// ListForAsset returns the groups an asset is a direct member of.
func (s *GroupService) ListForAsset(ctx context.Context, assetID int64) ([]GroupRow, error) {
	rows, err := s.db.Queries.ListGroupsForAssetDetail(ctx, sql.NullInt64{Int64: assetID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]GroupRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, GroupRow{
			ID:       r.ID,
			Name:     r.Name,
			Category: r.GroupCategory,
			Scope:    r.GroupScope,
			Source:   "Direct",
			AddedAt:  r.AddedAt,
		})
	}
	return out, nil
}

// ListForIdentity returns the groups an identity is a direct member of.
func (s *GroupService) ListForIdentity(ctx context.Context, identityID int64) ([]GroupRow, error) {
	rows, err := s.db.Queries.ListGroupsForIdentityDetail(ctx, sql.NullInt64{Int64: identityID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]GroupRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, GroupRow{
			ID:       r.ID,
			Name:     r.Name,
			Category: r.GroupCategory,
			Scope:    r.GroupScope,
			Source:   "Direct",
			AddedAt:  r.AddedAt,
		})
	}
	return out, nil
}

// ListByTenant returns every group in the tenant (for the add-to-group
// picker).
func (s *GroupService) ListByTenant(ctx context.Context, tenantID int64) ([]db.Group, error) {
	return s.db.Queries.ListGroupsByTenant(ctx, tenantID)
}

// Get returns one group; used by handlers to verify tenant ownership
// before mutating memberships.
func (s *GroupService) Get(ctx context.Context, groupID int64) (db.Group, error) {
	return s.db.Queries.GetGroup(ctx, groupID)
}

// AddAssetMember adds an asset to a group (idempotent: duplicate adds
// are ignored by the underlying ON CONFLICT DO NOTHING).
func (s *GroupService) AddAssetMember(ctx context.Context, groupID, assetID int64) error {
	return s.db.Queries.AddAssetToGroup(ctx, db.AddAssetToGroupParams{
		GroupID: groupID,
		AssetID: sql.NullInt64{Int64: assetID, Valid: true},
	})
}

// AddIdentityMember adds an identity to a group (idempotent).
func (s *GroupService) AddIdentityMember(ctx context.Context, groupID, identityID int64) error {
	return s.db.Queries.AddIdentityToGroup(ctx, db.AddIdentityToGroupParams{
		GroupID:    groupID,
		IdentityID: sql.NullInt64{Int64: identityID, Valid: true},
	})
}

// RemoveAssetMember removes an asset from a group.
func (s *GroupService) RemoveAssetMember(ctx context.Context, groupID, assetID int64) error {
	return s.db.Queries.RemoveAssetFromGroup(ctx, db.RemoveAssetFromGroupParams{
		GroupID: groupID,
		AssetID: sql.NullInt64{Int64: assetID, Valid: true},
	})
}

// RemoveIdentityMember removes an identity from a group.
func (s *GroupService) RemoveIdentityMember(ctx context.Context, groupID, identityID int64) error {
	return s.db.Queries.RemoveIdentityFromGroup(ctx, db.RemoveIdentityFromGroupParams{
		GroupID:    groupID,
		IdentityID: sql.NullInt64{Int64: identityID, Valid: true},
	})
}
