/* module-script-editor.js — CP3 of the Scripts+Enforcement redesign: the
 * standalone, full-window per-script code editor
 * (web/templates/module_script_editor.templ), opened in a new browser
 * window/tab from the Scripts tab's row links (moduleScriptsTab,
 * target="_blank").
 *
 * Relocates the param-tree fetch/render + click-to-insert logic that
 * used to live in policy-module-editor.js's Scripts-tab code (now dead
 * there -- CP2 removed its `data-pm-scripts-root` host) and layers the
 * spec's "two-part insert" (design doc section 3 / decision 8) on top:
 * clicking a parameter in the tree BOTH (a) ensures a declaration line
 * for it exists in a managed header block at the very top of the
 * script (deduplicated) and (b) inserts `{{ param "path" }}` at the
 * current cursor position.
 *
 * Loaded after the CM6 vendor bundle and code-editor.js (see the templ
 * page's <script> tags). Everything here is defensive: a missing
 * #mse-root is a no-op, matching this repo's other page-specific
 * scripts.
 */
(function () {
	'use strict';

	document.addEventListener('DOMContentLoaded', init);

	// paramCompletionData / paramCompletionSource -- same shape and
	// contract as policy-module-editor.js's version: the CodeMirror
	// CompletionSource fed from the same /api/params response that
	// builds the tree (see loadParamTree below), triggering on `{{ ` and
	// replacing from the `{{` onward with the full canonical token.
	var paramCompletionData = [];

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

	// commentPrefixFor -- sh, powershell, and python all use `#` for
	// line comments, so the header-block delimiters and each imported
	// param's declaration line use the same prefix for every language
	// PlurisCodeEditor currently supports (Task 3.1). If a future
	// language without `#` comments is added, this is the one place to
	// extend.
	function commentPrefixFor() {
		return '#';
	}

	var HEADER_START = 'pluris:params';
	var HEADER_END = 'pluris:end';

	// ensureParamHeader ensures a declaration line for `path` exists in
	// the managed header block at the very top of `text`, deduplicated.
	// Returns { text, changed }. The header format:
	//
	//   # pluris:params
	//   # {{ param "some/path" }}
	//   # pluris:end
	//
	// One line per imported param; each line contains the exact
	// `{{ param "<path>" }}` token so the header itself is part of the
	// security allow-list (ReferencedParams, design doc section 6) even
	// if the cursor usage is later deleted. Creates the block at
	// position 0 if it doesn't exist yet; inserts one new line just
	// before the end marker if it does and `path` isn't already
	// declared; is a no-op (changed:false) if `path` is already
	// declared anywhere in the block.
	function ensureParamHeader(text, path, prefix) {
		var token = '{{ param "' + path + '" }}';
		var startMarker = prefix + ' ' + HEADER_START;
		var endMarker = prefix + ' ' + HEADER_END;
		var lines = text.split('\n');

		var startIdx = -1;
		var endIdx = -1;
		for (var i = 0; i < lines.length; i++) {
			if (startIdx === -1 && lines[i].trim() === startMarker) {
				startIdx = i;
			} else if (startIdx !== -1 && lines[i].trim() === endMarker) {
				endIdx = i;
				break;
			}
		}

		if (startIdx === -1) {
			// No header block yet -- create one at the very top.
			var block = [startMarker, prefix + ' ' + token, endMarker, ''];
			return { text: block.concat(lines).join('\n'), changed: true };
		}

		if (endIdx === -1) {
			// Malformed (start marker with no matching end) -- close it
			// right after the start marker rather than guessing how far
			// a broken block extends.
			endIdx = startIdx + 1;
			lines.splice(endIdx, 0, endMarker);
		}

		for (var j = startIdx + 1; j < endIdx; j++) {
			if (lines[j].indexOf(token) !== -1) {
				// Already imported -- no-op.
				return { text: lines.join('\n'), changed: false };
			}
		}

		lines.splice(endIdx, 0, prefix + ' ' + token);
		return { text: lines.join('\n'), changed: true };
	}

	var root = null;
	var sourceTextarea = null;
	var languageSelect = null;
	var editorHandle = null; // PlurisCodeEditor handle, or null pre-mount
	var editorHost = null;

	function currentLanguage() {
		if (languageSelect) return languageSelect.value;
		return (sourceTextarea && sourceTextarea.getAttribute('data-code-editor')) || 'sh';
	}

	function getSource() {
		if (editorHandle) return editorHandle.getValue();
		return sourceTextarea ? sourceTextarea.value : '';
	}

	// mountEditor (re)mounts PlurisCodeEditor on the source textarea for
	// `language`, preserving `value`. Called once on load and again
	// every time the language <select> changes (Task 3.4: "changing the
	// language select re-mounts/re-tokenises the editor with the new
	// language, preserve current text").
	function mountEditor(language, value, readOnly) {
		if (editorHandle) {
			try { editorHandle.destroy(); } catch (e) { /* ignore */ }
			editorHandle = null;
		}
		if (editorHost && editorHost.parentNode) {
			editorHost.parentNode.removeChild(editorHost);
		}
		editorHost = null;

		if (!window.PlurisCodeEditor) {
			// code-editor.js itself didn't load -- leave the plain
			// textarea as the only editing surface.
			sourceTextarea.hidden = false;
			sourceTextarea.value = value;
			return;
		}

		var host = document.createElement('div');
		host.className = 'pluris-code-editor';
		sourceTextarea.parentNode.insertBefore(host, sourceTextarea.nextSibling);

		var handle = window.PlurisCodeEditor.mount(host, {
			language: language,
			value: value,
			readOnly: readOnly,
			completionSource: paramCompletionSource,
		});

		if (handle.view) {
			// Real CM6 editor mounted -- hide the plain textarea (kept
			// in the DOM, off-screen, never removed) and drive
			// everything (getValue/cursor inserts) through the CM6
			// view from here on, mirroring code-editor.js's own
			// upgradeTextareas hidden-textarea pattern.
			sourceTextarea.hidden = true;
			sourceTextarea.style.position = 'absolute';
			sourceTextarea.style.width = '1px';
			sourceTextarea.style.height = '1px';
			sourceTextarea.style.overflow = 'hidden';
			sourceTextarea.style.opacity = '0';
			sourceTextarea.style.pointerEvents = 'none';
			editorHost = host;
			editorHandle = handle;
		} else {
			// window.CM6 missing or mount() failed -- degrade to the
			// plain textarea per the brief ("the tree insert must
			// still work against the textarea fallback"). Remove the
			// empty host and keep the real textarea visible/usable.
			if (host.parentNode) host.parentNode.removeChild(host);
			sourceTextarea.hidden = false;
			sourceTextarea.value = value;
			sourceTextarea.readOnly = readOnly;
			editorHost = null;
			editorHandle = null;
		}
	}

	// insertParam is the two-part insert (Task 3.4 / design doc section
	// 3 & decision 8): ensure the header import line for `path` exists
	// (deduplicated), then insert `{{ param "path" }}` at the cursor.
	function insertParam(path) {
		var prefix = commentPrefixFor(currentLanguage());
		if (editorHandle && editorHandle.view) {
			insertParamCM6(editorHandle.view, path, prefix);
			return;
		}
		insertParamFallback(path, prefix);
	}

	function insertParamCM6(view, path, prefix) {
		var doc = view.state.doc.toString();
		var header = ensureParamHeader(doc, path, prefix);
		if (header.changed) {
			view.dispatch({ changes: { from: 0, to: doc.length, insert: header.text } });
		}
		// view.state.selection was remapped through the header dispatch
		// above (CM6 maps the prior selection through every change), so
		// this cursor position already accounts for however many
		// characters the header insert added ahead of it.
		var pos = view.state.selection.main.head;
		var token = '{{ param "' + path + '" }}';
		view.dispatch({ changes: { from: pos, to: pos, insert: token } });
		view.focus();
	}

	// insertParamFallback mirrors insertParamCM6 for the no-CM6 plain
	// textarea path: manually remap the textarea's cursor position by
	// however many characters the header insert added ahead of it
	// (the header is always inserted at/near the very top of the
	// document, i.e. at or before the user's cursor in the normal case
	// of editing the script body below it).
	function insertParamFallback(path, prefix) {
		var ta = sourceTextarea;
		if (!ta) return;
		var oldText = ta.value || '';
		var cursorPos = typeof ta.selectionStart === 'number' ? ta.selectionStart : oldText.length;
		var header = ensureParamHeader(oldText, path, prefix);
		var delta = header.text.length - oldText.length;
		var newCursor = cursorPos + delta;
		var token = '{{ param "' + path + '" }}';
		var finalText = header.text.slice(0, newCursor) + token + header.text.slice(newCursor);
		ta.value = finalText;
		ta.dispatchEvent(new Event('input', { bubbles: true }));
		var finalPos = newCursor + token.length;
		try { ta.setSelectionRange(finalPos, finalPos); } catch (e) { /* ignore */ }
		ta.focus();
	}

	// ---- Parameter tree (relocated from policy-module-editor.js) -----

	function loadParamTree(moduleUrn) {
		var body = document.getElementById('pm-param-tree-body');
		var search = document.getElementById('pm-param-search');
		if (!body) return;

		var url = '/api/params';
		if (moduleUrn) url += '?module_id=' + encodeURIComponent(moduleUrn);

		fetch(url).then(function (r) {
			if (!r.ok) throw new Error('status ' + r.status);
			return r.json();
		}).then(function (data) {
			renderParamTree(body, data.sources || []);
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

	function renderParamTree(body, sources) {
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
					li.title = 'Click to insert {{ param "' + p.path + '" }}';
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
			insertParam(item.getAttribute('data-pm-param-path'));
		});
	}

	function filterParamTree(body, needle) {
		var items = body.querySelectorAll('[data-pm-param-path]');
		for (var i = 0; i < items.length; i++) {
			var hay = items[i].getAttribute('data-pm-param-search') || '';
			items[i].style.display = (!needle || hay.indexOf(needle) !== -1) ? '' : 'none';
		}
	}

	// ---- Save (+ rename-before-save) + opener refresh -------------------

	// scriptScopedURL builds one of this module version's script-scoped
	// endpoints (save / rename) for `name`, mirroring
	// moduleScriptSourceSaveURL/moduleVersionActionURL server-side.
	function scriptScopedURL(moduleUrn, versionID, name, suffix) {
		var u = '/policy/modules/' + encodeURIComponent(moduleUrn) + '/versions/' + encodeURIComponent(versionID) + '/scripts/' + encodeURIComponent(name);
		return suffix ? u + '/' + suffix : u;
	}

	// renameIfChanged posts to the CP2 rename endpoint when the header's
	// name input no longer matches `scriptName` (the name the page
	// loaded with / was last saved under), so the "editable script name
	// field" in the header actually renames the script rather than
	// silently discarding the edit. Resolves to the name Save should now
	// use (the new name on success, the old name unchanged/on failure).
	function renameIfChanged(moduleUrn, versionID, csrf, nameInput) {
		if (!nameInput) return Promise.resolve(scriptName);
		var newName = nameInput.value.trim();
		if (!newName || newName === scriptName) return Promise.resolve(scriptName);

		var form = new URLSearchParams();
		form.set('new_name', newName);
		form.set('_csrf', csrf);
		return fetch(scriptScopedURL(moduleUrn, versionID, scriptName, 'rename'), {
			method: 'POST',
			headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'X-CSRF-Token': csrf },
			body: form.toString(),
		}).then(function (r) {
			// The rename handler 302-redirects on success (fetch follows
			// it transparently) or returns an HTTPError JSON/body on
			// failure -- either way r.ok tells us which happened.
			if (!r.ok) {
				alert('Rename failed -- keeping the current name "' + scriptName + '".');
				nameInput.value = scriptName;
				return scriptName;
			}
			scriptName = newName;
			root.setAttribute('data-script-name', newName);
			try {
				var url = new URL(location.href);
				url.pathname = url.pathname.replace(/\/scripts\/[^/]+\/edit$/, '/scripts/' + encodeURIComponent(newName) + '/edit');
				history.replaceState(null, '', url.toString());
			} catch (e) { /* older browsers without URL()/history -- ignore, cosmetic only */ }
			return newName;
		});
	}

	function wireSave(moduleUrn, versionID, csrf, readOnly, nameInput) {
		var saveBtn = document.getElementById('mse-save');
		var saveStatus = document.getElementById('mse-save-status');
		if (readOnly || !saveBtn) return;

		saveBtn.addEventListener('click', function () {
			saveBtn.disabled = true;
			if (saveStatus) saveStatus.textContent = 'Saving…';
			renameIfChanged(moduleUrn, versionID, csrf, nameInput).then(function (name) {
				var body = { source: getSource(), language: currentLanguage() };
				return fetch(scriptScopedURL(moduleUrn, versionID, name, ''), {
					method: 'POST',
					headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
					body: JSON.stringify(body),
				});
			}).then(function (r) {
				saveBtn.disabled = false;
				if (r.ok) {
					if (saveStatus) {
						saveStatus.textContent = 'Saved ✓';
						setTimeout(function () { saveStatus.textContent = ''; }, 1500);
					}
					if (window.opener) {
						try {
							window.opener.postMessage('pluris:script-saved', location.origin);
						} catch (e) { /* cross-origin/closed opener -- ignore */ }
					}
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

	// ---- Init ------------------------------------------------------------

	var scriptName = '';

	function init() {
		root = document.getElementById('mse-root');
		if (!root) return;

		var moduleUrn = root.getAttribute('data-module-urn');
		var versionID = root.getAttribute('data-version-id');
		var readOnly = root.getAttribute('data-read-only') === 'true';
		var csrf = root.getAttribute('data-csrf') || '';
		scriptName = root.getAttribute('data-script-name') || '';

		sourceTextarea = document.getElementById('mse-source');
		languageSelect = document.getElementById('mse-language');
		var nameInput = document.getElementById('mse-name');

		if (!sourceTextarea) return;

		mountEditor(currentLanguage(), sourceTextarea.value, readOnly);
		loadParamTree(moduleUrn);
		wireSave(moduleUrn, versionID, csrf, readOnly, nameInput);

		if (languageSelect && !readOnly) {
			languageSelect.addEventListener('change', function () {
				mountEditor(languageSelect.value, getSource(), readOnly);
			});
		}
	}
})();
