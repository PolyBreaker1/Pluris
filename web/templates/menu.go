// Package templates contains the Templ templates and supporting helpers
// for the Pluris management console.
//
// The Menu variable below is the SINGLE SOURCE for left-sidebar navigation
// (per docs/UX_INVARIANTS.md §VI). Every change to the sidebar must be
// reflected in:
//  1. docs/UX_INVARIANTS.md §VI (sidebar list)
//  2. this Menu variable
//  3. pluris/console/server/server.go (route registration)
//  4. pluris/console/server/server_test.go (mount-point test row)
package templates

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/configgroups"
	"github.com/pluris/pluris/catalog/policies"
	"github.com/pluris/pluris/catalog/policymodules"
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
	{Label: "Users", Href: "/users", Key: "users"},
	{
		Label: "Assets", Href: "/assets/computers", Key: "assets",
		Children: []MenuItem{
			{Label: "Computers", Href: "/assets/computers", Key: "assets-computers"},
			{Label: "Servers", Href: "/assets/servers", Key: "assets-servers"},
			{Label: "Printers", Href: "/assets/printers", Key: "assets-printers"},
			{Label: "Desks", Href: "/assets/desks", Key: "assets-desks"},
		},
	},
	{
		Label: "Policy", Href: "/policy/catalog", Key: "policy",
		Children: []MenuItem{
			{Label: "Policy Catalog", Href: "/policy/catalog", Key: "policy-catalog"},
			{Label: "Configuration Groups", Href: "/policy/groups", Key: "policy-groups"},
			{Label: "Modules", Href: "/policy/modules", Key: "policy-modules"},
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
	{Label: "Server Administration", Href: "/server-admin", Key: "server-admin"},
	{Label: "User / Admin Preferences", Href: "/preferences", Key: "preferences"},
}

// isItemActive reports whether the menu item (or any of its children) is the active page.
func isItemActive(item MenuItem, active string) bool {
	if item.Key == active {
		return true
	}
	for _, c := range item.Children {
		if c.Key == active {
			return true
		}
	}
	return false
}

// expandChildren reports whether the item's children should render in the sidebar
// (i.e. the user is currently on a page belonging to this item's subtree).
func expandChildren(item MenuItem, active string) bool {
	for _, c := range item.Children {
		if c.Key == active {
			return true
		}
	}
	// Also expand for the parent's own key (so e.g. /assets shows children).
	return item.Key == active
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
// Configuration Groups page helpers + dialog wiring script.
// ----------------------------------------------------------------------

// cgSearchBlob — lowercase, space-joined searchable blob for the
// Configuration Groups list-view filter (mirrors policySearchBlob). Lives
// on each <tr data-cg-searchable="…">.
func cgSearchBlob(g configgroups.ConfigurationGroup) string {
	parts := []string{
		g.Name,
		g.ID,
		g.Description,
		string(g.TargetKind),
		g.TargetRef,
		configgroups.LookupTargetLabel(g.TargetKind, g.TargetRef),
		g.UpdatedBy,
	}
	return strings.ToLower(strings.Join(parts, " "))
}

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

// formatBindingValues — single-line "k=v, k=v" rendering for the dialog
// preview. Sorted by key so output is deterministic for snapshot tests.
func formatBindingValues(v map[string]string) string {
	if len(v) == 0 {
		return "(no values)"
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+v[k])
	}
	return strings.Join(parts, ", ")
}

// configGroupsPageScript — list filter + dialog open/close/save wiring
// for /policy/groups. The script is intentionally self-contained:
//
//   - Triggers: any element with `data-cg-open="<id>"` opens the dialog.
//     id == "new" creates a blank form; otherwise the row's serialised
//     payload (data-cg-* attrs on its <tr>) hydrates the form.
//   - Restore: snapshots the form state at open time and reverts on click.
//   - Save: stub — fires a CustomEvent("cg:save", { detail }) so other
//     pages embedding the dialog can hook persistence without touching
//     this file. Hidden by default — backend slice lands later.
//   - Close: dialog.close() + restores body scroll.
//
// To embed the dialog from another page, mount @ConfigurationGroupDialog
// alongside any element carrying data-cg-open and include this script
// once on the page.
const configGroupsPageScript = `<script>
(function () {
	'use strict';

	// Defer wiring until DOMContentLoaded — the script tag is emitted by
	// configGroupsTable, which renders BEFORE @ConfigurationGroupDialog,
	// so #cg-dialog isn't in the DOM at script-run time. Waiting fixes
	// the "buttons don't do anything" bug without forcing a template
	// re-order (the script + dialog can be mounted in any order from
	// any embedding page).
	function ready(fn) {
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', fn, { once: true });
		} else { fn(); }
	}
	ready(init);

function init() {
	const dlg = document.getElementById('cg-dialog');
	if (!dlg) return;
	const form = dlg.querySelector('form.cg-dialog-form');
	const titleEl = document.getElementById('cg-dialog-title');

	let snapshot = null; // form values at open time (for Restore)

	// Search filter is now handled by the unified list engine
	// (web/static/lists.js + lists.css). The toolbar wires it via
	// data-pluris-filter on the input + data-pluris-list on the
	// section / table. See INV-L9.

	// --- Dialog open / close ----------------------------------------
	function openDialog(id) {
		// Hydrate form for "new" vs "edit" — for edit, pull values from the
		// row's data-cg-* attrs. Backend slice will replace this with a
		// fetch, but the data shape stays the same.
		const isNew = id === 'new' || !id;
		titleEl.textContent = isNew ? 'New configuration group' : 'Edit configuration group';
		dlg.dataset.mode = isNew ? 'new' : 'edit';
		dlg.dataset.cgId = isNew ? '' : id;

		if (!isNew) {
			const row = document.querySelector('tr[data-cg-id="' + cssEscape(id) + '"]');
			if (row) {
				// Minimal hydration for the mock — populates the visible name field
				// from the row label so the dialog feels "loaded". Real binding
				// editor mounts in a later increment.
				const name = (row.querySelector('.cg-name') || {}).textContent || '';
				const f = form.querySelector('input[name="name"]');
				if (f) f.value = name.trim();
			}
		} else {
			form.querySelectorAll('input[type="text"], input:not([type]), textarea').forEach(el => { el.value = ''; });
		}

		// Take snapshot AFTER hydration so Restore reverts to the loaded state.
		snapshot = serialiseForm();

		if (typeof dlg.showModal === 'function') {
			dlg.showModal();
		} else {
			dlg.setAttribute('open', 'open');
		}
		document.body.classList.add('cg-dialog-locked');
	}

	function closeDialog() {
		if (typeof dlg.close === 'function') dlg.close();
		else dlg.removeAttribute('open');
		document.body.classList.remove('cg-dialog-locked');
	}

	function serialiseForm() {
		const out = {};
		new FormData(form).forEach((v, k) => { out[k] = v; });
		return out;
	}

	function restoreForm() {
		if (!snapshot) return;
		Object.keys(snapshot).forEach(k => {
			const fields = form.querySelectorAll('[name="' + cssEscape(k) + '"]');
			fields.forEach(el => {
				if (el.type === 'radio') {
					el.checked = (el.value === snapshot[k]);
				} else {
					el.value = snapshot[k];
				}
			});
		});
	}

	// --- Triggers ----------------------------------------------------
	document.addEventListener('click', e => {
		const opener = e.target.closest('[data-cg-open]');
		if (opener) {
			e.preventDefault();
			openDialog(opener.dataset.cgOpen);
			return;
		}
		if (e.target.closest('[data-cg-close]')) {
			e.preventDefault();
			closeDialog();
			return;
		}
		if (e.target.closest('[data-cg-restore]')) {
			e.preventDefault();
			restoreForm();
			return;
		}
		if (e.target.closest('[data-cg-save]')) {
			e.preventDefault();
			const detail = { id: dlg.dataset.cgId || null, mode: dlg.dataset.mode, values: serialiseForm() };
			document.dispatchEvent(new CustomEvent('cg:save', { detail }));
			closeDialog();
			return;
		}
	});

	// Click on a row body opens edit (skip the row's own button to avoid double-fire).
	document.querySelectorAll('.cg-table tbody tr').forEach(row => {
		row.addEventListener('click', e => {
			if (e.target.closest('[data-cg-open]')) return;
			if (row.dataset.cgId) openDialog(row.dataset.cgId);
		});
	});

	// Esc to close (browser native for <dialog>, but we add for fallback).
	dlg.addEventListener('cancel', e => { e.preventDefault(); closeDialog(); });

	// --- Target picker integration ----------------------------------
	// Listen for picks from the reusable TargetPickerDialog and update
	// the hidden form inputs + the visible summary card. We don't import
	// any DOM from the picker — the contract is a CustomEvent only.
	const KIND_META = {
		'computer':            { iconClass: 'tp-icon-computer',         bgClass: 'tp-bg-computer', icon: 'target-computer',         label: 'Computer',                       chip: 'cg-target-chip cg-target-computer' },
		'user':                { iconClass: 'tp-icon-user',             bgClass: 'tp-bg-user',     icon: 'target-user',             label: 'User',                           chip: 'cg-target-chip cg-target-user' },
		'computer_group':      { iconClass: 'tp-icon-computer',         bgClass: 'tp-bg-computer', icon: 'target-computer-group',   label: 'Computer group',                 chip: 'cg-target-chip cg-target-computer' },
		'user_group':          { iconClass: 'tp-icon-user',             bgClass: 'tp-bg-user',     icon: 'target-user-group',       label: 'User group',                     chip: 'cg-target-chip cg-target-user' },
		'configuration_group': { iconClass: 'tp-icon-cg',               bgClass: 'tp-bg-cg',       icon: 'target-config-group',     label: 'Configuration group (members)',  chip: 'cg-target-chip cg-target-cg' },
		'regex':               { iconClass: 'tp-icon-regex',            bgClass: 'tp-bg-regex',    icon: 'target-regex',            label: 'Regex match',                    chip: 'cg-target-chip cg-target-regex' },
	};

	function renderTargetSummary(detail) {
		const host = document.getElementById('cg-target-current');
		if (!host) return;
		host.dataset.kind = detail.kind || '';
		host.dataset.ref  = detail.ref  || '';
		const m = KIND_META[detail.kind];
		if (!m) {
			host.innerHTML = '<div class="cg-target-summary cg-target-summary-empty"><div class="cg-target-icon cg-target-icon-empty"></div><div><div class="cg-target-summary-label">No target selected</div><div class="cg-target-summary-meta">Click "Change target…" to choose one.</div></div></div>';
			return;
		}
		// Use the icon's <use> trick: server-rendered icon shape comes from
		// the picker's row icon. Cheaper to clone than to maintain a JS
		// duplicate of every Lucide path.
		const proto = document.querySelector('tr[data-tp-kind="' + detail.kind + '"] .tp-row-icon');
		const iconHTML = proto ? proto.outerHTML.replace('tp-row-icon', '') : '';
		host.className = 'cg-target-current';
		host.innerHTML =
			'<div class="cg-target-summary ' + m.bgClass + '">' +
				iconHTML +
				'<div>' +
					'<div class="cg-target-summary-label">' +
						escapeHTML(detail.label || detail.ref) +
						' <span class="' + m.chip + '">' + escapeHTML(m.label) + '</span>' +
					'</div>' +
					'<div class="cg-target-summary-meta">' + escapeHTML(detail.meta || '') + '</div>' +
				'</div>' +
			'</div>';
	}

	function escapeHTML(s) {
		return String(s == null ? '' : s).replace(/[&<>"']/g, ch => ({
			'&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;'
		}[ch]));
	}

	document.addEventListener('target:pick', e => {
		const d = e.detail || {};
		const kindEl = document.getElementById('cg-f-target-kind');
		const refEl  = document.getElementById('cg-f-target-ref');
		if (kindEl) kindEl.value = d.kind || '';
		if (refEl)  refEl.value  = d.ref  || '';
		renderTargetSummary(d);
	});

	// CSS.escape polyfill (very narrow — only for ID-safe slugs/numerics).
	function cssEscape(s) {
		return (window.CSS && CSS.escape) ? CSS.escape(s) : String(s).replace(/(["\\])/g, '\\$1');
	}
}
})();
</script>`

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
// the reusable TargetPickerDialog. Like configGroupsPageScript it waits
// for DOMContentLoaded so it works regardless of where the script tag
// is emitted relative to the dialog.
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
// Policy Catalog: Custom Policy Wizard helpers + script.
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

// policymodulesAllPhases — exposes the lifecycle phase enum to the
// wizard's "Module" step (templ can only call package-level functions
// from inside a template). Returning the slice from the policymodules
// package keeps a single source of truth for the order + labels.
func policymodulesAllPhases() []policymodules.LifecyclePhase {
	return policymodules.AllLifecyclePhases
}

// customPolicyWizardScript — open / close / step / save wiring for the
// CustomPolicyWizard dialog. Like the other dialog scripts on this page
// it waits for DOMContentLoaded so the wizard can be embedded
// regardless of where its script tag lands relative to the dialog.
//
// The wizard intentionally keeps all five panes in one DOM and toggles
// `hidden` on each — no per-step server round-trip; state is collected
// at "Sign & publish" and dispatched as a `cpw:save` CustomEvent for
// the backend slice to wire persistence later.
//
// The Step-4 script editors are dynamically built from the phase
// checkboxes the user ticks in Step 2 — each enabled phase gets a tab
// + a textarea; the phase's runtime (bash | wasm) is shown in the tab
// label. Real Monaco editor lands with Phase 3 of ADR-007.
const customPolicyWizardScript = `<script>
(function () {
'use strict';
function ready(fn) {
if (document.readyState === 'loading') {
document.addEventListener('DOMContentLoaded', fn, { once: true });
} else { fn(); }
}
ready(init);

function init() {
const dlg = document.getElementById('cpw-dialog');
if (!dlg) return;
const form = dlg.querySelector('form.cpw-form');

const STEP_COUNT = 5;
const TENANT_ID = 'acme'; // mock: real value comes from session in Phase 1

function setStep(n) {
n = Math.max(1, Math.min(STEP_COUNT, n));
dlg.dataset.step = String(n);
dlg.querySelectorAll('[data-cpw-pane]').forEach(p => {
p.hidden = (Number(p.dataset.cpwPane) !== n);
});
dlg.querySelectorAll('[data-cpw-step]').forEach(s => {
s.classList.toggle('is-active', Number(s.dataset.cpwStep) === n);
s.classList.toggle('is-done', Number(s.dataset.cpwStep) < n);
});
dlg.querySelector('[data-cpw-back]').disabled = (n === 1);
dlg.querySelector('[data-cpw-next]').hidden = (n === STEP_COUNT);
dlg.querySelector('[data-cpw-save]').hidden = (n !== STEP_COUNT);
if (n === 4) buildScriptEditors();
if (n === 5) buildSummary();
}

function openDialog() {
// Reset state.
form.reset();
setStep(1);
updateURNPreview();
if (typeof dlg.showModal === 'function') dlg.showModal();
else dlg.setAttribute('open', 'open');
document.body.classList.add('cg-dialog-locked');
}
function closeDialog() {
if (typeof dlg.close === 'function') dlg.close();
else dlg.removeAttribute('open');
document.body.classList.remove('cg-dialog-locked');
}

// --- URN live preview (step 1) ----------------------------------
function updateURNPreview() {
const slug = (form.querySelector('input[name="id"]').value || '').trim();
const out = document.getElementById('cpw-id-preview');
if (out) out.textContent = slug ? ('tenant.' + TENANT_ID + '.' + slug) : ('tenant.' + TENANT_ID + '.…');
}

// --- Script editors built from the phase checkboxes (step 4) ----
function buildScriptEditors() {
const tabs = document.getElementById('cpw-script-tabs');
const eds  = document.getElementById('cpw-script-editors');
if (!tabs || !eds) return;
const enabled = Array.from(form.querySelectorAll('input[name="phases"]:checked'))
.map(c => ({ phase: c.value, label: c.closest('.cpw-phase-card').querySelector('.cpw-phase-label').firstChild.textContent.trim(),
            runtime: c.closest('.cpw-phase-card').querySelector('.cpw-phase-runtime').textContent.trim() }));
// Preserve any text the user already typed by reading from existing textareas.
const prev = {};
eds.querySelectorAll('textarea[data-cpw-script]').forEach(ta => prev[ta.dataset.cpwScript] = ta.value);
tabs.innerHTML = '';
eds.innerHTML  = '';
enabled.forEach((p, idx) => {
const tabBtn = document.createElement('button');
tabBtn.type = 'button';
tabBtn.className = 'cpw-script-tab' + (idx === 0 ? ' is-active' : '');
tabBtn.dataset.cpwScriptTab = p.phase;
tabBtn.innerHTML = '<span>' + p.label + '</span><span class="cpw-phase-runtime">' + p.runtime + '</span>';
tabs.appendChild(tabBtn);
const ta = document.createElement('textarea');
ta.className = 'input font-mono cpw-script-area';
ta.dataset.cpwScript = p.phase;
ta.placeholder = '#!/usr/bin/env bash\nset -euo pipefail\n# ' + p.label + ' (' + p.runtime + ')\n';
ta.hidden = (idx !== 0);
ta.value = prev[p.phase] || '';
eds.appendChild(ta);
});
tabs.addEventListener('click', e => {
const btn = e.target.closest('[data-cpw-script-tab]');
if (!btn) return;
tabs.querySelectorAll('.cpw-script-tab').forEach(b => b.classList.toggle('is-active', b === btn));
eds.querySelectorAll('textarea[data-cpw-script]').forEach(ta => {
ta.hidden = (ta.dataset.cpwScript !== btn.dataset.cpwScriptTab);
});
}, { once: false });
}

// --- Manifest summary preview (step 5) --------------------------
function buildSummary() {
const fd = new FormData(form);
const phases = fd.getAll('phases');
const slug = (fd.get('id') || '').toString().trim();
const fullURN = slug ? ('tenant.' + TENANT_ID + '.' + slug) : '(unset)';
const lines = [];
lines.push('# tenant.' + TENANT_ID + ' — Custom Policy + Module bundle');
lines.push('catalog_policy:');
lines.push('  id:   ' + fullURN);
lines.push('  name: ' + (fd.get('name') || ''));
lines.push('  scope: ' + (fd.get('scope') || ''));
lines.push('  category: ' + (fd.get('category') || ''));
lines.push('  description: |');
(fd.get('description') || '').toString().split('\n').forEach(l => lines.push('    ' + l));
lines.push('module:');
lines.push('  id: tenant.' + TENANT_ID + '.' + slug);
lines.push('  version: 0.1.0');
lines.push('  satisfies: [' + fullURN + ']');
lines.push('  scope: ' + (fd.get('scope') || ''));
lines.push('  capabilities:');
lines.push('    user: ' + (fd.get('user') || 'root'));
lines.push('    filesystem:');
lines.push('      write: ' + JSON.stringify(splitLines(fd.get('fs_write'))));
lines.push('      read:  ' + JSON.stringify(splitLines(fd.get('fs_read'))));
lines.push('    network:');
lines.push('      egress: ' + JSON.stringify(splitLines(fd.get('net_egress'))));
lines.push('  lifecycle:');
phases.forEach(p => lines.push('    ' + p + ': ' + p + (p === 'validate' || p === 'report' ? '.wasm' : '.sh')));
lines.push('  signing:');
lines.push('    algo: ed25519');
lines.push('    signed_by: tenant:' + TENANT_ID + ':key:1');

const host = document.getElementById('cpw-summary');
host.innerHTML = '<pre class="cpw-summary-pre font-mono">' + escapeHTML(lines.join('\n')) + '</pre>';
}
function splitLines(s) {
return String(s || '').split('\n').map(l => l.trim()).filter(Boolean);
}
function escapeHTML(s) {
return String(s).replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
}

// --- Triggers ---------------------------------------------------
document.addEventListener('click', e => {
if (e.target.closest('[data-cpw-open]')) { e.preventDefault(); openDialog(); return; }
if (e.target.closest('[data-cpw-close]')) { e.preventDefault(); closeDialog(); return; }
if (e.target.closest('[data-cpw-back]'))  { e.preventDefault(); setStep(Number(dlg.dataset.step) - 1); return; }
if (e.target.closest('[data-cpw-next]'))  { e.preventDefault(); setStep(Number(dlg.dataset.step) + 1); return; }
if (e.target.closest('[data-cpw-save]'))  {
e.preventDefault();
const detail = collect();
document.dispatchEvent(new CustomEvent('cpw:save', { detail }));
closeDialog();
}
});

// Step 4 needs to rebuild when phase checkboxes change between visits.
form.addEventListener('change', e => {
if (e.target.matches('input[name="phases"]')) buildScriptEditors();
});
const idEl = form.querySelector('input[name="id"]');
if (idEl) idEl.addEventListener('input', updateURNPreview);

function collect() {
const fd = new FormData(form);
const scripts = {};
document.querySelectorAll('textarea[data-cpw-script]').forEach(ta => {
scripts[ta.dataset.cpwScript] = ta.value;
});
return {
tenant:      TENANT_ID,
policy: {
id:          'tenant.' + TENANT_ID + '.' + (fd.get('id') || ''),
name:        fd.get('name') || '',
description: fd.get('description') || '',
scope:       fd.get('scope') || '',
category:    String(fd.get('category') || '').split('/').map(s => s.trim()).filter(Boolean),
},
module: {
id:        'tenant.' + TENANT_ID + '.' + (fd.get('id') || ''),
version:   '0.1.0',
phases:    fd.getAll('phases'),
sandbox: {
fs_write: splitLines(fd.get('fs_write')),
fs_read:  splitLines(fd.get('fs_read')),
egress:   splitLines(fd.get('net_egress')),
user:     fd.get('user') || 'root',
},
scripts: scripts,
},
};
}

dlg.addEventListener('cancel', e => { e.preventDefault(); closeDialog(); });
}
})();
</script>`

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

// pddPayload — the per-policy data shape PolicyDetailDialog hydrates
// from. JSON tags are short to keep the embedded data island compact;
// the JS hydrator reads them by these names.
type pddPayload struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Cat      string      `json:"cat"`
	Scope    string      `json:"scope"`
	Origin   string      `json:"origin"` // "bundled" | "custom"
	TenantID string      `json:"tenant,omitempty"`
	WinName  string      `json:"winName"`
	WinPath  string      `json:"winPath"`
	Linux    string      `json:"linux"`
	Desc     string      `json:"desc"`
	Modules  []pddModule `json:"modules"`
}

