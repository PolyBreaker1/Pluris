/* Pluris unified list engine.
 *
 * One script handles every list table in the admin console:
 *   - real-time text/select/numeric filtering
 *   - click-to-sort with persistent state per list
 *   - cat-divider visibility + "M of N" counts
 *   - empty-state toggling
 *   - search highlighting via <mark class="pf-mark">
 *   - tree-leaf integration (hide leaf when its category is empty;
 *     clear sort when a tree leaf is clicked so anchor scroll lands
 *     on a visible divider)
 *
 * HTML contract — see docs/UX_INVARIANTS.md INV-L9.
 *
 *   <section data-pluris-list="<id>">
 *     <input  data-pluris-filter="<id>" data-filter-attr="searchable"
 *             data-filter-mode="contains" data-pluris-highlight="1">
 *     <select data-pluris-filter="<id>" data-filter-attr="scope"
 *             data-filter-mode="equals">
 *     <table data-list-id="<id>" class="pluris-list">
 *       <thead>… <th class="th-sortable" data-sort-type="alpha"
 *                   data-field="name">
 *                  <button class="th-sort-btn" data-sort-key="name">…</button>
 *                </th> …</thead>
 *       <tbody>
 *         <tr class="cat-divider" data-row-index="0">…</tr>     // optional
 *         <tr data-row-id="…" data-searchable="…" data-scope="…"
 *             data-row-index="1">
 *           <td data-field="name" data-sort="…">…</td>
 *         </tr>
 *       </tbody>
 *     </table>
 *     <div data-pluris-empty="<id>" hidden>No matches.</div>
 *     <div data-pluris-count="<id>" data-total="123"></div>
 *   </section>
 *
 *   <a class="policy-tree-leaf" data-pluris-tree="<id>" href="#cat-…">…</a>
 *
 * Persistence:
 *   pluris.filter.<id>  — { "<attr>": "<value>", … }
 *   pluris.sort.<id>    — { "key": "<field>", "dir": "asc|desc" } | null
 *
 * Events fired:
 *   pluris:list-filtered  detail = { listId, matching, total }
 *   pluris:list-reordered detail = { listId, sortKey, dir }
 */
