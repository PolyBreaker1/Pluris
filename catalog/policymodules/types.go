// Package policymodules holds the domain types for Pluris' Policy Module
// catalog. The shape of these types is the IA contract from ADR-007 §
// "Manifest extensions" and the UI invariants doc's §VI-adjacent module
// rules (INV-M1..M13, docs/endpoint-management/ui/invariants.md). Real
// persistence landed in migration 008 + `pkg/services/policymodules.go`
// (draft/publish/supersede/revoke state machine); this package stays
// DB-free and is fed by a catalog provider (`SetCatalogProvider`/
// `Catalog()` in catalog.go) rather than a hardcoded literal slice.
//
// Vocabulary recap (read these once before reading the code):
//
//   - Module        = a versioned, signed package containing apply/disable/
//     uninstall scripts (+ optional validate/report). One
//     "module ID" with multiple immutable versions.
//   - Manifest      = the YAML at the top of a module bundle; `manifest_yaml`
//     on the persisted version is now a derived export artifact, not the
//     source of truth — the structured columns (parameters_schema,
//     sandbox_profile, satisfies, etc.) and the policy_module_scripts
//     table are authoritative. See
//     docs/history/specs/2026-07-12-module-persistence-and-param-injection.md.
//   - Catalog Policy = an entry from pluris/catalog/policies — the
//     vocabulary item a module *satisfies*.
//   - Custom Policy  = a tenant-authored catalog policy, marked with
//     `Custom: true` (see policies.Policy). There is no dedicated
//     authoring wizard for these today — the former Custom Policy Wizard
//     was a pure stub (no save path ever persisted anything) and was
//     deleted; the module editor (`/policy/modules/new`) is the canonical
//     authoring surface for policy MODULES. custom_policies.parameters_schema
//     remains unreachable dead schema (documented in the spec above).
//   - Installation   = a row in ModuleInstallation: "this asset has this
//     module version installed because of these bindings".
//     Refcount is computed from incoming edges.
package policymodules

// Runtime — which interpreter/engine executes a lifecycle script.
// ADR-007 INV-M5 freezes this to two options. Adding a third requires an
// ADR; do not extend this enum without that.
type Runtime string

const (
	RuntimeBash Runtime = "bash" // apply/disable/uninstall
	RuntimeWASM Runtime = "wasm" // validate/report only
)

// LifecyclePhase — every script in a module belongs to one of these.
// `apply` is mandatory; the rest are optional but each module declares
// which it supplies in its manifest.
type LifecyclePhase string

const (
	PhaseApply     LifecyclePhase = "apply"
	PhaseDisable   LifecyclePhase = "disable"
	PhaseUninstall LifecyclePhase = "uninstall"
	PhaseValidate  LifecyclePhase = "validate"
	PhaseReport    LifecyclePhase = "report"
)

// AllLifecyclePhases — used by UI iteration. Order matches the module
// editor's Scripts-tab phase tab order (web/templates/policy_module_editor.templ).
var AllLifecyclePhases = []LifecyclePhase{
	PhaseApply, PhaseDisable, PhaseUninstall, PhaseValidate, PhaseReport,
}

// IsRequired — phases mandatory for any complete module.
func (p LifecyclePhase) IsRequired() bool { return p == PhaseApply }

// Runtime — which interpreter executes this phase. Enforced by INV-M5.
func (p LifecyclePhase) Runtime() Runtime {
	switch p {
	case PhaseApply, PhaseDisable, PhaseUninstall:
		return RuntimeBash
	case PhaseValidate, PhaseReport:
		return RuntimeWASM
	}
	return RuntimeBash
}

// Short — one-letter pill label used by the lifecycle-phase strip
// (Apply=A, Disable=D, Uninstall=U, Validate=V, Report=R). Centralised
// so the templ-side phase strip and the lists/cell renderer agree.
func (p LifecyclePhase) Short() string {
	switch p {
	case PhaseApply:
		return "A"
	case PhaseDisable:
		return "D"
	case PhaseUninstall:
		return "U"
	case PhaseValidate:
		return "V"
	case PhaseReport:
		return "R"
	}
	return "·"
}

// Label — human title for the phase tab.
func (p LifecyclePhase) Label() string {
	switch p {
	case PhaseApply:
		return "Apply"
	case PhaseDisable:
		return "Disable"
	case PhaseUninstall:
		return "Uninstall"
	case PhaseValidate:
		return "Validate"
	case PhaseReport:
		return "Report"
	}
	return string(p)
}

