package policymodules

// Tenant module defaults + binding resolution.
//
// Confirmed design 2026-05-16: when an admin chooses a module to
// satisfy a policy, the choice can live at two levels — tenant-wide
// default (per policy URN) and per-binding override. The agent
// resolution walks both before falling back to the Pluris default.
//
// Resolution order at agent check-in for a given (binding, policy):
//
//   1. binding.ModuleOverride            ← per-binding pin
//   2. TenantDefault(tenant, policyURN)  ← tenant-wide default
//   3. PlurisDefault(policyURN, deviceOS) = first bundled module that
//      satisfies the URN and supports the device OS, ordered by
//      AllModules() stable order.
//
// Lock as INV-M11 (resolution order) in UX_INVARIANTS §VII.B.
//
// v1 storage: in-memory mock keyed on (tenant_id, policy_urn). Backend
// slice replaces this with an Ent entity holding the same fields.

import "sync"

// TenantDefault — one tenant-wide default. A tenant has at most one
// default per policy URN; setting one overwrites any prior. Removing a
// default lets the binding fall through to the Pluris default.
type TenantDefault struct {
	TenantID  string // "" in the single-tenant mock; multi-tenant later
	PolicyURN string // catalog policy ID this default applies to
	ModuleID  string // module that satisfies the URN
	// ModuleVersion — "" means "latest published"; pinning to a
	// specific semver freezes rollout for this tenant until they pick
	// a newer version explicitly.
	ModuleVersion string
	// Audit fields (mock).
	SetBy string
	SetAt string
}

// tenantDefaultsMu guards the mock map. Safe-for-tests; the backend
// will use Ent's transaction primitives instead.
var (
	tenantDefaultsMu sync.RWMutex
	// Keyed by tenant_id|policy_urn for O(1) lookup. "" tenant_id is
	// the single-tenant mock's well-known key.
	tenantDefaults = map[string]TenantDefault{}
)

// keyTD — composite map key. tenant_id first so per-tenant iteration
// can prefix-scan in the real backend.
func keyTD(tenant, urn string) string { return tenant + "|" + urn }

