package services

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// ErrGroupNotFound is returned by Get/resolve helpers when a group id
// doesn't exist or belongs to a different tenant.
var ErrGroupNotFound = errors.New("group not found")

// ErrGroupNameRequired is returned by Create when name is blank.
var ErrGroupNameRequired = errors.New("group name is required")

// ErrGroupReferenced is returned by Delete when the group is still
// targeted by a Configuration Group assignment (target_type='group').
// configuration_group_assignments.target_id carries no FK (it's a
// polymorphic column -- see 001_initial.sql's comment on that table), so
// deleting a referenced group would leave a dangling assignment row
// pointing at a group id that no longer exists; the admin must remove
// those assignments first. group_roles and group_memberships need no
// guard: both carry ON DELETE CASCADE FKs to groups(id).
var ErrGroupReferenced = errors.New("group is still targeted by one or more configuration group assignments")

// ErrMemberNotDirect is returned by RemoveAssetMember/RemoveIdentityMember
// when the membership row being removed has source='rule': rule-sourced
// members are only removed by EvaluateDynamicMembership (via rule
// edit/delete/recalculate), never by a direct "Remove" click -- an admin
// removing a rule-sourced row would just have it reappear on the next
// recalculation, silently undoing their action.
var ErrMemberNotDirect = errors.New("member is managed by a dynamic-membership rule and cannot be removed directly")

// GroupService handles AD-style groups: membership for the user/asset
// detail Groups tabs (Task 9), group meta + dynamic-membership rules
// (Task 6.1, group_rules.go), and the standardized group pages'
// list/create/delete/members needs (Task 6.2).
type GroupService struct {
	db *database.Database
}

// NewGroupService creates a new group service.
func NewGroupService(db *database.Database) *GroupService {
	return &GroupService{db: db}
}

// membershipSourceLabel maps a group_memberships.source value to its
// display chip label: 'rule' rows read "Dynamic" (the membership is
// computed from the group's rules), everything else "Direct".
func membershipSourceLabel(source string) string {
	if source == membershipSourceRule {
		return "Dynamic"
	}
	return "Direct"
}

// GroupRow is one row on a user/asset detail Groups tab. Source reflects
// the membership row's real source column: "Direct" for admin-added
// rows, "Dynamic" for rule-computed ones (Task 6.2 -- previously
// hardcoded to "Direct").
type GroupRow struct {
	ID       int64
	Name     string
	Category string
	Scope    string
	Source   string
	AddedAt  time.Time
}

// ListForAsset returns the groups an asset is a member of.
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
			Source:   membershipSourceLabel(r.Source),
			AddedAt:  r.AddedAt,
		})
	}
	return out, nil
}

// ListForIdentity returns the groups an identity is a member of.
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
			Source:   membershipSourceLabel(r.Source),
			AddedAt:  r.AddedAt,
		})
	}
	return out, nil
}

// ListByTenant returns every group in the tenant (list page +
// add-to-group pickers).
func (s *GroupService) ListByTenant(ctx context.Context, tenantID int64) ([]db.Group, error) {
	return s.db.Queries.ListGroupsByTenant(ctx, tenantID)
}

// Get returns one group; used by handlers to verify tenant ownership
// before mutating memberships.
func (s *GroupService) Get(ctx context.Context, groupID int64) (db.Group, error) {
	return s.db.Queries.GetGroup(ctx, groupID)
}

// Create makes a new tenant-scoped group (Task 6.2's /groups/new page).
// memberKind/membership/rulesMatchMode are validated against the same
// Go-enforced enums SetGroupMeta uses; category/scope pass through (003's
// columns default 'security'/'global' -- the create form offers the same
// free-form pair the seeded groups use). The slug is derived from the
// name; on a UNIQUE(tenant_id, slug) collision a -2/-3/... suffix is
// probed (bounded), mirroring how humans disambiguate duplicates in AD.
func (s *GroupService) Create(ctx context.Context, tenantID int64, name, description, memberKind, membership, category, scope string) (db.Group, error) {
	if name == "" {
		return db.Group{}, ErrGroupNameRequired
	}
	if !validMemberKind(memberKind) {
		return db.Group{}, ErrInvalidMemberKind
	}
	if !validMembershipMode(membership) {
		return db.Group{}, ErrInvalidMembershipMode
	}
	if category == "" {
		category = "security"
	}
	if scope == "" {
		scope = "global"
	}
	base := slugify(name)
	if base == "" {
		base = "group"
	}
	slug := base
	for i := 2; ; i++ {
		if _, err := s.db.Queries.GetGroupBySlug(ctx, db.GetGroupBySlugParams{TenantID: tenantID, Slug: slug}); errors.Is(err, sql.ErrNoRows) {
			break
		} else if err != nil {
			return db.Group{}, err
		}
		if i > 50 {
			// Pathological duplicate storm; let the UNIQUE constraint speak.
			break
		}
		slug = base + "-" + strconv.Itoa(i)
	}
	return s.db.Queries.CreateGroupFull(ctx, db.CreateGroupFullParams{
		TenantID:       tenantID,
		SiteID:         sql.NullInt64{},
		Name:           name,
		Slug:           slug,
		GroupCategory:  category,
		GroupScope:     scope,
		Description:    description,
		MemberKind:     memberKind,
		Membership:     membership,
		RulesMatchMode: "all",
	})
}

// Delete removes a group after checking the polymorphic references the
// schema can't cascade for us: Configuration Group assignments with
// target_type='group' still pointing at this group block deletion with
// ErrGroupReferenced (see that error's doc comment). group_memberships,
// group_membership_rules and group_roles all cascade via their FKs.
func (s *GroupService) Delete(ctx context.Context, groupID int64) error {
	refs, err := s.db.Queries.CountConfigGroupAssignmentsForGroupTarget(ctx, groupID)
	if err != nil {
		return err
	}
	if refs > 0 {
		return ErrGroupReferenced
	}
	return s.db.Queries.DeleteGroup(ctx, groupID)
}