type pddModule struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Origin    string     `json:"origin"`
	Scope     string     `json:"scope"`
	Version   string     `json:"version"`
	Status    string     `json:"status"`
	Published string     `json:"published"`
	Signer    string     `json:"signer"`
	KeyID     string     `json:"keyId"`
	Phases    []pddPhase `json:"phases"`
	Deps      []pddDep   `json:"deps"`
	Conflicts []string   `json:"conflicts"`
	Sandbox   pddSandbox `json:"sandbox"`
	Satisfies []string   `json:"satisfies"`
}

type pddPhase struct {
	Phase    string `json:"phase"`
	Runtime  string `json:"runtime"`
	Filename string `json:"filename"`
	Preview  string `json:"preview"`
}

type pddDep struct {
	ModuleID   string `json:"id"`
	Constraint string `json:"constraint"`
}

type pddSandbox struct {
	FsRead    []string `json:"fsRead"`
	FsWrite   []string `json:"fsWrite"`
	NetEgress []string `json:"net"`
	User      string   `json:"user"`
}

// truncatePreview — keeps the embedded JSON small while still letting
// the dialog show enough script to recognise its shape. Real "view full
// source" lands with editors/PolicyModuleEditor in a later increment.
func truncatePreview(s string) string {
	const max = 600
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n…(truncated; open the module in Policy → Modules for full source)"
}

