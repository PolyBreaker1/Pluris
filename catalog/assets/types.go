// Package assets is the v1 mock catalog for the Asset entity that
// every device-management surface in Pluris targets. The shape here
// is the IA contract from docs/endpoint-management/ui/invariants.md §VII.A1 (codified by
// ADR-005); when the backend lands in Increment 3 these types become
// the seed data for db/schema/asset.go and the JSON payload schemas.
//
// The package is intentionally allocation-free — every getter returns
// the underlying slice/struct by value or by pointer, and the mock
// builder rebuilds the catalog on demand. Callers that need stable
// ordering get it from the deterministic mock.
package assets

import "time"

// ----------------------------------------------------------------------
// Subtype — closed-extensible enum. New subtypes are additive: a new
// constant + a new SubtypePayload struct + a new tab in the Assets page.
// ----------------------------------------------------------------------

// Subtype discriminates the four subtype tabs documented in §VII.A1.
// String values match the URL slug used by the Assets page tabs and
// the mount-point tests so the wire is one shape end-to-end.
type Subtype string

const (
	// SubtypeComputer — end-user workstations and laptops. The default
	// tab on /assets and the largest population for a typical tenant.
	SubtypeComputer Subtype = "computer"

	// SubtypeServer — multi-user hosts running services. Distinguished
	// from Computer by the additional `Services` payload field.
	SubtypeServer Subtype = "server"

	// SubtypePrinter — IPP / network printers. Carry consumables and
	// queue state on the payload.
	SubtypePrinter Subtype = "printer"

	// SubtypeDesk — physical workstation locations. May reference a
	// guest Profile that auto-merges when a user docks here. The Desk
	// is the asset, not the computer plugged into it.
	SubtypeDesk Subtype = "desk"
)

// IsValid reports whether s is a registered subtype. Used by the
// router and the editor to reject hand-crafted URLs early.
func (s Subtype) IsValid() bool {
	switch s {
	case SubtypeComputer, SubtypeServer, SubtypePrinter, SubtypeDesk:
		return true
	}
	return false
}

// Slug is the URL/test-id slug; identical to the string value but
// rendered through this accessor so callers don't string-cast.
func (s Subtype) Slug() string { return string(s) }

// Label returns the human display name used in tab headers, breadcrumb
// trails, and editor titles.
func (s Subtype) Label() string {
	switch s {
	case SubtypeComputer:
		return "Computer"
	case SubtypeServer:
		return "Server"
	case SubtypePrinter:
		return "Printer"
	case SubtypeDesk:
		return "Desk"
	}
	return string(s)
}

// PluralLabel — used in tab labels and "N Computers" counts.
func (s Subtype) PluralLabel() string {
	switch s {
	case SubtypeComputer:
		return "Computers"
	case SubtypeServer:
		return "Servers"
	case SubtypePrinter:
		return "Printers"
	case SubtypeDesk:
		return "Desks"
	}
	return string(s) + "s"
}

// AllSubtypes returns the registered subtypes in tab order. Used by
// tests that want to iterate every kind without hard-coding the list.
func AllSubtypes() []Subtype {
	return []Subtype{SubtypeComputer, SubtypeServer, SubtypePrinter, SubtypeDesk}
}

// ----------------------------------------------------------------------
// EnrollmentState — agent-side lifecycle. Mirrors the Windows MDM
// enrollment vocabulary so AD-migrating admins recognise it.
// ----------------------------------------------------------------------

// EnrollmentState is the agent-side enrollment status. Distinct from
// LifecycleState (which is asset-management / CMDB).
type EnrollmentState string

