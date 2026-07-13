// Package lists — Users (Identity) column registry.
//
// Mirrors assets.go: ALL column definitions derive from the param
// registry (catalog/params.SchemaIdentity). No independent field
// declarations. Unlike assets.go, there is only one schema to merge
// (identities aren't split into subtypes the way assets are), so the
// group/field builder here iterates SchemaIdentity.Sections directly
// instead of unioning several SubtypeSchemas.
//
// The only identity-specific logic here is the cell renderer (how to
// extract and format a value from an identities.Identity for display).
package lists

import (
	"strconv"
	"time"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/params"
)

// init registers the Users list with fields derived from the param registry.
func init() {
	groups, fields := buildIdentitiesFieldsFromParams()
	Register(ListIDIdentities, "Users", groups, fields)
}

// buildIdentitiesFieldsFromParams generates FieldGroup and FieldDef slices
// from catalog/params.SchemaIdentity. Group keys are the schema's section
// keys verbatim ("identity", "organization", "contact", "location",
// "profile", "security", "preferences", "metadata") — Register panics at
// init() time if any field references a group key not in this list, so
// this function is the single place that must stay in lockstep with
// schemas.go's SchemaIdentity.Sections.
func buildIdentitiesFieldsFromParams() ([]FieldGroup, []FieldDef) {
	schema := params.SchemaIdentity

	groups := make([]FieldGroup, 0, len(schema.Sections))
	for _, sec := range schema.Sections {
		groups = append(groups, FieldGroup{
			Key:   sec.Key,
			Label: sec.Label,
		})
	}

	defaultCols := map[string]bool{}
	for _, key := range schema.DefaultColumns {
		defaultCols[key] = true
	}

	fields := make([]FieldDef, 0, 40)
	for _, sec := range schema.Sections {
		for _, key := range sec.Params {
			def := params.DefByKey(key)
			if def == nil {
				continue
			}
			// Skip compound sub-fields (they're rendered as part of their parent).
			if def.Type == params.TypeCompound {
				continue
			}
			fields = append(fields, FieldDef{
				Key:            def.Key,
				Label:          def.Label,
				Description:    def.Description,
				Group:          sec.Key,
				Width:          paramWidth(def),
				Sort:           string(def.Sort),
				DefaultVisible: defaultCols[def.Key],
			})
		}
	}

	return groups, fields
}

// =========================================================================
// Cell Renderer — extracts and formats values from identities.Identity
// =========================================================================

// RenderIdentityCell returns the HTML content for a table cell.
func RenderIdentityCell(key string, u identities.Identity) string {
	val := getIdentityParamValue(key, u)
	def := params.DefByKey(key)
	if def == nil {
		return esc(val)
	}
	return formatIdentityParamValue(val, def, u)
}

// RenderIdentityCellSort returns the sortable value for data-sort attribute.
func RenderIdentityCellSort(key string, u identities.Identity) string {
	return getIdentityParamValue(key, u)
}

// GetIdentityRawValue returns the raw string value of a param for
// clipboard/display (mirrors GetAssetRawValue; used by a future detail
// page in the same way buildDetailSections uses GetAssetRawValue today).
func GetIdentityRawValue(key string, u identities.Identity) string {
	return getIdentityParamValue(key, u)
}

