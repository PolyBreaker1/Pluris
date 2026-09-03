package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/params"
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

func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
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

// List returns identities for tenantID, paginated, with site names resolved
// (INV-L: list columns show display names, never raw foreign-key ids).
func (s *IdentityService) List(ctx context.Context, tenantID int64, limit, offset int64) ([]identities.Identity, error) {
	rows, err := s.db.Queries.ListIdentitiesByTenantWithSite(ctx, db.ListIdentitiesByTenantWithSiteParams{
		TenantID: tenantID, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	out := make([]identities.Identity, 0, len(rows))
	for _, row := range rows {
		ident := s.convert(row.Identity)
		ident.SiteName = row.SiteName.String
		out = append(out, ident)
	}
	return out, nil
}

func (s *IdentityService) ListDeleted(ctx context.Context, tenantID int64, limit, offset int64) ([]identities.Identity, error) {
	rows, err := s.db.Queries.ListDeletedIdentitiesByTenantWithSite(ctx, db.ListDeletedIdentitiesByTenantWithSiteParams{
		TenantID: tenantID, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list deleted identities: %w", err)
	}
	out := make([]identities.Identity, 0, len(rows))
	for _, row := range rows {
		ident := s.convert(row.Identity)
		ident.SiteName = row.SiteName.String
		out = append(out, ident)
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
		ID:                   in.ID,
		DisplayName:          in.DisplayName,
		GivenName:            nullString(in.GivenName),
		Surname:              nullString(in.Surname),
		Initials:             nullString(in.Initials),
		Email:                in.Email,
		Title:                nullString(in.Title),
		Department:           nullString(in.Department),
		Company:              nullString(in.Company),
		EmployeeID:           nullString(in.EmployeeID),
		EmployeeType:         nullString(in.EmployeeType),
		ManagerID:            nullInt64(in.ManagerID),
		PhoneOffice:          nullString(in.PhoneOffice),
		PhoneMobile:          nullString(in.PhoneMobile),
		PhoneHome:            nullString(in.PhoneHome),
		Fax:                  nullString(in.Fax),
		Office:               nullString(in.Office),
		StreetAddress:        nullString(in.StreetAddress),
		City:                 nullString(in.City),
		State:                nullString(in.State),
		PostalCode:           nullString(in.PostalCode),
		Country:              nullString(in.Country),
		CountryCode:          nullString(in.CountryCode),
		HomeDirectory:        nullString(in.HomeDirectory),
		HomeDrive:            nullString(in.HomeDrive),
		ProfilePath:          nullString(in.ProfilePath),
		LogonScript:          nullString(in.LogonScript),
		AccountEnabled:       in.AccountEnabled,
		AccountLocked:        in.AccountLocked,
		AccountExpiresAt:     nullTimePtr(in.AccountExpiresAt),
		PasswordNeverExpires: in.PasswordNeverExpires,
		MustChangePassword:   in.MustChangePassword,
		Locale:               in.Locale,
		Timezone:             in.Timezone,
		Description:          nullString(in.Description),
		Notes:                nullString(in.Notes),
		SiteID:               nullInt64(in.SiteID),
		AvatarUrl:            nullString(in.AvatarURL),
	})
	if err != nil {
		return identities.Identity{}, fmt.Errorf("update identity %d: %w", in.ID, err)
	}
	return s.convert(row), nil
}

// UpdateFields applies a partial set of param-keyed field updates to the
// identity identified by id, scoped to tenantID (cross-tenant or missing
// ids fail closed with ErrFieldNotFound). section and each field key are
// validated against catalog/params.SchemaByPathEntity("user") -- unknown
// sections/keys, keys the section does not mount, non-editable keys
// (id/tenant/username/groups/site/manager and other TypeLink params, plus
// role and the system-managed audit fields), and type-coercion failures
// all return ErrFieldValidation naming the offending key. On success the
// identity is persisted via Update and the list of applied keys is
// returned (handler order is not guaranteed; callers needing a stable
// order should sort).
func (s *IdentityService) UpdateFields(ctx context.Context, tenantID, id int64, section string, fields map[string]string) ([]string, error) {
	identity, err := s.Get(ctx, id)
	if err != nil {
		return nil, ErrFieldNotFound
	}
	if identity.TenantID != tenantID {
		return nil, ErrFieldNotFound
	}

	schema := params.SchemaByPathEntity("user")
	sec := sectionByKey(schema, section)
	if sec == nil {
		return nil, fmt.Errorf("%w: unknown section %q", ErrFieldValidation, section)
	}

	updated := make([]string, 0, len(fields))
	for key, raw := range fields {
		if !sectionHasParam(sec, key) {
			return nil, fieldErr(key, "not a field of section %q", section)
		}
		def := params.DefByKey(key)
		if def == nil {
			return nil, fieldErr(key, "unknown parameter")
		}
		if !identityFieldEditable(key, def) {
			return nil, fieldErr(key, "not editable")
		}
		val, err := coerceParamValue(def, raw)
		if err != nil {
			return nil, fieldErr(key, "%s", err)
		}
		if err := applyIdentityField(&identity, key, val); err != nil {
			return nil, fieldErr(key, "%s", err)
		}
		updated = append(updated, key)
	}

	if _, err := s.Update(ctx, identity); err != nil {
		return nil, err
	}
	return updated, nil
}

// identityFieldEditable reports whether key can be set through
// UpdateFields. Rejects every key in identities.NonEditableFieldKeys --
// the single source of truth shared with the detail UI (see
// web/templates/users.templ's userGeneralTab) -- plus, as a belt-and-
// braces guard against future TypeLink params that aren't yet named in
// that map, every TypeLink param.
func identityFieldEditable(key string, def *params.ParamDef) bool {
	if identities.NonEditableFieldKeys[key] {
		return false
	}
	return def.Type != params.TypeLink
}

// applyIdentityField sets the struct field on identity corresponding to
// key, given val already coerced by coerceParamValue to key's ParamDef
// type. This is the reverse of web/lists/identities.go's
// getIdentityParamValue for the editable-field subset (see
// identityFieldEditable for what that subset excludes).
func applyIdentityField(identity *identities.Identity, key string, val interface{}) error {
	switch key {
	case "display_name":
		identity.DisplayName = val.(string)
	case "given_name":
		identity.GivenName = val.(string)
	case "surname":
		identity.Surname = val.(string)
	case "initials":
		identity.Initials = val.(string)
	case "user_principal_name":
		identity.UserPrincipalName = val.(string)
	case "email":
		identity.Email = val.(string)
	case "title":
		identity.Title = val.(string)
	case "department":
		identity.Department = val.(string)
	case "company":
		identity.Company = val.(string)
	case "employee_id":
		identity.EmployeeID = val.(string)
	case "employee_type":
		identity.EmployeeType = val.(string)
	case "phone_office":
		identity.PhoneOffice = val.(string)
	case "phone_mobile":
		identity.PhoneMobile = val.(string)
	case "phone_home":
		identity.PhoneHome = val.(string)
	case "fax":
		identity.Fax = val.(string)
	case "office":
		identity.Office = val.(string)
	case "street_address":
		identity.StreetAddress = val.(string)
	case "city":
		identity.City = val.(string)
	case "state":
		identity.State = val.(string)
	case "postal_code":
		identity.PostalCode = val.(string)
	case "country":
		identity.Country = val.(string)
	case "country_code":
		identity.CountryCode = val.(string)
	case "home_directory":
		identity.HomeDirectory = val.(string)
	case "home_drive":
		identity.HomeDrive = val.(string)
	case "profile_path":
		identity.ProfilePath = val.(string)
	case "logon_script":
		identity.LogonScript = val.(string)
	case "account_enabled":
		identity.AccountEnabled = val.(bool)
	case "account_locked":
		identity.AccountLocked = val.(bool)
	case "account_expires_at":
		s := val.(string)
		if s == "" {
			identity.AccountExpiresAt = nil
			return nil
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return fmt.Errorf("expected an RFC3339 timestamp, got %q", s)
		}
		identity.AccountExpiresAt = &t
	case "password_never_expires":
		identity.PasswordNeverExpires = val.(bool)
	case "must_change_password":
		identity.MustChangePassword = val.(bool)
	case "locale":
		identity.Locale = val.(string)
	case "timezone":
		identity.Timezone = val.(string)
	case "description":
		identity.Description = val.(string)
	case "notes":
		identity.Notes = val.(string)
	default:
		return fmt.Errorf("field %q is mounted but not supported by UpdateFields", key)
	}
	return nil
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

// Delete follows the identity retention setting.
func (s *IdentityService) Delete(ctx context.Context, tenantID, id, actorID int64) error {
	row, err := s.db.Queries.GetIdentityIncludingDeleted(ctx, id)
	if err != nil {
		return fmt.Errorf("delete identity %d: %w", id, err)
	}
	if row.TenantID != tenantID {
		return fmt.Errorf("delete identity %d: %w", id, sql.ErrNoRows)
	}
	setting, err := s.db.Queries.GetRetentionSetting(ctx, EntityKindIdentity)
	if err != nil {
		return err
	}
	if setting.Mode == RetentionModeImmediate {
		return s.PermanentlyDelete(ctx, tenantID, id)
	}
	if _, err := s.db.Queries.SoftDeleteIdentity(ctx, db.SoftDeleteIdentityParams{DeletedBy: actorID, ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("delete identity %d: %w", id, err)
	}
	return nil
}

func (s *IdentityService) Restore(ctx context.Context, tenantID, id int64) error {
	_, err := s.db.Queries.RestoreIdentity(ctx, db.RestoreIdentityParams{ID: id, TenantID: tenantID})
	return err
}

func (s *IdentityService) PermanentlyDelete(ctx context.Context, tenantID, id int64) error {
	row, err := s.db.Queries.GetIdentityIncludingDeleted(ctx, id)
	if err != nil {
		return fmt.Errorf("delete identity %d: %w", id, err)
	}
	if row.TenantID != tenantID {
		return fmt.Errorf("delete identity %d: %w", id, sql.ErrNoRows)
	}
	return s.db.Queries.DeleteIdentity(ctx, id)
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
