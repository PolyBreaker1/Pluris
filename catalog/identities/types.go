// Package identities is the canonical in-memory shape for the Identity
// entity — the end-user/directory record that both owns assets and (when
// role-gated) logs into the console. pkg/services.IdentityService adapts
// database rows into this shape, mirroring how catalog/assets is the
// shape pkg/services.AssetService adapts into.
package identities

import "time"

// Role gates access per docs/UX_INVARIANTS.md's locked permission matrix.
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
	SiteID   int64 // 0 when unset

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