const (
	// EnrollmentPending — discovered (e.g. AD import, network scan)
	// but no agent has reported in yet.
	EnrollmentPending EnrollmentState = "pending"

	// EnrollmentApproved — admin approved enrollment; the asset is
	// allowed to register a certificate. Transitional state.
	EnrollmentApproved EnrollmentState = "approved"

	// EnrollmentEnrolled — agent has reported in within the last
	// drift window. The healthy state.
	EnrollmentEnrolled EnrollmentState = "enrolled"

	// EnrollmentDisabled — admin temporarily disabled the asset; agent
	// stops applying policy until re-enabled. Reversible.
	EnrollmentDisabled EnrollmentState = "disabled"

	// EnrollmentRevoked — asset's certificate revoked. Terminal for
	// this enrollment instance; re-enrolling issues a new cert.
	EnrollmentRevoked EnrollmentState = "revoked"
)

// IsValid reports whether e is a registered state.
func (e EnrollmentState) IsValid() bool {
	switch e {
	case EnrollmentPending, EnrollmentApproved, EnrollmentEnrolled,
		EnrollmentDisabled, EnrollmentRevoked:
		return true
	}
	return false
}

// IsHealthy reports whether the agent is expected to be applying policy
// right now. Used by the dashboard "healthy / drift / unmanaged" tile.
func (e EnrollmentState) IsHealthy() bool { return e == EnrollmentEnrolled }

// ----------------------------------------------------------------------
// LifecycleState — asset-management / CMDB lifecycle. Independent of
// EnrollmentState: a decommissioned asset can still be enrolled until
// the agent is removed; a pending asset can be marked active in CMDB.
// ----------------------------------------------------------------------

// LifecycleState is the CMDB lifecycle from §VII.A1.
type LifecycleState string

const (
	// LifecycleActive — in service.
	LifecycleActive LifecycleState = "active"

	// LifecycleInRepair — temporarily out of service for hardware work.
	LifecycleInRepair LifecycleState = "in_repair"

	// LifecycleDecommissioned — retired but not yet disposed; kept for
	// audit. Drift checks may stop.
	LifecycleDecommissioned LifecycleState = "decommissioned"

	// LifecycleDisposed — physically destroyed / sold / e-cycled.
	// Records remain for warranty + audit; agent never reports again.
	LifecycleDisposed LifecycleState = "disposed"
)

// IsValid reports whether l is a registered state.
func (l LifecycleState) IsValid() bool {
	switch l {
	case LifecycleActive, LifecycleInRepair, LifecycleDecommissioned, LifecycleDisposed:
		return true
	}
	return false
}

// ----------------------------------------------------------------------
// SubtypePayload — typed JSON payload per subtype. The marker interface
// keeps the union explicit; helpers below give safe accessors.
// ----------------------------------------------------------------------

// SubtypePayload is the marker interface every subtype-specific payload
// satisfies. The concrete struct type announces the subtype via Kind().
type SubtypePayload interface {
	// Kind returns the subtype this payload belongs to. The Asset's
	// Subtype field must match this value; the constructor enforces it.
	Kind() Subtype
}

// ComputerPayload is the §VII.A1 payload for SubtypeComputer.
type ComputerPayload struct {
	Hostname       string
	FQDN           string
	OSFamily       string // "linux" | "windows" | "macos"
	OSDistribution string // "Ubuntu" | "Windows 11 Pro" | …
	OSVersion      string
	KernelVersion  string // "6.8.0-40-generic" | "10.0.22631" | …
	Architecture   string // "x86_64" | "arm64" | …
	CPUSummary     string
	RAMMB          int
	StorageMB      int
	ScreenshotURL  string // asset photo or rendered OS screenshot
}

// Kind implements SubtypePayload.
func (ComputerPayload) Kind() Subtype { return SubtypeComputer }

// ServerRole is the coarse classification used in the Servers tab.
type ServerRole string

const (
	ServerRoleDB    ServerRole = "db"
	ServerRoleWeb   ServerRole = "web"
	ServerRoleDNS   ServerRole = "dns"
	ServerRoleFile  ServerRole = "file"
	ServerRoleApp   ServerRole = "app"
	ServerRoleOther ServerRole = "other"
)

// ServerService is one declared service running on a server.
type ServerService struct {
	Name string // "nginx" | "postgresql" | …
	Port int    // 0 if not network-bound
}

