package lists

import (
	"html"
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/web/glossary"
)

// ListIDPolicyCatalog — string id used by the column picker, the
// templ template, and localStorage. Exported as a constant so other
// packages can reference it without a string literal.
const ListIDPolicyCatalog = "policy-catalog"

// PolicyCatalogGroups — picker groups for the Policy Catalog list. The
// order here is the order the picker renders top-to-bottom. Adding a
// new group here ONLY adds it to the picker UI; you still need at
// least one field referencing the new group key for it to appear.
var PolicyCatalogGroups = []FieldGroup{
	{Key: "identity", Label: "Identity", Hint: "Stable identifiers and where the policy lives in the catalog."},
	{Key: "windows-gp", Label: "Windows Group Policy", Hint: "AD/GP equivalents — admins migrating from Windows find the same lever in the same place."},
	{Key: "linux", Label: "Linux mechanism & description", Hint: "Long-form admin context."},
	{Key: "modular", Label: "Modular shape", Hint: "Candidate Policy Modules that satisfy this policy (ADR-007)."},
}

// PolicyCatalogFields — the registry entries. Default visible set keeps
// the original 4 columns so the page looks identical out of the box;
// every other field is opt-in via the column picker.
var PolicyCatalogFields = []FieldDef{
	// Identity
	{Key: "name", Label: "Pluris name", Group: "identity", Width: "26%", Sort: "alpha",
		Description:    "Display name + custom badge. The first column users scan; almost always visible.",
		DefaultVisible: true},
	{Key: "urn", Label: "Catalog URN", Group: "identity", Width: "16%", Sort: "alpha",
		Description: "Stable dotted slug used in scripts, audit logs, GP imports."},
	{Key: "scope", Label: "Scope", Group: "identity", Width: "8%", Sort: "alpha",
		Description:    "Computer / User / Both — mirrors the Windows GPEdit branch.",
		DefaultVisible: true},
	{Key: "category", Label: "Category", Group: "identity", Width: "16%", Sort: "alpha",
		Description: "Breadcrumb path inside the policy tree."},
	{Key: "origin", Label: "Origin", Group: "identity", Width: "8%", Sort: "alpha",
		Description: "Bundled (ships with Pluris) or Custom (authored by tenant)."},
	{Key: "tenant", Label: "Tenant", Group: "identity", Width: "8%", Sort: "alpha",
		Description: "Owning tenant for custom policies; blank for bundled."},

	// Windows GP
	{Key: "gpName", Label: "Windows GP equivalent", Group: "windows-gp", Width: "22%", Sort: "alpha",
		Description:    "Exact Windows Group Policy setting name (or '— (custom)' for tenant policies).",
		DefaultVisible: true},
	{Key: "gpPath", Label: "Windows GP path", Group: "windows-gp", Width: "20%", Sort: "alpha",
		Description: "Pipe-separated GP tree location."},

	// Linux + description
	{Key: "linuxImpl", Label: "Linux mechanism", Group: "linux", Width: "14%", Sort: "alpha",
		Description: "Hint at the Linux subsystem the module engine uses (pam_pwquality, faillock, …)."},
	{Key: "description", Label: "Description", Group: "linux", Sort: "alpha",
		Description:    "Admin-facing explanation. Wraps; takes whatever space is left.",
		DefaultVisible: true},

	// Modular shape
	{Key: "moduleCount", Label: "# candidate modules", Group: "modular", Width: "9%", Sort: "num",
		Description: "How many Policy Modules in the catalog satisfy this policy."},
	{Key: "modulePhases", Label: "Lifecycle coverage", Group: "modular", Width: "12%", Sort: "num",
		Description: "Which lifecycle phases the recommended module supplies (Apply / Disable / Uninstall / Validate / Report)."},
	{Key: "moduleVersion", Label: "Module version", Group: "modular", Width: "10%", Sort: "alpha",
		Description: "Version + status of the recommended module."},
	{Key: "moduleSigner", Label: "Module signer", Group: "modular", Width: "12%", Sort: "alpha",
		Description: "Who signed the module's latest published version."},
}

func init() {
	Register(ListIDPolicyCatalog, "Policy Catalog", PolicyCatalogGroups, PolicyCatalogFields)
}

// PolicyCellCtx — extra data the renderers need per-row. Built once by
// the caller (templ template) and passed in to avoid re-computing
// per-policy module candidates inside each cell call.
type PolicyCellCtx struct {
	// Modules — every module in the tenant catalog. Cell renderers
	// filter to candidates by URN; the per-cell filter is O(M) which
	// is fine for hundreds of modules. If this grows we'll precompute
	// a urn→[]Module index in PolicyCellCtx.
	Modules []policymodules.Module
}

