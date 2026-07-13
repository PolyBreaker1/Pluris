package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
)

// newAuthzTestContext builds a bare echo.Context carrying sess (or no
// session at all when sess is nil), mirroring the pattern used by
// roles_test.go for handler-level unit tests.
func newAuthzTestContext(sess *auth.UserSession) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := req.Context()
	if sess != nil {
		ctx = auth.WithSession(ctx, sess)
	}
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec)
}

// TestRequirePermission covers the Task 5 helper directly: a technician
// session is denied identity.delete but allowed identity.create; a nil
// session is always denied.
func TestRequirePermission(t *testing.T) {
	tech := &auth.UserSession{IdentityID: 1, TenantID: 1, Grants: authz.Grants(permissions.TemplateGrants("technician"))}

	c := newAuthzTestContext(tech)
	if err := requirePermission(c, "identity.delete"); err == nil {
		t.Fatal("technician identity.delete: want 403, got nil")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("technician identity.delete: want 403 HTTPError, got %v", err)
	}

	c = newAuthzTestContext(tech)
	if err := requirePermission(c, "identity.create"); err != nil {
		t.Fatalf("technician identity.create: want nil, got %v", err)
	}

	c = newAuthzTestContext(nil)
	if err := requirePermission(c, "identity.create"); err == nil {
		t.Fatal("nil session: want 403, got nil")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("nil session: want 403 HTTPError, got %v", err)
	}
}

// TestRequirePermissionScoped covers the scoped variant: a user-template
// session may act on its own identity but not another's, and a nil
// session is always denied.
func TestRequirePermissionScoped(t *testing.T) {
	user := &auth.UserSession{IdentityID: 42, TenantID: 1, Grants: authz.Grants(permissions.TemplateGrants("user"))}

	c := newAuthzTestContext(user)
	if err := requirePermissionScoped(c, "identity.update", 42); err != nil {
		t.Fatalf("own id: want nil, got %v", err)
	}

	c = newAuthzTestContext(user)
	if err := requirePermissionScoped(c, "identity.update", 99); err == nil {
		t.Fatal("other id: want 403, got nil")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("other id: want 403 HTTPError, got %v", err)
	}

	c = newAuthzTestContext(nil)
	if err := requirePermissionScoped(c, "identity.update", 42); err == nil {
		t.Fatal("nil session: want 403, got nil")
	} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusForbidden {
		t.Fatalf("nil session: want 403 HTTPError, got %v", err)
	}
}