(function () {
'use strict';

// ----------------------------------------------------------------------
// Init / discovery
// ----------------------------------------------------------------------
function ready(fn) {
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', fn, { once: true });
    } else { fn(); }
}
ready(init);

function init() {
    document.querySelectorAll('[data-pluris-list]').forEach(host => {
        if (host.dataset.plurisListInit === '1') return;
        host.dataset.plurisListInit = '1';
        wire(host);
    });
}

// ----------------------------------------------------------------------
// Per-list wiring
// ----------------------------------------------------------------------
function wire(host) {
    const listId = host.dataset.plurisList;
    const sel    = '[data-list-id="' + cssAttr(listId) + '"]';
    const tables = Array.from(document.querySelectorAll('table' + sel));
    if (!tables.length) return;

    const controls = Array.from(document.querySelectorAll(
        '[data-pluris-filter="' + cssAttr(listId) + '"]'));
    const emptyEl  = document.querySelector('[data-pluris-empty="' + cssAttr(listId) + '"]');
    const countEl  = document.querySelector('[data-pluris-count="' + cssAttr(listId) + '"]');

    const filterStorageKey = 'pluris.filter.' + listId;
    const sortStorageKey   = 'pluris.sort.'   + listId;

    // ---- Restore filter state -----------------------------------------
    try {
        const raw = localStorage.getItem(filterStorageKey);
        if (raw) {
            const saved = JSON.parse(raw) || {};
            controls.forEach(c => {
                const attr = c.dataset.filterAttr;
                if (attr in saved) c.value = saved[attr];
            });
        }
    } catch (e) { /* ignore */ }

    // ---- Persist filter state -----------------------------------------
    function saveFilters() {
        const obj = {};
        controls.forEach(c => { obj[c.dataset.filterAttr] = c.value; });
        try { localStorage.setItem(filterStorageKey, JSON.stringify(obj)); } catch (e) {}
    }

    // ---- Filter modes -------------------------------------------------
    const MODES = {
        contains: (rowVal, q) => !q || (rowVal || '').toLowerCase().indexOf(q.toLowerCase()) !== -1,
        equals:   (rowVal, q) => !q || q === 'all' || rowVal === q,
        prefix:   (rowVal, q) => !q || (rowVal || '').toLowerCase().indexOf(q.toLowerCase()) === 0,
        numGte:   (rowVal, q) => { const n = parseFloat(q); return isNaN(n) || (parseFloat(rowVal) || 0) >= n; },
    };

    function rowMatches(row) {
        for (let i = 0; i < controls.length; i++) {
            const c = controls[i];
            const mode = c.dataset.filterMode || 'contains';
            const attr = c.dataset.filterAttr;
            const val  = c.value || '';
            const fn   = MODES[mode] || MODES.contains;
            const rowVal = row.dataset[camel(attr)] || '';
            if (!fn(rowVal, val)) return false;
        }
        return true;
    }

    // ---- Highlighting (only for inputs flagged data-pluris-highlight) -
    function escapeRegExp(s) { return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'); }
    function clearMarks(root) {
        const marks = root.querySelectorAll('mark.pf-mark');
        marks.forEach(m => {
            const t = document.createTextNode(m.textContent);
            m.parentNode.replaceChild(t, m);
        });
        if (marks.length) root.normalize();
    }
    function applyHighlight(row, query) {
        if (!query) return;
        const re = new RegExp(escapeRegExp(query), 'gi');
        const walker = document.createTreeWalker(row, NodeFilter.SHOW_TEXT, {
            acceptNode: function (n) {
                if (!n.nodeValue || !n.nodeValue.trim()) return NodeFilter.FILTER_REJECT;
                const tag = n.parentElement && n.parentElement.tagName;
                if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'MARK') return NodeFilter.FILTER_REJECT;
                return NodeFilter.FILTER_ACCEPT;
            }
        });
        const nodes = [];
        while (walker.nextNode()) nodes.push(walker.currentNode);
        nodes.forEach(node => {
            const text = node.nodeValue;
            re.lastIndex = 0;
            if (!re.test(text)) return;
            re.lastIndex = 0;
            const frag = document.createDocumentFragment();
            let last = 0, m;
            while ((m = re.exec(text)) !== null) {
                if (m.index > last) frag.appendChild(document.createTextNode(text.slice(last, m.index)));
                const mk = document.createElement('mark');
                mk.className = 'pf-mark';
                mk.textContent = m[0];
                frag.appendChild(mk);
                last = m.index + m[0].length;
                if (m.index === re.lastIndex) re.lastIndex++;
            }
            if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
            node.parentNode.replaceChild(frag, node);
        });
    }
    function highlightControl() {
        return controls.find(c => c.dataset.plurisHighlight === '1');
    }

    // ---- Apply (filter + counts + dividers + empty + highlight) -------
    function applyFilters() {
        const hi = highlightControl();
        const hiQuery = hi ? hi.value.trim() : '';
        let matching = 0, total = 0;

        tables.forEach(tbl => {
            const tbody = tbl.tBodies[0];
            if (!tbody) return;
            Array.from(tbody.children).forEach(row => {
                if (row.classList.contains('cat-divider')) return;
                total++;
                const keep = rowMatches(row);
                row.classList.toggle('pf-hidden', !keep);
                clearMarks(row);
                if (keep) {
                    if (hiQuery) applyHighlight(row, hiQuery);
                    matching++;
                }
            });
        });

        // Walk dividers per table; hide any whose run has zero visible rows.
        tables.forEach(tbl => {
            const tbody = tbl.tBodies[0];
            if (!tbody) return;
            const dividers = tbody.querySelectorAll('tr.cat-divider');
            dividers.forEach(div => {
                let visible = 0, runTotal = 0;
                let n = div.nextElementSibling;
                while (n && !n.classList.contains('cat-divider')) {
                    if (n.dataset && n.dataset.rowId !== undefined ||
                        n.dataset && n.dataset.policyId !== undefined ||
                        n.dataset && n.dataset.cgId    !== undefined ||
                        n.dataset && n.dataset.pmId    !== undefined) {
                        runTotal++;
                        if (!n.classList.contains('pf-hidden')) visible++;
                    }
                    n = n.nextElementSibling;
                }
                div.classList.toggle('pf-hidden', visible === 0);
                const c = div.querySelector('[data-divider-count]');
                if (c) {
                    const declared = parseInt(c.dataset.total || runTotal, 10);
                    const noun = declared === 1 ? (c.dataset.singular || 'item')
                                                : (c.dataset.plural   || 'items');
                    c.textContent = visible === declared
                        ? declared + ' ' + noun
                        : visible + ' of ' + declared + ' ' + noun;
                }
            });
        });

        // Tree leaves linked to dividers in this list.
        document.querySelectorAll('[data-pluris-tree="' + cssAttr(listId) + '"]').forEach(leaf => {
            const href = leaf.getAttribute('href') || '';
            if (href.charAt(0) !== '#') return;
            const target = document.getElementById(href.slice(1));
            leaf.classList.toggle('pf-hidden', !!target && target.classList.contains('pf-hidden'));
        });

        // Count chip.
        if (countEl) {
            const tpl = countEl.dataset.template || '{visible} of {total}';
            countEl.textContent = tpl.replace('{visible}', matching).replace('{total}', total);
            countEl.classList.toggle('chip-filtered', matching !== total);
        }

        // Empty state.
        if (emptyEl) {
            if ('hidden' in emptyEl) emptyEl.hidden = (matching !== 0);
            emptyEl.classList.toggle('pf-show', matching === 0);
        }

        document.dispatchEvent(new CustomEvent('pluris:list-filtered', {
            detail: { listId: listId, matching: matching, total: total }
        }));
    }

    // ---- Wire control listeners ---------------------------------------
    const debounced = debounce(() => { applyFilters(); saveFilters(); }, 80);
    controls.forEach(c => {
        const handler = () => { applyFilters(); saveFilters(); };
        c.addEventListener('input', debounced);
        c.addEventListener('change', handler);
        if (c.tagName === 'INPUT' && c.type === 'search') {
            c.addEventListener('keydown', e => {
                if (e.key === 'Escape') { c.value = ''; handler(); }
            });
        }
    });

    // Optional clear button: [data-pluris-clear="<id>"].
    const clearBtn = document.querySelector('[data-pluris-clear="' + cssAttr(listId) + '"]');
    if (clearBtn) {
        clearBtn.addEventListener('click', () => {
            controls.forEach(c => {
                if (c.tagName === 'SELECT') c.value = c.querySelector('option') ? c.querySelector('option').value : '';
                else c.value = '';
            });
            applyFilters();
            saveFilters();
        });
    }

    // ----------------------------------------------------------------
    // Sort engine — shared across every table[data-list-id=<id>].
    // ----------------------------------------------------------------
    let sortState = loadSort();

    function loadSort() {
        try {
            const raw = localStorage.getItem(sortStorageKey);
            if (!raw) return null;
            const v = JSON.parse(raw);
            if (v && v.key && (v.dir === 'asc' || v.dir === 'desc')) return v;
        } catch (e) {}
        return null;
    }
    function saveSort() {
        try {
            if (sortState) localStorage.setItem(sortStorageKey, JSON.stringify(sortState));
            else           localStorage.removeItem(sortStorageKey);
        } catch (e) {}
    }
    function thFor(key) {
        return tables[0].querySelector('th[data-field="' + cssAttr(key) + '"]');
    }
    function sortType(key) {
        const th = thFor(key);
        return th ? (th.dataset.sortType || 'alpha') : 'alpha';
    }
    function cmp(a, b, type, dir) {
        const av = a == null ? '' : String(a);
        const bv = b == null ? '' : String(b);
        const aE = av === '', bE = bv === '';
        if (aE && !bE) return 1;
        if (!aE && bE) return -1;
        if (aE && bE)  return 0;
        let r;
        if (type === 'num') {
            const an = parseFloat(av), bn = parseFloat(bv);
            r = (isNaN(an) ? 0 : an) - (isNaN(bn) ? 0 : bn);
        } else {
            r = av.localeCompare(bv, undefined, { numeric: true, sensitivity: 'base' });
        }
        return dir === 'desc' ? -r : r;
    }
    function applySort() {
        // Reset indicators on all parallel <th>s.
        tables.forEach(tbl => {
            tbl.querySelectorAll('th.th-sortable').forEach(th => {
                th.classList.remove('is-sorted-asc', 'is-sorted-desc');
                const ind = th.querySelector('.th-sort-ind');
                if (ind) ind.textContent = '↕';
            });
        });
        if (!sortState) {
            // Restore natural order via data-row-index.
            tables.forEach(tbl => {
                const tbody = tbl.tBodies[0];
                if (!tbody) return;
                const rows = Array.from(tbody.children);
                rows.sort((a, b) => (parseInt(a.dataset.rowIndex || '0', 10)
                                  -  parseInt(b.dataset.rowIndex || '0', 10)));
                const frag = document.createDocumentFragment();
                rows.forEach(r => {
                    if (r.classList.contains('cat-divider')) r.classList.remove('sort-hidden');
                    frag.appendChild(r);
                });
                tbody.appendChild(frag);
            });
            applyFilters();
            document.dispatchEvent(new CustomEvent('pluris:list-reordered', {
                detail: { listId: listId, sortKey: null, dir: null }
            }));
            return;
        }
        const type = sortType(sortState.key);
        tables.forEach(tbl => {
            const th = tbl.querySelector('th[data-field="' + cssAttr(sortState.key) + '"]');
            if (th && th.classList.contains('th-sortable')) {
                th.classList.add(sortState.dir === 'desc' ? 'is-sorted-desc' : 'is-sorted-asc');
                const ind = th.querySelector('.th-sort-ind');
                if (ind) ind.textContent = sortState.dir === 'desc' ? '↓' : '↑';
            }
            const tbody = tbl.tBodies[0];
            if (!tbody) return;
            const all = Array.from(tbody.children);
            const dividers = all.filter(r => r.classList.contains('cat-divider'));
            const data     = all.filter(r => !r.classList.contains('cat-divider'));
            data.sort((ra, rb) => {
                const ca = ra.querySelector('td[data-field="' + cssAttr(sortState.key) + '"]');
                const cb = rb.querySelector('td[data-field="' + cssAttr(sortState.key) + '"]');
                return cmp(ca ? ca.dataset.sort : '',
                           cb ? cb.dataset.sort : '',
                           type, sortState.dir);
            });
            // While sorted, dividers don't reflect contiguous categories.
            dividers.forEach(d => d.classList.add('sort-hidden'));
            const frag = document.createDocumentFragment();
            dividers.forEach(d => frag.appendChild(d));
            data.forEach(d => frag.appendChild(d));
            tbody.appendChild(frag);
        });
        applyFilters();
        document.dispatchEvent(new CustomEvent('pluris:list-reordered', {
            detail: { listId: listId, sortKey: sortState.key, dir: sortState.dir }
        }));
    }
    function clickSort(key) {
        const type = sortType(key);
        if (sortState && sortState.key === key) {
            sortState.dir = sortState.dir === 'asc' ? 'desc' : 'asc';
        } else {
            sortState = { key: key, dir: type === 'num' ? 'desc' : 'asc' };
        }
        saveSort();
        applySort();
    }
    tables.forEach(tbl => {
        tbl.querySelectorAll('button[data-sort-key]').forEach(btn => {
            btn.addEventListener('click', e => {
                e.preventDefault();
                e.stopPropagation();
                clickSort(btn.dataset.sortKey);
            });
        });
    });

    // Tree-leaf clicks on this list: clear sort first so the anchor
    // target divider is in the DOM in its natural position.
    document.addEventListener('click', e => {
        const leaf = e.target.closest('[data-pluris-tree="' + cssAttr(listId) + '"][href^="#"]');
        if (!leaf) return;
        if (sortState) {
            sortState = null;
            saveSort();
            applySort();
            const id = leaf.getAttribute('href').slice(1);
            e.preventDefault();
            requestAnimationFrame(() => {
                const target = document.getElementById(id);
                if (target) target.scrollIntoView({ behavior: 'smooth', block: 'start' });
                if (history.replaceState) history.replaceState(null, '', '#' + id);
            });
        }
    });

    // ----------------------------------------------------------------
    // Detail-dialog row-click engine (INV-L10).
    // Opt-in: the host section contains a <dialog data-pluris-detail="<id>">
    // and each row carries data-detail-id="<recordID>". On row click
    // (anywhere outside an interactive element) we open the dialog and
    // fire a `pluris:detail-open` event that the per-list hydrate
    // script listens for. Buttons / links / inputs / [data-pluris-no-detail]
    // suppress the click — they keep their own handlers.
    // ----------------------------------------------------------------
    const detailDlg = document.querySelector(
        'dialog[data-pluris-detail="' + cssAttr(listId) + '"]');
    if (detailDlg) {
        // Mark rows that have a detail id so CSS can offer a cursor +
        // hover affordance without per-list rules.
        tables.forEach(tbl => {
            tbl.classList.add('pluris-list-clickable');
        });

        function openDetail(detailId, row) {
            detailDlg.dataset.detailId = detailId;
            document.dispatchEvent(new CustomEvent('pluris:detail-open', {
                detail: { listId: listId, detailId: detailId, row: row }
            }));
            if (typeof detailDlg.showModal === 'function') {
                detailDlg.showModal();
            } else {
                detailDlg.setAttribute('open', 'open');
            }
            document.body.classList.add('cg-dialog-locked');
        }
        function closeDetail() {
            if (typeof detailDlg.close === 'function') {
                detailDlg.close();
            } else {
                detailDlg.removeAttribute('open');
            }
            document.body.classList.remove('cg-dialog-locked');
            document.dispatchEvent(new CustomEvent('pluris:detail-close', {
                detail: { listId: listId, detailId: detailDlg.dataset.detailId || '' }
            }));
        }

        tables.forEach(tbl => {
            tbl.addEventListener('click', e => {
                // Suppress when the click landed on something interactive
                // — buttons, links, form controls, anything explicitly
                // opted out via [data-pluris-no-detail].
                if (e.target.closest(
                    'button, a, input, select, textarea, label,' +
                    ' [data-pluris-no-detail]')) return;
                const row = e.target.closest('tbody tr[data-detail-id]');
                if (!row) return;
                e.preventDefault();
                openDetail(row.dataset.detailId, row);
            });
            // Keyboard: Enter on a focused row opens the detail.
            tbl.addEventListener('keydown', e => {
                if (e.key !== 'Enter') return;
                const row = e.target.closest('tbody tr[data-detail-id]');
                if (!row) return;
                e.preventDefault();
                openDetail(row.dataset.detailId, row);
            });
        });

        // Generic close handlers: any element marked data-pluris-detail-close
        // inside the dialog closes it. The dialog's native `cancel` event
        // (Esc key) also closes.
        detailDlg.addEventListener('click', e => {
            if (e.target.closest('[data-pluris-detail-close]')) {
                e.preventDefault();
                closeDetail();
            }
        });
        detailDlg.addEventListener('cancel', e => { e.preventDefault(); closeDetail(); });
    }

    // First paint: apply sort (which calls applyFilters at the end).
    applySort();
    // Re-apply on bfcache restore.
    window.addEventListener('pageshow', applyFilters);
}

// ----------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------
function debounce(fn, ms) {
    let t;
    return function () {
        const args = arguments;
        clearTimeout(t);
        t = setTimeout(() => fn.apply(null, args), ms);
    };
}
// Convert kebab-attr to dataset camelCase: "root-category" -> "rootCategory".
function camel(s) {
    return (s || '').replace(/-([a-z])/g, (_, c) => c.toUpperCase());
}
// Escape a value for inclusion in an attribute selector. Conservative.
function cssAttr(s) {
    return String(s == null ? '' : s).replace(/(["\\])/g, '\\$1');
}
})();
