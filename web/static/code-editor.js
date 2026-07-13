/* PlurisCodeEditor — vanilla-JS wrapper around the vendored CodeMirror 6
 * bundle (`web/static/vendor/codemirror/codemirror-pluris.js`, global
 * `window.CM6`). No build step, no CDN — both files are loaded via plain
 * `<script>` tags, vendor bundle FIRST.
 *
 * Public API:
 *
 *   PlurisCodeEditor.mount(el, {
 *     language: 'bash' | 'yaml' | 'json',   // default 'bash'
 *     value: '...',                          // initial doc, default ''
 *     readOnly: false,
 *     onChange: function(newValue) {},       // called on every doc change
 *     completionSource: function(context) {} // CodeMirror autocomplete
 *                                             // CompletionSource; wired via
 *                                             // autocompletion({override:[fn]})
 *   }) -> { getValue(), setValue(v), destroy(), view }
 *
 *   PlurisCodeEditor.upgradeTextareas(root, opts)
 *     opts (optional): { completionSource } — forwarded to mount() for
 *     every textarea upgraded by this call (see upgradeTextareas' own
 *     doc comment below for the idempotency caveat).
 *     Finds `textarea[data-code-editor]` under `root` (defaults to
 *     `document`), mounts a PlurisCodeEditor next to each one, hides the
 *     original textarea (kept in the DOM, `hidden` + off-screen — never
 *     removed), and keeps `textarea.value` in sync BOTH ways:
 *       - editor -> textarea: on every CM6 doc change (so existing code
 *         that reads `textarea.value` at save time keeps working
 *         unmodified — e.g. condition-builder.js's `scriptSourceEl.value`
 *         reads in `currentValues()`/save).
 *       - textarea -> editor: `.value` is redefined as an accessor on the
 *         upgraded element instance, so code that ASSIGNS
 *         `textarea.value = '...'` (e.g. condition-builder.js's
 *         `applyPrefill()` / `resetForm()` setting `scriptSourceEl.value`)
 *         also updates the live editor content. This is what makes
 *         prefilled script sources show up correctly in the mounted
 *         editor without touching condition-builder.js at all.
 *     The `data-code-editor` attribute value is the language ('bash' by
 *     default when the attribute is present but empty).
 *     Idempotent: an element already upgraded (tracked via a WeakSet) is
 *     skipped on repeat calls, so callers can call this liberally (e.g.
 *     on every dialog open) without double-mounting.
 *
 * Defensive by construction: if `window.CM6` (the vendor bundle) isn't
 * loaded, `mount()` logs ONE console.warn and returns a no-op handle
 * backed by a plain in-memory value (getValue/setValue/destroy all still
 * work, just without syntax highlighting) — pages that forget to load the
 * vendor bundle, or that load this file speculatively, must not crash.
 * `upgradeTextareas()` degrades the same way per textarea: still hides
 * nothing and leaves the plain textarea fully usable if CM6 is absent.
 */
