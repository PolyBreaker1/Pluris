package params

// This file is the SINGLE SOURCE OF TRUTH for all asset parameter definitions.
// Adding a new parameter = one entry here + mounting it in the relevant schema(s).
//
// Naming convention: keys use lowercase_underscores matching the Go struct field
// names in catalog/assets/types.go (but normalized: RAMMB → ram_mb).

// All registered parameter definitions, keyed by ParamDef.Key.
var Definitions = map[string]ParamDef{}

// All registered link definitions, keyed by ParamDef.Key.
var Links = map[string]LinkDef{}

func init() {
	for _, d := range allDefs {
		Definitions[d.Key] = d
	}
	for _, l := range allLinks {
		Links[l.ParamKey] = l
	}
}

// DefByKey returns a parameter definition by key, or nil if not found.
func DefByKey(key string) *ParamDef {
	if d, ok := Definitions[key]; ok {
		return &d
	}
	return nil
}

// DefsForKeys returns definitions for a list of keys in order.
// Unknown keys are silently skipped.
func DefsForKeys(keys []string) []ParamDef {
	out := make([]ParamDef, 0, len(keys))
	for _, k := range keys {
		if d, ok := Definitions[k]; ok {
			out = append(out, d)
		}
	}
	return out
}

// SharedWith returns all subtype keys that also mount the given param key.
// Enables cross-subtype queries: "which subtypes have RAM?"
func SharedWith(paramKey string) []string {
	var out []string
	for _, s := range Schemas {
		if s.HasParam(paramKey) {
			out = append(out, s.Subtype)
		}
	}
	return out
}

// HasParam reports whether the schema includes the given parameter key.
func (s *SubtypeSchema) HasParam(key string) bool {
	for _, sec := range s.Sections {
		for _, k := range sec.Params {
			if k == key {
				return true
			}
		}
	}
	return false
}

// AllParamKeys returns every parameter key mounted by this schema, in section order.
func (s *SubtypeSchema) AllParamKeys() []string {
	var out []string
	for _, sec := range s.Sections {
		out = append(out, sec.Params...)
	}
	return out
}

// ---------------------------------------------------------------------------
// Parameter definitions — one entry per concept. Alphabetical by key.
// ---------------------------------------------------------------------------