// ServerPayload is the §VII.A1 payload for SubtypeServer.
type ServerPayload struct {
	Hostname        string
	FQDN            string
	OSFamily        string
	OSDistribution  string
	OSVersion       string
	KernelVersion   string
	Architecture    string
	Services        []ServerService
	UptimeStartedAt time.Time
	Role            ServerRole
}

// Kind implements SubtypePayload.
func (ServerPayload) Kind() Subtype { return SubtypeServer }

// PrinterConsumable is one tracked consumable (toner, drum, …).
type PrinterConsumable struct {
	Kind     string // "toner.black" | "toner.cyan" | "drum" | …
	LevelPct int    // 0–100; -1 for "unknown"
}

// PrinterPayload is the §VII.A1 payload for SubtypePrinter.
type PrinterPayload struct {
	Model             string
	Vendor            string
	IP                string
	Queues            []string // configured queue names
	SupportedFormats  []string // "PCL6" | "PostScript" | "PDF" | …
	CurrentConsumable []PrinterConsumable
}

// Kind implements SubtypePayload.
func (PrinterPayload) Kind() Subtype { return SubtypePrinter }

// DeskPayload is the §VII.A1 payload for SubtypeDesk. The `GuestProfileID`
// reference drives the §III user-vs-computer scope merge: when an
// Identity docks to this Desk, the resolver merges that Profile into
// the user's effective policy.
type DeskPayload struct {
	LocationLabel  string
	DockModel      string
	MonitorCount   int
	GuestProfileID string // empty if no guest profile configured
}

// Kind implements SubtypePayload.
func (DeskPayload) Kind() Subtype { return SubtypeDesk }

// ----------------------------------------------------------------------
// Asset — the entity itself. Common fields plus a typed payload.
// ----------------------------------------------------------------------

// Asset is the v1 mock-data shape of the Asset entity. Every device
// management surface in Pluris (Assets pages, dashboard tiles, policy
// assignment pickers, search) reads through this type; the canonical
// editor (templates.AssetEditor) is the only writer.
type Asset struct {
	// ID is the human-readable stable id ("desk.acme.hq.07"). Distinct
	// from UUID so it can survive certificate re-issuance.
	ID string

	// UUID is the agent-side stable identifier; binds to the mTLS
	// certificate subject CN per ADR-005. Empty for Subtype=desk
	// (desks don't run an agent).
	UUID string

	// TenantID is the owning tenant. v1 mock uses a single tenant
	// label so fixtures stay readable.
	TenantID string

	// Subtype must equal Payload.Kind(). NewAsset enforces this; if
	// you assemble Asset literals by hand in tests, run Validate.
	Subtype Subtype

	// Payload carries the subtype-specific fields. Type-assert when
	// you need them; the editor uses a switch on Subtype.
	Payload SubtypePayload

	// Site is the human-readable site label ("HQ — Floor 3"). Becomes
	// an edge to the Site entity in Increment 3.
	Site string

	// Groups are membership labels ("engineering", "vip-laptops").
	// Become Group edges in Increment 3.
	Groups []string

	// Labels are free-form key/value pairs (cloud-style tags). Used
	// by the dashboard tile filter.
	Labels map[string]string

	// EnrollmentState — see EnrollmentState.
	EnrollmentState EnrollmentState

	// EnrolledAt is the moment the agent first checked in. Zero when
	// EnrollmentState is "pending".
	EnrolledAt time.Time

	// LastSeenAt is the most recent agent heartbeat. Zero for desks
	// (which don't run an agent) and for pending assets.
	LastSeenAt time.Time

	// AgentVersion is the running agent's semver string. Empty for
	// desks and pending assets.
	AgentVersion string

	// LifecycleState — CMDB lifecycle, see LifecycleState.
	LifecycleState LifecycleState

	// Location is a free-text location ("HQ Floor 3 / Desk 07").
	// Complementary to Site; not a cross-asset reference.
	Location string

	// OwnerIdentity is the assigned user's stable ID. Empty if shared
	// or pool asset.
	OwnerIdentity string

	// Description is the free-text description (AD: description).
	Description string

	// ManagedBy is the responsible identity's display name (AD: managedBy).
	// Resolved via JOIN; empty when unassigned.
	ManagedBy string

	// Vendor is the manufacturer ("Lenovo", "HP", "Brother").
	Vendor string

	// PurchaseDate is the date the org acquired the asset.
	PurchaseDate time.Time

	// PurchasePrice is recorded in tenant currency. v1 stores cents
	// as int64 to avoid float; 0 = unknown.
	PurchasePriceCents int64

	// WarrantyExpiresAt is zero if no warranty tracked.
	WarrantyExpiresAt time.Time
}

