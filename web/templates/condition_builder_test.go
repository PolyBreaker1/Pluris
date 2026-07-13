package templates

import (
	"strings"
	"testing"

	"github.com/pluris/pluris/catalog/dependencygroups"
)

// TestConditionBuilderDialogRendersTabsAndOperators covers the static
// contract of the popup: both tabs present, the supported-operators data
// attribute matches dependencygroups.AllOperators() exactly (so the JS
// intersection can't silently drift from the eval engine), and the
// documented open/prefill/save attributes appear on the rendered markup.
func TestConditionBuilderDialogRendersTabsAndOperators(t *testing.T) {
	html := renderToString(t, ConditionBuilderDialog())

	if !strings.Contains(html, `id="condition-builder"`) {
		t.Fatalf("missing dialog root id: %s", html)
	}
	if !strings.Contains(html, `data-cb-tab="param"`) || !strings.Contains(html, `data-cb-tab="script"`) {
		t.Fatalf("missing both tabs: %s", html)
	}
	if !strings.Contains(html, "Parameter") || !strings.Contains(html, "Custom script") {
		t.Fatalf("missing tab labels: %s", html)
	}

	// data-supported-operators must be exactly dependencygroups.AllOperators(),
	// comma-joined, in order — the JS intersects a param's API-advertised
	// operators against this set, so it is the eval engine's contract
	// surface into the client. Any drift here is a silent capability bug.
	want := make([]string, 0)
	for _, o := range dependencygroups.AllOperators() {
		want = append(want, string(o))
	}
	wantAttr := `data-supported-operators="` + strings.Join(want, ",") + `"`
	if !strings.Contains(html, wantAttr) {
		t.Fatalf("supported-operators attribute mismatch.\nwant substring: %s\ngot html: %s", wantAttr, html)
	}

	// Script tab fields.
	for _, want := range []string{
		`id="cb-script-source"`, `data-code-editor="bash"`,
		`id="cb-script-exit-code"`, `id="cb-script-output-equals"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing script-tab element %q in html: %s", want, html)
		}
	}

	// Parameter tab skeleton (populated client-side from /api/params).
	for _, want := range []string{
		`id="cb-param-tree"`, `id="cb-param-search"`, `id="cb-operator"`, `id="cb-value-container"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing parameter-tab element %q in html: %s", want, html)
		}
	}

	// Save is disabled until JS validates a complete condition; Cancel/×
	// close without dispatching (data-cb-close) — see doc comment.
	if !strings.Contains(html, "data-cb-save") || !strings.Contains(html, "disabled") {
		t.Fatalf("save button should render disabled by default: %s", html)
	}
	if got := strings.Count(html, "data-cb-close"); got < 2 {
		t.Fatalf("expected at least 2 close triggers (× and Cancel), got %d: %s", got, html)
	}

	// The delivery mechanism: a <script src> tag for the standalone JS
	// file, emitted by the component itself (see doc comment on why this
	// differs from targetPickerScript's inline templ.Raw const string).
	if !strings.Contains(html, `<script src="/static/condition-builder.js"`) {
		t.Fatalf("missing condition-builder.js script tag: %s", html)
	}

	// Code-editor wiring (task 4.1): the vendor CodeMirror 6 bundle and
	// the PlurisCodeEditor wrapper must both be present, and in the right
	// order — vendor bundle (defines window.CM6) BEFORE the wrapper
	// (reads window.CM6), both before condition-builder.js (whose
	// openDialog() calls PlurisCodeEditor.upgradeTextareas right after
	// showModal(), so the wrapper global must already exist when the
	// dialog first opens). All three are `defer` scripts WITH src, so
	// they execute in tag order after parsing — an inline (src-less)
	// wiring script would NOT be deferred and was rejected for exactly
	// that reason.
	vendorIdx := strings.Index(html, `<script src="/static/vendor/codemirror/codemirror-pluris.js"`)
	wrapperIdx := strings.Index(html, `<script src="/static/code-editor.js"`)
	cbIdx := strings.Index(html, `<script src="/static/condition-builder.js"`)
	if vendorIdx == -1 {
		t.Fatalf("missing vendor CodeMirror bundle script tag: %s", html)
	}
	if wrapperIdx == -1 {
		t.Fatalf("missing code-editor.js wrapper script tag: %s", html)
	}
	if !(vendorIdx < wrapperIdx && wrapperIdx < cbIdx) {
		t.Fatalf("expected script order vendor < wrapper < condition-builder.js, got indices %d, %d, %d: %s", vendorIdx, wrapperIdx, cbIdx, html)
	}
}

// TestJoinOperatorsEmptyAndOrdered covers the helper directly: empty
// slice must render an empty attribute value (not "," artifacts), and
// order/format must be a plain comma join with no spaces (the JS splits
// on ",").
func TestJoinOperatorsEmptyAndOrdered(t *testing.T) {
	if got := joinOperators(nil); got != "" {
		t.Fatalf("joinOperators(nil) = %q, want empty string", got)
	}
	ops := []dependencygroups.Operator{dependencygroups.OpIn, dependencygroups.OpExists}
	if got, want := joinOperators(ops), "in,exists"; got != want {
		t.Fatalf("joinOperators(%v) = %q, want %q", ops, got, want)
	}
}

// TestConditionBuilderDialogHasExactlyOneOfEachTabPanel is a light sanity
// check that the two-tab structure isn't accidentally duplicated by a
// future edit (panels are toggled via `hidden`, not re-rendered).
func TestConditionBuilderDialogHasExactlyOneOfEachTabPanel(t *testing.T) {
	html := renderToString(t, ConditionBuilderDialog())
	if got := strings.Count(html, `data-cb-panel="param"`); got != 1 {
		t.Fatalf("want exactly 1 param panel, got %d", got)
	}
	if got := strings.Count(html, `data-cb-panel="script"`); got != 1 {
		t.Fatalf("want exactly 1 script panel, got %d", got)
	}
}
