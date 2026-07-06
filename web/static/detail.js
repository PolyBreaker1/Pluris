/* Pluris detail-page tab switcher.
 *
 * One shared script for every standardized detail page (INV-L9: no
 * per-page tab scripts). The server renders ALL tab panels; this file
 * only toggles visibility and syncs the active tab to location.hash so
 * tabs are deep-linkable and survive refresh.
 *
 * HTML contract (see web/templates/detail_shell.templ):
 *   <div class="asset-detail-tabs">
 *     <div class="asset-detail-tab [is-active]" data-tab="<slug>">…</div>
 *   </div>
 *   <div class="detail-tab-panel [is-active]" data-panel="<slug>">…</div>
 *
 * Tabs and panels are scoped to the tabs container's parent element, so
 * a second shell on one page won't cross-activate (only one shell per
 * page is supported for now).
 */
(function () {
	'use strict';

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

	// Keyboard: Enter or Space activates a focused tab (tabs carry
	// tabindex="0" and role="tab" in detail_shell.templ).
	document.addEventListener('keydown', function (e) {
		if (e.key !== 'Enter' && e.key !== ' ') return;
		var tab = e.target.closest('.asset-detail-tabs .asset-detail-tab[data-tab]');
		if (!tab) return;
		if (e.key === ' ') e.preventDefault(); // stop page scroll
		activate(tab);
	});

	document.addEventListener('DOMContentLoaded', function () {
		var slug = (location.hash || '').slice(1);
		if (!slug) return;
		var tab = document.querySelector('.asset-detail-tabs .asset-detail-tab[data-tab="' + CSS.escape(slug) + '"]');
		if (tab) activate(tab);
	});
})();