// policyDetailJSON — builds the data island. One pass over the catalog +
// one inner pass per policy over the module catalog. O(P*M) for small
// P, M (a few hundred); fine for v1. Phase 1 will paginate the module
// catalog and lazy-load deep details on demand.
func policyDetailJSON() string {
	mods := policymodules.AllModules()
	out := make(map[string]pddPayload, len(policies.Catalog()))

	for _, p := range policies.Catalog() {
		pp := pddPayload{
			ID:       p.ID,
			Name:     p.Name,
			Cat:      strings.Join(p.Category, " / "),
			Scope:    string(p.Scope),
			Origin:   policyOrigin(p),
			TenantID: p.TenantID,
			WinName:  p.WinGPName,
			WinPath:  p.WinGPPath,
			Linux:    p.LinuxImpl,
			Desc:     p.Description,
		}
		for _, m := range mods {
			if !m.SatisfiesURN(p.ID) {
				continue
			}
			ver := m.LatestVersion()
			pm := pddModule{
				ID:        m.ID,
				Title:     m.Title,
				Origin:    m.Origin,
				Scope:     m.Scope,
				Satisfies: m.Satisfies,
			}
			if ver != nil {
				pm.Version = ver.Version
				pm.Status = ver.Status
				pm.Published = ver.PublishedAt
				pm.Signer = ver.Signature.Signer
				pm.KeyID = ver.Signature.KeyID
				pm.Conflicts = ver.Conflicts
				pm.Sandbox = pddSandbox{
					FsRead:    ver.Sandbox.FsRead,
					FsWrite:   ver.Sandbox.FsWrite,
					NetEgress: ver.Sandbox.NetEgress,
					User:      ver.Sandbox.User,
				}
				for _, d := range ver.Dependencies {
					pm.Deps = append(pm.Deps, pddDep{ModuleID: d.ModuleID, Constraint: d.VersionConstraint})
				}
				for _, s := range ver.Scripts {
					pm.Phases = append(pm.Phases, pddPhase{
						Phase:    string(s.Phase),
						Runtime:  string(s.Phase.Runtime()),
						Filename: s.Filename,
						Preview:  truncatePreview(s.Source),
					})
				}
			}
			pp.Modules = append(pp.Modules, pm)
		}
		out[p.ID] = pp
	}

	b, err := json.Marshal(out)
	if err != nil {
		// Marshal of plain-data structs cannot fail in practice; if it
		// does, render an empty object so the page still renders and the
		// dialog gracefully shows "no data".
		return "{}"
	}
	return string(b)
}

