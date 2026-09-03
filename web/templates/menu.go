// Package templates contains the Templ templates and supporting helpers
// for the Pluris management console.
//
// The Menu variable below is the SINGLE SOURCE for left-sidebar navigation
// (per docs/endpoint-management/ui/invariants.md §VI). Every change to the sidebar must be
// reflected in:
//  1. docs/endpoint-management/ui/invariants.md §VI (sidebar list)
//  2. this Menu variable
//  3. pluris/console/server/server.go (route registration)
//  4. pluris/console/server/server_test.go (mount-point test row)
package templates

import (
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/extension"
)

// MenuItem is one entry in the left sidebar (top-level or sub-item).
type MenuItem struct {
	Label    string
	Href     string
	Key      string // matches the `active` argument passed by handlers
	Children []MenuItem
}

// Menu is the locked top-level sidebar (10 items).
var Menu = []MenuItem{
	{Label: "Dashboard", Href: "/", Key: "dashboard"},
	{
		Label: "Users", Href: "/users", Key: "users",
		Children: []MenuItem{
			// Task 6.2: ONE canonical group list at /groups, surfaced twice
			// (here and under Assets); the kind query param presets the
			// list's member-kind view. MenuItemVisible's RoutePermissionKey
			// prefix match ignores the query string, so both entries gate
			// on group.view.
			{Label: "User Groups", Href: "/groups?kind=identity", Key: "users-groups"},
		},
	},
	{
		Label: "Assets", Href: "/assets/computers", Key: "assets",
		Children: []MenuItem{
			{Label: "Computers", Href: "/assets/computers", Key: "assets-computers"},
			{Label: "Servers", Href: "/assets/servers", Key: "assets-servers"},
			{Label: "Printers", Href: "/assets/printers", Key: "assets-printers"},
			{Label: "Desks", Href: "/assets/desks", Key: "assets-desks"},
			{Label: "Groups", Href: "/groups?kind=asset", Key: "assets-groups"},
		},
	},
	{
		Label: "Policy", Href: "/policy/catalog", Key: "policy",
		Children: []MenuItem{
			{Label: "Policy Catalog", Href: "/policy/catalog", Key: "policy-catalog"},
			{Label: "Configuration Groups", Href: "/policy/groups", Key: "policy-groups"},
			{Label: "Modules", Href: "/policy/modules", Key: "policy-modules"},
			{Label: "Dependency Groups", Href: "/policy/dependency-groups", Key: "policy-dependency-groups"},
			{Label: "Pluris Policy", Href: "/policy/pluris", Key: "policy-pluris"},
		},
	},
	{Label: "Profiles", Href: "/profiles", Key: "profiles"},
	{Label: "Scripts", Href: "/scripts", Key: "scripts"},
	{Label: "Wine", Href: "/wine", Key: "wine"},
	{
		Label: "Package Management", Href: "/packages/managers", Key: "packages",
		Children: []MenuItem{
			{Label: "Package Managers", Href: "/packages/managers", Key: "packages-managers"},
			{Label: "Packages", Href: "/packages/packages", Key: "packages-packages"},
			{Label: "Update Cycles", Href: "/packages/cycles", Key: "packages-cycles"},
		},
	},
	{
		Label: "Server Administration", Href: "/server-admin", Key: "server-admin",
		Children: []MenuItem{
			{Label: "Data Management", Href: "/server-admin/data", Key: "server-admin-data"},
		},
	},
	{Label: "User / Admin Preferences", Href: "/preferences", Key: "preferences"},
}

// MenuItemVisible reports whether item should render in the sidebar for
// sess. Returns true when sess is nil (the sidebar only ever renders on
// already-authenticated pages, so a nil session here is a defensive
// fallback -- render unchanged rather than hide navigation), when the
// item's route carries no gated permission key, or when sess's grants
// pass the route's permission check. Used for both top-level items and
// children in layout.templ's Sidebar/menuRow templates.
func MenuItemVisible(sess *auth.UserSession, item MenuItem) bool {
	if sess == nil {
		return true
	}
	if auth.RoutePermissionKey(item.Href) == "" {
		return true
	}
	return auth.CanAccessGrants(sess.Grants, item.Href)
}