// RenderPolicyCell — single source of truth for how each Policy Catalog
// column renders a row. Returns inner HTML (already escaped) suitable
// for `@templ.Raw(...)` inside a <td>. Adding a new field key requires
// one entry in PolicyCatalogFields above and one case here.
func RenderPolicyCell(key string, p policies.Policy, ctx PolicyCellCtx) string {
	switch key {
	case "name":
		out := `<div class="policy-name">` + esc(p.Name)
		if p.Custom {
			out += ` <span class="policy-custom-chip" title="Custom policy authored by tenant ` + esc(p.TenantID) + `">Custom</span>`
		}
		out += `</div>`
		out += `<div class="policy-id font-mono">` + esc(p.ID) + `</div>`
		return out
	case "urn":
		return `<div class="font-mono" style="font-size:11.5px;">` + esc(p.ID) + `</div>`
	case "scope":
		return scopeChipHTML(string(p.Scope))
	case "category":
		return `<div style="font-size:12px;">` + esc(strings.Join(p.Category, " › ")) + `</div>`
	case "origin":
		if p.Custom {
			return `<span class="policy-custom-chip">Custom</span>`
		}
		return `<span class="pdd-origin-bundled">Bundled</span>`
	case "tenant":
		if p.TenantID == "" {
			return `<span class="policy-muted">—</span>`
		}
		return `<span class="font-mono" style="font-size:11.5px;">` + esc(p.TenantID) + `</span>`
	case "gpName":
		if p.WinGPName == "" {
			return `<div class="policy-muted">— (custom)</div>`
		}
		return `<div>` + esc(p.WinGPName) + `</div>` +
			`<div class="policy-winpath font-mono">` + esc(elideMiddleWinPath(p.WinGPPath)) + `</div>`
	case "gpPath":
		if p.WinGPPath == "" {
			return `<span class="policy-muted">—</span>`
		}
		return `<span class="font-mono" style="font-size:11.5px;">` + esc(p.WinGPPath) + `</span>`
	case "linuxImpl":
		if p.LinuxImpl == "" {
			return `<span class="policy-muted">—</span>`
		}
		return `<span class="font-mono" style="font-size:11.5px;">` + glossifyTokens(p.LinuxImpl) + `</span>`
	case "description":
		out := `<div>` + esc(p.Description) + `</div>`
		if p.LinuxImpl != "" {
			out += `<div class="policy-impl font-mono"><span class="label">Linux:</span> ` + glossifyTokens(p.LinuxImpl) + `</div>`
		}
		return out
	case "moduleCount":
		n := candidatesCount(p.ID, ctx.Modules)
		cls := ""
		if n == 0 {
			cls = ` class="policy-muted"`
		}
		return `<span` + cls + `>` + strconv.Itoa(n) + `</span>`
	case "modulePhases":
		ver := primaryModuleVersion(p.ID, ctx.Modules)
		if ver == nil {
			return `<span class="policy-muted">—</span>`
		}
		out := `<div class="pm-phase-strip">`
		for _, ph := range policymodules.AllLifecyclePhases {
			has := false
			for _, s := range ver.Scripts {
				if s.Phase == ph {
					has = true
					break
				}
			}
			if has {
				out += `<span class="pm-phase-pill pm-phase-` + esc(string(ph)) + `" title="` + esc(string(ph)) + ` present">` + ph.Short() + `</span>`
			} else {
				out += `<span class="pm-phase-pill pm-phase-missing" title="` + esc(string(ph)) + ` not provided">·</span>`
			}
		}
		out += `</div>`
		return out
	case "moduleVersion":
		ver := primaryModuleVersion(p.ID, ctx.Modules)
		if ver == nil {
			return `<span class="policy-muted">—</span>`
		}
		return `<div class="pm-ver font-mono">` + esc(ver.Version) + `</div>` +
			`<div class="pm-status pm-status-` + esc(ver.Status) + `">` + esc(ver.Status) + `</div>`
	case "moduleSigner":
		ver := primaryModuleVersion(p.ID, ctx.Modules)
		if ver == nil {
			return `<span class="policy-muted">—</span>`
		}
		return `<div>` + esc(ver.Signature.Signer) + `</div>` +
			`<div class="pm-key font-mono">` + esc(ver.Signature.KeyID) + `</div>`
	}
	return ""
}

// RenderPolicyCellSort — the parallel renderer that returns each
// field's sort value as a plain string, emitted into `<td data-sort=...>`.
// Numeric fields return a decimal string; alphabetical fields return
// the lower-cased visible text (no markup). Empty values sort last via
// the comparator (the JS treats "" as +Infinity for asc / -Infinity
// for desc when explicitly empty).
//
// Adding a new sortable field key is one switch case here, paired with
// the FieldDef.Sort = "alpha"|"num" entry above and the visual case in
// RenderPolicyCell.
func RenderPolicyCellSort(key string, p policies.Policy, ctx PolicyCellCtx) string {
	switch key {
	case "name":
		return strings.ToLower(p.Name)
	case "urn":
		return p.ID
	case "scope":
		return string(p.Scope)
	case "category":
		return strings.ToLower(strings.Join(p.Category, " / "))
	case "origin":
		if p.Custom {
			return "1custom"
		}
		return "0bundled"
	case "tenant":
		return p.TenantID
	case "gpName":
		return strings.ToLower(p.WinGPName)
	case "gpPath":
		return strings.ToLower(p.WinGPPath)
	case "linuxImpl":
		return strings.ToLower(p.LinuxImpl)
	case "description":
		return strings.ToLower(p.Description)
	case "moduleCount":
		return strconv.Itoa(candidatesCount(p.ID, ctx.Modules))
	case "modulePhases":
		ver := primaryModuleVersion(p.ID, ctx.Modules)
		if ver == nil {
			return "0"
		}
		return strconv.Itoa(len(ver.Scripts))
	case "moduleVersion":
		ver := primaryModuleVersion(p.ID, ctx.Modules)
		if ver == nil {
			return ""
		}
		return ver.Version
	case "moduleSigner":
		ver := primaryModuleVersion(p.ID, ctx.Modules)
		if ver == nil {
			return ""
		}
		return strings.ToLower(ver.Signature.Signer)
	}
	return ""
}

