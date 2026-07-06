package templates

import (
	"html"
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/identities"
)

// Helpers for the UsersPage template. Kept in a plain .go file (not
// _templ.go) so they can be tested directly and so the template stays
// focused on rendering. Mirrors assets_helpers.go's filter-attrs /
// search-blob helpers.

// identitySearchBlob — single lowercased string the lists.js engine
// matches against in `contains` mode (data-searchable attribute).
// Mirrors assetSearchBlob's shape.
func identitySearchBlob(u identities.Identity) string {
	parts := []string{
		strconv.FormatInt(u.ID, 10),
		u.DisplayName, u.Username, u.UserPrincipalName, u.Email,
		u.GivenName, u.Surname,
		u.Title, u.Department, u.Company, u.EmployeeID, u.EmployeeType,
		u.PhoneOffice, u.PhoneMobile,
		u.Office, u.City, u.State, u.Country,
		string(u.Role),
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// identitySearchBlobEscaped — HTML-escaped version for embedding in raw
// attribute strings.
func identitySearchBlobEscaped(u identities.Identity) string {
	return html.EscapeString(identitySearchBlob(u))
}

// identityFilterAttrs returns HTML data attributes for each param registry
// key mounted by catalog/params.SchemaIdentity. These are emitted on each
// <tr> so the advanced filter's getVal() can look up values by param key
// directly via row.dataset[camelCase]. Mirrors assetFilterAttrs.
//
// Keys use the param registry format (lowercase_underscores). The browser
// converts data-account-enabled="true" to dataset.accountEnabled.
func identityFilterAttrs(u identities.Identity) string {
	var b strings.Builder
	w := func(key, val string) {
		if val == "" {
			return
		}
		b.WriteString(` data-`)
		b.WriteString(strings.ReplaceAll(key, "_", "-"))
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(val))
		b.WriteString(`"`)
	}
	// Identity
	w("display-name", u.ResolvedDisplayName())
	w("username", u.Username)
	w("user-principal-name", u.UserPrincipalName)
	w("email", u.Email)
	w("tenant", strconv.FormatInt(u.TenantID, 10))
	if u.SiteID != 0 {
		w("site", strconv.FormatInt(u.SiteID, 10))
	}
	w("role", string(u.Role))
	// Organization
	w("title", u.Title)
	w("department", u.Department)
	w("company", u.Company)
	w("employee-id", u.EmployeeID)
	w("employee-type", u.EmployeeType)
	if u.ManagerID != 0 {
		w("manager", strconv.FormatInt(u.ManagerID, 10))
	}
	// Contact
	w("phone-office", u.PhoneOffice)
	w("phone-mobile", u.PhoneMobile)
	w("phone-home", u.PhoneHome)
	w("fax", u.Fax)
	// Location
	w("office", u.Office)
	w("street-address", u.StreetAddress)
	w("city", u.City)
	w("state", u.State)
	w("postal-code", u.PostalCode)
	w("country", u.Country)
	w("country-code", u.CountryCode)
	// Profile & scripts
	w("home-directory", u.HomeDirectory)
	w("home-drive", u.HomeDrive)
	w("profile-path", u.ProfilePath)
	w("logon-script", u.LogonScript)
	// Security
	w("account-enabled", strconv.FormatBool(u.AccountEnabled))
	w("account-locked", strconv.FormatBool(u.AccountLocked))
	if u.AccountExpiresAt != nil && !u.AccountExpiresAt.IsZero() {
		w("account-expires-at", u.AccountExpiresAt.UTC().Format("2006-01-02"))
	}
	if u.PasswordLastSetAt != nil && !u.PasswordLastSetAt.IsZero() {
		w("password-last-set-at", u.PasswordLastSetAt.UTC().Format("2006-01-02"))
	}
	w("password-never-expires", strconv.FormatBool(u.PasswordNeverExpires))
	w("must-change-password", strconv.FormatBool(u.MustChangePassword))
	if u.LastLogonAt != nil && !u.LastLogonAt.IsZero() {
		w("last-logon-at", u.LastLogonAt.UTC().Format("2006-01-02T15:04:05Z"))
	}
	w("logon-count", strconv.FormatInt(u.LogonCount, 10))
	w("bad-password-count", strconv.FormatInt(u.BadPasswordCount, 10))
	// Preferences
	w("locale", u.Locale)
	w("timezone", u.Timezone)
	// Metadata
	w("description", u.Description)
	w("notes", u.Notes)
	return b.String()
}
