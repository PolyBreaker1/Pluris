/* Pluris detail-page tab switcher + hero dropdown + per-section inline edit.
 *
 * Tab switching:
 *   One shared script for every standardized detail page (INV-L9: no
 *   per-page tab scripts). The server renders ALL tab panels; this file
 *   only toggles visibility and syncs the active tab to location.hash so
 *   tabs are deep-linkable and survive refresh.
 *
 * HTML contract (see web/templates/detail_shell.templ):
 *   <div class="asset-detail-tabs">
 *     <div class="asset-detail-tab [is-active]" data-tab="<slug>">…</div>
 *   </div>
 *   <div class="detail-tab-panel [is-active]" data-panel="<slug>">…</div>
 *
 * Hero dropdown (see detail_shell.templ for the ⋮ menu):
 *   .hero-actions > button[data-menu-btn] toggles .hero-dropdown
 *   Click outside closes; Esc key closes.
 *
 * Per-section inline edit (see users.templ / pages.templ):
 *   Each section.card has [data-section] and a .section-header with:
 *     .section-edit-btn    — pencil icon, triggers edit mode
 *     .section-cancel-btn  — hidden until edit mode, reverts
 *     .section-save-btn    — hidden until edit mode, collects values
 *   Fields have [data-editable="true"] + [data-field-key] or [data-readonly="true"].
 */