// TenantDefaults — all defaults for one tenant, ordered by URN for a
// stable UI snapshot. Used by the "Defaults" tab on /policy/modules.
func TenantDefaults(tenant string) []TenantDefault {
	tenantDefaultsMu.RLock()
	defer tenantDefaultsMu.RUnlock()
	var out []TenantDefault
	for _, d := range tenantDefaults {
		if d.TenantID == tenant {
			out = append(out, d)
		}
	}
	// Sort by URN for determinism.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].PolicyURN < out[j-1].PolicyURN; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// TenantDefaultFor — get the default for one (tenant, policy URN) or
// the zero TenantDefault if none is set.
func TenantDefaultFor(tenant, urn string) TenantDefault {
	tenantDefaultsMu.RLock()
	defer tenantDefaultsMu.RUnlock()
	return tenantDefaults[keyTD(tenant, urn)]
}

// SetTenantDefault — upsert the default. Empty ModuleID clears it.
// The caller is responsible for verifying the module actually
// satisfies the URN — this layer is dumb storage.
func SetTenantDefault(d TenantDefault) {
	tenantDefaultsMu.Lock()
	defer tenantDefaultsMu.Unlock()
	if d.ModuleID == "" {
		delete(tenantDefaults, keyTD(d.TenantID, d.PolicyURN))
		return
	}
	tenantDefaults[keyTD(d.TenantID, d.PolicyURN)] = d
}

// ClearTenantDefault — remove the default for (tenant, urn). Bindings
// that relied on it fall back to the Pluris default.
func ClearTenantDefault(tenant, urn string) {
	tenantDefaultsMu.Lock()
	defer tenantDefaultsMu.Unlock()
	delete(tenantDefaults, keyTD(tenant, urn))
}

// SeedMockTenantDefaults — populate a small set so the UI exercises
// every branch in the resolver. Called once from init() with the
// well-known mock tenant id ("").
func SeedMockTenantDefaults() {
	tenantDefaultsMu.Lock()
	defer tenantDefaultsMu.Unlock()
	tenantDefaults[keyTD("", "sec.remote-access.ssh.password-auth")] = TenantDefault{
		PolicyURN:     "sec.remote-access.ssh.password-auth",
		ModuleID:      "pluris.sshd.password-auth-disable",
		ModuleVersion: "", // latest
		SetBy:         "admin@acme.local",
		SetAt:         "2026-04-25T09:14:00Z",
	}
}

func init() { SeedMockTenantDefaults() }

// ResolvedPick — what ResolveBindingModule returns. Names the picked
// module + version AND the source level so the UI can render a
// "Pluris default", "Tenant default", or "Binding override" chip.
type ResolvedPick struct {
	ModuleID      string
	ModuleVersion string
	// Source — which level of the resolution chain provided the pick.
	// One of "binding", "tenant", "pluris", or "" when no compatible
	// module exists (the binding is unsatisfiable as written).
	Source string
	// Reason — human-readable explanation. Used in the picker chip
	// tooltip and the agent's plan log.
	Reason string
}

// IsResolved — true when a module was found at any level.
func (p ResolvedPick) IsResolved() bool { return p.ModuleID != "" }

// ResolveBindingModule — pure function that implements INV-M11 for
// one binding + policy URN + device OS. No I/O; safe to call from any
// goroutine.
//
//   - bindingOverrideID/Ver: from configgroups.Binding.ModuleOverride
//     (empty IDs mean "no override"). Caller must pass these in
//     directly because this package cannot import configgroups (cycle).
//   - tenant: tenant id; "" in the single-tenant mock.
//   - policyURN: the policy this binding configures.
//   - deviceOS: optional OS filter ("" = no narrowing).
//   - catalog: module catalog snapshot (typically AllModules()).
func ResolveBindingModule(
	bindingOverrideID, bindingOverrideVer, tenant, policyURN string,
	deviceOS TargetOS, catalog []Module,
) ResolvedPick {
	// 1. Binding override — verify the chosen module still exists,
	// still satisfies the URN, and supports the device OS. A stale
	// override (module deleted, URN no longer satisfied, OS mismatch)
	// falls through to lower levels — the picker UI surfaces the
	// staleness as a warning chip.
	if bindingOverrideID != "" {
		if m := findModule(catalog, bindingOverrideID); m != nil {
			if m.SatisfiesURN(policyURN) && (deviceOS == "" || m.SupportsOS(deviceOS)) {
				ver := pickVersion(m, bindingOverrideVer)
				if ver != "" {
					return ResolvedPick{
						ModuleID: m.ID, ModuleVersion: ver,
						Source: "binding",
						Reason: "pinned on this binding",
					}
				}
			}
		}
	}

	// 2. Tenant default for this URN.
	if td := TenantDefaultFor(tenant, policyURN); td.ModuleID != "" {
		if m := findModule(catalog, td.ModuleID); m != nil {
			if m.SatisfiesURN(policyURN) && (deviceOS == "" || m.SupportsOS(deviceOS)) {
				ver := pickVersion(m, td.ModuleVersion)
				if ver != "" {
					return ResolvedPick{
						ModuleID: m.ID, ModuleVersion: ver,
						Source: "tenant",
						Reason: "tenant default for this policy",
					}
				}
			}
		}
	}

	// 3. Pluris default = first bundled module in stable catalog order
	// that satisfies and supports the OS. ADR-006: "the highest-
	// priority compatible module" — v1 priority is "bundled origin +
	// first in AllModules() order".
	for i := range catalog {
		m := &catalog[i]
		if m.Origin != "bundled" {
			continue
		}
		if !m.SatisfiesURN(policyURN) {
			continue
		}
		if deviceOS != "" && !m.SupportsOS(deviceOS) {
			continue
		}
		if ver := pickVersion(m, ""); ver != "" {
			return ResolvedPick{
				ModuleID: m.ID, ModuleVersion: ver,
				Source: "pluris",
				Reason: "Pluris default (first bundled module that satisfies)",
			}
		}
	}

	// 4. No compatible module at any level. Binding is unsatisfiable
	// as written. CG editor must surface a hard error before save.
	return ResolvedPick{Source: "", Reason: "no compatible module for this policy + OS"}
}

// findModule — O(n) lookup. Catalog is small (low thousands max).
func findModule(catalog []Module, id string) *Module {
	for i := range catalog {
		if catalog[i].ID == id {
			return &catalog[i]
		}
	}
	return nil
}

// pickVersion — resolve a version pin against a module's history.
//
// Status semantics (ADR-007):
//
//   - published   — immutable, deployable, currently-default for "latest"
//   - superseded  — older published version; deployable for explicit pins
//     (this is the rollback path)
//   - draft       — author in-progress, unsigned; never deployable
//   - revoked     — admin pulled it; agents MUST NOT run it; never deployable
//
// `requested == ""` returns the newest *published* version (i.e. the
// current default). A non-empty `requested` is a pin: the version is
// accepted if `published` OR `superseded`, rejected if `draft` or
// `revoked`. Returns "" when nothing valid was found.
func pickVersion(m *Module, requested string) string {
	if m == nil || len(m.Versions) == 0 {
		return ""
	}
	if requested == "" {
		// AllModules() emits versions newest-first.
		for _, v := range m.Versions {
			if v.Status == "published" {
				return v.Version
			}
		}
		return ""
	}
	for _, v := range m.Versions {
		if v.Version != requested {
			continue
		}
		switch v.Status {
		case "published", "superseded":
			return v.Version
		}
		return "" // draft / revoked / unknown — explicit reject
	}
	return ""
}
