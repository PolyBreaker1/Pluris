package services

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// IdentityService handles identity (user directory) operations, mirroring
// AssetService's role for the Asset entity.
type IdentityService struct {
	db *database.Database
}

// NewIdentityService creates a new identity service
func NewIdentityService(db *database.Database) *IdentityService {
	return &IdentityService{db: db}
}

func nullInt64(v int64) sql.NullInt64 {
	if v == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

func nullString(v string) sql.NullString {
	if v == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: v, Valid: true}
}

// Create inserts a new identity for tenantID.
func (s *IdentityService) Create(ctx context.Context, tenantID int64, in identities.Identity) (identities.Identity, error) {
	row, err := s.db.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
		TenantID:          tenantID,
		SiteID:            nullInt64(in.SiteID),
		Username:          in.Username,
		UserPrincipalName: nullString(in.UserPrincipalName),
		Email:             in.Email,
		DisplayName:       in.DisplayName,
		GivenName:         nullString(in.GivenName),
		Surname:           nullString(in.Surname),
		Title:             nullString(in.Title),
		Department:        nullString(in.Department),
		Company:           nullString(in.Company),
		EmployeeID:        nullString(in.EmployeeID),
		EmployeeType:      nullString(in.EmployeeType),
		ManagerID:         nullInt64(in.ManagerID),
		PhoneOffice:       nullString(in.PhoneOffice),
		PhoneMobile:       nullString(in.PhoneMobile),
		Role:              string(in.Role),
	})
	if err != nil {
		return identities.Identity{}, fmt.Errorf("create identity: %w", err)
	}
	return s.convert(row), nil
}

// Get fetches a single identity by ID.
func (s *IdentityService) Get(ctx context.Context, id int64) (identities.Identity, error) {
	row, err := s.db.Queries.GetIdentity(ctx, id)
	if err != nil {
		return identities.Identity{}, fmt.Errorf("get identity %d: %w", id, err)
	}
	return s.convert(row), nil
}

// GetByEmailGlobal fetches an identity by email across all tenants — used
// at login, where the user hasn't identified a tenant yet. Explicitly
// fails closed: returns an error if zero or more than one identity
// shares this email (the underlying query is :many precisely so this
// check is possible).
func (s *IdentityService) GetByEmailGlobal(ctx context.Context, email string) (identities.Identity, error) {
	rows, err := s.db.Queries.GetIdentityByEmailGlobal(ctx, email)
	if err != nil {
		return identities.Identity{}, fmt.Errorf("get identity by email %q: %w", email, err)
	}
	if len(rows) == 0 {
		return identities.Identity{}, fmt.Errorf("no identity found for email %q", email)
	}
	if len(rows) > 1 {
		return identities.Identity{}, fmt.Errorf("ambiguous email %q: matches %d identities across tenants", email, len(rows))
	}
	return s.convert(rows[0]), nil
}

