// Package params is the canonical parameter definition registry for Pluris.
//
// It implements the "Parameter Definitions + Subtype Schemas + Links" architecture:
//
//   - A ParamDef defines WHAT a parameter IS (type, unit, filter behavior, display).
//     Each parameter is defined exactly once. "RAM is RAM" — whether it appears on
//     a computer or a server, it references the same definition.
//
//   - A Schema declares which ParamDefs a given subtype MOUNTS. When an admin views
//     a computer, they see exactly the parameters the computer schema declares.
//     No printer noise. The schema also controls display order and grouping.
//
//   - A LinkDef declares a relationship parameter that resolves through another
//     entity (owner → User, site → Site, groups → Group). These appear in the
//     GUI like regular parameters but the system knows they're traversals.
//
// This package is the single source of truth for:
//   - What parameters exist and what they mean
//   - Which subtypes carry which parameters
//   - How parameters are filtered, sorted, and displayed
//   - Which parameters are shared across subtypes (for cross-subtype queries)
//
// Rule alignment:
//   - R1: One definition per parameter concept. No duplication.
//   - R2: All surfaces (list columns, detail pages, filters, future APIs)
//         read from this registry. No parallel field declarations.
//   - R3: Hierarchy parity maintained — links resolve through the shared
//         Tenant → Site → Group → (Asset | Identity) model.
package params

// ParamType classifies the value type of a parameter definition.
type ParamType string

const (
	TypeString  ParamType = "string"  // Free-text string
	TypeInt     ParamType = "int"     // Integer (may have Unit)
	TypeFloat   ParamType = "float"   // Floating point (may have Unit)
	TypeEnum    ParamType = "enum"    // Constrained set of values
	TypeBool    ParamType = "bool"    // True/false
	TypeTime    ParamType = "time"    // Timestamp (RFC3339)
	TypeDate    ParamType = "date"    // Date only (YYYY-MM-DD)
	TypeList    ParamType = "list"    // Ordered list of strings
	TypeMap     ParamType = "map"     // Key-value map (labels)
	TypeLink    ParamType = "link"    // Reference to another entity (see LinkDef)
	TypeCompound ParamType = "compound" // Structured sub-fields (e.g. consumable: {kind, level})
)

// FilterMode defines how a parameter can be filtered in list views.
type FilterMode string

const (
	FilterNone     FilterMode = ""         // Not filterable
	FilterContains FilterMode = "contains" // Substring match (text search)
	FilterEquals   FilterMode = "equals"   // Exact match / enum select
	FilterPrefix   FilterMode = "prefix"   // Starts-with
	FilterNumGte   FilterMode = "numGte"   // Numeric >= threshold
	FilterNumLte   FilterMode = "numLte"   // Numeric <= threshold
	FilterDateGte  FilterMode = "dateGte"  // Date on or after
	FilterDateLte  FilterMode = "dateLte"  // Date on or before
	FilterHas      FilterMode = "has"      // List/map contains value
)

// SortType defines how a parameter sorts in list views.
type SortType string

const (
	SortNone    SortType = ""      // Not sortable
	SortAlpha   SortType = "alpha" // Case-insensitive string, default asc
	SortNum     SortType = "num"   // Numeric, default desc
	SortDate    SortType = "date"  // Chronological, default desc
)

// ParamDef is a single canonical parameter definition. Defined once,
// referenced by any number of subtype schemas.
type ParamDef struct {
	// Key is the stable identifier. Used in code, storage, URLs, and
	// filter query params. Must be unique across the entire registry.
	// Format: lowercase, underscores. Examples: "ram_mb", "os_distribution".
	Key string

	// Label is the short human display name. Shown in column headers,
	// detail page <dt> elements, and filter dropdowns.
	Label string

	// Description is a one-sentence explanation. Shown in column picker
	// tooltips and documentation.
	Description string

	// Category groups parameters for display in detail pages and the
	// column picker. Examples: "hardware", "network", "enrollment".
	Category string

	// Type classifies the value for rendering, validation, and filtering.
	Type ParamType

	// Unit is the display unit suffix (e.g. "MB", "GB", "%"). Empty if unitless.
	Unit string

	// EnumValues lists valid values when Type == TypeEnum. Order matters (display order).
	EnumValues []string

	// Filter declares the primary filter mode for list views. FilterNone = not filterable.
	Filter FilterMode

	// Sort declares the sort behavior. SortNone = not sortable.
	Sort SortType

	// Mono renders the value in monospace font (IDs, FQDNs, IPs, versions).
	Mono bool

	// LinkTarget is the target entity kind when Type == TypeLink.
	// Examples: "user", "site", "group", "profile", "asset".
	LinkTarget string

	// CompoundFields lists sub-field keys when Type == TypeCompound.
	// Each sub-field is itself a registered ParamDef (compound nesting is flat).
	CompoundFields []string
}

// SubtypeSchema declares which parameters a given asset subtype mounts,
// in what order, and with what grouping. This is what the admin sees
// when viewing assets of this subtype.
type SubtypeSchema struct {
	// Subtype is the asset subtype key ("computer", "server", "printer", "desk").
	Subtype string

	// Label is the human name ("Computer", "Server", "Printer", "Desk").
	Label string

	// PluralLabel is the plural form ("Computers", "Servers", ...).
	PluralLabel string

	// PathEntity is the canonical-path entity slug for this schema
	// (INV-CPP): the first segment of every parameter path,
	// "<entity>/<section>/<param>". It differs from Subtype only for
	// the identity schema, whose paths use "user".
	PathEntity string

	// Sections defines the ordered grouping of parameters for detail page
	// and column picker display.
	Sections []SchemaSection

	// DefaultColumns lists param keys shown by default in the list view.
	// Order determines column order.
	DefaultColumns []string
}

// SchemaSection groups parameters within a subtype schema for display.
type SchemaSection struct {
	// Key is stable (used in localStorage for collapsed state, etc.)
	Key string

	// Label is the human section header ("Hardware", "Enrollment", "Network").
	Label string

	// Params lists the parameter keys in this section, in display order.
	Params []string
}

// LinkDef describes a relationship-type parameter that resolves through
// another entity rather than being a stored value on the asset.
type LinkDef struct {
	// ParamKey matches the ParamDef.Key this link extends.
	ParamKey string

	// TargetEntity is the kind of entity this link resolves to.
	// Examples: "user", "site", "group", "profile".
	TargetEntity string

	// Cardinality: "one" (single reference) or "many" (list of references).
	Cardinality string

	// Bidirectional: if true, the target entity also shows this relationship.
	// (e.g. User sees "assigned assets", Asset sees "owner").
	Bidirectional bool

	// InverseLabel is what the target entity calls this relationship.
	// e.g. Asset→User link: Label="Owner", InverseLabel="Assigned assets".
	InverseLabel string
}