// --- private helpers -------------------------------------------------

// esc — html-escape a string. Cell renderers concatenate raw HTML, so
// every interpolated value must go through this.
func esc(s string) string { return html.EscapeString(s) }

// scopeChipHTML — mirrors the @scopeChip templ component but as an
// HTML string. Kept here (not in templ) so the renderer stays a pure
// function callable from any context (cell, detail dialog, CSV export
// in the future).
func scopeChipHTML(s string) string {
	switch s {
	case "computer":
		return `<span class="scope-chip scope-computer">Computer</span>`
	case "user":
		return `<span class="scope-chip scope-user">User</span>`
	case "both":
		return `<span class="scope-chip scope-both">Both</span>`
	}
	return `<span class="scope-chip">` + esc(s) + `</span>`
}

// candidatesCount — number of modules in the catalog that satisfy the
// given policy URN. Uses the public SatisfiesURN method so the rule for
// "what counts as a candidate" stays in policymodules (DRY with the
// dependency resolver and the detail dialog).
func candidatesCount(urn string, mods []policymodules.Module) int {
	n := 0
	for _, m := range mods {
		if m.SatisfiesURN(urn) {
			n++
		}
	}
	return n
}

// primaryModuleVersion — the module version users see in the
// "Module version" / "Lifecycle coverage" / "Module signer" columns.
// v1 picks the FIRST candidate's latest version (which matches the
// detail dialog's expansion order). When binding-aware UI lands, this
// will switch to "the version chosen by the binding currently
// inspected" — but the API stays the same.
func primaryModuleVersion(urn string, mods []policymodules.Module) *policymodules.ModuleVersion {
	for _, m := range mods {
		if m.SatisfiesURN(urn) {
			return m.LatestVersion()
		}
	}
	return nil
}

// glossifyTokens — split a "Linux mechanism" cell value on the natural
// punctuation, look each piece up in the glossary, and emit recognized
// tokens wrapped in the .term span (with hover tooltip carrying the
// AD-equivalent). Unknown pieces stay plain. INV-O3.
//
// Splits on commas, slashes, plus signs, " + ", " / ", and whitespace
// runs while preserving the separator so the rendered cell visually
// matches the source.
func glossifyTokens(s string) string {
	if s == "" {
		return ""
	}
	// Walk the string; accumulate token / non-token runs. Token char:
	// letter, digit, underscore, hyphen, dot. Anything else is a
	// separator (comma, space, slash, parens, etc.) and passes through
	// unchanged.
	var out strings.Builder
	tokenRune := func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
	}
	rs := []rune(s)
	i := 0
	for i < len(rs) {
		// Run of separator chars.
		j := i
		for j < len(rs) && !tokenRune(rs[j]) {
			j++
		}
		if j > i {
			out.WriteString(esc(string(rs[i:j])))
			i = j
			continue
		}
		// Run of token chars.
		j = i
		for j < len(rs) && tokenRune(rs[j]) {
			j++
		}
		tok := string(rs[i:j])
		if t := glossary.Lookup(tok); t.OneLine != "" {
			out.WriteString(`<span class="term" tabindex="0" data-term-key="`)
			out.WriteString(esc(t.Key))
			out.WriteString(`" title="`)
			out.WriteString(esc(t.OneLine + "\n— " + t.ADEquivalent))
			out.WriteString(`"><span class="term-text">`)
			out.WriteString(esc(tok))
			out.WriteString(`</span><span class="term-mark" aria-hidden="true">?</span></span>`)
		} else {
			out.WriteString(esc(tok))
		}
		i = j
	}
	return out.String()
}

// elideMiddleWinPath — when the Windows GP path has more than four
// segments, replace the middle segments with "…" so the path fits in
// the cell. Distinct from templates.stripWinPathPrefix, which trims
// the well-known leading roots ("Computer Configuration|Policies|…").
// Kept local to lists/ to avoid a layering dependency on templates/.
func elideMiddleWinPath(p string) string {
	if p == "" {
		return ""
	}
	parts := strings.Split(p, "|")
	if len(parts) <= 4 {
		return p
	}
	return parts[0] + " | … | " + strings.Join(parts[len(parts)-3:], " | ")
}
