package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

func TestDataManagementPermissionAndSaveValidation(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "data-admin")

	denied := &auth.UserSession{TenantID: tenantID, IdentityID: actorID, Role: identities.RoleUser, Grants: authz.Grants{}}
	req := formRequest(http.MethodGet, "/server-admin/data", nil, denied)
	c, _ := newEchoCtx(req)
	if err := h.DataManagement(c); err == nil {
		t.Fatal("DataManagement without manage_data grant succeeded")
	} else {
		assertHTTPErrorStatus(t, err, http.StatusForbidden)
	}

	allowed := &auth.UserSession{TenantID: tenantID, IdentityID: actorID, Role: identities.RoleAdmin, Grants: authz.Grants{manageDataPermission: "yes"}}
	req = formRequest(http.MethodGet, "/server-admin/data", nil, allowed)
	c, rec := newEchoCtx(req)
	if err := h.DataManagement(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !containsAll(rec.Body.String(), `data-testid="page-data-management"`, `name="purge_after_days"`, `name="mode"`) {
		t.Fatalf("unexpected page: status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = formRequest(http.MethodPost, "/server-admin/data", url.Values{
		"entity_kind": {services.EntityKindPolicyModule}, "mode": {services.RetentionModeSoft}, "purge_after_days": {"-1"},
	}, allowed)
	c, rec = newEchoCtx(req)
	if err := h.DataManagementSave(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative days status = %d, want 400", rec.Code)
	}

	req = formRequest(http.MethodPost, "/server-admin/data", url.Values{
		"entity_kind": {services.EntityKindPolicyModule}, "mode": {services.RetentionModeImmediate}, "purge_after_days": {"30"},
	}, allowed)
	c, rec = newEchoCtx(req)
	if err := h.DataManagementSave(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("valid save status = %d, want 302", rec.Code)
	}
	setting, err := h.db.Queries.GetRetentionSetting(context.Background(), services.EntityKindPolicyModule)
	if err != nil {
		t.Fatal(err)
	}
	days, ok, err := testNullableInt64(setting.PurgeAfterDays)
	if err != nil || !ok || days != 30 || setting.Mode != services.RetentionModeImmediate {
		t.Fatalf("saved setting = %+v, days=%d ok=%v err=%v", setting, days, ok, err)
	}
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}

func testNullableInt64(value any) (int64, bool, error) {
	if value == nil {
		return 0, false, nil
	}
	if days, ok := value.(int64); ok {
		return days, true, nil
	}
	return 0, false, errors.New("unexpected retention value")
}