var allDefs = []ParamDef{
	// --- Identity ---
	{Key: "id", Label: "Asset ID", Description: "Stable dotted slug (comp.acme.hq.lt-0142). Unique per tenant.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "uuid", Label: "UUID", Description: "Agent-side mTLS certificate subject CN. Empty for desks.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "name", Label: "Name", Description: "Primary display name (hostname for computers/servers, model for printers, location for desks).", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "tenant", Label: "Tenant", Description: "Owning tenant identifier.", Category: "identity", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},

	// --- Links (displayed as values, resolved through entities) ---
	{Key: "site", Label: "Site", Description: "Physical or logical site.", Category: "identity", Type: TypeLink, LinkTarget: "site", Filter: FilterEquals, Sort: SortAlpha},
	{Key: "owner", Label: "Owner", Description: "Assigned end-user identity. Blank for shared/pool assets.", Category: "identity", Type: TypeLink, LinkTarget: "user", Filter: FilterEquals, Sort: SortAlpha},
	{Key: "groups", Label: "Groups", Description: "Group memberships for policy assignment.", Category: "identity", Type: TypeLink, LinkTarget: "group", Filter: FilterHas, Sort: SortAlpha},
	{Key: "labels", Label: "Labels", Description: "Free-form key=value tags for dashboard filters.", Category: "identity", Type: TypeMap, Filter: FilterHas},

	// --- Enrollment (agent-reported state) ---
	{Key: "enrollment_state", Label: "Enrollment", Description: "Agent enrollment state (pending/approved/enrolled/disabled/revoked).", Category: "enrollment", Type: TypeEnum, EnumValues: []string{"pending", "approved", "enrolled", "disabled", "revoked"}, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "enrolled_at", Label: "Enrolled at", Description: "Timestamp of first agent check-in.", Category: "enrollment", Type: TypeTime, Filter: FilterDateGte, Sort: SortDate},
	{Key: "last_seen_at", Label: "Last seen", Description: "Most recent agent heartbeat.", Category: "enrollment", Type: TypeTime, Filter: FilterDateGte, Sort: SortDate},
	{Key: "agent_version", Label: "Agent version", Description: "Running agent semver. Empty for desks and pending assets.", Category: "enrollment", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},

	// --- Lifecycle (CMDB / asset management) ---
	{Key: "lifecycle_state", Label: "Lifecycle", Description: "CMDB lifecycle (active/in_repair/decommissioned/disposed).", Category: "lifecycle", Type: TypeEnum, EnumValues: []string{"active", "in_repair", "decommissioned", "disposed"}, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "vendor", Label: "Vendor", Description: "Hardware manufacturer.", Category: "lifecycle", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "location", Label: "Location", Description: "Free-text physical location.", Category: "lifecycle", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "purchase_date", Label: "Purchased", Description: "Date the org acquired the asset.", Category: "lifecycle", Type: TypeDate, Filter: FilterDateGte, Sort: SortDate},
	{Key: "warranty_expires", Label: "Warranty until", Description: "Warranty expiry date.", Category: "lifecycle", Type: TypeDate, Filter: FilterDateLte, Sort: SortDate},

	// --- Hardware: OS (shared by computer + server) ---
	{Key: "hostname", Label: "Hostname", Description: "Network hostname.", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "fqdn", Label: "FQDN", Description: "Fully qualified domain name.", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "os_family", Label: "OS family", Description: "Operating system family (linux/windows/macos).", Category: "hardware", Type: TypeEnum, EnumValues: []string{"linux", "windows", "macos"}, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "os_package_family", Label: "Package format", Description: "OS package family (rpm/deb/arch/apk).", Category: "hardware", Type: TypeEnum, EnumValues: []string{"rpm", "deb", "arch", "apk", "other"}, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "disk_encryption", Label: "Disk encryption", Description: "Primary disk encryption mechanism.", Category: "hardware", Type: TypeEnum, EnumValues: []string{"none", "bitlocker", "luks", "filevault", "other"}, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "os_distribution", Label: "OS distribution", Description: "Distribution name (Ubuntu, Fedora, Windows 11 Pro, ...).", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "os_version", Label: "OS version", Description: "Distribution version string.", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "kernel_version", Label: "Kernel", Description: "Kernel version string (e.g., 6.8.0-40-generic).", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "architecture", Label: "Architecture", Description: "CPU architecture (x86_64/arm64/...).", Category: "hardware", Type: TypeEnum, EnumValues: []string{"x86_64", "arm64", "armv7", "riscv64"}, Filter: FilterEquals, Sort: SortAlpha, Mono: true},

	// --- Hardware: Compute (shared by computer + server) ---
	{Key: "cpu", Label: "CPU", Description: "Processor summary.", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "ram_mb", Label: "RAM", Description: "Installed memory.", Category: "hardware", Type: TypeInt, Unit: "MB", Filter: FilterNumGte, Sort: SortNum},
	{Key: "serial_number", Label: "Serial Number", Description: "Manufacturer serial number for warranty tracking.", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "storage_mb", Label: "Storage", Description: "Primary storage capacity.", Category: "hardware", Type: TypeInt, Unit: "MB", Filter: FilterNumGte, Sort: SortNum},

	// --- Hardware: Server-specific ---
	{Key: "server_role", Label: "Role", Description: "Server role classification (db/web/dns/file/app/other).", Category: "hardware", Type: TypeEnum, EnumValues: []string{"db", "web", "dns", "file", "app", "other"}, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "services", Label: "Services", Description: "Declared services running on the server.", Category: "hardware", Type: TypeList, Filter: FilterHas, Sort: SortAlpha},
	{Key: "uptime_since", Label: "Uptime since", Description: "Timestamp of last reboot.", Category: "hardware", Type: TypeTime, Filter: FilterDateGte, Sort: SortDate},

	// --- Hardware: Printer-specific ---
	{Key: "printer_model", Label: "Model", Description: "Printer model name.", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "printer_ip", Label: "IP address", Description: "Network address.", Category: "network", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "queues", Label: "Queues", Description: "Configured print queue names.", Category: "hardware", Type: TypeList, Filter: FilterHas},
	{Key: "supported_formats", Label: "Formats", Description: "Supported print formats (PCL6, PostScript, PDF, ...).", Category: "hardware", Type: TypeList, Filter: FilterHas},
	{Key: "consumables", Label: "Consumables", Description: "Toner/drum levels.", Category: "hardware", Type: TypeCompound, CompoundFields: []string{"consumable_kind", "consumable_level"}},
	{Key: "consumable_kind", Label: "Type", Description: "Consumable type (toner.black, drum, ...).", Category: "hardware", Type: TypeString},
	{Key: "consumable_level", Label: "Level", Description: "Current level percentage.", Category: "hardware", Type: TypeInt, Unit: "%", Filter: FilterNumLte, Sort: SortNum},

	// --- Hardware: Desk-specific ---
	{Key: "desk_location", Label: "Location", Description: "Desk location label.", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "dock_model", Label: "Dock model", Description: "Docking station model.", Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "monitor_count", Label: "Monitors", Description: "Number of connected monitors.", Category: "hardware", Type: TypeInt, Filter: FilterNumGte, Sort: SortNum},
	{Key: "guest_profile", Label: "Guest profile", Description: "Profile auto-merged when a user docks here.", Category: "hardware", Type: TypeLink, LinkTarget: "profile", Filter: FilterEquals, Sort: SortAlpha, Mono: true},

	// --- Identity (account) ---
	{Key: "username", Label: "Username", Description: "Login/sAMAccountName-style short name.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "user_principal_name", Label: "UPN", Description: "user@domain-style principal name.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "email", Label: "Email", Description: "Primary email address.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "display_name", Label: "Name", Description: "Primary display name shown across the console.", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "given_name", Label: "First name", Description: "Given name (AD: givenName).", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "surname", Label: "Last name", Description: "Surname (AD: sn).", Category: "identity", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "role", Label: "Role", Description: "Console access role (super_admin/admin/technician/user).", Category: "identity", Type: TypeEnum, EnumValues: []string{"super_admin", "admin", "technician", "user"}, Filter: FilterEquals, Sort: SortAlpha},

	// --- Organization ---
	{Key: "title", Label: "Job title", Description: "Job title.", Category: "organization", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "department", Label: "Department", Description: "Department name.", Category: "organization", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "company", Label: "Company", Description: "Company name.", Category: "organization", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "employee_id", Label: "Employee ID", Description: "Internal employee identifier.", Category: "organization", Type: TypeString, Filter: FilterContains, Sort: SortAlpha, Mono: true},
	{Key: "employee_type", Label: "Employee type", Description: "full-time / contractor / etc.", Category: "organization", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "manager", Label: "Manager", Description: "Reporting manager.", Category: "organization", Type: TypeLink, LinkTarget: "user", Filter: FilterEquals, Sort: SortAlpha},

	// --- Contact ---
	{Key: "phone_office", Label: "Office phone", Description: "Office telephone number.", Category: "contact", Type: TypeString, Filter: FilterContains, Sort: SortNone},
	{Key: "phone_mobile", Label: "Mobile phone", Description: "Mobile telephone number.", Category: "contact", Type: TypeString, Filter: FilterContains, Sort: SortNone},
	{Key: "phone_home", Label: "Home phone", Description: "Home telephone number.", Category: "contact", Type: TypeString, Filter: FilterNone, Sort: SortNone},
	{Key: "fax", Label: "Fax", Description: "Fax number.", Category: "contact", Type: TypeString, Filter: FilterNone, Sort: SortNone},

	// --- Location (free-text physical address; distinct from the Site hierarchy link) ---
	{Key: "office", Label: "Office", Description: "Office name/number.", Category: "location", Type: TypeString, Filter: FilterContains, Sort: SortAlpha},
	{Key: "street_address", Label: "Street address", Description: "Street address.", Category: "location", Type: TypeString, Filter: FilterNone, Sort: SortNone},
	{Key: "city", Label: "City", Description: "City.", Category: "location", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "state", Label: "State/Province", Description: "State or province.", Category: "location", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "postal_code", Label: "Postal code", Description: "Postal/ZIP code.", Category: "location", Type: TypeString, Filter: FilterNone, Sort: SortNone},
	{Key: "country", Label: "Country", Description: "Country name.", Category: "location", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "country_code", Label: "Country code", Description: "ISO country code.", Category: "location", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha, Mono: true},

	// --- Profile & scripts (Windows-familiar) ---
	{Key: "home_directory", Label: "Home directory", Description: "Network home directory path.", Category: "profile", Type: TypeString, Filter: FilterNone, Sort: SortNone, Mono: true},
	{Key: "home_drive", Label: "Home drive", Description: "Mapped drive letter (e.g. H:).", Category: "profile", Type: TypeString, Filter: FilterNone, Sort: SortNone, Mono: true},
	{Key: "profile_path", Label: "Profile path", Description: "Roaming profile path.", Category: "profile", Type: TypeString, Filter: FilterNone, Sort: SortNone, Mono: true},
	{Key: "logon_script", Label: "Logon script", Description: "Logon script path.", Category: "profile", Type: TypeString, Filter: FilterNone, Sort: SortNone, Mono: true},

	// --- Security / login (display-only; not user-editable via the generic form) ---
	{Key: "account_enabled", Label: "Enabled", Description: "Whether this account can log in.", Category: "security", Type: TypeBool, Filter: FilterEquals, Sort: SortNone},
	{Key: "account_locked", Label: "Locked", Description: "Whether this account is locked after repeated failed logins.", Category: "security", Type: TypeBool, Filter: FilterEquals, Sort: SortNone},
	{Key: "account_expires_at", Label: "Account expires", Description: "Optional account expiry date.", Category: "security", Type: TypeTime, Filter: FilterDateLte, Sort: SortDate},
	{Key: "password_last_set_at", Label: "Password last set", Description: "When the password was last changed.", Category: "security", Type: TypeTime, Filter: FilterDateGte, Sort: SortDate},
	{Key: "password_never_expires", Label: "Password never expires", Description: "Password expiry exemption flag.", Category: "security", Type: TypeBool, Filter: FilterEquals, Sort: SortNone},
	{Key: "must_change_password", Label: "Must change password", Description: "Forces a password change on next login.", Category: "security", Type: TypeBool, Filter: FilterEquals, Sort: SortNone},
	{Key: "last_logon_at", Label: "Last logon", Description: "Most recent successful login.", Category: "security", Type: TypeTime, Filter: FilterDateGte, Sort: SortDate},
	{Key: "logon_count", Label: "Logon count", Description: "Total successful logins.", Category: "security", Type: TypeInt, Filter: FilterNumGte, Sort: SortNum},
	{Key: "bad_password_count", Label: "Failed logins", Description: "Consecutive failed login attempts since the last success.", Category: "security", Type: TypeInt, Filter: FilterNumGte, Sort: SortNum},

	// --- Preferences ---
	{Key: "locale", Label: "Locale", Description: "Preferred locale (e.g. en-US).", Category: "preferences", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "timezone", Label: "Timezone", Description: "Preferred timezone.", Category: "preferences", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha},
	{Key: "avatar_url", Label: "Avatar", Description: "Profile picture URL.", Category: "preferences", Type: TypeString, Filter: FilterNone, Sort: SortNone},

	// --- Metadata ---
	{Key: "description", Label: "Description", Description: "Free-text description.", Category: "metadata", Type: TypeString, Filter: FilterContains, Sort: SortNone},
	{Key: "notes", Label: "Notes", Description: "Internal admin notes.", Category: "metadata", Type: TypeString, Filter: FilterNone, Sort: SortNone},

	// --- Task 4 additions ---
	{Key: "managed_by", Label: "Managed by", Description: "Responsible administrator or owner of record.", Category: "identity", Type: TypeLink, LinkTarget: "user", Filter: FilterEquals, Sort: SortAlpha},
	{Key: "initials", Label: "Initials", Description: "Name initials.", Category: "identity", Type: TypeString, Filter: FilterNone, Sort: SortNone},
}

// ---------------------------------------------------------------------------
// Link definitions — relationships that resolve through other entities.
// ---------------------------------------------------------------------------

var allLinks = []LinkDef{
	{ParamKey: "owner", TargetEntity: "user", Cardinality: "one", Bidirectional: true, InverseLabel: "Assigned assets"},
	{ParamKey: "site", TargetEntity: "site", Cardinality: "one", Bidirectional: true, InverseLabel: "Assets at site"},
	{ParamKey: "groups", TargetEntity: "group", Cardinality: "many", Bidirectional: true, InverseLabel: "Member assets"},
	{ParamKey: "guest_profile", TargetEntity: "profile", Cardinality: "one", Bidirectional: true, InverseLabel: "Applied at desks"},
	{ParamKey: "manager", TargetEntity: "user", Cardinality: "one", Bidirectional: true, InverseLabel: "Direct reports"},
	{ParamKey: "managed_by", TargetEntity: "user", Cardinality: "one", Bidirectional: true, InverseLabel: "Manages assets"},
}
