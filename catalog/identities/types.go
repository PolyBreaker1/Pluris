// Package identities is the canonical in-memory shape for the Identity
// entity — the end-user/directory record that both owns assets and (when
// role-gated) logs into the console. pkg/services.IdentityService adapts
// database rows into this shape, mirroring how catalog/assets is the
// shape pkg/services.AssetService adapts into.
package identities

import "time"

// Role gates access per docs/endpoint-management/ui/invariants.md's locked permission matrix.
type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleAdmin      Role = "admin"
	RoleTechnician Role = "technician"
	RoleUser       Role = "user"
)

// IsValid reports whether r is one of the four locked builtin roles.
func (r Role) IsValid() bool {
	switch r {
	case RoleSuperAdmin, RoleAdmin, RoleTechnician, RoleUser:
		return true
	}
	return false
}

// Label returns the human display name for the role.
func (r Role) Label() string {
	switch r {
	case RoleSuperAdmin:
		return "Super Admin"
	case RoleAdmin:
		return "Admin"
	case RoleTechnician:
		return "Technician"
	case RoleUser:
		return "User"
	}
	return string(r)
}

// Identity is the canonical shape used by every Users surface (list,
// detail, editor) and by the Asset owner picker.
type Identity struct {
	ID       int64
	TenantID int64
	SiteID   int64  // 0 when unset
	SiteName string // resolved display name; "" when SiteID is unset or unresolved

	Username          string
	UserPrincipalName string
	Email             string
	DisplayName       string
	GivenName         string
	Surname           string
	Initials          string

	Title        string
	Department   string
	Company      string
	EmployeeID   string
	EmployeeType string
	ManagerID    int64 // 0 when unset

	PhoneOffice string
	PhoneMobile string
	PhoneHome   string
	Fax         string

	Office        string
	StreetAddress string
	City          string
	State         string
	PostalCode    string
	Country       string
	CountryCode   string

	HomeDirectory string
	HomeDrive     string
	ProfilePath   string
	LogonScript   string

	AccountEnabled       bool
	AccountLocked        bool
	AccountExpiresAt     *time.Time
	PasswordLastSetAt    *time.Time
	PasswordNeverExpires bool
	MustChangePassword   bool
	LastLogonAt          *time.Time
	LogonCount           int64
	BadPasswordCount     int64

	Role      Role
	AvatarURL string
	Locale    string
	Timezone  string

	Description string
	Notes       string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// SelfServiceEditableKeys is the allowlist of param keys a user holding
// only "own" scope on identity.update may set on THEIR OWN identity via
// the inline-edit field API (console/handlers/field_api.go). It is
// deliberately narrow: contact/personal details only, never
// role/employment/site/security fields -- those require "all" scope.
// See docs/history/specs/2026-07-08-pluris-policy-authz-design.md
// section "5. User management backend".
var SelfServiceEditableKeys = map[string]bool{
	"display_name":   true,
	"given_name":     true,
	"surname":        true,
	"initials":       true,
	"email":          true,
	"phone_office":   true,
	"phone_mobile":   true,
	"phone_home":     true,
	"fax":            true,
	"office":         true,
	"street_address": true,
	"city":           true,
	"state":          true,
	"postal_code":    true,
	"country":        true,
	"locale":         true,
	"timezone":       true,
}

// NonEditableFieldKeys are identity params that render read-only in the
// detail UI and are refused by the field-update service
// (pkg/services/identities.go's identityFieldEditable): identifiers,
// tenancy/site/manager links, denormalized caches, and system-maintained
// audit fields. Roles are managed on the Roles tab, never as a text
// field. This is the single source of truth for both the UI (users.templ)
// and the service, so the two can never diverge again.
var NonEditableFieldKeys = map[string]bool{
	"id":                   true,
	"tenant":               true,
	"site":                 true,
	"username":             true,
	"role":                 true,
	"avatar_url":           true,
	"groups":               true,
	"manager":              true,
	"password_last_set_at": true,
	"last_logon_at":        true,
	"logon_count":          true,
	"bad_password_count":   true,
}

// ResolvedDisplayName returns DisplayName if set, else "GivenName Surname"
// if either is set, else Username. Used wherever a human-readable label
// is needed and DisplayName might not have been backfilled.
func (i Identity) ResolvedDisplayName() string {
	if i.DisplayName != "" {
		return i.DisplayName
	}
	full := i.GivenName
	if i.Surname != "" {
		if full != "" {
			full += " "
		}
		full += i.Surname
	}
	if full != "" {
		return full
	}
	return i.Username
}