// HelpText — one-liner shown under the phase tab heading in the editor.
// Mirrors the wording in ADR-007 §"Module lifecycle".
func (p LifecyclePhase) HelpText() string {
	switch p {
	case PhaseApply:
		return "Idempotent. Runs on every binding apply; cheap re-runs expected."
	case PhaseDisable:
		return "Idempotent and reversible. Leaves files in place but disables effect."
	case PhaseUninstall:
		return "Idempotent. Runs only when refcount reaches zero (INV-M1)."
	case PhaseValidate:
		return "Pure / read-only. Periodic drift check; runs in WASM sandbox."
	case PhaseReport:
		return "Pure / read-only. Returns structured data validated against report_schema."
	}
	return ""
}

// Language — the interpreter a first-class named Script runs under
// (migration 012, replaces the phase-derived Runtime split above).
// Kept separate from Runtime: Runtime still describes the two lifecycle
// sandboxes (bash vs wasm); Language is the finer-grained script
// authoring language the editor and agent actually invoke.
type Language string

const (
	LangSh         Language = "sh"
	LangPowershell Language = "powershell"
	LangPython     Language = "python"
)

// Valid reports whether l is one of the three languages the
// policy_module_scripts.language CHECK constraint (migration 012)
// allows.
func (l Language) Valid() bool {
	switch l {
	case LangSh, LangPowershell, LangPython:
		return true
	}
	return false
}

// Script — one first-class named script row (migration 012). Replaces
// the phase-keyed LifecycleScript for new persistence; LifecycleScript
// is kept for the read side of the module editor/export/UI (built from
// Script rows by the service layer) until CP2/CP4 replace it.
type Script struct {
	Name     string
	Language string
	Source   string
	Origin   string // "default" | "custom"
	Seq      int
}

// ModuleAction — one row of the enforcement wiring table (migration
// 012): what runs, either by referencing a Script by name (kind=
// "script", value=<script name>) or an inline command (kind=
// "command", value=<command text>).
type ModuleAction struct {
	Key    string // action_key
	Label  string
	Kind   string // "command" | "script"
	Value  string
	Origin string // "default" | "custom"
	Seq    int
}

// DefaultActionKeys — the default action-key seed list. Reuses the
// five lifecycle phase strings verbatim (apply/disable/uninstall/
// validate/report) since those remain the canonical default wiring
// names even though scripts are no longer phase-keyed.
var DefaultActionKeys = func() []string {
	keys := make([]string, 0, len(AllLifecyclePhases))
	for _, p := range AllLifecyclePhases {
		keys = append(keys, string(p))
	}
	return keys
}()

// TargetOS — which operating systems a module may run on. Used by the
// compatibility filter on every entry point that surfaces modules.
type TargetOS string

const (
	OSLinux   TargetOS = "linux"
	OSWindows TargetOS = "windows"
	OSMacOS   TargetOS = "macos"
	OSAny     TargetOS = "any"
)

// SandboxProfile — declared in the manifest, enforced at exec time.
// INV-M8: agent applies this regardless of script content.
type SandboxProfile struct {
	// FsRead/FsWrite — absolute paths or globs the script may touch.
	// Default is empty = denied. PLURIS_TMPDIR is always writable.
	FsRead  []string
	FsWrite []string
	// NetEgress — explicit allow-list of host:port or URI patterns.
	// Empty = no network. v1 supports: "host:port", "https://host/", "*:53/udp".
	NetEgress []string
	// User — which UID/GID the script runs as. "root", "$target_user"
	// (the Identity bound to the binding when scope=user), or "nobody".
	User string
}

// Dependency — one entry in a module's depends_on list.
//
// VersionConstraint follows the npm-style range syntax (we'll wire a
// real semver lib in Phase 1; for the mock the string is opaque).
type Dependency struct {
	ModuleID          string
	VersionConstraint string // ">=1.0.0 <2.0.0", "1.x", "*"
	// Optional — caller can require a specific phase to be present on
	// the dependency (e.g. validate must exist if the dependent's
	// validate calls into it). v1 mock ignores this; left for forward-
	// compat.
	RequiredPhases []LifecyclePhase
}

// LifecycleScript — one script blob inside a module version. v1 stores
// inline source; Phase 2 wraps with sandbox-compiled bytecode for WASM
// phases. The `Filename` is what shows in the editor tab.
type LifecycleScript struct {
	Phase    LifecyclePhase
	Filename string
	Source   string // inline bash for shell phases; placeholder for WASM phases
}

// Signature — Ed25519 signature over the manifest + script hashes
// (INV-M7). For the mock we only carry the metadata so the UI can
// render trust badges; real bytes land with Phase 1.
type Signature struct {
	KeyID  string // "tenant:<tenant_id>:key:<key_id>" or "pluris:bundled:1"
	Algo   string // "ed25519"
	Bytes  string // base64; empty in mock
	Signer string // human label, e.g. "Pluris (bundled)" or "alice.chen@acme.local"
}

