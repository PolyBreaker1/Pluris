/* Policy Module editor (Task 4.3) — page-specific JS for the Scripts tab
 * (permission-filtered parameter tree + phase-tabbed PlurisCodeEditor
 * panes + per-phase Save) and the Parameters tab's structured
 * key/label/type/default/required/enum editor.
 *
 * Loaded AFTER web/static/vendor/codemirror/codemirror-pluris.js and
 * web/static/code-editor.js (see policy_module_editor.templ). Everything
 * here is defensive: a missing root element is a no-op, matching the
 * rest of the console's page-specific scripts (e.g.
 * dependencyGroupConditionScript in dependency_groups.templ).
 */
(function () {
	'use strict';

	document.addEventListener('DOMContentLoaded', function () {
		initScriptsTab();
		initParametersTab();
		wireScriptEditorRefresh();
	});

	// wireScriptEditorRefresh — CP3: the standalone per-script editor
	// (module-script-editor.js, opened via window in a new tab/window
	// from the Scripts tab's row links) posts
	// `window.opener.postMessage('pluris:script-saved', location.origin)`
	// after a successful save. Listening here (rather than in the popup)
	// means the opener — this module editor page — reloads to pick up
	// the new script content/list state, matching the "Add/rename/delete
	// script" actions' existing full-page-reload pattern (plain
	// form POSTs) instead of introducing a partial-refresh path.
	function wireScriptEditorRefresh() {
		window.addEventListener('message', function (e) {
			if (e.origin !== location.origin) return;
			if (e.data !== 'pluris:script-saved') return;
			window.location.reload();
		});
	}

	// ---- Scripts tab: param tree + phase panes + save -------------------

	// paramCompletionData — the flat list of {path, label, type} the
	// completion source below offers. Populated by renderParamTree from
	// the SAME /api/params fetch that builds the tree (single feed, no
	// second request); empty until that resolves, in which case the
	// completion source simply offers nothing yet.
	var paramCompletionData = [];

	// paramCompletionSource — CodeMirror CompletionSource wired into the
	// phase editors via upgradeTextareas' opts.completionSource (see
	// code-editor.js: autocompletion({override:[fn]})). Triggers when the
	// text before the cursor looks like an opening `{{ ...` (optionally
	// already partway into `param "..."`), and each completion REPLACES
	// from the `{{` onward with the full canonical `{{ param "<path>" }}`
	// token. label = the param path (what the user types to filter),
	// detail = the human label + type.
	function paramCompletionSource(context) {
		var word = context.matchBefore(/\{\{\s*(?:param\s*)?"?[\w\/.-]*$/);
		if (!word) return null;
		if (word.from === word.to && !context.explicit) return null;
		if (paramCompletionData.length === 0) return null;
		return {
			from: word.from,
			options: paramCompletionData.map(function (p) {
				return {
					label: p.path,
					apply: '{{ param "' + p.path + '" }}',
					detail: p.label + (p.type ? ' (' + p.type + ')' : ''),
					type: 'variable',
				};
			}),
		};
	}

	function initScriptsTab() {
		var root = document.querySelector('[data-pm-scripts-root]');
		if (!root) return;

		var canEdit = root.getAttribute('data-can-edit') === 'true';
		var moduleUrn = root.getAttribute('data-module-urn');
		var versionID = root.getAttribute('data-version-id');
		var csrf = root.getAttribute('data-csrf') || '';

		if (window.PlurisCodeEditor) {
			// completionSource wired here so typing `{{` in any phase
			// editor offers param completions built from the same data as
			// the tree. Passed through upgradeTextareas (rather than
			// explicit mount() calls) to keep the hidden-textarea .value
			// two-way sync the save buttons read — see code-editor.js's
			// upgradeTextareas(root, opts) contract.
			window.PlurisCodeEditor.upgradeTextareas(root, { completionSource: paramCompletionSource });
		}

		wirePhaseTabs(root);
		loadParamTree(root, moduleUrn);
		wireScriptSaves(root, moduleUrn, versionID, csrf, canEdit);
	}

	function wirePhaseTabs(root) {
		var tabs = root.querySelectorAll('.pm-phase-tab');
		var panes = root.querySelectorAll('.pm-phase-pane');
		if (tabs.length === 0) return;

		function activate(phase) {
			for (var i = 0; i < tabs.length; i++) {
				tabs[i].classList.toggle('is-active', tabs[i].getAttribute('data-pm-phase') === phase);
			}
			for (var j = 0; j < panes.length; j++) {
				panes[j].style.display = (panes[j].getAttribute('data-pm-phase-pane') === phase) ? '' : 'none';
			}
			root.setAttribute('data-pm-active-phase', phase);
		}

		for (var i = 0; i < tabs.length; i++) {
			tabs[i].addEventListener('click', function (e) {
				activate(e.currentTarget.getAttribute('data-pm-phase'));
			});
		}
		activate(tabs[0].getAttribute('data-pm-phase'));
	}

	// activeEditorHost returns the .pluris-code-editor host element for
	// whichever phase pane is currently visible, or null.
	function activeEditorHost(root) {
		var phase = root.getAttribute('data-pm-active-phase');
		if (!phase) return null;
		var pane = root.querySelector('.pm-phase-pane[data-pm-phase-pane="' + phase + '"]');
		if (!pane) return null;
		return pane.querySelector('.pluris-code-editor');
	}

	function activeEditorTextarea(root) {
		var phase = root.getAttribute('data-pm-active-phase');
		if (!phase) return null;
		var pane = root.querySelector('.pm-phase-pane[data-pm-phase-pane="' + phase + '"]');
		if (!pane) return null;
		return pane.querySelector('textarea[data-pm-source]');
	}

	// insertParamToken inserts `{{ param "path" }}` at the active editor's
	// cursor (CM6 mounted) or appends to the textarea value as a fallback
	// (no CM6 / mount failed) — the click-to-insert path the brief calls
	// out as acceptable v1 even if drag-drop proves unreliable.
	function insertParamToken(root, path) {
		var token = '{{ param "' + path + '" }}';
		var host = activeEditorHost(root);
		if (host && host.plurisCodeEditor && host.plurisCodeEditor.view) {
			var view = host.plurisCodeEditor.view;
			var pos = view.state.selection.main.head;
			view.dispatch({ changes: { from: pos, to: pos, insert: token } });
			view.focus();
			return;
		}
		var ta = activeEditorTextarea(root);
		if (ta) {
			ta.value = (ta.value || '') + token;
			ta.dispatchEvent(new Event('input', { bubbles: true }));
		}
	}

	// dropParamToken inserts at the drop coordinates when possible
	// (CM6's posAtCoords), falling back to insertParamToken's
	// cursor/append behavior otherwise. Best-effort: HTML5 DnD into a
	// CodeMirror 6 view is inherently approximate across browsers.
	function dropParamToken(root, path, clientX, clientY) {
		var host = activeEditorHost(root);
		if (host && host.plurisCodeEditor && host.plurisCodeEditor.view) {
			var view = host.plurisCodeEditor.view;
			var pos = null;
			try {
				pos = view.posAtCoords({ x: clientX, y: clientY });
			} catch (e) { /* older CM6 builds without posAtCoords */ }
			if (typeof pos === 'number') {
				var token = '{{ param "' + path + '" }}';
				view.dispatch({ changes: { from: pos, to: pos, insert: token } });
				view.focus();
				return;
			}
		}
		insertParamToken(root, path);
	}

	function wireScriptSaves(root, moduleUrn, versionID, csrf, canEdit) {
		if (!canEdit) return;
		var buttons = root.querySelectorAll('[data-pm-save-script]');
		for (var i = 0; i < buttons.length; i++) {
			buttons[i].addEventListener('click', function (e) {
				var btn = e.currentTarget;
				var phase = btn.getAttribute('data-pm-save-script');
				var pane = root.querySelector('.pm-phase-pane[data-pm-phase-pane="' + phase + '"]');
				if (!pane) return;
				var filenameEl = pane.querySelector('[data-pm-filename="' + phase + '"]');
				var sourceEl = pane.querySelector('[data-pm-source="' + phase + '"]');
				var body = {
					phase: phase,
					filename: filenameEl ? filenameEl.value : '',
					source: sourceEl ? sourceEl.value : '',
				};
				var origLabel = btn.textContent;
				btn.disabled = true;
				fetch('/api/modules/' + encodeURIComponent(moduleUrn) + '/versions/' + encodeURIComponent(versionID) + '/scripts', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
					body: JSON.stringify(body),
				}).then(function (r) {
					btn.disabled = false;
					if (r.ok) {
						btn.textContent = 'Saved ✓';
						setTimeout(function () { btn.textContent = origLabel; }, 1500);
						return;
					}
					return r.json().then(function (err) {
						alert('Save failed: ' + (err.message || err.error || 'unknown error'));
					}, function () { alert('Save failed: unknown error'); });
				}).catch(function (err) {
					btn.disabled = false;
					alert('Save failed: ' + err.message);
				});
			});
		}

		var deleteButtons = root.querySelectorAll('[data-pm-delete-script]');
		for (var j = 0; j < deleteButtons.length; j++) {
			deleteButtons[j].addEventListener('click', function (e) {
				var btn = e.currentTarget;
				var phase = btn.getAttribute('data-pm-delete-script');
				if (!confirm('Remove the ' + phase + ' script from this draft?')) return;
				btn.disabled = true;
				fetch('/api/modules/' + encodeURIComponent(moduleUrn) + '/versions/' + encodeURIComponent(versionID) + '/scripts/delete', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
					body: JSON.stringify({ phase: phase }),
				}).then(function (r) {
					if (r.ok) { window.location.reload(); return; }
					btn.disabled = false;
					return r.json().then(function (err) {
						alert('Remove failed: ' + (err.message || err.error || 'unknown error'));
					}, function () { alert('Remove failed: unknown error'); });
				}).catch(function (err) {
					btn.disabled = false;
					alert('Remove failed: ' + err.message);
				});
			});
		}
	}

	// loadParamTree fetches the session-filtered parameter tree from
	// /api/params (the same feed the dependency-condition builder uses)
	// and renders it into #pm-param-tree-body, wiring click-to-insert and
	// a search filter. Passing ?module_id=<moduleUrn> (Task 4.4) appends
	// this module's own module/input/<key> parameters -- parsed from
	// whatever it currently has saved in the Parameters tab -- to the
	// SAME feed, so the tree and the `{{` completion source (fed from
	// this same response, see paramCompletionData above) both offer a
	// module's own inputs without a second request or a second source
	// of truth.
	function loadParamTree(root, moduleUrn) {
		var body = document.getElementById('pm-param-tree-body');
		var search = document.getElementById('pm-param-search');
		if (!body) return;

		var url = '/api/params';
		if (moduleUrn) url += '?module_id=' + encodeURIComponent(moduleUrn);

		fetch(url).then(function (r) {
			if (!r.ok) throw new Error('status ' + r.status);
			return r.json();
		}).then(function (data) {
			renderParamTree(root, body, data.sources || []);
			if (search) {
				search.addEventListener('input', function () {
					filterParamTree(body, search.value.toLowerCase());
				});
			}
		}).catch(function (err) {
			body.innerHTML = '';
			var p = document.createElement('p');
			p.className = 'cg-muted';
			p.textContent = 'Could not load parameters: ' + err.message;
			body.appendChild(p);
		});
	}

	function renderParamTree(root, body, sources) {
		// Feed the `{{` completion source from the same response (see
		// paramCompletionData's doc comment above).
		paramCompletionData = [];
		for (var s = 0; s < sources.length; s++) {
			var secs = sources[s].sections || [];
			for (var t = 0; t < secs.length; t++) {
				var ps = secs[t].params || [];
				for (var u = 0; u < ps.length; u++) {
					paramCompletionData.push({
						path: ps[u].path,
						label: ps[u].label || ps[u].key,
						type: ps[u].type || '',
					});
				}
			}
		}

		body.innerHTML = '';
		if (sources.length === 0) {
			var empty = document.createElement('p');
			empty.className = 'cg-muted';
			empty.textContent = 'No parameters visible to your session.';
			body.appendChild(empty);
			return;
		}
		for (var i = 0; i < sources.length; i++) {
			var src = sources[i];
			var entityEl = document.createElement('details');
			entityEl.className = 'pm-param-entity';
			entityEl.open = true;
			var entitySummary = document.createElement('summary');
			entitySummary.textContent = src.pluralLabel || src.label || src.entity;
			entityEl.appendChild(entitySummary);

			for (var j = 0; j < (src.sections || []).length; j++) {
				var sec = src.sections[j];
				var secEl = document.createElement('details');
				secEl.className = 'pm-param-section';
				var secSummary = document.createElement('summary');
				secSummary.textContent = sec.label || sec.key;
				secEl.appendChild(secSummary);

				var list = document.createElement('ul');
				list.className = 'pm-param-list';
				for (var k = 0; k < (sec.params || []).length; k++) {
					var p = sec.params[k];
					var li = document.createElement('li');
					li.className = 'pm-param-item';
					li.setAttribute('data-pm-param-path', p.path);
					li.setAttribute('data-pm-param-search', (p.path + ' ' + p.label).toLowerCase());
					li.setAttribute('draggable', 'true');
					li.title = 'Click to insert {{ param "' + p.path + '" }} — or drag onto an editor';
					var label = document.createElement('span');
					label.className = 'pm-param-label';
					label.textContent = p.label || p.key;
					var pathEl = document.createElement('code');
					pathEl.className = 'pm-param-path font-mono';
					pathEl.textContent = p.path;
					li.appendChild(label);
					li.appendChild(pathEl);
					list.appendChild(li);
				}
				secEl.appendChild(list);
				entityEl.appendChild(secEl);
			}
			body.appendChild(entityEl);
		}

		body.addEventListener('click', function (e) {
			var item = e.target.closest('[data-pm-param-path]');
			if (!item) return;
			insertParamToken(root, item.getAttribute('data-pm-param-path'));
		});
		body.addEventListener('dragstart', function (e) {
			var item = e.target.closest('[data-pm-param-path]');
			if (!item) return;
			e.dataTransfer.setData('text/plain', item.getAttribute('data-pm-param-path'));
			e.dataTransfer.effectAllowed = 'copy';
		});

		// Wire drop targets on every editor host + fallback textarea so a
		// drag from the tree onto any phase pane inserts the token, even
		// though only the active pane is visible at a time.
		var editors = root.querySelectorAll('.pluris-code-editor, textarea[data-pm-source]');
		for (var e2 = 0; e2 < editors.length; e2++) {
			wireDropTarget(root, editors[e2]);
		}
	}

	function wireDropTarget(root, el) {
		el.addEventListener('dragover', function (e) {
			e.preventDefault();
			e.dataTransfer.dropEffect = 'copy';
		});
		el.addEventListener('drop', function (e) {
			e.preventDefault();
			var path = e.dataTransfer.getData('text/plain');
			if (!path) return;
			dropParamToken(root, path, e.clientX, e.clientY);
		});
	}

	function filterParamTree(body, needle) {
		var items = body.querySelectorAll('[data-pm-param-path]');
		for (var i = 0; i < items.length; i++) {
			var hay = items[i].getAttribute('data-pm-param-search') || '';
			items[i].style.display = (!needle || hay.indexOf(needle) !== -1) ? '' : 'none';
		}
	}

	// ---- Parameters tab: structured JSON-Schema row editor ---------------

	function initParametersTab() {
		var roots = document.querySelectorAll('[data-pm-params-root]');
		for (var i = 0; i < roots.length; i++) {
			initSchemaEditor(roots[i]);
		}
	}

	// One structured JSON-Schema row editor per root — the Parameters tab
	// (field parameters_schema) and the Report tab (field report_schema)
	// mount the SAME component with a different data-field-key, never a
	// fork (INV-U).
	function initSchemaEditor(root) {
		var canEdit = root.getAttribute('data-can-edit') === 'true';
		var saveURL = root.getAttribute('data-save-url');
		var csrf = root.getAttribute('data-csrf') || '';
		var fieldKey = root.getAttribute('data-field-key') || 'parameters_schema';
		var tbody = root.querySelector('[data-pm-rows]');
		var preview = root.querySelector('[data-pm-preview]');
		var addBtn = root.querySelector('[data-pm-add-row]');
		var saveBtn = root.querySelector('[data-pm-save-schema]');

		if (window.PlurisCodeEditor) {
			window.PlurisCodeEditor.upgradeTextareas(root);
		}

		var schema = parseSchema(preview ? preview.value : '');
		renderRows(tbody, schema, canEdit);

		if (addBtn) {
			addBtn.addEventListener('click', function () {
				addRow(tbody, { key: '', label: '', type: 'string', def: '', required: false, enumValues: '' }, canEdit);
			});
		}
		if (saveBtn) {
			saveBtn.addEventListener('click', function () {
				var built = buildSchema(tbody);
				var json = JSON.stringify(built, null, 2);
				if (preview) setPreviewValue(preview, json);
				saveBtn.disabled = true;
				var fields = {};
				fields[fieldKey] = JSON.stringify(built);
				fetch(saveURL, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
					body: JSON.stringify({ section: fieldKey, fields: fields }),
				}).then(function (r) {
					saveBtn.disabled = false;
					if (r.ok) {
						saveBtn.textContent = 'Saved ✓';
						setTimeout(function () { saveBtn.textContent = 'Save schema'; }, 1500);
						return;
					}
					return r.json().then(function (err) {
						alert('Save failed: ' + (err.message || err.error || 'unknown error'));
					}, function () { alert('Save failed: unknown error'); });
				}).catch(function (err) {
					saveBtn.disabled = false;
					alert('Save failed: ' + err.message);
				});
			});
		}
	}

	// setPreviewValue relies on upgradeTextareas' redefined `.value`
	// setter (see code-editor.js) to route the assignment through the
	// mounted CM6 editor when one exists; a plain assignment is enough
	// either way (falls back to the real textarea when CM6 didn't mount).
	function setPreviewValue(textarea, value) {
		textarea.value = value;
	}

	function parseSchema(raw) {
		if (!raw) return { properties: {}, order: [], required: [] };
		try {
			var parsed = JSON.parse(raw);
			return {
				properties: parsed.properties || {},
				order: Object.keys(parsed.properties || {}),
				required: parsed.required || [],
			};
		} catch (e) {
			return { properties: {}, order: [], required: [] };
		}
	}

	function renderRows(tbody, schema, canEdit) {
		if (!tbody) return;
		tbody.innerHTML = '';
		for (var i = 0; i < schema.order.length; i++) {
			var key = schema.order[i];
			var def = schema.properties[key] || {};
			var enumValues = Array.isArray(def.enum) ? def.enum.join(', ') : '';
			addRow(tbody, {
				key: key,
				label: def.title || '',
				type: def.type || 'string',
				def: def.default !== undefined ? String(def.default) : '',
				required: schema.required.indexOf(key) !== -1,
				enumValues: enumValues,
			}, canEdit);
		}
	}

	function addRow(tbody, row, canEdit) {
		if (!tbody) return;
		var tr = document.createElement('tr');
		tr.className = 'pm-params-row';

		tr.appendChild(cellInput('pm-p-key', row.key, canEdit));
		tr.appendChild(cellInput('pm-p-label', row.label, canEdit));
		tr.appendChild(cellSelect(row.type, canEdit));
		tr.appendChild(cellInput('pm-p-default', row.def, canEdit));
		tr.appendChild(cellCheckbox(row.required, canEdit));
		tr.appendChild(cellInput('pm-p-enum', row.enumValues, canEdit));

		var actionsTd = document.createElement('td');
		if (canEdit) {
			var rm = document.createElement('button');
			rm.type = 'button';
			rm.className = 'btn btn-sm btn-danger';
			rm.textContent = 'Remove';
			rm.addEventListener('click', function () { tr.remove(); });
			actionsTd.appendChild(rm);
		}
		tr.appendChild(actionsTd);
		tbody.appendChild(tr);
	}

	function cellInput(cls, value, canEdit) {
		var td = document.createElement('td');
		var input = document.createElement('input');
		input.type = 'text';
		input.className = 'input ' + cls;
		input.value = value || '';
		input.disabled = !canEdit;
		td.appendChild(input);
		return td;
	}

	function cellSelect(value, canEdit) {
		var td = document.createElement('td');
		var sel = document.createElement('select');
		sel.className = 'input pm-p-type';
		sel.disabled = !canEdit;
		['string', 'number', 'boolean', 'enum'].forEach(function (t) {
			var opt = document.createElement('option');
			opt.value = t;
			opt.textContent = t;
			if (t === value) opt.selected = true;
			sel.appendChild(opt);
		});
		td.appendChild(sel);
		return td;
	}

	function cellCheckbox(checked, canEdit) {
		var td = document.createElement('td');
		var input = document.createElement('input');
		input.type = 'checkbox';
		input.className = 'pm-p-required';
		input.checked = !!checked;
		input.disabled = !canEdit;
		td.appendChild(input);
		return td;
	}

	function buildSchema(tbody) {
		var properties = {};
		var required = [];
		var order = [];
		if (!tbody) return { type: 'object', properties: properties, required: required };
		var rows = tbody.querySelectorAll('tr.pm-params-row');
		for (var i = 0; i < rows.length; i++) {
			var row = rows[i];
			var key = (row.querySelector('.pm-p-key') || {}).value;
			if (!key) continue;
			var label = (row.querySelector('.pm-p-label') || {}).value || '';
			var type = (row.querySelector('.pm-p-type') || {}).value || 'string';
			var def = (row.querySelector('.pm-p-default') || {}).value || '';
			var required_ = (row.querySelector('.pm-p-required') || {}).checked;
			var enumRaw = (row.querySelector('.pm-p-enum') || {}).value || '';

			var prop = { type: (type === 'enum') ? 'string' : type };
			if (label) prop.title = label;
			if (def !== '') prop.default = coerceDefault(type, def);
			if (type === 'enum' && enumRaw) {
				prop.enum = enumRaw.split(',').map(function (s) { return s.trim(); }).filter(Boolean);
			}
			properties[key] = prop;
			order.push(key);
			if (required_) required.push(key);
		}
		return { type: 'object', properties: properties, required: required };
	}

	function coerceDefault(type, raw) {
		if (type === 'number') {
			var n = Number(raw);
			return isNaN(n) ? raw : n;
		}
		if (type === 'boolean') {
			return raw === 'true';
		}
		return raw;
	}
})();
