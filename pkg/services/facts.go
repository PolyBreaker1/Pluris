package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pluris/pluris/db"
)

// FactsForAsset and FactsForIdentity build the device/identity fact maps
// the dependency-group eval engine (catalog/dependencygroups/eval.go)
// consumes. They are the SINGLE source of truth for turning a stored
// asset/identity row into facts: DependencyGroupService.EvaluateForAsset
// (dependencygroups.go) and EvaluateDynamicMembership (groups.go, Task
// 6.1) both call these rather than each re-deriving facts from a row —
// one rule system across the product means one fact-building path too.
//
// Keys match catalog/dependencygroups/eval.go's paramKey: the TRAILING
// segment of a canonical path (e.g. "computer/hardware/os_family" ->
// "os_family"), never the full path. That is what every existing
// dependency-group condition (see pkg/services/dependencygroups.go's
// builtinGroups) and every group_membership_rules row already assumes.

// toFactString renders an arbitrary decoded JSON value (string, bool,
// float64, or anything else via fmt.Sprint) as the plain string facts
// are compared against. nil is omitted by the caller before this is
// reached.
func toFactString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(t)
	}
}

// FactsForAsset builds the fact map for one asset: every key in its
// subtype_payload JSON (hostname, os_family, os_package_family,
// disk_encryption, etc. — subtype specific), plus the common asset
// columns that also appear as canonical param paths under the
// "computer"/"server"/"printer"/"desk" entities (see
// catalog/params/schemas.go's "hardware"/"enrollment"/"lifecycle"
// sections). A malformed subtype_payload yields facts from the columns
// alone rather than erroring — evalCondition already treats a missing
// fact as "unknown", not a false pass, so a partial fact map degrades
// gracefully.
func FactsForAsset(row db.Asset) map[string]string {
	facts := map[string]string{}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(row.SubtypePayload), &payload); err == nil {
		for k, v := range payload {
			if v == nil {
				continue
			}
			facts[k] = toFactString(v)
		}
	}

	facts["subtype"] = row.Subtype
	facts["uuid"] = row.Uuid
	facts["enrollment_state"] = row.EnrollmentState
	if row.HumanID.Valid {
		facts["human_id"] = row.HumanID.String
	}
	if row.LifecycleState.Valid {
		facts["lifecycle_state"] = row.LifecycleState.String
	}
	if row.Vendor.Valid {
		facts["vendor"] = row.Vendor.String
	}
	if row.Location.Valid {
		facts["location"] = row.Location.String
	}
	if row.AgentVersion.Valid {
		facts["agent_version"] = row.AgentVersion.String
	}
	if row.Description.Valid {
		facts["description"] = row.Description.String
	}
	return facts
}

// FactsForIdentity builds the fact map for one identity, keyed the same
// way as FactsForAsset — trailing canonical-path segment, matching the
// "user/..." paths catalog/params/schemas.go's SchemaIdentity (PathEntity
// "user") registers (e.g. "user/organization/department" -> "department").
// Boolean/numeric columns render the same as JSON facts would
// (strconv, not fmt, so "true"/"false" and plain integers).
func FactsForIdentity(row db.Identity) map[string]string {
	facts := map[string]string{
		"display_name":           row.DisplayName,
		"username":               row.Username,
		"email":                  row.Email,
		"role":                   row.Role,
		"account_enabled":        strconv.FormatBool(row.AccountEnabled),
		"account_locked":         strconv.FormatBool(row.AccountLocked),
		"password_never_expires": strconv.FormatBool(row.PasswordNeverExpires),
		"must_change_password":   strconv.FormatBool(row.MustChangePassword),
		"logon_count":            strconv.FormatInt(row.LogonCount, 10),
		"bad_password_count":     strconv.FormatInt(row.BadPasswordCount, 10),
		"locale":                 row.Locale,
		"timezone":               row.Timezone,
	}
	strFacts := map[string]sql.NullString{
		"user_principal_name": row.UserPrincipalName,
		"given_name":          row.GivenName,
		"surname":             row.Surname,
		"initials":            row.Initials,
		"title":               row.Title,
		"department":          row.Department,
		"company":             row.Company,
		"employee_id":         row.EmployeeID,
		"employee_type":       row.EmployeeType,
		"phone_office":        row.PhoneOffice,
		"phone_mobile":        row.PhoneMobile,
		"phone_home":          row.PhoneHome,
		"fax":                 row.Fax,
		"office":              row.Office,
		"street_address":      row.StreetAddress,
		"city":                row.City,
		"state":               row.State,
		"postal_code":         row.PostalCode,
		"country":             row.Country,
		"country_code":        row.CountryCode,
		"home_directory":      row.HomeDirectory,
		"home_drive":          row.HomeDrive,
		"profile_path":        row.ProfilePath,
		"logon_script":        row.LogonScript,
		"avatar_url":          row.AvatarUrl,
		"description":         row.Description,
		"notes":               row.Notes,
	}
	for k, v := range strFacts {
		if v.Valid {
			facts[k] = v.String
		}
	}
	return facts
}
