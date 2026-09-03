/* Pluris condition-builder dialog — vanilla JS, no deps.
 *
 * Contract (kept in lockstep with the doc comment on
 * `templates.ConditionBuilderDialog` in web/templates/condition_builder.templ
 * — that comment is the source of truth if this ever drifts):
 *
 *   - Open:   any element with `data-condition-builder-open` opens the
 *             dialog (`#condition-builder`). An optional `data-cb-prefill`
 *             JSON string on the SAME element pre-fills an existing
 *             condition for edit flows:
 *               {"kind":"param"|"command"|"script","paramPath":"...",
 *                "operator":"...","values":["..."],"scriptSource":"...",
 *                "scriptRef":"..."}
 *   - Save:   dispatches `condition:save` (CustomEvent) on `document`:
 *               detail = { kind, paramPath, operator, values, scriptSource,
 *                          scriptRef }
 *             `values` is always a string array. The embedding page owns
 *             persistence — this file never calls a save API.
 *   - Kinds:  every kind is subject -> operator -> expected value
 *             (INV-TEST). `data-cb-allowed-kinds` on the dialog hides
 *             tabs a page doesn't allow. The script tab's library picker
 *             is fed by GET /api/scripts (empty feed until the Scripts
 *             library ships).
 *   - Cancel/Esc/×: closes without dispatching.
 *   - Data:   GET /api/params is fetched fresh on every open (same-origin
 *             credentials) and rendered as an entity → section → param
 *             tree. Fetch failure shows an inline error banner.
 *   - Operators offered for a selected param = that param's own
 *     `operators[].key` list INTERSECTED with the dialog's
 *     `data-supported-operators` (dependencygroups.AllOperators(),
 *     server-rendered) — never a hardcoded operator list here.
 *
 * Defensive by construction: every DOM lookup below tolerates the dialog
 * (or any of its children) being absent — this script is safe to load on
 * pages that don't mount `@ConditionBuilderDialog()` at all.
 */
