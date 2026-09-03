package templates

import (
	"strings"
	"testing"

	"github.com/pluris/pluris/catalog/policymodules"
)

func TestPolicyModulesListMassActionContract(t *testing.T) {
	mods := []policymodules.Module{
		{ID: "pluris.bundled", Title: "Bundled", Origin: "bundled"},
		{ID: "tenant.acme.local", Title: "Tenant", Origin: "tenant"},
		{ID: "imported.community.one", Title: "Imported", Origin: "imported"},
	}
	html := renderToString(t, policyModulesList(mods, nil, nil, "csrf-token", true, false, ""))

	for _, want := range []string{
		`data-pluris-select="policy-modules"`,
		`data-pluris-select-all="policy-modules"`,
		`data-mass-action-toolbar="policy-modules"`,
		`data-mass-action="duplicate"`,
		`data-mass-action="revoke"`,
		`data-mass-action="delete"`,
		`id="mass-action-dialog"`,
		`data-select-caps="duplicate"`,
		`data-select-caps="duplicate,revoke,delete"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("module list missing %q", want)
		}
	}
	for _, unwanted := range []string{"pm-row-actions", ">Management<", "data-pmp-open", "return confirm("} {
		if strings.Contains(html, unwanted) {
			t.Errorf("module list still contains removed row-action contract %q", unwanted)
		}
	}
	if got := strings.Count(html, `href="/policy/modules/new"`); got != 1 {
		t.Errorf("new-module links = %d, want only the ConceptEmptyState CTA", got)
	}
}