// GroupMemberRow is one row on the group detail Members tab: either an
// asset or an identity member, plus the membership's source ("Direct" /
// "Dynamic") and when it was established.
type GroupMemberRow struct {
	Kind    string // "asset" | "identity"
	ID      int64  // assets.id / identities.id
	Label   string // display name / human id
	Detail  string // email / subtype
	Href    string // detail-page link
	Source  string // "Direct" | "Dynamic"
	Direct  bool   // true when the row may be removed by hand
	AddedAt time.Time
}

// ListMembers returns the group's full membership -- asset members first
// (human-id order), then identity members (display-name order) -- with
// each row's real source for the Members tab's chips and the
// direct-only Remove affordance.
func (s *GroupService) ListMembers(ctx context.Context, groupID int64) ([]GroupMemberRow, error) {
	assets, err := s.db.Queries.ListAssetMembersForGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	identities, err := s.db.Queries.ListIdentityMembersForGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	out := make([]GroupMemberRow, 0, len(assets)+len(identities))
	for _, a := range assets {
		label := a.HumanID.String
		if label == "" {
			label = a.Uuid
		}
		out = append(out, GroupMemberRow{
			Kind:    MemberKindAsset,
			ID:      a.ID,
			Label:   label,
			Detail:  a.Subtype,
			Href:    "/assets/" + a.Subtype + "s/" + a.Uuid,
			Source:  membershipSourceLabel(a.Source),
			Direct:  a.Source != membershipSourceRule,
			AddedAt: a.AddedAt,
		})
	}
	for _, i := range identities {
		out = append(out, GroupMemberRow{
			Kind:    MemberKindIdentity,
			ID:      i.ID,
			Label:   i.DisplayName,
			Detail:  i.Email,
			Href:    "/users/" + strconv.FormatInt(i.ID, 10),
			Source:  membershipSourceLabel(i.Source),
			Direct:  i.Source != membershipSourceRule,
			AddedAt: i.AddedAt,
		})
	}
	return out, nil
}

// CountMembers returns the group's total member count (assets +
// identities, direct and rule-sourced alike) for the list page column
// and the hero chip.
func (s *GroupService) CountMembers(ctx context.Context, groupID int64) (int64, error) {
	assets, err := s.db.Queries.CountAssetsInGroup(ctx, groupID)
	if err != nil {
		return 0, err
	}
	identities, err := s.db.Queries.CountIdentitiesInGroup(ctx, groupID)
	if err != nil {
		return 0, err
	}
	return assets + identities, nil
}

// ListIdentityMembers returns the identity members of a group (the
// group detail Members tab, Task 7). Asset members aren't shown there --
// no asset-member section exists on the group detail page yet.
func (s *GroupService) ListIdentityMembers(ctx context.Context, groupID int64) ([]db.Identity, error) {
	return s.db.Queries.ListIdentitiesInGroup(ctx, groupID)
}

// AddAssetMember adds an asset to a group as a DIRECT member (idempotent:
// duplicate adds are ignored by the underlying ON CONFLICT DO NOTHING, so
// an existing rule-sourced row is never "upgraded" to direct).
func (s *GroupService) AddAssetMember(ctx context.Context, groupID, assetID int64) error {
	return s.db.Queries.AddAssetToGroup(ctx, db.AddAssetToGroupParams{
		GroupID: groupID,
		AssetID: sql.NullInt64{Int64: assetID, Valid: true},
	})
}

// AddIdentityMember adds an identity to a group as a direct member
// (idempotent).
func (s *GroupService) AddIdentityMember(ctx context.Context, groupID, identityID int64) error {
	return s.db.Queries.AddIdentityToGroup(ctx, db.AddIdentityToGroupParams{
		GroupID:    groupID,
		IdentityID: sql.NullInt64{Int64: identityID, Valid: true},
	})
}

// RemoveAssetMember removes a DIRECT asset membership. A rule-sourced
// row fails with ErrMemberNotDirect (it would just be re-added by the
// next reconciliation); a missing row is a silent no-op, preserving the
// pre-6.2 idempotent-remove behavior.
func (s *GroupService) RemoveAssetMember(ctx context.Context, groupID, assetID int64) error {
	source, err := s.db.Queries.GetGroupMembershipSourceForAsset(ctx, db.GetGroupMembershipSourceForAssetParams{
		GroupID: groupID, AssetID: sql.NullInt64{Int64: assetID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if source == membershipSourceRule {
		return ErrMemberNotDirect
	}
	return s.db.Queries.RemoveAssetFromGroup(ctx, db.RemoveAssetFromGroupParams{
		GroupID: groupID,
		AssetID: sql.NullInt64{Int64: assetID, Valid: true},
	})
}

// RemoveIdentityMember removes a DIRECT identity membership; see
// RemoveAssetMember for the rule-sourced guard.
func (s *GroupService) RemoveIdentityMember(ctx context.Context, groupID, identityID int64) error {
	source, err := s.db.Queries.GetGroupMembershipSourceForIdentity(ctx, db.GetGroupMembershipSourceForIdentityParams{
		GroupID: groupID, IdentityID: sql.NullInt64{Int64: identityID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if source == membershipSourceRule {
		return ErrMemberNotDirect
	}
	return s.db.Queries.RemoveIdentityFromGroup(ctx, db.RemoveIdentityFromGroupParams{
		GroupID:    groupID,
		IdentityID: sql.NullInt64{Int64: identityID, Valid: true},
	})
}
