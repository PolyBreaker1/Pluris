package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/database"
)

// TestPolicyModulesRendersSeededModules covers Task 4.2's read-path
// swap: the /policy/modules Library page must render modules loaded
// from the DB (via PolicyModuleService.ListModules, seeded by
// SeedBundled on first request) rather than the old
// catalog/policymodules mock.AllModules() literal. Before this task the
// handler called policymodules.AllModules() directly; now it must
// reflect real persistence -- so this test asserts on a bundled module
// title that SeedBundled writes (see pkg/services/policymodules.go's
// bundledSeed table), confirming the page is DB-backed end to end.
func TestPolicyModulesRendersSeededModules(t *testing.T) {
	d, err := database.Open(filepath.Join(t.TempDir(), "policy_modules_handler_test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	tenant, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	h := New(d)
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/policy/modules", nil)
	req = req.WithContext(auth.WithSession(req.Context(), &auth.UserSession{
		TenantID: tenant.ID, IdentityID: 1, Role: identities.RoleAdmin,
		Grants: authz.Grants(permissions.TemplateGrants("admin")),
	}))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.PolicyModules(c); err != nil {
		t.Fatalf("PolicyModules: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Disable SSH password authentication") {
		t.Errorf("response missing seeded bundled module title; body did not contain expected text")
	}
	if !strings.Contains(body, "pluris.sshd.password-auth-disable") {
		t.Errorf("response missing seeded module URN")
	}
	if !strings.Contains(body, `data-row-href="/policy/modules/pluris.sshd.password-auth-disable"`) {
		t.Errorf("module row must use the shared row-navigation contract")
	}
	if strings.Contains(body, ">View</a>") || strings.Contains(body, ">Edit</span>") {
		t.Errorf("module list must not render a redundant Open/Edit/View action")
	}

	// Confirm the module actually landed in the DB (not just something
	// the handler happened to render): SeedBundled is idempotent, so a
	// second call to the service directly must see the same module.
	mods, err := h.moduleSvc.ListModules(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListModules: %v", err)
	}
	found := false
	for _, m := range mods {
		if m.ID == "pluris.sshd.password-auth-disable" {
			found = true
		}
	}
	if !found {
		t.Fatal("seeded bundled module not present via ListModules")
	}
}
