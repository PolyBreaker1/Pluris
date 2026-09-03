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
			Satisfies: []string{"urn:pluris:policy:demo"}, TargetOS: []policymodules.TargetOS{policymodules.OSLinux},
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

func TestPolicyModuleDetailPage_PickerEntryPoint(t *testing.T) {
	html := renderToString(t, PolicyModuleDetailPage(moduleEditorTestData()))
	for _, want := range []string{`data-pmp-open`, `>Pick</span>`, `id="pmp-dialog"`, `data-pmp-urn="urn:pluris:policy:demo"`} {
		if !strings.Contains(html, want) {
			t.Errorf("module detail page missing picker contract %q", want)
		}
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

// TestPolicyModuleDetailPage_ScriptsTabIsINVL locks CP2 of the
// Scripts+Enforcement redesign: the phase-tabbed inline editor markup
// is gone, replaced by the registry-driven module-scripts list with one
// row per named script (name/language/origin/used-by).
func TestPolicyModuleDetailPage_ScriptsTabIsINVL(t *testing.T) {
	d := moduleEditorTestData()
	d.ScriptList = []policymodules.Script{
		{Name: "apply.sh", Language: "sh", Origin: "default"},
		{Name: "custom-check.sh", Language: "sh", Origin: "custom"},
	}
	d.ScriptUsedBy = map[string][]string{"apply.sh": {"apply"}}
	html := renderToString(t, PolicyModuleDetailPage(d))

	for _, forbidden := range []string{"data-pm-phase", "data-pm-phase-pane", "pm-param-tree", "data-pm-scripts-root", "data-pm-filename", "data-pm-source", "data-pm-save-script", "data-pm-delete-script"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("Scripts tab still contains removed phase-editor markup %q", forbidden)
		}
	}
	for _, want := range []string{
		`data-detail-list="module-scripts"`,
		"apply.sh", "custom-check.sh",
		"(default)", "(custom)",
		"apply", // used-by label
		`name="name"`, `name="language"`, "Add script",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Scripts tab missing %q", want)
		}
	}
}

// TestPolicyModuleDetailPage_ScriptsTabReadOnlyWhenPublished asserts a
// published version's Scripts tab shows the list but no add/rename/
// delete controls.
func TestPolicyModuleDetailPage_ScriptsTabReadOnlyWhenPublished(t *testing.T) {
	d := moduleEditorTestData()
	d.IsDraft = false
	d.Selected.State = "published"
	d.ScriptList = []policymodules.Script{{Name: "apply.sh", Language: "sh", Origin: "default"}}
	html := renderToString(t, PolicyModuleDetailPage(d))

	if !strings.Contains(html, "apply.sh") {
		t.Error("published Scripts tab should still list scripts")
	}
	for _, forbidden := range []string{"Add script", "Rename", ">Delete<"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("read-only Scripts tab should not render %q", forbidden)
		}
	}
	if !strings.Contains(html, "Published versions are immutable") {
		t.Error("expected immutability banner on the Scripts tab for a published version")
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

func TestPolicyModuleDetailPage_DependenciesTabSections(t *testing.T) {
	d := moduleEditorTestData()
	d.DependsOn = []policymodules.Dependency{{ModuleID: "tenant.acme.base", VersionConstraint: ">=1.0.0"}}
	d.Conflicts = []string{"tenant.acme.enemy"}
	d.Conditions = []db.ModuleVersionCondition{
		{ID: 1, Kind: "param", ParamPath: "computer/hardware/os_family", Operator: "in", ValueJson: `["linux"]`},
		{ID: 2, Kind: "command", ScriptSource: "uname -r", Operator: "contains", ValueJson: `["3"]`},
		{ID: 3, Kind: "script", ScriptRef: "custom.sh", Operator: "contains", ValueJson: `["example"]`},
	}
	d.PickerModules = []ModulePickerOption{{URN: "tenant.acme.base", Title: "Base"}}
	html := renderToString(t, PolicyModuleDetailPage(d))

	for _, want := range []string{
		`data-testid="pm-deps-section"`, `data-testid="pm-conflicts-section"`,
		`data-testid="pm-tests-section"`, "Reusable dependency groups",
		"&gt;=1.0.0", "tenant.acme.enemy",
		"OS family · is any of · linux",
		"Bash · uname -r · contains · 3",
		"Script · custom.sh · contains · example",
		`name="version_constraint"`, `id="condition-builder"`,
		"Add test", "Add dependency", "Add conflict",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("dependencies tab missing %q", want)
		}
	}
}

func TestPolicyModuleDetailPage_DependenciesTabReadOnly(t *testing.T) {
	d := moduleEditorTestData()
	d.IsDraft = false
	d.Selected.State = "published"
	d.DependsOn = []policymodules.Dependency{{ModuleID: "tenant.acme.base", VersionConstraint: "*"}}
	d.Conditions = []db.ModuleVersionCondition{
		{ID: 1, Kind: "param", ParamPath: "computer/hardware/os_family", Operator: "in", ValueJson: `["linux"]`},
	}
	html := renderToString(t, PolicyModuleDetailPage(d))

	for _, forbidden := range []string{"Add test", "Add dependency", "Add conflict", `name="version_constraint"`} {
		if strings.Contains(html, forbidden) {
			t.Errorf("read-only dependencies tab should not render %q", forbidden)
		}
	}
	if !strings.Contains(html, "OS family · is any of · linux") {
		t.Error("read-only tab should still render test rows")
	}
	if !strings.Contains(html, `href="/policy/modules/tenant.acme.base"`) {
		t.Error("read-only dependency rows should link to the referenced module")
	}
}