// List returns identities for tenantID, paginated.
func (s *IdentityService) List(ctx context.Context, tenantID int64, limit, offset int64) ([]identities.Identity, error) {
	rows, err := s.db.Queries.ListIdentitiesByTenant(ctx, db.ListIdentitiesByTenantParams{
		TenantID: tenantID, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	out := make([]identities.Identity, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.convert(row))
	}
	return out, nil
}

// Search finds identities matching a free-text query within tenantID.
func (s *IdentityService) Search(ctx context.Context, tenantID int64, query string, limit int64) ([]identities.Identity, error) {
	rows, err := s.db.Queries.SearchIdentities(ctx, db.SearchIdentitiesParams{
		TenantID: tenantID, Search: nullString(query), Limit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("search identities: %w", err)
	}
	out := make([]identities.Identity, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.convert(row))
	}
	return out, nil
}

// Update writes back the editable fields of an existing identity.
func (s *IdentityService) Update(ctx context.Context, in identities.Identity) (identities.Identity, error) {
	row, err := s.db.Queries.UpdateIdentity(ctx, db.UpdateIdentityParams{
		ID:           in.ID,
		DisplayName:  in.DisplayName,
		GivenName:    nullString(in.GivenName),
		Surname:      nullString(in.Surname),
		Email:        in.Email,
		Title:        nullString(in.Title),
		Department:   nullString(in.Department),
		Company:      nullString(in.Company),
		EmployeeID:   nullString(in.EmployeeID),
		EmployeeType: nullString(in.EmployeeType),
		ManagerID:    nullInt64(in.ManagerID),
		PhoneOffice:  nullString(in.PhoneOffice),
		PhoneMobile:  nullString(in.PhoneMobile),
		SiteID:       nullInt64(in.SiteID),
	})
	if err != nil {
		return identities.Identity{}, fmt.Errorf("update identity %d: %w", in.ID, err)
	}
	return s.convert(row), nil
}

// ListAssignedAssets returns every asset owned by identityID within tenantID.
func (s *IdentityService) ListAssignedAssets(ctx context.Context, tenantID, identityID int64) ([]assets.Asset, error) {
	rows, err := s.db.Queries.ListAssetsByOwner(ctx, db.ListAssetsByOwnerParams{
		TenantID: tenantID,
		OwnerID:  sql.NullInt64{Int64: identityID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list assets owned by identity %d: %w", identityID, err)
	}
	out := make([]assets.Asset, 0, len(rows))
	for _, row := range rows {
		out = append(out, convertDBAssetToAsset(row))
	}
	return out, nil
}

// Delete permanently removes an identity.
func (s *IdentityService) Delete(ctx context.Context, id int64) error {
	if err := s.db.Queries.DeleteIdentity(ctx, id); err != nil {
		return fmt.Errorf("delete identity %d: %w", id, err)
	}
	return nil
}

// convert adapts a db.Identity row into the canonical identities.Identity shape.
func (s *IdentityService) convert(row db.Identity) identities.Identity {
	out := identities.Identity{
		ID:                   row.ID,
		TenantID:             row.TenantID,
		Username:             row.Username,
		UserPrincipalName:    row.UserPrincipalName.String,
		Email:                row.Email,
		DisplayName:          row.DisplayName,
		GivenName:            row.GivenName.String,
		Surname:              row.Surname.String,
		Initials:             row.Initials.String,
		Title:                row.Title.String,
		Department:           row.Department.String,
		Company:              row.Company.String,
		EmployeeID:           row.EmployeeID.String,
		EmployeeType:         row.EmployeeType.String,
		PhoneOffice:          row.PhoneOffice.String,
		PhoneMobile:          row.PhoneMobile.String,
		PhoneHome:            row.PhoneHome.String,
		Fax:                  row.Fax.String,
		Office:               row.Office.String,
		StreetAddress:        row.StreetAddress.String,
		City:                 row.City.String,
		State:                row.State.String,
		PostalCode:           row.PostalCode.String,
		Country:              row.Country.String,
		CountryCode:          row.CountryCode.String,
		HomeDirectory:        row.HomeDirectory.String,
		HomeDrive:            row.HomeDrive.String,
		ProfilePath:          row.ProfilePath.String,
		LogonScript:          row.LogonScript.String,
		AccountEnabled:       row.AccountEnabled,
		AccountLocked:        row.AccountLocked,
		PasswordNeverExpires: row.PasswordNeverExpires,
		MustChangePassword:   row.MustChangePassword,
		LogonCount:           row.LogonCount,
		BadPasswordCount:     row.BadPasswordCount,
		Role:                 identities.Role(row.Role),
		AvatarURL:            row.AvatarUrl.String,
		Locale:               row.Locale,
		Timezone:             row.Timezone,
		Description:          row.Description.String,
		Notes:                row.Notes.String,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
	if row.SiteID.Valid {
		out.SiteID = row.SiteID.Int64
	}
	if row.ManagerID.Valid {
		out.ManagerID = row.ManagerID.Int64
	}
	if row.AccountExpiresAt.Valid {
		t := row.AccountExpiresAt.Time
		out.AccountExpiresAt = &t
	}
	if row.PasswordLastSetAt.Valid {
		t := row.PasswordLastSetAt.Time
		out.PasswordLastSetAt = &t
	}
	if row.LastLogonAt.Valid {
		t := row.LastLogonAt.Time
		out.LastLogonAt = &t
	}
	return out
}