// keyMatches reports whether active belongs to itemKey's subtree:
// exact match, or a hierarchical extension at a '-' boundary
// (e.g. itemKey "policy-modules" matches active "policy-modules-library").
func keyMatches(itemKey, active string) bool {
	return active == itemKey || strings.HasPrefix(active, itemKey+"-")
}

// isItemActive reports whether the menu item (or any of its children) is the active page.
func isItemActive(item MenuItem, active string) bool {
	if keyMatches(item.Key, active) {
		return true
	}
	for _, c := range item.Children {
		if keyMatches(c.Key, active) {
			return true
		}
	}
	return false
}

// expandChildren reports whether the item's children should render in the sidebar
// (i.e. the user is currently on a page belonging to this item's subtree).
func expandChildren(item MenuItem, active string) bool {
	for _, c := range item.Children {
		if keyMatches(c.Key, active) {
			return true
		}
	}
	// Also expand for the parent's own key (so e.g. /assets shows children).
	return keyMatches(item.Key, active)
}

// subtypeLabel returns the human-readable label for an Asset subtype / tab key.
func subtypeLabel(s string) string {
	switch s {
	case "computers":
		return "Computers"
	case "servers":
		return "Servers"
	case "printers":
		return "Printers"
	case "desks":
		return "Desks"
	case "scripts":
		return "Scripts"
	case "policy-modules":
		return "Policy Modules"
	case "policy-dependency-groups":
		return "Dependency Groups"
	case "policy-pluris":
		return "Pluris Policy"
	case "catalog":
		return "Policy Catalog"
	case "groups":
		return "Configuration Groups"
	case "managers":
		return "Package Managers"
	case "packages":
		return "Packages"
	case "cycles":
		return "Update Cycles"
	}
	return s
}

// ------------------------------------------------------------------
// Policy page helpers — used by PolicyPage / policyTable templates.
// ------------------------------------------------------------------

// CategoryGroup is a leaf category in the catalog plus the policies that
// live directly under it. Produced by groupByCategory, in source order.
type CategoryGroup struct {
	Path     []string
	Policies []policies.Policy
}

// groupByCategory groups the flat catalog by its Category path, preserving
// the order in which categories first appear in the source slice (so the
// UI matches the curated ordering in catalog_computer.go / catalog_user.go).
func groupByCategory(all []policies.Policy) []CategoryGroup {
	seen := map[string]int{}
	var out []CategoryGroup
	for _, p := range all {
		key := strings.Join(p.Category, "|")
		if idx, ok := seen[key]; ok {
			out[idx].Policies = append(out[idx].Policies, p)
			continue
		}
		seen[key] = len(out)
		out = append(out, CategoryGroup{
			Path:     append([]string{}, p.Category...),
			Policies: []policies.Policy{p},
		})
	}
	return out
}

// categoryAnchor turns a category path into an HTML id suitable for
// anchor navigation from the tree.
func categoryAnchor(path []string) string {
	joined := strings.Join(path, "-")
	joined = strings.ToLower(joined)
	var b strings.Builder
	b.Grow(len(joined))
	for _, r := range joined {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '/', r == '_', r == '-':
			b.WriteByte('-')
		}
	}
	return "cat-" + b.String()
}

// stripWinPathPrefix drops the "Computer Configuration|Policies|" /
// "User Configuration|Policies|" prefixes that repeat on every row,
// to keep the Windows-GP-path column readable, and replaces the
// pipe separator with the › glyph.
func stripWinPathPrefix(p string) string {
	trim := []string{
		"Computer Configuration|Policies|",
		"Computer Configuration|",
		"User Configuration|Policies|",
		"User Configuration|",
	}
	for _, t := range trim {
		if strings.HasPrefix(p, t) {
			p = strings.TrimPrefix(p, t)
			break
		}
	}
	return strings.ReplaceAll(p, "|", " › ")
}

// pluralPolicy returns "policy" or "policies" per count.
func pluralPolicy(n int) string {
	if n == 1 {
		return "policy"
	}
	return "policies"
}

// rootCategoryOf — top of the category path ("Computer Configuration" /
// "User Configuration"). Empty if the path is empty.
func rootCategoryOf(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[0]
}

