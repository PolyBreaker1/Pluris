/* Pluris Modern Filter System
 * Inspired by ServiceNow, Jira Service Management, Freshservice
 * Features:
 * - Instant filtering (no Apply button)
 * - Filter chips showing active criteria
 * - Result count (X of Y assets)
 * - Quick filter buttons
 * - Advanced filter builder
 * - Saved views/presets
 */

(function () {
'use strict';

function ready(fn) {
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', fn, { once: true });
    } else { fn(); }
}

ready(init);

function init() {
    const host = document.querySelector('[data-filter-builder]');
    if (!host) return;

    const listId = host.getAttribute('data-filter-builder');
    const cfgEl = host.querySelector('[data-filter-config]');
    if (!cfgEl) return;

    let CFG;
    try {
        CFG = JSON.parse(cfgEl.textContent || cfgEl.innerHTML || '{}');
    } catch (e) {
        console.error('Failed to parse filter config:', e);
        return;
    }
    if (!CFG || !CFG.params) return;

    // State
    const state = {
        searchText: '',
        quickFilter: 'all',
        advancedCriteria: [],
        isAdvancedMode: false,
        activeFilters: [], // For display chips
    };

    // Cache table and rows
    const table = document.querySelector(`table[data-list-id="${listId}"]`);
    if (!table) return;
    const tbody = table.tBodies[0];
    if (!tbody) return;
    
    const allRows = Array.from(tbody.querySelectorAll('tr:not(.cat-divider)'));
    const totalCount = allRows.length;

    // Get elements
    const searchInput = host.querySelector('#filter-search-input');
    const searchClear = host.querySelector('#filter-search-clear');
    const quickButtons = host.querySelectorAll('.filter-quick-btn');
    const addFilterBtn = host.querySelector('#filter-add-btn');
    const clearAllBtn = host.querySelector('#filter-clear-all');
    const toggleAdvancedBtn = host.querySelector('#filter-toggle-advanced');
    const filterBuilder = host.querySelector('#filter-builder');
    const filterActive = host.querySelector('#filter-active');
    const filterChips = host.querySelector('#filter-chips');
    const resultsCount = host.querySelector('#filter-results-count');
    const emptyState = host.querySelector('#filter-empty-state');

    // Initialize
    updateDisplay();

    // ===================================================================
    // SEARCH INPUT
    // ===================================================================
    if (searchInput) {
        searchInput.addEventListener('input', function (e) {
            state.searchText = e.target.value.trim();
            applyFilters();
            updateDisplay();
        });

        if (searchClear) {
            searchClear.addEventListener('click', function () {
                searchInput.value = '';
                state.searchText = '';
                applyFilters();
                updateDisplay();
            });
        }
    }

    // ===================================================================
    // QUICK FILTER BUTTONS
    // ===================================================================
    quickButtons.forEach(btn => {
        btn.addEventListener('click', function () {
            const filter = this.getAttribute('data-filter');
            state.quickFilter = filter;
            
            // Update active state
            quickButtons.forEach(b => b.classList.remove('active'));
            this.classList.add('active');
            
            applyFilters();
            updateDisplay();
        });
    });

    // ===================================================================
    // ADD FILTER (Advanced Mode)
    // ===================================================================
    if (addFilterBtn) {
        addFilterBtn.addEventListener('click', function () {
            if (!state.isAdvancedMode) {
                state.isAdvancedMode = true;
                if (filterBuilder) filterBuilder.classList.add('active');
                if (toggleAdvancedBtn) toggleAdvancedBtn.textContent = 'Simple';
            }
            
            // Add new criterion
            const firstParam = getFirstParam();
            state.advancedCriteria.push({
                id: Date.now(),
                param: firstParam,
                op: getFirstOperator(firstParam),
                value: '',
                logic: 'and'
            });
            
            renderAdvancedCriteria();
            applyFilters();
            updateDisplay();
        });
    }

    // ===================================================================
    // TOGGLE ADVANCED MODE
    // ===================================================================
    if (toggleAdvancedBtn) {
        toggleAdvancedBtn.addEventListener('click', function () {
            state.isAdvancedMode = !state.isAdvancedMode;
            
            if (state.isAdvancedMode) {
                if (filterBuilder) filterBuilder.classList.add('active');
                this.textContent = 'Simple';
                
                // Add initial criterion if empty
                if (state.advancedCriteria.length === 0) {
                    const firstParam = getFirstParam();
                    state.advancedCriteria.push({
                        id: Date.now(),
                        param: firstParam,
                        op: getFirstOperator(firstParam),
                        value: '',
                        logic: 'and'
                    });
                    renderAdvancedCriteria();
                }
            } else {
                if (filterBuilder) filterBuilder.classList.remove('active');
                this.textContent = 'Advanced';
                state.advancedCriteria = [];
            }
            
            applyFilters();
            updateDisplay();
        });
    }

    // ===================================================================
    // CLEAR ALL
    // ===================================================================
    if (clearAllBtn) {
        clearAllBtn.addEventListener('click', function () {
            state.searchText = '';
            state.quickFilter = 'all';
            state.advancedCriteria = [];
            if (searchInput) searchInput.value = '';
            
            quickButtons.forEach(b => b.classList.remove('active'));
            const allBtn = host.querySelector('[data-filter="all"]');
            if (allBtn) allBtn.classList.add('active');
            
            applyFilters();
            updateDisplay();
        });
    }

    // ===================================================================
    // FILTER APPLICATION
    // ===================================================================
    function applyFilters() {
        let visibleCount = 0;

        allRows.forEach(row => {
            let show = true;

            // Apply search text (searches across all fields)
            if (state.searchText) {
                const searchable = row.getAttribute('data-searchable') || '';
                show = searchable.toLowerCase().includes(state.searchText.toLowerCase());
            }

            // Apply quick filter
            if (show && state.quickFilter !== 'all') {
                show = matchQuickFilter(row, state.quickFilter);
            }

            // Apply advanced criteria
            if (show && state.advancedCriteria.length > 0) {
                show = evalCriteria(row, state.advancedCriteria);
            }

            row.classList.toggle('pf-hidden', !show);
            if (show) visibleCount++;
        });

        // Update results count
        if (resultsCount) {
            const numberSpan = resultsCount.querySelector('.filter-results-count-number');
            if (numberSpan) {
                numberSpan.textContent = `${visibleCount} of ${totalCount}`;
            }
        }

        // Show/hide empty state
        if (emptyState) {
            emptyState.classList.toggle('active', visibleCount === 0);
        }
    }

    function matchQuickFilter(row, filter) {
        switch (filter) {
            case 'enrolled':
                return getVal(row, 'enrollment_state') === 'enrolled';
            case 'pending':
                return getVal(row, 'enrollment_state') === 'pending';
            case 'approved':
                return getVal(row, 'enrollment_state') === 'approved';
            case 'linux':
                return getVal(row, 'os_family') === 'linux';
            case 'windows':
                return getVal(row, 'os_family') === 'windows';
            case 'macos':
                return getVal(row, 'os_family') === 'darwin';
            // Users list quick filters (web/templates/users.templ).
            case 'admin':
                return getVal(row, 'role') === 'super_admin' || getVal(row, 'role') === 'admin';
            case 'self-service':
                return getVal(row, 'role') === 'user';
            case 'enabled':
                return getVal(row, 'account_enabled') === 'true';
            case 'locked':
                return getVal(row, 'account_locked') === 'true';
            default:
                return true;
        }
    }

    function evalCriteria(row, criteria) {
        if (criteria.length === 0) return true;
        
        let result = matchCriterion(row, criteria[0]);
        
        for (let i = 1; i < criteria.length; i++) {
            const m = matchCriterion(row, criteria[i]);
            switch (criteria[i].logic) {
                case 'or':
                    result = result || m;
                    break;
                case 'nor':
                    result = result && !m;
                    break;
                default:
                    result = result && m;
                    break;
            }
        }
        
        return result;
    }

    function matchCriterion(row, c) {
        if (!c.value && needsValue(c.op)) return true; // Skip empty values
        
        const raw = getVal(row, c.param);
        const v = c.value || '';
        
        switch (c.op) {
            case 'contains':
                return lc(raw).includes(lc(v));
            case 'not_contains':
                return !lc(raw).includes(lc(v));
            case 'equals':
                return lc(raw) === lc(v);
            case 'not_equals':
                return lc(raw) !== lc(v);
            case 'starts_with':
                return lc(raw).startsWith(lc(v));
            case 'ends_with':
                return lc(raw).endsWith(lc(v));
            case 'gt':
                return pn(raw) > pn(v);
            case 'gte':
                return pn(raw) >= pn(v);
            case 'lt':
                return pn(raw) < pn(v);
            case 'lte':
                return pn(raw) <= pn(v);
            case 'is_empty':
                return !raw || !raw.trim();
            case 'is_not_empty':
                return !!raw && raw.trim() !== '';
            default:
                return true;
        }
    }

    function getVal(row, key) {
        const attr = 'data-' + key.replace(/_/g, '-');
        return row.getAttribute(attr) || '';
    }

    function needsValue(op) {
        return !['is_empty', 'is_not_empty'].includes(op);
    }

    // ===================================================================
    // UPDATE DISPLAY (Filter Chips, Result Count)
    // ===================================================================
    function updateDisplay() {
        // Update search clear button visibility
        if (searchClear && searchInput) {
            searchClear.style.display = state.searchText ? 'block' : 'none';
        }

        // Build active filters for chips
        state.activeFilters = [];
        
        if (state.searchText) {
            state.activeFilters.push({
                type: 'search',
                label: 'Search',
                value: state.searchText
            });
        }
        
        if (state.quickFilter !== 'all') {
            state.activeFilters.push({
                type: 'quick',
                label: getQuickFilterLabel(state.quickFilter),
                value: ''
            });
        }
        
        state.advancedCriteria.forEach(c => {
            if (c.value || !needsValue(c.op)) {
                const param = CFG.params[c.param];
                state.activeFilters.push({
                    type: 'advanced',
                    id: c.id,
                    label: param ? param.label : c.param,
                    operator: c.op,
                    value: c.value
                });
            }
        });

        // Render filter chips
        renderFilterChips();

        // Show/hide active filters section
        if (filterActive) {
            filterActive.classList.toggle('empty', state.activeFilters.length === 0);
        }
    }

    function renderFilterChips() {
        if (!filterChips) return;
        
        if (state.activeFilters.length === 0) {
            filterChips.innerHTML = '';
            return;
        }
        
        let html = '';
        state.activeFilters.forEach((f, idx) => {
            const opLabel = f.operator ? getOperatorLabel(f.operator) : '';
            const displayValue = f.value ? `${opLabel} "${f.value}"` : '';
            
            html += `<div class="filter-chip" data-filter-idx="${idx}">
                <span class="filter-chip-label">${esc(f.label)}</span>
                ${displayValue ? `<span class="filter-chip-value">${esc(displayValue)}</span>` : ''}
                <button class="filter-chip-remove" data-filter-idx="${idx}">×</button>
            </div>`;
        });
        
        filterChips.innerHTML = html;

        // Add click handlers for remove buttons
        filterChips.querySelectorAll('.filter-chip-remove').forEach(btn => {
            btn.addEventListener('click', function () {
                const idx = parseInt(this.getAttribute('data-filter-idx'));
                removeFilter(idx);
            });
        });
    }

    function removeFilter(idx) {
        const filter = state.activeFilters[idx];
        
        if (filter.type === 'search') {
            state.searchText = '';
            if (searchInput) searchInput.value = '';
        } else if (filter.type === 'quick') {
            state.quickFilter = 'all';
            quickButtons.forEach(b => b.classList.remove('active'));
            const allBtn = host.querySelector('[data-filter="all"]');
            if (allBtn) allBtn.classList.add('active');
        } else if (filter.type === 'advanced') {
            state.advancedCriteria = state.advancedCriteria.filter(c => c.id !== filter.id);
            renderAdvancedCriteria();
        }
        
        applyFilters();
        updateDisplay();
    }

    // ===================================================================
    // ADVANCED CRITERIA BUILDER
    // ===================================================================
    function renderAdvancedCriteria() {
        if (!filterBuilder) return;
        
        const container = filterBuilder.querySelector('#filter-builder-rows');
        if (!container) return;
        
        if (state.advancedCriteria.length === 0) {
            container.innerHTML = '';
            return;
        }
        
        let html = '';
        state.advancedCriteria.forEach((c, idx) => {
            html += renderCriterionRow(c, idx);
        });
        
        container.innerHTML = html;
        attachCriterionHandlers();
    }

    function renderCriterionRow(c, idx) {
        const param = CFG.params[c.param];
        const logic = idx === 0 ? '<div class="filter-logic">WHERE</div>' :
            `<select class="filter-logic" data-idx="${idx}">
                <option value="and" ${c.logic === 'and' ? 'selected' : ''}>AND</option>
                <option value="or" ${c.logic === 'or' ? 'selected' : ''}>OR</option>
                <option value="nor" ${c.logic === 'nor' ? 'selected' : ''}>NOR</option>
            </select>`;
        
        return `<div class="filter-builder-row" data-criterion-id="${c.id}">
            ${logic}
            <select class="filter-select filter-select-field" data-idx="${idx}" data-role="param">
                ${renderParamOptions(c.param)}
            </select>
            <select class="filter-select filter-select-operator" data-idx="${idx}" data-role="op">
                ${renderOperatorOptions(c.param, c.op)}
            </select>
            ${renderValueInput(c, idx)}
            <button class="filter-row-remove" data-idx="${idx}" data-role="remove">×</button>
        </div>`;
    }

    function renderParamOptions(selected) {
        let html = '';
        const sections = CFG.sections || [];
        sections.forEach(sec => {
            html += `<optgroup label="${esc(sec.label)}">`;
            (sec.params || []).forEach(pk => {
                const p = CFG.params[pk];
                if (p) {
                    html += `<option value="${esc(pk)}" ${pk === selected ? 'selected' : ''}>${esc(p.label)}</option>`;
                }
            });
            html += '</optgroup>';
        });
        return html;
    }

    function renderOperatorOptions(paramKey, selected) {
        const param = CFG.params[paramKey];
        if (!param || !param.operators) return '';
        
        let html = '';
        param.operators.forEach(op => {
            html += `<option value="${esc(op.key)}" ${op.key === selected ? 'selected' : ''}>${esc(op.label)}</option>`;
        });
        return html;
    }

    function renderValueInput(c, idx) {
        const param = CFG.params[c.param];
        if (!param) return '';
        
        if (!needsValue(c.op)) {
            return '<span class="filter-input-placeholder">—</span>';
        }
        
        // Enum select
        if (param.enumValues && param.enumValues.length > 0 && ['equals', 'not_equals'].includes(c.op)) {
            let html = `<select class="filter-select" data-idx="${idx}" data-role="val">`;
            html += '<option value="">Select...</option>';
            param.enumValues.forEach(val => {
                html += `<option value="${esc(val)}" ${val === c.value ? 'selected' : ''}>${esc(val)}</option>`;
            });
            html += '</select>';
            return html;
        }
        
        // Input
        let type = 'text';
        if (param.type === 'int' || param.type === 'float') type = 'number';
        if (param.type === 'date' || param.type === 'time') type = 'date';
        
        return `<input type="${type}" class="filter-input" data-idx="${idx}" data-role="val" value="${esc(c.value || '')}" placeholder="Value...">`;
    }

    function attachCriterionHandlers() {
        if (!filterBuilder) return;
        
        // Logic change
        filterBuilder.querySelectorAll('.filter-logic[data-idx]').forEach(el => {
            el.addEventListener('change', function () {
                const idx = parseInt(this.getAttribute('data-idx'));
                state.advancedCriteria[idx].logic = this.value;
                applyFilters();
                updateDisplay();
            });
        });
        
        // Parameter change
        filterBuilder.querySelectorAll('[data-role="param"]').forEach(el => {
            el.addEventListener('change', function () {
                const idx = parseInt(this.getAttribute('data-idx'));
                const c = state.advancedCriteria[idx];
                c.param = this.value;
                c.op = getFirstOperator(c.param);
                c.value = '';
                renderAdvancedCriteria();
                applyFilters();
                updateDisplay();
            });
        });
        
        // Operator change
        filterBuilder.querySelectorAll('[data-role="op"]').forEach(el => {
            el.addEventListener('change', function () {
                const idx = parseInt(this.getAttribute('data-idx'));
                state.advancedCriteria[idx].op = this.value;
                renderAdvancedCriteria();
                applyFilters();
                updateDisplay();
            });
        });
        
        // Value change
        filterBuilder.querySelectorAll('[data-role="val"]').forEach(el => {
            const handler = function () {
                const idx = parseInt(this.getAttribute('data-idx'));
                state.advancedCriteria[idx].value = this.value;
                applyFilters();
                updateDisplay();
            };
            
            el.addEventListener('input', handler);
            el.addEventListener('change', handler);
        });
        
        // Remove button
        filterBuilder.querySelectorAll('[data-role="remove"]').forEach(el => {
            el.addEventListener('click', function () {
                const idx = parseInt(this.getAttribute('data-idx'));
                const id = state.advancedCriteria[idx].id;
                state.advancedCriteria.splice(idx, 1);
                renderAdvancedCriteria();
                applyFilters();
                updateDisplay();
            });
        });
    }

    // ===================================================================
    // HELPERS
    // ===================================================================
    function getFirstParam() {
        const sections = CFG.sections || [];
        if (sections[0] && sections[0].params && sections[0].params[0]) {
            return sections[0].params[0];
        }
        return 'name';
    }

    function getFirstOperator(paramKey) {
        const param = CFG.params[paramKey];
        if (param && param.operators && param.operators[0]) {
            return param.operators[0].key;
        }
        return 'contains';
    }

    function getQuickFilterLabel(filter) {
        const labels = {
            'enrolled': 'Enrolled',
            'pending': 'Pending',
            'approved': 'Approved',
            'linux': 'Linux',
            'windows': 'Windows',
            'macos': 'macOS'
        };
        return labels[filter] || filter;
    }

    function getOperatorLabel(op) {
        const labels = {
            'contains': 'contains',
            'equals': '=',
            'not_equals': '≠',
            'gt': '>',
            'gte': '≥',
            'lt': '<',
            'lte': '≤',
            'starts_with': 'starts with',
            'ends_with': 'ends with',
            'is_empty': 'is empty',
            'is_not_empty': 'is not empty'
        };
        return labels[op] || op;
    }

    function lc(s) {
        return (s || '').toLowerCase();
    }

    function pn(s) {
        const n = parseFloat(s);
        return isNaN(n) ? 0 : n;
    }

    function esc(s) {
        return String(s == null ? '' : s)
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;');
    }
}

})();
