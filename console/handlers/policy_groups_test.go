package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
)

// createSecondTenant creates an extra tenant for isolation assertions.
func createSecondTenant(t *testing.T, h *Handler, slug string) int64 {
	t.Helper()
	tenant, err := h.db.Queries.CreateTenant(context.Background(), db.CreateTenantParams{Name: slug, Slug: slug})
	if err != nil {
		t.Fatalf("create second tenant: %v", err)
	}
	return tenant.ID
}

// TestPolicyGroupsListPage covers the Task 5.2 list page: /policy/groups
// renders real, tenant-scoped Configuration Group rows through the
// web/lists registry (registry-driven columns), links rows to the
// detail page, carries no popup-dialog markup (the old cg-dialog is
// retired), and does not leak another tenant's groups.
//
// (The TargetPickerDialog mount moved from this list page to the detail
// page in Task 5.2 -- see TestPolicyGroupDetailPage in
// config_groups_test.go for the picker's real-data assertions that
// Task 5.1 originally pinned here.)
func TestPolicyGroupsListPage(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "policy_groups_list_test.db", "policy-groups-list-tenant")
	ctx := context.Background()

	if _, err := h.configGroupSvc.Create(ctx, tenantID, "Baseline workstations", "Security baseline", true); err != nil {
		t.Fatalf("create config group: %v", err)
	}

	// A second tenant's group must not appear.
	tenantB := createSecondTenant(t, h, "policy-groups-list-tenant-b")
	if _, err := h.configGroupSvc.Create(ctx, tenantB, "Tenant B only group", "", true); err != nil {
		t.Fatalf("create tenant b group: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/policy/groups", nil)
	req = req.WithContext(auth.WithSession(req.Context(), adminSession(tenantID)))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := h.PolicyGroups(c); err != nil {
		t.Fatalf("PolicyGroups failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, `data-testid="page-policy-groups"`) {
		t.Fatal("body missing page-policy-groups testid")
	}
	if !strings.Contains(body, "Baseline workstations") {
		t.Error("body missing the seeded group's name (real row not rendered)")
	}
	if strings.Contains(body, "Tenant B only group") {
		t.Error("tenant B's group leaked into tenant A's list")
	}
	// Registry-driven columns (web/lists/config_groups.go).
	for _, col := range []string{"Assignments", "Bindings", "Enabled", "Created"} {
		if !strings.Contains(body, ">"+col+"<") {
			t.Errorf("body missing registry column header %q", col)
		}
	}
	// The popup dialog is retired: no cg-dialog markup, no cg:save wiring.
	if strings.Contains(body, `id="cg-dialog"`) || strings.Contains(body, "cg:save") {
		t.Error("body still contains retired ConfigurationGroupDialog markup")
	}
	// Mock catalog retired -- its hand-curated rows must not render.
	if strings.Contains(body, "cg.baseline.workstations") || strings.Contains(body, "alice-laptop") {
		t.Error("body still contains rows from the retired mock")
	}
	// Create button links to the create page.
	if !strings.Contains(body, `href="/policy/groups/new"`) {
		t.Error("body missing the create-page link")
	}
}