// rootCategoryOptions — distinct top-level categories, in the order they
// first appear in the catalog. Powers the branch filter dropdown.
func rootCategoryOptions() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range policies.Catalog() {
		r := rootCategoryOf(p.Category)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	return out
}

// policySearchBlob — lowercase, space-joined concatenation of every text
// field a user might search on. Stored in the <tr data-searchable="…"> attr
// and substring-matched by the filter engine.
func policySearchBlob(p policies.Policy) string {
	parts := []string{
		p.Name,
		p.ID,
		p.WinGPName,
		stripWinPathPrefix(p.WinGPPath),
		string(p.Scope),
		strings.Join(p.Category, " "),
		p.Description,
		p.LinuxImpl,
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// treeCount — sum of policy counts in this subtree (recursive leaves +
// direct). Lets group headers show a useful total next to the label.
func treeCount(node *policies.CategoryTree) int {
	n := len(node.Policies)
	for _, c := range node.Children {
		n += treeCount(c)
	}
	return n
}

// treeGroupClass / treeLeafClass — depth-indexed classes so CSS can
// visually distinguish branch (0) / section (1) / group (2+) / leaf.
func treeGroupClass(depth int) string {
	return "policy-tree-group policy-tree-depth-" + strconv.Itoa(clampDepth(depth))
}

func treeLeafClass(depth int) string {
	return "policy-tree-leaf policy-tree-depth-" + strconv.Itoa(clampDepth(depth))
}

func clampDepth(d int) int {
	if d < 0 {
		return 0
	}
	if d > 4 {
		return 4
	}
	return d
}

// ----------------------------------------------------------------------
// Target picker chip helpers (Configuration Groups' popup dialog +
// its configGroupsPageScript were retired in Task 5.2 — see
// config_groups.templ for the standardized pages that replaced them;
// the chip/icon helpers below stay because TargetPickerDialog uses them).
// ----------------------------------------------------------------------

// targetKindClass — colour-coded chip class for the target column. Same
// hue family as the scope chips so the visual language stays consistent.
func targetKindClass(k configgroups.TargetKind) string {
	switch k {
	case configgroups.KindComputer, configgroups.KindComputerGroup:
		return "cg-target-chip cg-target-computer"
	case configgroups.KindUser, configgroups.KindUserGroup:
		return "cg-target-chip cg-target-user"
	case configgroups.KindConfigurationGroup:
		return "cg-target-chip cg-target-cg"
	case configgroups.KindRegex:
		return "cg-target-chip cg-target-regex"
	}
	return "cg-target-chip"
}

// ----------------------------------------------------------------------
// Target picker helpers + dialog wiring script.
// ----------------------------------------------------------------------

// targetKindIconKey — Lucide icon key for the small chip / row leading
// icon. Centralised so the picker, the CG dialog, and the list-view
// chip all use the same glyph for the same kind.
func targetKindIconKey(k configgroups.TargetKind) string {
	switch k {
	case configgroups.KindComputer:
		return "target-computer"
	case configgroups.KindUser:
		return "target-user"
	case configgroups.KindComputerGroup:
		return "target-computer-group"
	case configgroups.KindUserGroup:
		return "target-user-group"
	case configgroups.KindConfigurationGroup:
		return "target-config-group"
	case configgroups.KindRegex:
		return "target-regex"
	}
	return ""
}

// targetKindBgClass — pastel background tint class for a kind. Used by
// the kind-filter chips and the "current target" summary block.
func targetKindBgClass(k configgroups.TargetKind) string {
	switch k {
	case configgroups.KindComputer, configgroups.KindComputerGroup:
		return "tp-bg-computer"
	case configgroups.KindUser, configgroups.KindUserGroup:
		return "tp-bg-user"
	case configgroups.KindConfigurationGroup:
		return "tp-bg-cg"
	case configgroups.KindRegex:
		return "tp-bg-regex"
	}
	return ""
}

// targetKindIconClass — saturated foreground class for the leading icon
// circle. Same hue family as the chip + bg classes.
func targetKindIconClass(k configgroups.TargetKind) string {
	switch k {
	case configgroups.KindComputer, configgroups.KindComputerGroup:
		return "tp-icon-computer"
	case configgroups.KindUser, configgroups.KindUserGroup:
		return "tp-icon-user"
	case configgroups.KindConfigurationGroup:
		return "tp-icon-cg"
	case configgroups.KindRegex:
		return "tp-icon-regex"
	}
	return ""
}

// targetSearchBlob — case-insensitive haystack for the picker's search
// input. Includes ref, label, meta, kind name, and tags so an admin
// can find a target by hostname, email, OS, group membership, etc.
func targetSearchBlob(t configgroups.Target) string {
	parts := []string{t.Label, t.Ref, t.Meta, string(t.Kind), t.Kind.Label()}
	parts = append(parts, t.Tags...)
	return strings.ToLower(strings.Join(parts, " "))
}

// joinKinds — comma-joined string list of kinds, for the picker's
// `data-allowed-kinds` attribute. JS reads it back as a Set.
func joinKinds(ks []configgroups.TargetKind) string {
	if len(ks) == 0 {
		return ""
	}
	out := make([]string, 0, len(ks))
	for _, k := range ks {
		out = append(out, string(k))
	}
	return strings.Join(out, ",")
}

// targetPickerScript — open / close / search / filter / pick wiring for
// the reusable TargetPickerDialog. It waits for DOMContentLoaded so it
// works regardless of where the script tag is emitted relative to the
// dialog.
//
// Selection contract:
//
//   - Open: any element with `data-target-picker-open` opens the dialog.
//     Optional `data-allowed-kinds="computer,user"` on the trigger
//     overrides the dialog's default allow-list for that one open.
//   - Pick: clicking a row dispatches `target:pick` on `document` with
//     CustomEvent.detail = { kind, ref, label, meta }. The picker then
//     closes itself.
//   - Cancel: × / Cancel / Esc close without dispatching.
//
// Filter behaviour:
//
//   - The kind chips are mutually exclusive ("All" + one kind), since
//     each row has exactly one kind. Multi-select would just be the
//     union of single-selects.
//   - Search ANDs with the kind chip — both must match for a row to
//     show. Match is substring on the precomputed `data-tp-search` blob.
const targetPickerScript = `<script>
(function () {
	'use strict';
	function ready(fn) {
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', fn, { once: true });
		} else { fn(); }
	}
	ready(init);

function init() {
	const dlg = document.getElementById('target-picker');
	if (!dlg) return;
	const search = document.getElementById('tp-search');
	const kindChipsEl = document.getElementById('tp-kind-chips');
	const tableBody = dlg.querySelector('.tp-table tbody');
	const empty = document.getElementById('tp-empty');
	const countEl = document.getElementById('tp-count');

	let activeKind = '';            // '' = All
	let allowedKinds = parseAllowed(dlg.dataset.allowedKinds);

	function parseAllowed(s) {
		if (!s) return null; // null = no restriction
		return new Set(s.split(',').filter(Boolean));
	}

	function applyFilter() {
		const q = (search.value || '').trim().toLowerCase();
		let shown = 0, total = 0;
		tableBody.querySelectorAll('tr').forEach(row => {
			total++;
			const k = row.dataset.tpKind;
			const blob = row.dataset.tpSearch || '';
			let visible = true;
			if (allowedKinds && !allowedKinds.has(k)) visible = false;
			if (visible && activeKind && k !== activeKind) visible = false;
			if (visible && q && blob.indexOf(q) === -1) visible = false;
			row.classList.toggle('tp-hidden', !visible);
			if (visible) shown++;
		});
		empty.style.display = shown === 0 ? 'block' : 'none';
		countEl.textContent = shown + ' of ' + total + ' targets';
	}

	function openPicker(opener) {
		// Per-trigger override of allowed kinds.
		if (opener && opener.dataset.allowedKinds) {
			allowedKinds = parseAllowed(opener.dataset.allowedKinds);
		} else {
			allowedKinds = parseAllowed(dlg.dataset.allowedKinds);
		}
		// Hide chips that aren't allowed for this open.
		kindChipsEl.querySelectorAll('[data-tp-kind]').forEach(chip => {
			const k = chip.dataset.tpKind;
			const allow = !allowedKinds || k === '' || allowedKinds.has(k);
			chip.style.display = allow ? '' : 'none';
		});
		// Reset state.
		activeKind = '';
		kindChipsEl.querySelectorAll('.tp-kind-chip').forEach(c => c.classList.remove('is-active'));
		kindChipsEl.querySelector('[data-tp-kind=""]').classList.add('is-active');
		search.value = '';
		applyFilter();

		if (typeof dlg.showModal === 'function') dlg.showModal();
		else dlg.setAttribute('open', 'open');
		document.body.classList.add('cg-dialog-locked');
		setTimeout(() => search.focus(), 0);
	}

	function closePicker() {
		if (typeof dlg.close === 'function') dlg.close();
		else dlg.removeAttribute('open');
		document.body.classList.remove('cg-dialog-locked');
	}

	// --- Triggers ----------------------------------------------------
	document.addEventListener('click', e => {
		const opener = e.target.closest('[data-target-picker-open]');
		if (opener) {
			e.preventDefault();
			openPicker(opener);
			return;
		}
		if (e.target.closest('[data-tp-close]')) {
			e.preventDefault();
			closePicker();
		}
	});

	// Live search.
	search.addEventListener('input', applyFilter);
	search.addEventListener('keydown', e => {
		if (e.key === 'Escape') { e.preventDefault(); closePicker(); }
	});

	// Kind chips (single-select; "All" clears).
	kindChipsEl.addEventListener('click', e => {
		const chip = e.target.closest('.tp-kind-chip');
		if (!chip) return;
		kindChipsEl.querySelectorAll('.tp-kind-chip').forEach(c => c.classList.remove('is-active'));
		chip.classList.add('is-active');
		activeKind = chip.dataset.tpKind || '';
		applyFilter();
	});

	// Row click → pick + dispatch.
	tableBody.addEventListener('click', e => {
		const row = e.target.closest('tr');
		if (!row) return;
		const detail = {
			kind:  row.dataset.tpKind,
			ref:   row.dataset.tpRef,
			label: row.dataset.tpLabel,
			meta:  row.dataset.tpMeta,
		};
		document.dispatchEvent(new CustomEvent('target:pick', { detail }));
		closePicker();
	});

	dlg.addEventListener('cancel', e => { e.preventDefault(); closePicker(); });
}
})();
</script>`

// ----------------------------------------------------------------------
// Policy Catalog: origin/lifecycle helpers.
// ----------------------------------------------------------------------

// policyOrigin — emitted as data-origin on each catalog row so the
// origin filter (#pf-origin) and any future "show only my custom"
// checkbox can pick the right bucket. Bundled = ships in code; custom
// = tenant-authored via the wizard. INV-M10 says they live in the same
// list but the data attr lets us filter without splitting tabs.
func policyOrigin(p policies.Policy) string {
	if p.Custom {
		return "custom"
	}
	return "bundled"
}

// policymodulesAllPhases — exposes the lifecycle phase enum to templates
// (templ can only call package-level functions from inside a template).
// Returning the slice from the policymodules package keeps a single
// source of truth for the order + labels. Used by the Library's
// per-module lifecycle-phase pill strip.
func policymodulesAllPhases() []policymodules.LifecyclePhase {
	return policymodules.AllLifecyclePhases
}

// ----------------------------------------------------------------------
// Scripts → Policy Modules list helpers + script.
// ----------------------------------------------------------------------

// moduleSearchBlob — case-insensitive haystack for the modules-list
// search input. Includes id, title, satisfies URNs, signer, and origin
// so admins can find a module by any of those.
func moduleSearchBlob(m policymodules.Module) string {
	parts := []string{m.ID, m.Title, m.Description, m.Origin, m.Scope}
	parts = append(parts, m.Satisfies...)
	if v := m.LatestVersion(); v != nil {
		parts = append(parts, v.Signature.Signer, v.Signature.KeyID, v.Status, v.Version)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func moduleSelectCaps(m policymodules.Module, deleted bool) []string {
	if deleted {
		return []string{"restore", "purge"}
	}
	if m.Origin == "bundled" {
		return []string{"duplicate"}
	}
	return []string{"duplicate", "revoke", "delete"}
}

// moduleHasPhase — quick predicate used by the lifecycle phase strip in
// the modules table. nil-version-safe (custom modules-in-progress
// without a published version render the strip as all-empty).
func moduleHasPhase(v *policymodules.ModuleVersion, ph policymodules.LifecyclePhase) bool {
	if v == nil {
		return false
	}
	for _, s := range v.Scripts {
		if s.Phase == ph {
			return true
		}
	}
	return false
}

// policyModulePickerScript — open/close shim and row-action stubs for
// the canonical PolicyModulePicker dialog. Any element carrying
// data-pmp-open opens the dialog and hydrates it from the element's
// data attributes (urn, os, source, current-id, current-source).
//
// Save is a stub in v1: it logs the chosen module to the console and
// closes the dialog. The real save path lands when the picker is
// wired to a binding row in the CG editor (next slice).
//
// Row-action handlers (data-pm-action="edit|clone|disable|delete") are
// also stubs: they flash a toast that names the action + module. They
// exist now so the UX contract is locked and the affordances are
// discoverable.
const policyModulePickerScript = `<script>
(function () {
'use strict';
function ready(fn) {
if (document.readyState === 'loading') {
document.addEventListener('DOMContentLoaded', fn, { once: true });
} else { fn(); }
}
ready(init);

function init() {
const dlg = document.getElementById('pmp-dialog');
if (!dlg) return;
const titleURN = document.getElementById('pmp-target-urn');
const titleOS = document.getElementById('pmp-target-os');
const titleSrc = document.getElementById('pmp-target-source');
const curName = document.getElementById('pmp-current-name');
const curSrc = document.getElementById('pmp-current-source');
const empty = document.getElementById('pmp-candidates-empty');
const list = document.getElementById('pmp-candidates');
const saveBtn = document.getElementById('pmp-save');

document.addEventListener('click', function (ev) {
const opener = ev.target.closest('[data-pmp-open]');
if (opener) {
ev.preventDefault();
hydrate(opener);
if (typeof dlg.showModal === 'function') {
dlg.showModal();
} else {
dlg.setAttribute('open', '');
}
return;
}
const action = ev.target.closest('[data-pm-action]');
if (action) {
ev.preventDefault();
const act = action.dataset.pmAction;
const id = action.dataset.pmId;
toast(act + ': ' + id + ' (stub — wires to editors/PolicyModuleEditor in next slice)');
return;
}
const clear = ev.target.closest('[data-pm-default-clear]');
if (clear) {
ev.preventDefault();
toast('clear tenant default for ' + clear.dataset.pmDefaultClear + ' (stub)');
return;
}
});

function hydrate(opener) {
const urn = opener.dataset.pmpUrn || '—';
const os = opener.dataset.pmpOs || 'any';
const source = opener.dataset.pmpSource || 'binding';
const currentId = opener.dataset.pmpCurrentId || '';
const currentSrc = opener.dataset.pmpCurrentSource || '';
titleURN.textContent = urn;
titleOS.textContent = os;
titleSrc.textContent = sourceLabel(source);
if (currentId) {
curName.textContent = currentId;
curSrc.textContent = currentSrc || 'Source: ' + sourceLabel(source);
} else {
curName.textContent = '— Pluris default —';
curSrc.textContent = 'No pick set; first bundled module that satisfies this policy is used.';
}
// v1: no candidate list (wired in next slice from policymodules.CandidatesForPolicy).
list.innerHTML = '';
if (empty) empty.hidden = false;
saveBtn.disabled = true;
}

function sourceLabel(s) {
if (s === 'tenant') return 'tenant-wide default';
if (s === 'wizard') return 'wizard step';
return 'binding override';
}

function toast(msg) {
let t = document.getElementById('pluris-toast');
if (!t) {
t = document.createElement('div');
t.id = 'pluris-toast';
t.style.cssText = 'position:fixed;bottom:1rem;right:1rem;background:#0a1628;color:#fff;padding:.75rem 1rem;border-radius:.5rem;border:1px solid #00d4ff;font-size:.875rem;z-index:9999;box-shadow:0 4px 12px rgba(0,0,0,.4)';
document.body.appendChild(t);
}
t.textContent = msg;
t.style.opacity = '1';
clearTimeout(t._hide);
t._hide = setTimeout(() => { t.style.opacity = '0'; t.style.transition = 'opacity .3s'; }, 3000);
}
}
})();
</script>`

// ----------------------------------------------------------------------
// Policy Detail Dialog (PDD) — payload builder + hydration script.
// ----------------------------------------------------------------------

// (PolicyDetailDialog popup removed in Task 15 - the policy detail page replaced it.)

// ----------------------------------------------------------------------
// Column picker — generic across every list registered with web/lists.
// ----------------------------------------------------------------------
//
// One script handles every column picker on the page. Each picker is
// scoped by its `[data-col-picker=<listId>]` host element. Visibility
// state is persisted in `pluris.cols.<listId>` localStorage as a JSON
// array of visible field keys; loaded on init, written on each change.
// The picker hides cells by setting `style.display = 'none'` on every
// `<th>` and `<td>` whose `data-field` is not in the visible set
// scoped to tables that share the same `data-list-id`.
//
// This script is included once via `@templ.Raw(columnPickerScript)`
// inside any list template that mounts a ColumnPickerButton. Mounting
// it more than once is harmless (the init guard checks for already-
// processed hosts) but unnecessary.
const columnPickerScript = `<script>
(function () {
'use strict';
function ready(fn) {
if (document.readyState === 'loading') {
document.addEventListener('DOMContentLoaded', fn, { once: true });
} else { fn(); }
}
ready(init);

function init() {
document.querySelectorAll('[data-col-picker]').forEach(host => {
if (host.dataset.colPickerInit === '1') return;
host.dataset.colPickerInit = '1';
wire(host);
});
}

function wire(host) {
const listId = host.dataset.colPicker;
const pop    = host.querySelector('[data-col-picker-pop]');
const toggle = host.querySelector('[data-col-picker-toggle]');
const close  = host.querySelector('[data-col-picker-close]');
const reset  = host.querySelector('[data-col-picker-reset]');
const allBtn = host.querySelector('[data-col-picker-all]');
const noneBtn= host.querySelector('[data-col-picker-none]');
const checkboxes = host.querySelectorAll('input[type="checkbox"][data-col-key]');
const countEl = host.querySelector('[data-col-picker-count]');
const wrap   = document.querySelector('[data-list-id="' + listId + '"]');
const defaultKeys = (wrap && wrap.dataset.defaultCols ? wrap.dataset.defaultCols : '').split(',').filter(Boolean);
const allKeys = Array.from(checkboxes).map(cb => cb.dataset.colKey);
const storageKey = 'pluris.cols.' + listId;

function load() {
try {
const raw = localStorage.getItem(storageKey);
if (!raw) return defaultKeys.slice();
const arr = JSON.parse(raw);
// An empty persisted set would render every cell display:none, which
// collapses each row to zero height -- the row is then impossible to
// click, and because the state is persisted the list looks
// permanently broken. Fall back to defaults instead (self-heals a
// browser already holding an empty set).
if (!Array.isArray(arr) || arr.length === 0) return defaultKeys.slice();
return arr;
} catch (e) { return defaultKeys.slice(); }
}
function save(keys) {
try { localStorage.setItem(storageKey, JSON.stringify(keys)); } catch (e) {}
}
function visibleSet() {
const set = new Set();
checkboxes.forEach(cb => { if (cb.checked) set.add(cb.dataset.colKey); });
return set;
}
function apply() {
let set = visibleSet();
// Never leave the table with zero visible columns: every cell would be
// display:none, every row would collapse to zero height, and rows
// would stop being clickable (INV-L10 row navigation). Snap back to
// the registry defaults and reflect that in the checkboxes.
if (set.size === 0) {
checkboxes.forEach(cb => { cb.checked = defaultKeys.indexOf(cb.dataset.colKey) !== -1; });
set = visibleSet();
}
// Every table on the page belonging to this list. Keeps the
// per-leaf-category mini-tables of policyTable in sync.
document.querySelectorAll('table[data-list-id="' + listId + '"]').forEach(tbl => {
tbl.querySelectorAll('th[data-field],td[data-field]').forEach(cell => {
cell.style.display = set.has(cell.dataset.field) ? '' : 'none';
});
});
if (countEl) {
const total = allKeys.length;
const shown = set.size;
countEl.textContent = shown === total ? '· all' : '· ' + shown + '/' + total;
}
save(Array.from(set));
}

// --- initial state from localStorage ---
const initial = new Set(load());
checkboxes.forEach(cb => { cb.checked = initial.has(cb.dataset.colKey); });
// Apply registry-declared column widths (templ rejects dynamic
// style= attrs, so we propagate data-width → style.width here once).
document.querySelectorAll('table[data-list-id="' + listId + '"] th[data-field][data-width]').forEach(th => {
const w = th.dataset.width;
if (w) th.style.width = w;
});
apply();

// --- interactions ---
function openPop()  { pop.hidden = false; }
function closePop() { pop.hidden = true; }
toggle.addEventListener('click', e => { e.stopPropagation(); pop.hidden ? openPop() : closePop(); });
if (close) close.addEventListener('click', closePop);
checkboxes.forEach(cb => cb.addEventListener('change', apply));
if (reset)  reset.addEventListener('click',  () => { checkboxes.forEach(cb => { cb.checked = defaultKeys.indexOf(cb.dataset.colKey) !== -1; }); apply(); });
if (allBtn) allBtn.addEventListener('click', () => { checkboxes.forEach(cb => { cb.checked = true; });  apply(); });
if (noneBtn)noneBtn.addEventListener('click',() => { checkboxes.forEach(cb => { cb.checked = false; }); apply(); });

// --- search/filter functionality ---
const searchInput = host.querySelector('[data-col-picker-search]');
const searchClear = host.querySelector('[data-col-picker-search-clear]');
if (searchInput) {
  searchInput.addEventListener('input', () => {
    const query = searchInput.value.toLowerCase().trim();
    const allItems = host.querySelectorAll('.col-picker-list li[data-col-search-text]');
    const allGroups = host.querySelectorAll('.col-picker-group');
    
    // Show/hide clear button
    if (searchClear) {
      searchClear.hidden = query === '';
    }
    
    if (query === '') {
      // No filter - show everything
      allItems.forEach(li => li.removeAttribute('data-filtered-hidden'));
      allGroups.forEach(grp => grp.removeAttribute('data-filtered-hidden'));
    } else {
      // Filter items
      allItems.forEach(li => {
        const searchText = li.dataset.colSearchText || '';
        const matches = searchText.includes(query);
        if (matches) {
          li.removeAttribute('data-filtered-hidden');
        } else {
          li.setAttribute('data-filtered-hidden', '');
        }
      });
      
      // Hide groups that have no visible items
      allGroups.forEach(grp => {
        const visibleItems = grp.querySelectorAll('.col-picker-list li:not([data-filtered-hidden])');
        if (visibleItems.length === 0) {
          grp.setAttribute('data-filtered-hidden', '');
        } else {
          grp.removeAttribute('data-filtered-hidden');
        }
      });
    }
  });
  
  if (searchClear) {
    searchClear.addEventListener('click', () => {
      searchInput.value = '';
      searchInput.dispatchEvent(new Event('input'));
      searchInput.focus();
    });
  }
}

// Click outside the popover → close. Keyboard Esc → close.
document.addEventListener('click', e => {
if (pop.hidden) return;
if (host.contains(e.target)) return;
closePop();
});
document.addEventListener('keydown', e => { if (e.key === 'Escape' && !pop.hidden) closePop(); });
}
})();
</script>`

// ----------------------------------------------------------------------
// Extension framework helpers used by the Sources sub-view.
// ----------------------------------------------------------------------

// SourceCount — live count of extensions for one Kind + Source. Reads
// through pkg/extension so the same number powers chrome regardless of
// which kind we're rendering. Today the Sources page only renders the
// policy-module kind; when themes / glossary packs / dashboard tiles
// register, this helper serves them too without changes.
//
// Returned as a pre-formatted string so the templ caller can drop it
// into a meta line ("3 modules · always enabled · …") without doing
// strconv inside the template.
func SourceCount(kind extension.Kind, src extension.Source) string {
	counts := extension.CountBySource(kind)
	n := counts[src]
	noun := kindNoun(kind, n)
	return strconv.Itoa(n) + " " + noun
}

// kindNoun — singular / plural label per kind. Concrete kinds keep
// adding cases here (or — better — register a Noun pair on KindSpec
// when the second kind ships and we promote this).
func kindNoun(kind extension.Kind, n int) string {
	switch kind {
	case extension.KindPolicyModule:
		if n == 1 {
			return "module"
		}
		return "modules"
	}
	if n == 1 {
		return "extension"
	}
	return "extensions"
}