(function () {
	'use strict';

	function ready(fn) {
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', fn, { once: true });
		} else {
			fn();
		}
	}
	ready(init);

	// Operators whose value input is a plain number.
	var NUMERIC_OPS = { gt: true, gte: true, lt: true, lte: true };
	// Operators that naturally take multiple values.
	var MULTI_OPS = { in: true, not_in: true };

	function init() {
		var dlg = document.getElementById('condition-builder');
		if (!dlg) return;

		var titleEl = document.getElementById('cb-dialog-title');
		var closeBtns = dlg.querySelectorAll('[data-cb-close]');
		var saveBtn = dlg.querySelector('[data-cb-save]');
		var tabBtns = dlg.querySelectorAll('[data-cb-tab]');
		var panels = dlg.querySelectorAll('[data-cb-panel]');

		var treeEl = document.getElementById('cb-param-tree');
		var loadingEl = document.getElementById('cb-param-loading');
		var errorEl = document.getElementById('cb-param-error');
		var searchEl = document.getElementById('cb-param-search');
		var selectedEl = document.getElementById('cb-param-selected');
		var operatorSel = document.getElementById('cb-operator');
		var valueContainer = document.getElementById('cb-value-container');

		var scriptSourceEl = document.getElementById('cb-script-source');
		var scriptRefEl = document.getElementById('cb-script-ref');
		var scriptRefEmptyEl = document.getElementById('cb-script-ref-empty');
		var scriptOperatorEl = document.getElementById('cb-script-operator');
		var scriptValueEl = document.getElementById('cb-script-value');
		var commandSourceEl = document.getElementById('cb-command-source');
		var commandOperatorEl = document.getElementById('cb-command-operator');
		var commandValueEl = document.getElementById('cb-command-value');

		if (!saveBtn || !treeEl || !operatorSel || !valueContainer) return;

		var supportedOperators = parseSet(dlg.dataset.supportedOperators);
		var allowedKinds = parseSet(dlg.dataset.cbAllowedKinds || 'param,command,script');

		tabBtns.forEach(function (b) {
			if (!allowedKinds[b.dataset.cbTab]) b.hidden = true;
		});

		// --- State -----------------------------------------------------
		var activeKind = 'param';       // 'param' | 'command' | 'script'
		var paramsByPath = {};          // path -> API param object
		var selectedPath = null;
		var pendingPrefill = null;      // prefill object awaiting tree load
		var fetchToken = 0;             // guards against a stale in-flight fetch

		function parseSet(s) {
			var out = {};
			(s || '').split(',').forEach(function (k) {
				if (k) out[k] = true;
			});
			return out;
		}

		function firstAllowedKind() {
			if (allowedKinds.param) return 'param';
			if (allowedKinds.command) return 'command';
			return 'script';
		}

		function setKind(kind) {
			if (kind !== 'script' && kind !== 'command') kind = 'param';
			if (!allowedKinds[kind]) kind = firstAllowedKind();
			activeKind = kind;
			tabBtns.forEach(function (b) {
				var active = b.dataset.cbTab === activeKind;
				b.classList.toggle('is-active', active);
				b.setAttribute('aria-selected', active ? 'true' : 'false');
			});
			panels.forEach(function (p) {
				p.hidden = p.dataset.cbPanel !== activeKind;
			});
			updateSaveEnabled();
		}

		// --- Library script feed (script tab) ---------------------------
		function loadScriptRefs() {
			if (!scriptRefEl) return;
			fetch('/api/scripts', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
				.then(function (r) { return r.ok ? r.json() : []; })
				.then(function (list) {
					list = Array.isArray(list) ? list : [];
					var current = scriptRefEl.value;
					scriptRefEl.innerHTML = '';
					var none = document.createElement('option');
					none.value = '';
					none.textContent = '— inline source below —';
					scriptRefEl.appendChild(none);
					list.forEach(function (s) {
						var opt = document.createElement('option');
						opt.value = s.id;
						opt.textContent = s.title || s.filename || s.id;
						scriptRefEl.appendChild(opt);
					});
					scriptRefEl.value = current;
					if (scriptRefEmptyEl) scriptRefEmptyEl.hidden = list.length > 0;
				})
				.catch(function () {
					if (scriptRefEmptyEl) scriptRefEmptyEl.hidden = false;
				});
		}

		// --- Fetch + render the param tree ------------------------------
		function loadParams() {
			var token = ++fetchToken;
			loadingEl && (loadingEl.hidden = false);
			setError('');
			treeEl.querySelectorAll('.cb-param-row, .cb-param-group').forEach(function (n) {
				n.remove();
			});
			paramsByPath = {};

			fetch('/api/params', { credentials: 'same-origin', headers: { Accept: 'application/json' } })
				.then(function (r) {
					if (token !== fetchToken) return null; // dialog re-opened/closed since
					if (!r.ok) {
						throw new Error('server responded ' + r.status);
					}
					return r.json();
				})
				.then(function (data) {
					if (data === null || token !== fetchToken) return;
					renderTree(data);
					if (pendingPrefill) {
						applyPrefillAfterLoad(pendingPrefill);
						pendingPrefill = null;
					}
				})
				.catch(function (err) {
					if (token !== fetchToken) return;
					setError('Could not load parameters: ' + (err && err.message ? err.message : 'unknown error'));
				})
				.then(function () {
					if (token === fetchToken && loadingEl) loadingEl.hidden = true;
				});
		}

		function setError(msg) {
			if (!errorEl) return;
			errorEl.textContent = msg || '';
			errorEl.hidden = !msg;
		}

		function renderTree(data) {
			var sources = (data && data.sources) || [];
			var frag = document.createDocumentFragment();
			sources.forEach(function (entity) {
				var sections = entity.sections || [];
				if (sections.length === 0) return;
				var entityGroup = document.createElement('div');
				entityGroup.className = 'cb-param-group cb-param-group-entity';
				var entityHead = document.createElement('div');
				entityHead.className = 'cb-param-group-label';
				entityHead.textContent = entity.label || entity.entity || '';
				entityGroup.appendChild(entityHead);

				sections.forEach(function (section) {
					var params = section.params || [];
					if (params.length === 0) return;
					var sectionGroup = document.createElement('div');
					sectionGroup.className = 'cb-param-group cb-param-group-section';
					var sectionHead = document.createElement('div');
					sectionHead.className = 'cb-param-group-label cb-param-group-label-section';
					sectionHead.textContent = section.label || section.key || '';
					sectionGroup.appendChild(sectionHead);

					params.forEach(function (p) {
						paramsByPath[p.path] = p;
						var row = document.createElement('div');
						row.className = 'cb-param-row';
						row.setAttribute('role', 'treeitem');
						row.tabIndex = 0;
						row.dataset.cbPath = p.path;
						row.dataset.cbSearch = (
							(p.path || '') + ' ' + (p.label || '') + ' ' + (p.key || '')
						).toLowerCase();

						var label = document.createElement('div');
						label.className = 'cb-param-row-label';
						label.textContent = p.label || p.key || p.path;
						var path = document.createElement('div');
						path.className = 'cb-param-row-path font-mono';
						path.textContent = p.path || '';

						row.appendChild(label);
						row.appendChild(path);
						sectionGroup.appendChild(row);
					});
					entityGroup.appendChild(sectionGroup);
				});
				frag.appendChild(entityGroup);
			});
			treeEl.appendChild(frag);
			applySearchFilter();
		}

		function applySearchFilter() {
			var q = ((searchEl && searchEl.value) || '').trim().toLowerCase();
			treeEl.querySelectorAll('.cb-param-row').forEach(function (row) {
				var visible = !q || (row.dataset.cbSearch || '').indexOf(q) !== -1;
				row.classList.toggle('tp-hidden', !visible);
			});
			// Hide section/entity groups that end up with no visible rows.
			treeEl.querySelectorAll('.cb-param-group-section').forEach(function (g) {
				var anyVisible = !!g.querySelector('.cb-param-row:not(.tp-hidden)');
				g.classList.toggle('tp-hidden', !anyVisible);
			});
			treeEl.querySelectorAll('.cb-param-group-entity').forEach(function (g) {
				var anyVisible = !!g.querySelector('.cb-param-group-section:not(.tp-hidden)');
				g.classList.toggle('tp-hidden', !anyVisible);
			});
		}

		// --- Selecting a param ------------------------------------------
		function selectParam(path) {
			var p = paramsByPath[path];
			if (!p) return;
			selectedPath = path;

			treeEl.querySelectorAll('.cb-param-row').forEach(function (row) {
				row.classList.toggle('is-selected', row.dataset.cbPath === path);
			});

			if (selectedEl) {
				selectedEl.innerHTML = '';
				var labelEl = document.createElement('div');
				labelEl.className = 'cb-param-selected-label';
				labelEl.textContent = p.label || p.key;
				var metaEl = document.createElement('div');
				metaEl.className = 'cb-param-selected-meta';
				metaEl.textContent = p.type + (p.unit ? (' · ' + p.unit) : '') + ' · ' + p.path;
				selectedEl.appendChild(labelEl);
				selectedEl.appendChild(metaEl);
			}

			populateOperators(p);
		}

		function populateOperators(p, presetOperator) {
			var ops = (p.operators || []).filter(function (o) {
				return !!supportedOperators[o.key];
			});
			operatorSel.innerHTML = '';
			operatorSel.disabled = ops.length === 0;
			if (ops.length === 0) {
				var opt = document.createElement('option');
				opt.value = '';
				opt.textContent = '— no supported operators for this parameter —';
				operatorSel.appendChild(opt);
				renderValueInput(null, p);
				updateSaveEnabled();
				return;
			}
			var placeholder = document.createElement('option');
			placeholder.value = '';
			placeholder.textContent = '— select an operator —';
			operatorSel.appendChild(placeholder);
			ops.forEach(function (o) {
				var opt = document.createElement('option');
				opt.value = o.key;
				opt.textContent = o.label || o.key;
				opt.dataset.needsValue = o.needsValue ? '1' : '';
				operatorSel.appendChild(opt);
			});
			if (presetOperator && ops.some(function (o) { return o.key === presetOperator; })) {
				operatorSel.value = presetOperator;
			} else {
				operatorSel.value = '';
			}
			renderValueInput(currentOperatorDef(p), p);
			updateSaveEnabled();
		}

		function currentOperatorDef(p) {
			var key = operatorSel.value;
			if (!key) return null;
			return (p.operators || []).find(function (o) { return o.key === key; }) || null;
		}

		// --- Value input, type-appropriate -------------------------------
		function renderValueInput(opDef, p, presetValues) {
			valueContainer.innerHTML = '';
			if (!opDef || !opDef.needsValue) {
				var none = document.createElement('div');
				none.className = 'cb-value-none';
				none.textContent = opDef ? 'No value needed for this operator.' : '';
				valueContainer.appendChild(none);
				return;
			}
			var op = opDef.key;
			var preset = presetValues || [];

			if (p.type === 'enum' && MULTI_OPS[op]) {
				var multi = document.createElement('select');
				multi.id = 'cb-value-input';
				multi.className = 'input cb-value-multi';
				multi.multiple = true;
				(p.enumValues || []).forEach(function (v) {
					var opt = document.createElement('option');
					opt.value = v;
					opt.textContent = v;
					opt.selected = preset.indexOf(v) !== -1;
					multi.appendChild(opt);
				});
				valueContainer.appendChild(multi);
				multi.addEventListener('change', updateSaveEnabled);
				return;
			}

			if (NUMERIC_OPS[op]) {
				var num = document.createElement('input');
				num.type = 'number';
				num.id = 'cb-value-input';
				num.className = 'input';
				num.value = preset[0] || '';
				valueContainer.appendChild(num);
				num.addEventListener('input', updateSaveEnabled);
				return;
			}

			// Generic single-operand text input (equals/not_equals/contains/
			// not_contains/starts_with/ends_with/matches, and in/not_in on a
			// non-enum param — comma-separated).
			var text = document.createElement('input');
			text.type = 'text';
			text.id = 'cb-value-input';
			text.className = 'input';
			if (op === 'matches') {
				text.placeholder = 'Go regexp';
			} else if (MULTI_OPS[op]) {
				text.placeholder = 'Comma-separated values';
			}
			text.value = preset.join(MULTI_OPS[op] ? ', ' : '');
			valueContainer.appendChild(text);
			text.addEventListener('input', updateSaveEnabled);
		}

		function currentValues() {
			var p = selectedPath && paramsByPath[selectedPath];
			var opDef = p ? currentOperatorDef(p) : null;
			if (!opDef || !opDef.needsValue) return [];
			var op = opDef.key;
			if (p.type === 'enum' && MULTI_OPS[op]) {
				var multi = document.getElementById('cb-value-input');
				if (!multi) return [];
				return Array.prototype.slice.call(multi.selectedOptions || []).map(function (o) { return o.value; });
			}
			var input = document.getElementById('cb-value-input');
			if (!input) return [];
			var raw = (input.value || '').trim();
			if (!raw) return [];
			if (MULTI_OPS[op]) {
				return raw.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
			}
			return [raw];
		}

		// --- Validation ---------------------------------------------------
		function updateSaveEnabled() {
			saveBtn.disabled = !isValid();
		}

		function stdoutOperatorNeedsValue(op) {
			return op !== 'exists';
		}

		function isValid() {
			if (activeKind === 'command') {
				if (!(commandSourceEl && commandSourceEl.value.trim())) return false;
				var cop = commandOperatorEl && commandOperatorEl.value;
				if (!cop) return false;
				return !stdoutOperatorNeedsValue(cop) || !!(commandValueEl && commandValueEl.value.trim());
			}
			if (activeKind === 'script') {
				var hasRef = !!(scriptRefEl && scriptRefEl.value);
				var hasSrc = !!(scriptSourceEl && scriptSourceEl.value.trim());
				if (hasRef === hasSrc) return false;
				var sop = scriptOperatorEl && scriptOperatorEl.value;
				if (!sop) return false;
				return !stdoutOperatorNeedsValue(sop) || !!(scriptValueEl && scriptValueEl.value.trim());
			}
			var p = selectedPath && paramsByPath[selectedPath];
			if (!p) return false;
			var opDef = currentOperatorDef(p);
			if (!opDef) return false;
			if (!opDef.needsValue) return true;
			return currentValues().length > 0;
		}

		// --- Prefill --------------------------------------------------------
		function applyPrefill(prefill) {
			if (!prefill || typeof prefill !== 'object') return;
			setKind(prefill.kind);
			if (prefill.kind === 'command') {
				if (commandSourceEl) commandSourceEl.value = prefill.scriptSource || '';
				if (commandOperatorEl && prefill.operator) commandOperatorEl.value = prefill.operator;
				if (commandValueEl) commandValueEl.value = (prefill.values && prefill.values[0]) || '';
				updateSaveEnabled();
				return;
			}
			if (prefill.kind === 'script') {
				if (scriptSourceEl) scriptSourceEl.value = prefill.scriptSource || '';
				if (scriptRefEl) scriptRefEl.value = prefill.scriptRef || '';
				if (scriptOperatorEl && prefill.operator) scriptOperatorEl.value = prefill.operator;
				if (scriptValueEl) scriptValueEl.value = (prefill.values && prefill.values[0]) || '';
				updateSaveEnabled();
				return;
			}
			// Param kind: tree may not be loaded yet — stash and apply once it is.
			if (Object.keys(paramsByPath).length === 0) {
				pendingPrefill = prefill;
			} else {
				applyPrefillAfterLoad(prefill);
			}
		}

		function applyPrefillAfterLoad(prefill) {
			var path = prefill.paramPath;
			if (!path || !paramsByPath[path]) return;
			selectParam(path);
			populateOperators(paramsByPath[path], prefill.operator);
			renderValueInput(currentOperatorDef(paramsByPath[path]), paramsByPath[path], prefill.values || []);
			updateSaveEnabled();
		}

		// --- Open / close ---------------------------------------------------
		function resetForm() {
			selectedPath = null;
			pendingPrefill = null;
			if (searchEl) searchEl.value = '';
			if (selectedEl) selectedEl.innerHTML = '<div class="cb-param-selected-empty">Select a parameter on the left.</div>';
			operatorSel.innerHTML = '<option value="">— select a parameter first —</option>';
			operatorSel.disabled = true;
			valueContainer.innerHTML = '';
			if (scriptSourceEl) scriptSourceEl.value = '';
			if (scriptRefEl) scriptRefEl.value = '';
			if (scriptOperatorEl) scriptOperatorEl.selectedIndex = 0;
			if (scriptValueEl) scriptValueEl.value = '';
			if (commandSourceEl) commandSourceEl.value = '';
			if (commandOperatorEl) commandOperatorEl.selectedIndex = 0;
			if (commandValueEl) commandValueEl.value = '';
			setError('');
		}

		function openDialog(opener) {
			resetForm();
			setKind(firstAllowedKind());
			if (titleEl) {
				titleEl.textContent = (opener && opener.dataset.cbPrefill) ? 'Edit condition' : 'Add condition';
			}
			loadParams();
			loadScriptRefs();

			var prefill = null;
			if (opener && opener.dataset.cbPrefill) {
				try {
					prefill = JSON.parse(opener.dataset.cbPrefill);
				} catch (e) {
					prefill = null;
				}
			}
			if (prefill) applyPrefill(prefill);

			if (typeof dlg.showModal === 'function') dlg.showModal();
			else dlg.setAttribute('open', 'open');
			document.body.classList.add('cg-dialog-locked');

			// Upgrade the script tab's textarea (data-code-editor="bash") to
			// a CodeMirror editor via the shared PlurisCodeEditor wrapper —
			// AFTER showModal, so CM6 mounts into a visible dialog and
			// measures real layout (mounting while the <dialog> is closed /
			// display:none yields a zero-size editor). Guarded: if the
			// wrapper (or the vendor bundle behind it) isn't loaded, this is
			// a no-op and the plain textarea keeps working — the dialog
			// component stays generic. upgradeTextareas is idempotent
			// (WeakSet-tracked), so calling it on every open is safe.
			if (window.PlurisCodeEditor && typeof window.PlurisCodeEditor.upgradeTextareas === 'function') {
				window.PlurisCodeEditor.upgradeTextareas(dlg);
			}
		}

		function closeDialog() {
			fetchToken++; // invalidate any in-flight fetch
			if (typeof dlg.close === 'function') dlg.close();
			else dlg.removeAttribute('open');
			document.body.classList.remove('cg-dialog-locked');
		}

		function stdoutDetail(kind, sourceEl, refVal, opEl, valEl) {
			var op = (opEl && opEl.value) || '';
			var val = (valEl && valEl.value.trim()) || '';
			return {
				kind: kind,
				paramPath: '',
				operator: op,
				values: (stdoutOperatorNeedsValue(op) && val) ? [val] : [],
				scriptSource: (sourceEl && sourceEl.value) || '',
				scriptRef: refVal || '',
			};
		}

		function save() {
			if (!isValid()) return;
			var detail;
			if (activeKind === 'command') {
				detail = stdoutDetail('command', commandSourceEl, '', commandOperatorEl, commandValueEl);
			} else if (activeKind === 'script') {
				var refVal = (scriptRefEl && scriptRefEl.value) || '';
				detail = stdoutDetail('script', refVal ? null : scriptSourceEl, refVal, scriptOperatorEl, scriptValueEl);
			} else {
				var p = paramsByPath[selectedPath];
				var opDef = currentOperatorDef(p);
				detail = {
					kind: 'param',
					paramPath: selectedPath,
					operator: opDef ? opDef.key : '',
					values: currentValues(),
					scriptSource: '',
					scriptRef: '',
				};
			}
			document.dispatchEvent(new CustomEvent('condition:save', { detail: detail }));
			closeDialog();
		}

		// --- Wiring -----------------------------------------------------
		document.addEventListener('click', function (e) {
			var opener = e.target.closest('[data-condition-builder-open]');
			if (opener) {
				e.preventDefault();
				openDialog(opener);
			}
		});

		closeBtns.forEach(function (b) {
			b.addEventListener('click', function (e) {
				e.preventDefault();
				closeDialog();
			});
		});

		tabBtns.forEach(function (b) {
			b.addEventListener('click', function () {
				setKind(b.dataset.cbTab);
			});
		});

		if (searchEl) searchEl.addEventListener('input', applySearchFilter);

		treeEl.addEventListener('click', function (e) {
			var row = e.target.closest('.cb-param-row');
			if (!row) return;
			selectParam(row.dataset.cbPath);
		});
		treeEl.addEventListener('keydown', function (e) {
			if (e.key !== 'Enter' && e.key !== ' ') return;
			var row = e.target.closest('.cb-param-row');
			if (!row) return;
			e.preventDefault();
			selectParam(row.dataset.cbPath);
		});

		operatorSel.addEventListener('change', function () {
			var p = paramsByPath[selectedPath];
			if (!p) return;
			renderValueInput(currentOperatorDef(p), p);
			updateSaveEnabled();
		});

		if (scriptSourceEl) scriptSourceEl.addEventListener('input', updateSaveEnabled);
		if (scriptRefEl) scriptRefEl.addEventListener('change', updateSaveEnabled);
		if (scriptOperatorEl) scriptOperatorEl.addEventListener('change', updateSaveEnabled);
		if (scriptValueEl) scriptValueEl.addEventListener('input', updateSaveEnabled);
		if (commandSourceEl) commandSourceEl.addEventListener('input', updateSaveEnabled);
		if (commandOperatorEl) commandOperatorEl.addEventListener('change', updateSaveEnabled);
		if (commandValueEl) commandValueEl.addEventListener('input', updateSaveEnabled);

		saveBtn.addEventListener('click', function (e) {
			e.preventDefault();
			save();
		});

		dlg.addEventListener('cancel', function (e) {
			e.preventDefault();
			closeDialog();
		});
	}
})();