// getIdentityParamValue extracts the raw string value of a param from an
// Identity. This is the single function that maps param keys to struct
// fields — the identity-flavored equivalent of getAssetParamValue.
func getIdentityParamValue(key string, u identities.Identity) string {
	switch key {
	case "display_name":
		return u.ResolvedDisplayName()
	case "given_name":
		return u.GivenName
	case "surname":
		return u.Surname
	case "initials":
		return u.Initials
	case "username":
		return u.Username
	case "user_principal_name":
		return u.UserPrincipalName
	case "email":
		return u.Email
	case "tenant":
		return strconv.FormatInt(u.TenantID, 10)
	case "site":
		// TODO: resolve to a display name once a site-lookup exists; renders the raw numeric ID for now.
		if u.SiteID == 0 {
			return ""
		}
		return strconv.FormatInt(u.SiteID, 10)
	case "role":
		return string(u.Role)
	case "title":
		return u.Title
	case "department":
		return u.Department
	case "company":
		return u.Company
	case "employee_id":
		return u.EmployeeID
	case "employee_type":
		return u.EmployeeType
	case "manager":
		// TODO: resolve to a display name once a user-lookup-by-ID exists; renders the raw numeric ID for now.
		if u.ManagerID == 0 {
			return ""
		}
		return strconv.FormatInt(u.ManagerID, 10)
	case "phone_office":
		return u.PhoneOffice
	case "phone_mobile":
		return u.PhoneMobile
	case "phone_home":
		return u.PhoneHome
	case "fax":
		return u.Fax
	case "office":
		return u.Office
	case "street_address":
		return u.StreetAddress
	case "city":
		return u.City
	case "state":
		return u.State
	case "postal_code":
		return u.PostalCode
	case "country":
		return u.Country
	case "country_code":
		return u.CountryCode
	case "home_directory":
		return u.HomeDirectory
	case "home_drive":
		return u.HomeDrive
	case "profile_path":
		return u.ProfilePath
	case "logon_script":
		return u.LogonScript
	case "account_enabled":
		return strconv.FormatBool(u.AccountEnabled)
	case "account_locked":
		return strconv.FormatBool(u.AccountLocked)
	case "account_expires_at":
		if u.AccountExpiresAt == nil || u.AccountExpiresAt.IsZero() {
			return ""
		}
		return u.AccountExpiresAt.UTC().Format(time.RFC3339)
	case "password_last_set_at":
		if u.PasswordLastSetAt == nil || u.PasswordLastSetAt.IsZero() {
			return ""
		}
		return u.PasswordLastSetAt.UTC().Format(time.RFC3339)
	case "password_never_expires":
		return strconv.FormatBool(u.PasswordNeverExpires)
	case "must_change_password":
		return strconv.FormatBool(u.MustChangePassword)
	case "last_logon_at":
		if u.LastLogonAt == nil || u.LastLogonAt.IsZero() {
			return ""
		}
		return u.LastLogonAt.UTC().Format(time.RFC3339)
	case "logon_count":
		return strconv.FormatInt(u.LogonCount, 10)
	case "bad_password_count":
		return strconv.FormatInt(u.BadPasswordCount, 10)
	case "locale":
		return u.Locale
	case "timezone":
		return u.Timezone
	case "avatar_url":
		return u.AvatarURL
	case "description":
		return u.Description
	case "notes":
		return u.Notes
	}
	return ""
}

// =========================================================================
// Value Formatting — renders raw param values as HTML for table cells
// =========================================================================
//
// Reuses the shared formatEnum/formatTime/formatInt/formatList/formatMap/
// formatLink/esc/mutedDash helpers already defined in assets.go (same
// package). The only new case is TypeBool, which no asset param uses
// today, so assets.go's formatParamValue never needed it.

func formatIdentityParamValue(val string, def *params.ParamDef, u identities.Identity) string {
	if val == "" {
		return mutedDash()
	}

	switch def.Type {
	case params.TypeEnum:
		return formatEnum(val, def)
	case params.TypeTime:
		return formatTime(val)
	case params.TypeDate:
		return esc(val)
	case params.TypeInt:
		return formatInt(val, def)
	case params.TypeBool:
		return formatIdentityBool(val == "true")
	case params.TypeList:
		return formatList(val)
	case params.TypeMap:
		return formatMap(val)
	case params.TypeLink:
		return formatLink(val, def)
	}

	// String type
	if def.Key == "display_name" {
		// Special: name cell shows display name + username, mirroring how
		// formatParamValue shows the asset's hostname + ID.
		return `<div class="policy-name">` + esc(val) + `</div>` +
			`<div class="policy-id font-mono">` + esc(u.Username) + `</div>`
	}
	if def.Mono {
		return `<span class="font-mono">` + esc(val) + `</span>`
	}
	return esc(val)
}

// formatIdentityBool renders a TypeBool param as a small chip.
func formatIdentityBool(b bool) string {
	if b {
		return `<span class="pm-origin-chip">Yes</span>`
	}
	return `<span class="policy-muted">No</span>`
}
