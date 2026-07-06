package params

// This file defines which parameters each asset subtype mounts.
// The admin sees exactly these parameters, in this order, when viewing
// assets of the given subtype. No cross-contamination.
//
// Adding a parameter to a subtype = append its key to the relevant section.
// The parameter must already be defined in definitions.go.

// Schemas is the registry of all subtype schemas, keyed by subtype slug.
var Schemas = map[string]*SubtypeSchema{
	"computer": SchemaComputer,
	"server":   SchemaServer,
	"printer":  SchemaPrinter,
	"desk":     SchemaDesk,
	"identity": SchemaIdentity,
}

// SchemaBySubtype returns the schema for a given subtype slug, or nil.
func SchemaBySubtype(subtype string) *SubtypeSchema {
	return Schemas[subtype]
}

// ---------------------------------------------------------------------------
// Computer schema
// ---------------------------------------------------------------------------

var SchemaComputer = &SubtypeSchema{
	Subtype:     "computer",
	PathEntity:  "computer",
	Label:       "Computer",
	PluralLabel: "Computers",
	Sections: []SchemaSection{
		{Key: "identity", Label: "Identity", Params: []string{
			"name", "id", "uuid", "tenant", "site", "owner", "groups", "labels",
			"description", "managed_by",
		}},
		{Key: "hardware", Label: "Hardware", Params: []string{
			"hostname", "fqdn", "os_family", "os_distribution", "os_version", "kernel_version",
			"architecture", "cpu", "ram_mb", "serial_number", "storage_mb",
		}},
		{Key: "enrollment", Label: "Enrollment", Params: []string{
			"enrollment_state", "enrolled_at", "last_seen_at", "agent_version",
		}},
		{Key: "lifecycle", Label: "Lifecycle", Params: []string{
			"lifecycle_state", "vendor", "location", "purchase_date", "warranty_expires",
		}},
	},
	DefaultColumns: []string{"name", "os_distribution", "site", "owner", "enrollment_state", "last_seen_at"},
}

// ---------------------------------------------------------------------------
// Server schema
// ---------------------------------------------------------------------------

var SchemaServer = &SubtypeSchema{
	Subtype:     "server",
	PathEntity:  "server",
	Label:       "Server",
	PluralLabel: "Servers",
	Sections: []SchemaSection{
		{Key: "identity", Label: "Identity", Params: []string{
			"name", "id", "uuid", "tenant", "site", "owner", "groups", "labels",
			"description", "managed_by",
		}},
		{Key: "hardware", Label: "Hardware", Params: []string{
			"hostname", "fqdn", "os_family", "os_distribution", "os_version", "kernel_version",
			"architecture", "server_role", "services", "uptime_since", "ram_mb", "serial_number",
		}},
		{Key: "enrollment", Label: "Enrollment", Params: []string{
			"enrollment_state", "enrolled_at", "last_seen_at", "agent_version",
		}},
		{Key: "lifecycle", Label: "Lifecycle", Params: []string{
			"lifecycle_state", "vendor", "location", "purchase_date", "warranty_expires",
		}},
	},
	DefaultColumns: []string{"name", "server_role", "os_distribution", "site", "enrollment_state", "last_seen_at"},
}

// ---------------------------------------------------------------------------
// Printer schema
// ---------------------------------------------------------------------------

var SchemaPrinter = &SubtypeSchema{
	Subtype:     "printer",
	PathEntity:  "printer",
	Label:       "Printer",
	PluralLabel: "Printers",
	Sections: []SchemaSection{
		{Key: "identity", Label: "Identity", Params: []string{
			"name", "id", "tenant", "site", "owner", "groups", "labels", "description",
		}},
		{Key: "hardware", Label: "Hardware", Params: []string{
			"printer_model", "vendor", "printer_ip", "queues", "supported_formats", "consumables",
		}},
		{Key: "enrollment", Label: "Enrollment", Params: []string{
			"enrollment_state", "last_seen_at",
		}},
		{Key: "lifecycle", Label: "Lifecycle", Params: []string{
			"lifecycle_state", "location", "purchase_date", "warranty_expires",
		}},
	},
	DefaultColumns: []string{"name", "printer_ip", "site", "enrollment_state", "last_seen_at"},
}

// ---------------------------------------------------------------------------
// Desk schema
// ---------------------------------------------------------------------------

var SchemaDesk = &SubtypeSchema{
	Subtype:     "desk",
	PathEntity:  "desk",
	Label:       "Desk",
	PluralLabel: "Desks",
	Sections: []SchemaSection{
		{Key: "identity", Label: "Identity", Params: []string{
			"name", "id", "tenant", "site", "groups", "labels", "description",
		}},
		{Key: "hardware", Label: "Hardware", Params: []string{
			"desk_location", "dock_model", "monitor_count", "guest_profile",
		}},
		{Key: "lifecycle", Label: "Lifecycle", Params: []string{
			"lifecycle_state", "location", "purchase_date", "warranty_expires",
		}},
	},
	DefaultColumns: []string{"name", "desk_location", "dock_model", "monitor_count", "guest_profile"},
}

// ---------------------------------------------------------------------------
// Identity schema — the Users directory. Reuses the shared "tenant" and
// "site" ParamDefs already defined for Assets (docs/UX_INVARIANTS.md's
// INV-H1 shared hierarchy: Tenant → Site → Group → (Asset | Identity)).
// ---------------------------------------------------------------------------

var SchemaIdentity = &SubtypeSchema{
	Subtype:     "identity",
	PathEntity:  "user",
	Label:       "User",
	PluralLabel: "Users",
	Sections: []SchemaSection{
		{Key: "identity", Label: "Identity", Params: []string{
			"display_name", "initials", "username", "user_principal_name", "email", "tenant", "site", "role",
		}},
		{Key: "organization", Label: "Organization", Params: []string{
			"title", "department", "company", "employee_id", "employee_type", "manager",
		}},
		{Key: "contact", Label: "Contact", Params: []string{
			"phone_office", "phone_mobile", "phone_home", "fax",
		}},
		{Key: "location", Label: "Location", Params: []string{
			"office", "street_address", "city", "state", "postal_code", "country", "country_code",
		}},
		{Key: "profile", Label: "Profile & Scripts", Params: []string{
			"home_directory", "home_drive", "profile_path", "logon_script",
		}},
		{Key: "security", Label: "Security", Params: []string{
			"account_enabled", "account_locked", "account_expires_at",
			"password_last_set_at", "password_never_expires", "must_change_password",
			"last_logon_at", "logon_count", "bad_password_count",
		}},
		{Key: "preferences", Label: "Preferences", Params: []string{
			"locale", "timezone", "avatar_url",
		}},
		{Key: "metadata", Label: "Metadata", Params: []string{
			"description", "notes",
		}},
	},
	DefaultColumns: []string{
		"display_name", "username", "email", "department", "role", "site", "account_enabled",
	},
}
