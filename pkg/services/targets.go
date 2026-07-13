package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// targetCatalogLimit bounds the per-kind fetch for the target picker.
// The picker itself documents its expectations as "low thousands of
// rows, client-side filtered" (see TargetPickerDialog's doc comment);
// this is comfortably above that for the life of the mock-retirement
// increment. Real pagination arrives with the backend slice mentioned
// there.
const targetCatalogLimit = 10000

// TargetService assembles the tenant-scoped catalog of pickable rows
// (assets, identities, groups, configuration groups) for the reusable
// TargetPickerDialog. It builds configgroups.Target rows — the same
// shape the dialog rendered from the retired in-memory mock
// (catalog/configgroups/targets.go's AllTargets), so the dialog's
// template and JS contract are untouched; only the data source moved
// from a hand-curated slice to real, tenant-scoped queries.
type TargetService struct {
	db *database.Database
}

// NewTargetService creates a new target service.
func NewTargetService(db *database.Database) *TargetService {
	return &TargetService{db: db}
}

// wantsKind reports whether kind should appear in the catalog given
// allowedKinds. Empty/nil allowedKinds means "every kind" — the same
// "empty = all" convention TargetPickerDialog's own doc comment uses
// for its allowedKinds prop.
func wantsKind(allowedKinds []configgroups.TargetKind, kind configgroups.TargetKind) bool {
	if len(allowedKinds) == 0 {
		return true
	}
	for _, k := range allowedKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// Catalog builds the tenant-scoped target catalog, restricted to the
// kinds present in allowedKinds (empty = all, matching the dialog's own
// convention). Row order is stable within each kind block (computers,
// then users, then computer groups, then user groups, then
// configuration groups) so template/handler tests can assert on
// position; the picker re-sorts by relevance client-side once a query
// is typed, same as it did over the mock.
//
// Ref format per kind (the stable identifier callers — e.g. the
// Configuration Group save path — parse back out of a picked row):
//
//   - KindComputer: the asset's numeric assets.id, as a decimal string.
//   - KindUser: the identity's numeric identities.id, as a decimal string.
//   - KindComputerGroup / KindUserGroup: the group's numeric groups.id,
//     as a decimal string.
//   - KindConfigurationGroup: the numeric configuration_groups.id, as a
//     decimal string.
//
// This matches the numeric target_id convention configuration_group_assignments
// and console/handlers/policy_picker.go already use for asset/identity
// targets, so a picked ref converts straight to an assignment row with
// strconv.ParseInt, no extra lookup.
//
// Asset subtype -> KindComputer mapping: only the "computer" and
// "server" subtypes are included. Both run the Pluris agent and can
// receive and enforce policy (TargetKind.HelpText's "Apply to a single
// computer asset" reads naturally for a server too — GP-style policy
// targeting has never distinguished "workstation" from "server", only
// "computer" from "user"). The "printer" and "desk" subtypes are
// excluded: neither runs an agent (see catalog/assets/types.go's Subtype
// doc comments — desks explicitly "don't run an agent"; printers are
// unmanaged IPP endpoints with no OS payload), so neither can ever be a
// valid enforcement target.
func (s *TargetService) Catalog(ctx context.Context, tenantID int64, allowedKinds []configgroups.TargetKind) ([]configgroups.Target, error) {
	var out []configgroups.Target

	if wantsKind(allowedKinds, configgroups.KindComputer) {
		rows, err := s.computerTargets(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("target catalog: assets: %w", err)
		}
		out = append(out, rows...)
	}

	if wantsKind(allowedKinds, configgroups.KindUser) {
		rows, err := s.userTargets(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("target catalog: identities: %w", err)
		}
		out = append(out, rows...)
	}

	if wantsKind(allowedKinds, configgroups.KindComputerGroup) || wantsKind(allowedKinds, configgroups.KindUserGroup) {
		rows, err := s.groupTargets(ctx, tenantID, allowedKinds)
		if err != nil {
			return nil, fmt.Errorf("target catalog: groups: %w", err)
		}
		out = append(out, rows...)
	}

	if wantsKind(allowedKinds, configgroups.KindConfigurationGroup) {
		rows, err := s.configGroupTargets(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("target catalog: configuration groups: %w", err)
		}
		out = append(out, rows...)
	}

	return out, nil
}

// computerTargets returns one KindComputer row per "computer" and
// "server" asset in tenantID (see subtype-mapping doc on Catalog).
func (s *TargetService) computerTargets(ctx context.Context, tenantID int64) ([]configgroups.Target, error) {
	var out []configgroups.Target
	for _, subtype := range []string{"computer", "server"} {
		rows, err := s.db.Queries.ListAssetsBySubtype(ctx, db.ListAssetsBySubtypeParams{
			TenantID: tenantID,
			Subtype:  subtype,
			Limit:    targetCatalogLimit,
			Offset:   0,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, assetRowToTarget(row))
		}
	}
	return out, nil
}

// assetRowToTarget converts one ListAssetsBySubtype row into a
// KindComputer Target row.
func assetRowToTarget(row db.ListAssetsBySubtypeRow) configgroups.Target {
	var payload map[string]interface{}
	_ = json.Unmarshal([]byte(row.SubtypePayload), &payload)

	hostname, _ := payload["hostname"].(string)
	osFamily, _ := payload["os_family"].(string)
	osDistribution, _ := payload["os_distribution"].(string)
	osVersion, _ := payload["os_version"].(string)

	label := hostname
	if label == "" {
		label = row.HumanID.String
	}
	if label == "" {
		label = row.Uuid
	}

	osPart := strings.TrimSpace(strings.TrimSpace(osDistribution) + " " + strings.TrimSpace(osVersion))
	var metaParts []string
	if osPart != "" {
		metaParts = append(metaParts, osPart)
	}
	if row.SiteName.Valid && row.SiteName.String != "" {
		metaParts = append(metaParts, row.SiteName.String)
	}
	metaParts = append(metaParts, "last seen "+relativeAge(row.LastSeenAt.Time))

	return configgroups.Target{
		Kind:  configgroups.KindComputer,
		Ref:   strconv.FormatInt(row.ID, 10),
		Label: label,
		Meta:  strings.Join(metaParts, " · "),
		Tags: []string{
			row.Subtype, hostname, osFamily, osDistribution,
			row.Uuid, row.HumanID.String, row.SiteName.String,
		},
	}
}

// userTargets returns one KindUser row per identity in tenantID.
func (s *TargetService) userTargets(ctx context.Context, tenantID int64) ([]configgroups.Target, error) {
	rows, err := s.db.Queries.ListIdentitiesByTenant(ctx, db.ListIdentitiesByTenantParams{
		TenantID: tenantID,
		Limit:    targetCatalogLimit,
		Offset:   0,
	})
	if err != nil {
		return nil, err
	}
	out := make([]configgroups.Target, 0, len(rows))
	for _, row := range rows {
		var metaParts []string
		if row.Email != "" {
			metaParts = append(metaParts, row.Email)
		}
		if row.Title.Valid && row.Title.String != "" {
			metaParts = append(metaParts, row.Title.String)
		} else if row.Department.Valid && row.Department.String != "" {
			metaParts = append(metaParts, row.Department.String)
		}
		out = append(out, configgroups.Target{
			Kind:  configgroups.KindUser,
			Ref:   strconv.FormatInt(row.ID, 10),
			Label: row.DisplayName,
			Meta:  strings.Join(metaParts, " · "),
			Tags:  []string{row.Username, row.Email, row.Department.String, row.Title.String},
		})
	}
	return out, nil
}

// groupTargets returns Target rows for every group in tenantID, dual-
// listed under KindComputerGroup and KindUserGroup as requested by
// allowedKinds.
//
// TODO(phase-6-member-kind): the `groups` table has no member_kind
// column yet, and group_memberships can mix asset and identity rows in
// the same group — there is no way today to say a group is
// "computer-only" or "user-only". Until that column lands, every group
// appears under BOTH KindComputerGroup and KindUserGroup (when both are
// allowed); this TODO tracks removing the dual-listing once member_kind
// exists.
func (s *TargetService) groupTargets(ctx context.Context, tenantID int64, allowedKinds []configgroups.TargetKind) ([]configgroups.Target, error) {
	rows, err := s.db.Queries.ListGroupsByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	wantComputerGroup := wantsKind(allowedKinds, configgroups.KindComputerGroup)
	wantUserGroup := wantsKind(allowedKinds, configgroups.KindUserGroup)

	var computerGroups, userGroups []configgroups.Target
	for _, row := range rows {
		assetCount, err := s.db.Queries.CountAssetsInGroup(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		identityCount, err := s.db.Queries.CountIdentitiesInGroup(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		total := assetCount + identityCount
		ref := strconv.FormatInt(row.ID, 10)
		tags := []string{row.Slug, row.GroupCategory, row.GroupScope}

		if wantComputerGroup {
			computerGroups = append(computerGroups, configgroups.Target{
				Kind:  configgroups.KindComputerGroup,
				Ref:   ref,
				Label: row.Name,
				Meta:  fmt.Sprintf("%d member(s) · %s", total, row.GroupCategory),
				Tags:  tags,
			})
		}
		if wantUserGroup {
			userGroups = append(userGroups, configgroups.Target{
				Kind:  configgroups.KindUserGroup,
				Ref:   ref,
				Label: row.Name,
				Meta:  fmt.Sprintf("%d member(s) · %s", total, row.GroupCategory),
				Tags:  tags,
			})
		}
	}
	return append(computerGroups, userGroups...), nil
}

// configGroupTargets returns one KindConfigurationGroup row per enabled
// configuration group in tenantID. Disabled groups are omitted: picking
// a disabled group as the composition source of another Configuration
// Group (KindConfigurationGroup's "apply to the targets of another
// group" semantics) isn't a meaningful admin action — there would be
// nothing live to compose on top of.
func (s *TargetService) configGroupTargets(ctx context.Context, tenantID int64) ([]configgroups.Target, error) {
	rows, err := s.db.Queries.ListEnabledConfigurationGroups(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]configgroups.Target, 0, len(rows))
	for _, row := range rows {
		out = append(out, configgroups.Target{
			Kind:  configgroups.KindConfigurationGroup,
			Ref:   strconv.FormatInt(row.ID, 10),
			Label: row.Name,
			Meta:  "Configuration group · " + row.Scope + " scope",
			Tags:  []string{row.Scope, row.Description.String},
		})
	}
	return out, nil
}

// LabelFor resolves one (target_type, target_id) assignment row to a
// human-readable label for the Configuration Group detail page's
// Assignments table (Task 5.2). Cheap single-row lookups per type;
// unknown/missing targets fall back to "type #id" so a dangling
// assignment still renders rather than erroring the page.
func (s *TargetService) LabelFor(ctx context.Context, targetType string, targetID int64) string {
	fallback := targetType + " #" + strconv.FormatInt(targetID, 10)
	switch targetType {
	case "asset":
		row, err := s.db.Queries.GetAsset(ctx, targetID)
		if err != nil {
			return fallback
		}
		var payload map[string]interface{}
		_ = json.Unmarshal([]byte(row.SubtypePayload), &payload)
		if hostname, _ := payload["hostname"].(string); hostname != "" {
			return hostname
		}
		if row.HumanID.Valid && row.HumanID.String != "" {
			return row.HumanID.String
		}
		return row.Uuid
	case "identity":
		row, err := s.db.Queries.GetIdentity(ctx, targetID)
		if err != nil {
			return fallback
		}
		if row.DisplayName != "" {
			return row.DisplayName
		}
		return row.Username
	case "group":
		row, err := s.db.Queries.GetGroup(ctx, targetID)
		if err != nil {
			return fallback
		}
		return row.Name
	}
	return fallback
}

// relativeAge renders t as a short "Xm ago" / "Xh ago" style string for
// the picker's Meta line. t.IsZero() (never seen) renders as "never".
func relativeAge(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h ago"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d/(24*time.Hour))) + "d ago"
	}
	return t.Format("2006-01-02")
}
