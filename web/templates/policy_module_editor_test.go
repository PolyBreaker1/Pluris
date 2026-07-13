package templates

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
)

// Task 4.3 template tests: the module editor page mounts its scripts in
// vendor -> wrapper -> page-script order, the param tree container is
// present, and the phase strip renders one tab per
// policymodules.AllLifecyclePhases entry.

func moduleEditorTestData() ModuleEditorData {
	return ModuleEditorData{
		Row: db.PolicyModule{ID: 1, ModuleUrn: "tenant.acme.demo", Title: "Demo", TenantID: sql.NullInt64{Int64: 1, Valid: true}},
		Module: policymodules.Module{
			ID: "tenant.acme.demo", Title: "Demo", Description: "a demo module",
		},
		CanEdit:     true,
		CanAdmin:    true,
		HasSelected: true,
		IsDraft:     true,
		SelectedID:  42,
		Selected:    db.PolicyModuleVersion{ID: 42, Version: "1.0.0", State: "draft", Scope: "machine"},
		Scripts:     map[string]db.PolicyModuleScript{},
	}
}

func TestPolicyModuleDetailPage_ScriptMountOrder(t *testing.T) {
	html := renderToString(t, PolicyModuleDetailPage(moduleEditorTestData()))

	vendorIdx := strings.Index(html, "/static/vendor/codemirror/codemirror-pluris.js")
	wrapperIdx := strings.Index(html, "/static/code-editor.js")
	pageIdx := strings.Index(html, "/static/policy-module-editor.js")
	if vendorIdx == -1 || wrapperIdx == -1 || pageIdx == -1 {
		t.Fatalf("missing one of the three mounted scripts in page html")
	}
	if !(vendorIdx < wrapperIdx && wrapperIdx < pageIdx) {
		t.Fatalf("script mount order wrong: vendor=%d wrapper=%d page=%d, want vendor < wrapper < page", vendorIdx, wrapperIdx, pageIdx)
	}
}

func TestPolicyModuleDetailPage_ParamTreeContainerPresent(t *testing.T) {
	html := renderToString(t, PolicyModuleDetailPage(moduleEditorTestData()))
	for _, want := range []string{`id="pm-param-tree"`, `id="pm-param-tree-body"`, `id="pm-param-search"`, `data-pm-scripts-root`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in scripts tab html", want)
		}
	}
}

func TestPolicyModuleDetailPage_PhaseTabsFromAllLifecyclePhases(t *testing.T) {
	html := renderToString(t, PolicyModuleDetailPage(moduleEditorTestData()))
	for _, ph := range policymodules.AllLifecyclePhases {
		want := `data-pm-phase="` + string(ph) + `"`
		if !strings.Contains(html, want) {
			t.Errorf("missing phase tab for %q", ph)
		}
		wantPane := `data-pm-phase-pane="` + string(ph) + `"`
		if !strings.Contains(html, wantPane) {
			t.Errorf("missing phase pane for %q", ph)
		}
	}
	if got := strings.Count(html, "pm-phase-tab"); got == 0 {
		t.Fatal("no phase tabs rendered at all")
	}
}

func TestPolicyModuleDetailPage_PublishedVersionShowsImmutableBanner(t *testing.T) {
	d := moduleEditorTestData()
	d.IsDraft = false
	d.Selected.State = "published"
	html := renderToString(t, PolicyModuleDetailPage(d))
	if !strings.Contains(html, "Published versions are immutable") {
		t.Error("expected immutability banner on the Scripts tab for a published version")
	}
}

func TestPolicyModuleNewPage_RendersForm(t *testing.T) {
	html := renderToString(t, PolicyModuleNewPage("csrf-token", ""))
	for _, want := range []string{`data-testid="page-policy-module-new"`, `name="urn"`, `name="title"`, `name="scope"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in create page html", want)
		}
	}
}