(function () {
	'use strict';

	var warnedMissingCM6 = false;

	function hasCM6() {
		return !!(window.CM6 && window.CM6.EditorState && window.CM6.EditorView);
	}

	function warnMissingOnce() {
		if (warnedMissingCM6) return;
		warnedMissingCM6 = true;
		if (window.console && typeof window.console.warn === 'function') {
			console.warn('[PlurisCodeEditor] window.CM6 (vendor bundle) not found — ' +
				'falling back to plain textareas / no-op editors. Load ' +
				'/static/vendor/codemirror/codemirror-pluris.js before code-editor.js.');
		}
	}

	function languageExtension(CM, language) {
		switch (language) {
			case 'json':
				return CM.json();
			case 'yaml':
				return CM.yaml();
			case 'bash':
			default:
				return CM.shellStreamLanguage;
		}
	}

	// Small theme overrides layered on top of one-dark so the editor sits
	// well inside Pluris's chrome (mono font var, subtle border/radius
	// matching .input elsewhere, no harsh corners). Uses var(--font-mono)
	// with the same fallback pattern as .cb-script-source in layout.templ.
	function themeExtension(CM) {
		return CM.EditorView.theme({
			'&': {
				fontSize: '12.5px',
				borderRadius: '6px',
				border: '1px solid rgba(255,255,255,0.08)',
			},
			'.cm-content': {
				fontFamily: "var(--font-mono, 'JetBrains Mono', ui-monospace, monospace)",
				padding: '8px 0',
			},
			'.cm-gutters': {
				fontFamily: "var(--font-mono, 'JetBrains Mono', ui-monospace, monospace)",
			},
			'&.cm-focused': {
				outline: 'none',
			},
		});
	}

	function makeFallbackHandle(initialValue, onChange) {
		var value = initialValue == null ? '' : String(initialValue);
		return {
			view: null,
			getValue: function () {
				return value;
			},
			setValue: function (v) {
				value = v == null ? '' : String(v);
				if (onChange) onChange(value);
			},
			destroy: function () {},
		};
	}

	function mount(el, opts) {
		opts = opts || {};
		var language = opts.language || 'bash';
		var initialValue = opts.value != null ? opts.value : '';
		var readOnly = !!opts.readOnly;
		var onChange = typeof opts.onChange === 'function' ? opts.onChange : null;
		var completionSource = typeof opts.completionSource === 'function' ? opts.completionSource : null;

		if (!el || !hasCM6()) {
			warnMissingOnce();
			return makeFallbackHandle(initialValue, onChange);
		}

		var CM = window.CM6;
		var extensions = CM.basicSetup.slice();
		extensions.push(languageExtension(CM, language));
		extensions.push(CM.oneDark);
		extensions.push(themeExtension(CM));
		extensions.push(CM.EditorView.lineWrapping);
		extensions.push(
			CM.autocompletion(completionSource ? { override: [completionSource] } : undefined)
		);
		if (readOnly) {
			extensions.push(CM.EditorState.readOnly.of(true));
			extensions.push(CM.EditorView.editable.of(false));
		}
		if (onChange) {
			extensions.push(
				CM.EditorView.updateListener.of(function (update) {
					if (update.docChanged) onChange(update.state.doc.toString());
				})
			);
		}

		var view;
		try {
			var state = CM.EditorState.create({ doc: String(initialValue), extensions: extensions });
			view = new CM.EditorView({ state: state, parent: el });
		} catch (e) {
			// Never let a CM6 construction error (bad extension, DOM issue,
			// mismatched bundle) take down the embedding page.
			if (window.console && typeof window.console.error === 'function') {
				console.error('[PlurisCodeEditor] mount failed, falling back:', e);
			}
			return makeFallbackHandle(initialValue, onChange);
		}

		return {
			view: view,
			getValue: function () {
				return view.state.doc.toString();
			},
			setValue: function (v) {
				var text = v == null ? '' : String(v);
				if (text === view.state.doc.toString()) return;
				view.dispatch({
					changes: { from: 0, to: view.state.doc.length, insert: text },
				});
			},
			destroy: function () {
				view.destroy();
			},
		};
	}

	var upgraded = typeof WeakSet !== 'undefined' ? new WeakSet() : null;

	// upgradeTextareas(root, opts) — opts is optional and currently
	// supports one key:
	//   completionSource: a CodeMirror CompletionSource function passed
	//     straight through to mount() for every textarea upgraded by
	//     THIS call (autocompletion({override:[fn]})). Chosen over
	//     explicit per-editor mount() calls in the module editor so the
	//     hidden-textarea `.value` two-way sync (which the phase-save
	//     buttons and template tests rely on) keeps working unchanged.
	//     Idempotency caveat: an already-upgraded textarea keeps its
	//     original completionSource — call with opts on FIRST upgrade.
	function upgradeTextareas(root, opts) {
		root = root || document;
		var textareas;
		try {
			textareas = root.querySelectorAll('textarea[data-code-editor]');
		} catch (e) {
			return;
		}
		for (var i = 0; i < textareas.length; i++) {
			upgradeOne(textareas[i], opts);
		}
	}

	function upgradeOne(textarea, opts) {
		if (!textarea) return;
		if (upgraded) {
			if (upgraded.has(textarea)) return;
			upgraded.add(textarea);
		} else if (textarea.getAttribute('data-code-editor-upgraded') === '1') {
			return;
		}
		if (!upgraded) textarea.setAttribute('data-code-editor-upgraded', '1');

		var language = textarea.getAttribute('data-code-editor') || 'bash';
		var initialValue = textarea.value || '';

		var host = document.createElement('div');
		host.className = 'pluris-code-editor';
		if (textarea.id) host.setAttribute('data-code-editor-for', textarea.id);
		textarea.parentNode.insertBefore(host, textarea.nextSibling);

		// Hide (not remove) the original textarea. Keeping it in the DOM,
		// still a real <textarea>, means any existing code that reads or
		// sets `.value` (form serialization, condition-builder.js, etc.)
		// keeps working with zero changes to that code.
		textarea.hidden = true;
		textarea.style.position = 'absolute';
		textarea.style.width = '1px';
		textarea.style.height = '1px';
		textarea.style.overflow = 'hidden';
		textarea.style.opacity = '0';
		textarea.style.pointerEvents = 'none';

		var syncingFromEditor = false;

		var handle = mount(host, {
			language: language,
			value: initialValue,
			readOnly: textarea.readOnly || textarea.disabled,
			completionSource: opts && typeof opts.completionSource === 'function' ? opts.completionSource : null,
			onChange: function (newValue) {
				syncingFromEditor = true;
				try {
					// Use the underlying prototype setter (not the overridden
					// instance accessor below) to avoid re-entrant feedback.
					nativeValueSetter.call(textarea, newValue);
					// Fire a real input event so any `addEventListener('input', ...)`
					// on the textarea (e.g. condition-builder.js's
					// updateSaveEnabled) still runs.
					var evt;
					try {
						evt = new Event('input', { bubbles: true });
					} catch (e2) {
						evt = document.createEvent('Event');
						evt.initEvent('input', true, true);
					}
					textarea.dispatchEvent(evt);
				} finally {
					syncingFromEditor = false;
				}
			},
		});

		// Bidirectional sync: redefine `.value` on THIS textarea instance so
		// that code assigning `textarea.value = someScript` (prefill / reset
		// flows) also updates the live editor, not just the hidden backing
		// textarea. Falls back to the native property untouched if CM6 never
		// mounted (handle.view is null in that case, editor === textarea).
		var proto = window.HTMLTextAreaElement && window.HTMLTextAreaElement.prototype;
		var nativeDescriptor = proto && Object.getOwnPropertyDescriptor(proto, 'value');
		var nativeValueGetter = nativeDescriptor ? nativeDescriptor.get : function () {
			return textarea.defaultValue;
		};
		var nativeValueSetter = nativeDescriptor ? nativeDescriptor.set : function (v) {
			textarea.defaultValue = v;
		};

		if (nativeDescriptor && handle.view) {
			try {
				Object.defineProperty(textarea, 'value', {
					configurable: true,
					enumerable: nativeDescriptor.enumerable,
					get: function () {
						return handle.getValue();
					},
					set: function (v) {
						nativeValueSetter.call(textarea, v);
						if (!syncingFromEditor) handle.setValue(v);
					},
				});
			} catch (e) {
				// Some environments may not allow redefining `.value` on a
				// live form element — degrade to one-way (editor->textarea)
				// sync only; still no crash.
			}
		}

		// Expose the handle on the host element as a debug/introspection
		// hook (e.g. for manual QA in devtools or automated DOM-level
		// tests) — not part of the documented public API, but harmless to
		// leave discoverable.
		host.plurisCodeEditor = handle;

		return handle;
	}

	window.PlurisCodeEditor = {
		mount: mount,
		upgradeTextareas: upgradeTextareas,
	};
})();