(function () {
	'use strict';

	// ---- Tab switching ----

	function activate(tab) {
		var slug = tab.getAttribute('data-tab');
		var scope = tab.closest('.asset-detail-tabs')
			? tab.closest('.asset-detail-tabs').parentElement
			: document;
		scope.querySelectorAll('.asset-detail-tab[data-tab], .detail-tab-panel[data-panel]')
			.forEach(function (el) { el.classList.remove('is-active'); });
		tab.classList.add('is-active');
		var panel = scope.querySelector('.detail-tab-panel[data-panel="' + slug + '"]');
		if (panel) panel.classList.add('is-active');
		history.replaceState(null, '', '#' + slug);
	}

	document.addEventListener('click', function (e) {
		var tab = e.target.closest('.asset-detail-tabs .asset-detail-tab[data-tab]');
		if (tab) activate(tab);
	});

	document.addEventListener('keydown', function (e) {
		if (e.key !== 'Enter' && e.key !== ' ') return;
		var tab = e.target.closest('.asset-detail-tabs .asset-detail-tab[data-tab]');
		if (!tab) return;
		if (e.key === ' ') e.preventDefault();
		activate(tab);
	});

	document.addEventListener('DOMContentLoaded', function () {
		var slug = (location.hash || '').slice(1);
		if (!slug) return;
		var tab = document.querySelector('.asset-detail-tabs .asset-detail-tab[data-tab="' + CSS.escape(slug) + '"]');
		if (tab) activate(tab);
	});

	// ---- Hero dropdown (⋮ menu) ----

	window.toggleHeroDropdown = function (btn) {
		var dd = btn.parentElement.querySelector('.hero-dropdown');
		if (!dd) return;
		var isOpen = dd.classList.contains('is-open');
		document.querySelectorAll('.hero-dropdown').forEach(function (d) {
			d.classList.remove('is-open');
		});
		document.querySelectorAll('.hero-menu-btn').forEach(function (b) {
			b.classList.remove('is-active');
		});
		if (!isOpen) {
			dd.classList.add('is-open');
			btn.classList.add('is-active');
		}
	};

	function closeAllDropdowns() {
		document.querySelectorAll('.hero-dropdown, .hero-menu-btn.is-active').forEach(function (el) {
			el.classList.remove('is-open', 'is-active');
		});
	}

	document.addEventListener('click', function (e) {
		if (e.target.closest('.hero-dropdown') || e.target.closest('.hero-menu-btn')) return;
		closeAllDropdowns();
	});

	document.addEventListener('keydown', function (e) {
		if (e.key === 'Escape') closeAllDropdowns();
	});

	// ---- Per-section inline edit ----
	// Each section.card with [data-section] has its own Edit/Cancel/Save
	// inside .section-actions. toggleSectionEdit finds the containing
	// section and swaps all [data-editable="true"] spans to <input>s.
	// Values pre-filled from data-copy (raw value) to avoid clear bug.

	function findSection(el) {
		while (el) {
			if (el.tagName === 'SECTION' && el.classList.contains('card') && el.hasAttribute('data-section')) {
				return el;
			}
			el = el.parentElement;
		}
		return null;
	}

	window.toggleSectionEdit = function (btn) {
		var section = findSection(btn);
		if (!section) return;

		var spans = section.querySelectorAll('span.field-value[data-editable="true"]');
		var i, span, val, key, input;
		for (i = 0; i < spans.length; i++) {
			span = spans[i];
			val = span.getAttribute('data-copy');
			if (val === null || val === undefined) {
				val = span.textContent.trim();
			}
			key = span.getAttribute('data-field-key') || '';
			input = document.createElement('input');
			input.type = 'text';
			input.className = 'inline-edit-input';
			input.value = val;
			input.setAttribute('data-field-key', key);
			input.setAttribute('data-original', val);
			span.style.display = 'none';
			span.parentNode.insertBefore(input, span.nextSibling);
		}

		var actions = section.querySelector('.section-actions');
		if (!actions) return;
		var editBtn = actions.querySelector('.section-edit-btn');
		var cancelBtn = actions.querySelector('.section-cancel-btn');
		var saveBtn = actions.querySelector('.section-save-btn');
		if (editBtn) editBtn.style.display = 'none';
		if (cancelBtn) cancelBtn.style.display = '';
		if (saveBtn) saveBtn.style.display = '';
	};

	window.cancelSectionEdit = function (btn) {
		var section = findSection(btn);
		if (!section) return;

		var inputs = section.querySelectorAll('.inline-edit-input');
		var i;
		for (i = 0; i < inputs.length; i++) {
			inputs[i].remove();
		}
		var spans = section.querySelectorAll('span.field-value[data-editable="true"]');
		for (i = 0; i < spans.length; i++) {
			spans[i].style.display = '';
		}

		var actions = section.querySelector('.section-actions');
		if (!actions) return;
		var editBtn = actions.querySelector('.section-edit-btn');
		var cancelBtn = actions.querySelector('.section-cancel-btn');
		var saveBtn = actions.querySelector('.section-save-btn');
		if (editBtn) editBtn.style.display = '';
		if (cancelBtn) cancelBtn.style.display = 'none';
		if (saveBtn) saveBtn.style.display = 'none';
	};

	// fieldUpdateURL derives the "/api/..." field-update endpoint from the
	// current page path: "/users/:id" -> "/api/users/:id/fields",
	// "/assets/:subtype/:id" -> "/api/assets/:subtype/:id/fields".
	// Returns '' if the path doesn't match either detail-page shape.
	//
	// A section that needs a DIFFERENT endpoint than this path-derived
	// default (e.g. the Policy Module editor, where "meta" fields save to
	// /api/modules/:id/fields but every other section saves to a
	// version-scoped /api/modules/:id/versions/:vid/fields) sets
	// data-save-url directly on its section.card; saveSectionEdit below
	// checks that FIRST and only falls back to this path-derived guess
	// when it's absent, so existing users/assets pages are unaffected.
	function fieldUpdateURL() {
		var parts = window.location.pathname.split('/').filter(function (p) { return p !== ''; });
		if (parts.length === 2 && parts[0] === 'users') {
			return '/api/users/' + parts[1] + '/fields';
		}
		if (parts.length === 3 && parts[0] === 'assets') {
			return '/api/assets/' + parts[1] + '/' + parts[2] + '/fields';
		}
		return '';
	}

	// finishSaveEdit applies the saved values to the display spans and
	// resets the section back out of edit mode. Shared by the success path
	// in saveSectionEdit below.
	function finishSaveEdit(section, changed) {
		var key, span;
		var inputs = section.querySelectorAll('.inline-edit-input');
		var i;
		for (i = 0; i < inputs.length; i++) {
			key = inputs[i].getAttribute('data-field-key');
			span = inputs[i].previousElementSibling;
			if (span && span.hasAttribute('data-editable')) {
				if (key && changed[key] !== undefined) {
					span.textContent = changed[key];
				}
				span.style.display = '';
			}
			inputs[i].remove();
		}

		var actions = section.querySelector('.section-actions');
		if (actions) {
			var editBtn = actions.querySelector('.section-edit-btn');
			var cancelBtn = actions.querySelector('.section-cancel-btn');
			var saveBtn = actions.querySelector('.section-save-btn');
			if (editBtn) editBtn.style.display = '';
			if (cancelBtn) cancelBtn.style.display = 'none';
			if (saveBtn) saveBtn.style.display = 'none';
		}

		var header = section.querySelector('.section-header');
		if (header) {
			var note = header.querySelector('.save-stub-note');
			if (note) note.remove();
		}
	}

	window.saveSectionEdit = function (btn) {
		var section = findSection(btn);
		if (!section) return;

		var changed = {}, key;
		var inputs = section.querySelectorAll('.inline-edit-input');
		var i;
		for (i = 0; i < inputs.length; i++) {
			key = inputs[i].getAttribute('data-field-key');
			if (key && inputs[i].value !== inputs[i].getAttribute('data-original')) {
				changed[key] = inputs[i].value;
			}
		}

		if (Object.keys(changed).length === 0) {
			window.cancelSectionEdit(btn);
			return;
		}

		var url = section.getAttribute('data-save-url') || fieldUpdateURL();
		if (!url) {
			alert('Save failed: could not determine entity from page URL');
			return;
		}

		var csrfInput = document.querySelector('[name=_csrf]');
		var csrf = csrfInput ? csrfInput.value : '';

		fetch(url, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrf },
			body: JSON.stringify({
				section: section.getAttribute('data-section'),
				fields: changed
			})
		}).then(function (r) {
			if (r.ok) {
				finishSaveEdit(section, changed);
				return r.json().then(function (body) {
					if (body && body.warning) alert(body.warning);
				}, function () { /* no JSON body */ });
			}
			return r.json().then(function (err) {
				alert('Save failed: ' + (err.error || err.message || 'unknown error'));
			}, function () {
				alert('Save failed: unknown error');
			});
		}).catch(function (err) {
			console.error('saveSectionEdit fetch error:', err);
			alert('Save failed: ' + err.message);
		});
	};

	// ---- Avatar expand + upload + drag-drop ----
	// Clicking the hero avatar animates it from its original position
	// to a centered 200x200 square.  From there the user can:
	//   • Click "Change photo" → native file picker
	//   • Drag an image file onto the expanded view or the small avatar
	// Drop target glows with var(--accent) (theme colour, never hardcoded).

	window.openAvatarModal = function (el) {
		if (document.querySelector('.avatar-expand-overlay.is-open')) return;

		var rect = el.getBoundingClientRect();
		var isImg = el.tagName === 'IMG';
		var src = isImg ? el.getAttribute('src') : '';
		var initials = isImg ? '' : el.textContent.trim();

		var overlay = document.createElement('div');
		overlay.className = 'avatar-expand-overlay';

		// Clone the source element at the original position
		var clone;
		if (isImg) {
			clone = document.createElement('img');
			clone.className = 'avatar-expand-clone avatar-expand-img';
			clone.src = src;
			clone.draggable = false;
		} else {
			clone = document.createElement('div');
			clone.className = 'avatar-expand-clone avatar-expand-initials';
			clone.textContent = initials;
		}

		setPosition(clone, rect);
		clone._srcEl = el;
		clone._isImg = isImg;
		overlay.appendChild(clone);

		// Hidden file input
		var fileInput = document.createElement('input');
		fileInput.type = 'file';
		fileInput.accept = 'image/*';
		fileInput.style.display = 'none';
		overlay.appendChild(fileInput);

		// Controls row
		var controls = document.createElement('div');
		controls.className = 'avatar-expand-controls';
		controls.style.display = 'none';

		var changeBtn = document.createElement('button');
		changeBtn.type = 'button';
		changeBtn.className = 'btn btn-primary';
		changeBtn.textContent = 'Change photo';
		changeBtn.onclick = function () { fileInput.click(); };
		controls.appendChild(changeBtn);

		var closeBtn = document.createElement('button');
		closeBtn.type = 'button';
		closeBtn.className = 'btn';
		closeBtn.textContent = 'Close';
		closeBtn.onclick = function () { closeExpand(overlay); };
		controls.appendChild(closeBtn);

		overlay.appendChild(controls);

		// Drag-and-drop on the clone
		setupAvatarDrop(clone, el, clone, overlay);
		// Also setup drop on the source element (small avatar)
		setupAvatarDrop(el, el, clone, overlay);

		// Close on overlay background click
		overlay.addEventListener('click', function (e) {
			if (e.target === overlay || e.target === controls) {
				closeExpand(overlay);
			}
		});

		document.body.appendChild(overlay);
		requestAnimationFrame(function () {
			overlay.classList.add('is-open');
			var targetRect = computeTargetRect();
			setPosition(clone, targetRect);
			clone.style.borderRadius = '50%';
		});

		// Show controls after expand animation
		var onDone = function () {
			controls.style.display = 'flex';
			clone.style.cursor = 'default';
			clone.style.pointerEvents = 'auto';
		};
		if (clone.style.transition !== 'none') {
			clone.addEventListener('transitionend', onDone, { once: true });
		} else {
			setTimeout(onDone, 300);
		}

		// File picker handler
		fileInput.onchange = function () {
			var file = fileInput.files[0];
			if (!file || !file.type.startsWith('image/')) return;
			applyAvatarFile(file, clone, el, overlay);
		};

		overlay._close = function () { closeExpand(overlay); };
	};

	function setPosition(el, r) {
		el.style.position = 'fixed';
		el.style.left = r.left + 'px';
		el.style.top = r.top + 'px';
		el.style.width = r.width + 'px';
		el.style.height = r.height + 'px';
	}

	function computeTargetRect() {
		var size = Math.min(240, window.innerWidth - 64, window.innerHeight - 180);
		size = Math.max(size, 120);
		return {
			left: (window.innerWidth - size) / 2,
			top: (window.innerHeight - size) / 2 - 30,
			width: size,
			height: size
		};
	}

	function closeExpand(overlay) {
		if (!overlay || !overlay.classList.contains('is-open')) return;
		var clone = overlay.querySelector('.avatar-expand-clone');
		var srcEl = clone && clone._srcEl;
		// Animate back to original position
		if (clone && srcEl) {
			var origRect = srcEl.getBoundingClientRect();
			setPosition(clone, origRect);
			clone.style.borderRadius = '50%';
		}
		overlay.classList.remove('is-open');
		var controls = overlay.querySelector('.avatar-expand-controls');
		if (controls) controls.style.display = 'none';
		setTimeout(function () { overlay.remove(); }, 350);
	}

	function applyAvatarFile(file, clone, srcEl, overlay) {
		var reader = new FileReader();
		reader.onload = function (e) {
			var dataUrl = e.target.result;
			// Update the clone (expanded view)
			if (!clone._isImg) {
				// Replace initials div with img
				var img = document.createElement('img');
				img.className = 'avatar-expand-clone avatar-expand-img';
				img.src = dataUrl;
				img.draggable = false;
				img._srcEl = srcEl;
				img._isImg = true;
				// Copy position from clone
				var r = clone.getBoundingClientRect();
				clone.replaceWith(img);
				setPosition(img, r);
				setupAvatarDrop(img, srcEl, img, overlay);
				clone = img;
			} else {
				clone.src = dataUrl;
			}

			// Update the source element (small hero avatar)
			if (srcEl) {
				if (srcEl.tagName !== 'IMG') {
					var newImg = document.createElement('img');
					newImg.className = 'user-avatar-img';
					newImg.src = dataUrl;
					newImg.alt = 'Profile picture';
					newImg.draggable = false;
					newImg.onclick = function () { window.openAvatarModal(this); };
					newImg.title = 'Click to expand';
					setupAvatarDrop(newImg, newImg, null, null);
					srcEl.parentElement.replaceChild(newImg, srcEl);
				} else {
					srcEl.src = dataUrl;
				}
			}

			// Close after a brief moment
			setTimeout(function () { closeExpand(overlay); }, 600);
		};
		reader.readAsDataURL(file);

		// POST the actual file to the backend so it's persisted server-side
		// (the code above is just the optimistic local preview). Same
		// "/users/:id" -> "/api/users/:id/..." URL derivation as
		// fieldUpdateURL, since this isn't a field-update body.
		var parts = window.location.pathname.split('/').filter(function (p) { return p !== ''; });
		if (parts.length !== 2 || parts[0] !== 'users') return;
		var url = '/api/users/' + parts[1] + '/avatar';

		var csrfInput = document.querySelector('[name=_csrf]');
		var csrf = csrfInput ? csrfInput.value : '';

		var fd = new FormData();
		fd.append('avatar', file);

		fetch(url, {
			method: 'POST',
			headers: { 'X-CSRF-Token': csrf },
			body: fd
		}).then(function (r) {
			if (r.ok) return;
			return r.json().then(function (err) {
				alert('Avatar upload failed: ' + (err.error || err.message || 'unknown error'));
			}, function () {
				alert('Avatar upload failed: unknown error');
			});
		}).catch(function (err) {
			console.error('avatar upload fetch error:', err);
			alert('Avatar upload failed: network error');
		});
	}

	function setupAvatarDrop(dropEl, srcEl, cloneEl, overlay) {
		if (!dropEl) return;
		// Guard against re-wiring listeners onto persistent elements (e.g. the
		// small hero avatar) that survive across modal opens — without this,
		// every openAvatarModal() call would stack duplicate dragover/dragenter/
		// dragleave/drop listeners on the same node.
		if (dropEl.dataset.dropWired === '1') {
			dropEl._avatarDropCtx = { srcEl: srcEl, cloneEl: cloneEl, overlay: overlay };
			return;
		}
		dropEl.dataset.dropWired = '1';
		dropEl._avatarDropCtx = { srcEl: srcEl, cloneEl: cloneEl, overlay: overlay };

		var addGlow = function () {
			dropEl.classList.add('avatar-drop-glow');
		};
		var removeGlow = function () {
			dropEl.classList.remove('avatar-drop-glow');
		};

		dropEl.addEventListener('dragover', function (e) {
			e.preventDefault();
			e.dataTransfer.dropEffect = 'copy';
			addGlow();
		});
		dropEl.addEventListener('dragenter', function (e) {
			e.preventDefault();
			addGlow();
		});
		dropEl.addEventListener('dragleave', function (e) {
			removeGlow();
		});
		dropEl.addEventListener('drop', function (e) {
			e.preventDefault();
			removeGlow();
			var file = e.dataTransfer.files[0];
			if (file && file.type.startsWith('image/')) {
				var ctx = dropEl._avatarDropCtx || { srcEl: srcEl, cloneEl: cloneEl, overlay: overlay };
				applyAvatarFile(file, ctx.cloneEl || dropEl, ctx.srcEl, ctx.overlay);
			}
		});
	}
})();