// ModuleVersion — one immutable version of a Module. Editing in the UI
// produces a NEW ModuleVersion; the previous version is retained for
// rollback and for assets pinned to it.
type ModuleVersion struct {
	Version      string // semver
	PublishedAt  string // ISO8601 (mock: "2026-04-12T10:33:00Z")
	PublishedBy  string // human label
	Sandbox      SandboxProfile
	Dependencies []Dependency
	Conflicts    []string // module IDs (any version) that may not coexist
	Scripts      []LifecycleScript
	ReportSchema string // JSON Schema source; "" = no report
	Signature    Signature
	// Status — `published` (immutable, deployable), `draft` (in-editor,
	// not yet signed), `superseded` (newer version exists), `revoked`
	// (admin pulled it; agents must not run it). v1 mock uses these
	// labels; backend will pin the enum.
	Status string
}

// Module — the catalog entry for a Policy Module. One ID, many versions.
// `Versions` is sorted newest-first by convention; the UI list shows
// the head and offers a version picker.
type Module struct {
	ID          string // dotted slug, lowercase. "pluris.sshd.password-auth-disable"
	Title       string // human title. "Disable SSH password authentication"
	Description string // 1-3 sentences; full doc lives in README inside the bundle
	TargetOS    []TargetOS
	Scope       string   // "machine" | "user" | "both" — mirrors policies.Scope
	Satisfies   []string // catalog Policy URNs this module implements (many, INV-U2-style)
	// Origin — "bundled" (ships with Pluris, read-only), "tenant" (authored
	// in this tenant via the module editor, `/policy/modules/new`),
	// "imported" (pulled from a community registry — Phase 4). The UI
	// gates Edit / Sign / Delete on this.
	Origin   string
	Versions []ModuleVersion
}

// LatestVersion — convenience accessor used by every list view. Returns
// nil-pointer-safe zero if the module is somehow versionless (which the
// resolver treats as an error — INV-M3 cannot be satisfied).
func (m *Module) LatestVersion() *ModuleVersion {
	if m == nil || len(m.Versions) == 0 {
		return nil
	}
	return &m.Versions[0]
}

// SatisfiesURN — true if this module declares it implements `urn`.
// Used by the Configuration Group binding's module picker to filter
// candidates.
func (m *Module) SatisfiesURN(urn string) bool {
	for _, s := range m.Satisfies {
		if s == urn {
			return true
		}
	}
	return false
}

// SupportsOS — compatibility filter. `any` matches everything.
func (m *Module) SupportsOS(os TargetOS) bool {
	for _, o := range m.TargetOS {
		if o == OSAny || o == os {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------
// Module installation & refcount (INV-M1..M4)
// ----------------------------------------------------------------------

// InstallationState — runtime status of a ModuleInstallation row.
// Mirrors docs/endpoint-management/ui/invariants.md → ModuleInstallation.
type InstallationState string

const (
	InstStatePending  InstallationState = "pending"  // queued; bundle not yet shipped
	InstStateApplied  InstallationState = "applied"  // last apply succeeded
	InstStateDisabled InstallationState = "disabled" // disable phase ran successfully
	InstStateFailing  InstallationState = "failing"  // last apply/validate errored
	InstStateOrphaned InstallationState = "orphaned" // refcount=0, scheduled for uninstall
)

// InstallReason — one reason a module is on an asset. The collection of
// reasons is the `installed_via` edge set in the IA contract; refcount
// is its length.
//
// Two flavours: a binding directly requested it, OR another installation
// transitively depends on it.
type InstallReason struct {
	// Exactly one of these is non-empty:
	BindingID        string // "binding:<configgroup_id>:<seq>" — direct request
	DependentInstall string // "install:<id>" — transitive
	// AddedAt — when this reason joined the edge set. Used by the UI to
	// say "added 2 days ago by Configuration Group X".
	AddedAt string
}

// ModuleInstallation — one row per (asset, module_version) actually
// present on an asset. Refcount-based uninstall safety lives here.
type ModuleInstallation struct {
	ID             string
	AssetID        string
	ModuleID       string
	ModuleVersion  string
	Reasons        []InstallReason // INV-M4: never empty; if it would become so, row is deleted
	State          InstallationState
	AppliedAt      string
	LastValidated  string
	LastReportJSON string // raw JSON of latest report; rendered by UI
}

// Refcount — INV-M1. The number of active reasons keeping this row
// alive. `uninstall` runs only when this hits zero (and the row then
// disappears).
func (mi *ModuleInstallation) Refcount() int {
	if mi == nil {
		return 0
	}
	return len(mi.Reasons)
}

// IsLoadBearing — quick predicate for the UI's "still required by N
// other groups" warning when a binding is removed.
func (mi *ModuleInstallation) IsLoadBearing() bool {
	return mi.Refcount() > 1
}