// Validate reports the first invariant violation found, or nil if the
// asset is well-formed. Cheap; safe to call on every UI render.
func (a Asset) Validate() error {
	if a.ID == "" {
		return errInvalid("Asset.ID must not be empty")
	}
	if !a.Subtype.IsValid() {
		return errInvalid("Asset.Subtype " + string(a.Subtype) + " is not registered")
	}
	if a.Payload == nil {
		return errInvalid("Asset.Payload must be non-nil")
	}
	if a.Payload.Kind() != a.Subtype {
		return errInvalid("Asset.Subtype " + string(a.Subtype) +
			" does not match payload kind " + string(a.Payload.Kind()))
	}
	if a.EnrollmentState != "" && !a.EnrollmentState.IsValid() {
		return errInvalid("Asset.EnrollmentState " + string(a.EnrollmentState) + " is not registered")
	}
	if a.LifecycleState != "" && !a.LifecycleState.IsValid() {
		return errInvalid("Asset.LifecycleState " + string(a.LifecycleState) + " is not registered")
	}
	return nil
}

// errInvalid is a tiny sentinel-style helper so Validate stays
// allocation-free on the hot path. Importing fmt would force an
// allocation per call.
type validationError string

func (v validationError) Error() string { return "assets: " + string(v) }

func errInvalid(msg string) error { return validationError(msg) }

// NonEditablePayloadKeys are asset params that render read-only in the
// detail UI and are always refused by the field-update service
// (pkg/services/assets.go's UpdateFields): identifiers, tenancy, and
// TypeLink params that are never column-backed and never live in the
// "hardware" section (the only section UpdateFields writes arbitrary
// payload keys for). See assetColumnBackedKeys's doc comment in
// pkg/services/assets.go, which this list mirrors -- the two must be
// kept in sync by hand since one lives in catalog (no service import)
// and the other in pkg/services (no UI import).
var NonEditablePayloadKeys = map[string]bool{
	"id":               true,
	"uuid":             true,
	"tenant":           true,
	"site":             true,
	"owner":            true,
	"groups":           true,
	"managed_by":       true,
	"enrollment_state": true,
	"enrolled_at":      true,
	"last_seen_at":     true,
	"agent_version":    true,
}

// ----------------------------------------------------------------------
// Convenience accessors — keep templ templates one-liners.
// ----------------------------------------------------------------------

// PrimaryHostname returns the hostname for subtypes that have one,
// or a stable fallback (e.g. the Desk's location label) for subtypes
// that don't. Used by the Assets list's primary column.
func (a Asset) PrimaryHostname() string {
	switch p := a.Payload.(type) {
	case ComputerPayload:
		return p.Hostname
	case ServerPayload:
		return p.Hostname
	case PrinterPayload:
		return p.Model
	case DeskPayload:
		return p.LocationLabel
	}
	return a.ID
}

// OSSummary returns "Ubuntu 24.04 (linux/x86_64)" or similar for
// subtypes that have an OS, else empty string. The Assets list shows
// it as a secondary column.
func (a Asset) OSSummary() string {
	switch p := a.Payload.(type) {
	case ComputerPayload:
		if p.OSDistribution == "" {
			return ""
		}
		return p.OSDistribution + " " + p.OSVersion
	case ServerPayload:
		if p.OSDistribution == "" {
			return ""
		}
		return p.OSDistribution + " " + p.OSVersion
	}
	return ""
}