// policyDetailDataIslandHTML — wraps the JSON payload in a script tag.
// Emitted via @templ.Raw because templ does not interpolate function
// calls inside <script> bodies (they render as literal text). The
// payload's "</" sequences are escaped to "<\/" so the JSON parser
// still accepts it but the HTML parser cannot terminate the script tag
// early — the standard JSON-in-HTML-script defense.
func policyDetailDataIslandHTML() string {
	payload := strings.ReplaceAll(policyDetailJSON(), "</", "<\\/")
	return `<script id="pdd-data" type="application/json">` + payload + `</script>`
}

// policyDetailScript — open / close / hydrate wiring for the PDD. Same
// DOMContentLoaded pattern as the other dialogs. Reads the data island
// once at init, then fields each row click in O(1).
//
// Visual contract — every section in the dialog binds to a stable id:
//
//	#pdd-name, #pdd-id, #pdd-urn, #pdd-category, #pdd-winname,
//	#pdd-winpath, #pdd-linux, #pdd-desc, #pdd-scope-host,
//	#pdd-origin-host, #pdd-modules
//
// The Edit button (#pdd-edit) is hidden by default and revealed only
// for custom policies; clicking it dispatches a `cpw:open` request that
// the wizard's existing trigger picks up (zero-coupling — same pattern
// the target picker uses).
const policyDetailScript = `<script>
(function () {
'use strict';
function ready(fn) {
if (document.readyState === 'loading') {
document.addEventListener('DOMContentLoaded', fn, { once: true });
} else { fn(); }
}
ready(init);

function init() {
const dlg = document.getElementById('pdd-dialog');
if (!dlg) return;
let DATA = {};
try {
const island = document.getElementById('pdd-data');
if (island) DATA = JSON.parse(island.textContent || '{}');
} catch (e) { /* leave empty; UI shows graceful empty */ }

function open(policyID) {
const p = DATA[policyID];
if (!p) return;
setText('pdd-title', p.name);
setText('pdd-id',    p.id);
setText('pdd-name',  p.name);
setText('pdd-urn',   p.id);
setText('pdd-category', p.cat || '—');
setText('pdd-winname',  p.winName || '— (no Windows GP equivalent)');
setText('pdd-winpath',  p.winPath || '');
setText('pdd-linux',    p.linux || '—');
setText('pdd-desc',     p.desc || '');

document.getElementById('pdd-scope-host').innerHTML  = scopeChipHTML(p.scope);
document.getElementById('pdd-origin-host').innerHTML = originChipHTML(p.origin, p.tenant);

const mods = document.getElementById('pdd-modules');
mods.innerHTML = (p.modules && p.modules.length) ? p.modules.map(renderModule).join('') :
'<div class="pdd-empty">No candidate modules ship for this policy yet. Author one via the wizard or import from a community registry (Phase 4).</div>';

const editBtn = document.getElementById('pdd-edit');
editBtn.hidden = (p.origin !== 'custom');

dlg.dataset.policyId = p.id;
if (typeof dlg.showModal === 'function') dlg.showModal();
else dlg.setAttribute('open', 'open');
document.body.classList.add('cg-dialog-locked');
}
function close() {
if (typeof dlg.close === 'function') dlg.close();
else dlg.removeAttribute('open');
document.body.classList.remove('cg-dialog-locked');
}

function setText(id, v) { const el = document.getElementById(id); if (el) el.textContent = v == null ? '' : String(v); }

function scopeChipHTML(s) {
const map = {
computer: ['scope-computer', 'Computer'],
user:     ['scope-user',     'User'],
both:     ['scope-both',     'Both'],
};
const m = map[s] || ['', s || '—'];
return '<span class="scope-chip ' + m[0] + '">' + escapeHTML(m[1]) + '</span>';
}
function originChipHTML(o, tenant) {
if (o === 'custom') {
return '<span class="policy-custom-chip">Custom' + (tenant ? ' · ' + escapeHTML(tenant) : '') + '</span>';
}
return '<span class="pdd-origin-bundled">Bundled</span>';
}

const PHASE_LABELS = { apply: 'Apply', disable: 'Disable', uninstall: 'Uninstall', validate: 'Validate', report: 'Report' };
function renderModule(m) {
const phaseTabs = (m.phases || []).map((ph, i) =>
'<button type="button" class="pdd-phase-tab' + (i === 0 ? ' is-active' : '') + '" data-pdd-phase="' + escAttr(m.id + ':' + ph.phase) + '">' +
'<span class="pm-phase-pill pm-phase-' + escAttr(ph.phase) + '">' + (PHASE_LABELS[ph.phase] || ph.phase).slice(0, 1) + '</span>' +
'<span>' + escapeHTML(PHASE_LABELS[ph.phase] || ph.phase) + '</span>' +
'<span class="cpw-phase-runtime">' + escapeHTML(ph.runtime) + '</span>' +
'</button>'
).join('');
const phasePanes = (m.phases || []).map((ph, i) =>
'<div class="pdd-phase-pane' + (i === 0 ? ' is-active' : '') + '" data-pdd-phase-pane="' + escAttr(m.id + ':' + ph.phase) + '">' +
'<div class="pdd-phase-meta"><span class="font-mono">' + escapeHTML(ph.filename) + '</span></div>' +
'<pre class="pdd-source font-mono">' + escapeHTML(ph.preview || '(empty)') + '</pre>' +
'</div>'
).join('');
const deps = (m.deps || []).map(d =>
'<li><span class="font-mono">' + escapeHTML(d.id) + '</span> <span class="pdd-muted font-mono">' + escapeHTML(d.constraint || '*') + '</span></li>'
).join('');
const conflicts = (m.conflicts || []).map(c => '<li class="pdd-conflict-item font-mono">' + escapeHTML(c) + '</li>').join('');
const sb = m.sandbox || {};
const sandboxRow = (label, arr) => arr && arr.length
? '<div class="pdd-sb-row"><span class="pdd-sb-label">' + label + '</span><span class="pdd-sb-vals">' + arr.map(v => '<code>' + escapeHTML(v) + '</code>').join(' ') + '</span></div>'
: '<div class="pdd-sb-row"><span class="pdd-sb-label">' + label + '</span><span class="pdd-muted">(none)</span></div>';

return '<article class="pdd-module">' +
'<header class="pdd-module-head">' +
'<div>' +
'<div class="pdd-module-title">' +
escapeHTML(m.title) +
' <span class="pm-origin-chip pm-origin-' + escAttr(m.origin) + '">' + escapeHTML(m.origin) + '</span>' +
' <span class="pm-status pm-status-' + escAttr(m.status || 'unknown') + '">' + escapeHTML(m.status || 'unknown') + '</span>' +
'</div>' +
'<div class="pdd-module-id font-mono">' + escapeHTML(m.id) + ' · v' + escapeHTML(m.version || '?') + '</div>' +
'</div>' +
'<div class="pdd-module-signer">' +
'<div>signed by <strong>' + escapeHTML(m.signer || '—') + '</strong></div>' +
'<div class="pdd-muted font-mono">' + escapeHTML(m.keyId || '') + '</div>' +
'</div>' +
'</header>' +
'<div class="pdd-module-grid">' +
'<section>' +
'<h5>Lifecycle</h5>' +
'<div class="pdd-phase-tabs">' + phaseTabs + '</div>' +
'<div class="pdd-phase-panes">' + phasePanes + '</div>' +
'</section>' +
'<aside>' +
'<h5>Sandbox profile <span class="pdd-muted">(INV-M8)</span></h5>' +
'<div class="pdd-sandbox">' +
sandboxRow('User', sb.user ? [sb.user] : []) +
sandboxRow('FS write', sb.fsWrite || []) +
sandboxRow('FS read',  sb.fsRead  || []) +
sandboxRow('Egress',   sb.net     || []) +
'</div>' +
'<h5>Dependencies <span class="pdd-muted">(INV-M2)</span></h5>' +
(deps ? '<ul class="pdd-deps">' + deps + '</ul>' : '<div class="pdd-muted">(none)</div>') +
(conflicts ? '<h5>Conflicts</h5><ul class="pdd-deps">' + conflicts + '</ul>' : '') +
'</aside>' +
'</div>' +
'</article>';
}

function escapeHTML(s) {
return String(s == null ? '' : s).replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]));
}
function escAttr(s) { return escapeHTML(s); }

// --- Triggers --------------------------------------------------
// Row click on the policy table → open. Use event delegation so
// future re-renders (e.g. catalog refresh) don't lose the binding.
document.addEventListener('click', e => {
const btn = e.target.closest('[data-pdd-close]');
if (btn) { e.preventDefault(); close(); return; }
const row = e.target.closest('.policy-table tbody tr[data-policy-id]');
if (row) { e.preventDefault(); open(row.dataset.policyId); return; }
// Phase-tab click inside an open dialog.
const tab = e.target.closest('[data-pdd-phase]');
if (tab) {
const key = tab.dataset.pddPhase;
const article = tab.closest('.pdd-module');
article.querySelectorAll('.pdd-phase-tab').forEach(t => t.classList.toggle('is-active', t === tab));
article.querySelectorAll('.pdd-phase-pane').forEach(p => p.classList.toggle('is-active', p.dataset.pddPhasePane === key));
return;
}
// Edit button → trigger the wizard. v1 just opens fresh; Phase 1
// will pre-fill from DATA[policyId].
const edit = e.target.closest('#pdd-edit');
if (edit) {
e.preventDefault();
close();
const opener = document.querySelector('[data-cpw-open]');
if (opener) opener.click();
return;
}
});

dlg.addEventListener('cancel', e => { e.preventDefault(); close(); });
}
})();
</script>`

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
return Array.isArray(arr) ? arr : defaultKeys.slice();
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
const set = visibleSet();
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
