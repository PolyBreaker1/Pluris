package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
)

// newJSONReq builds a POST request with a JSON body, matching how the
// real fetch() call in web/static/detail.js:saveSectionEdit shapes its
// request (Content-Type: application/json).
func newJSONReq(method, target string, body interface{}) *http.Request {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, target, bytes.NewReader(b))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	return req
}

// fieldUpdateContext builds an echo.Context for a field-update handler
// call: sess in the request context, :id (and optionally :subtype) route
// params set.
func fieldUpdateContext(e *echo.Echo, sess *auth.UserSession, body interface{}, idParam string, subtypeParam string) (echo.Context, *httptest.ResponseRecorder) {
	req := newJSONReq(http.MethodPost, "/api/x/fields", body)
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if subtypeParam != "" {
		c.SetParamNames("subtype", "id")
		c.SetParamValues(subtypeParam, idParam)
	} else {
		c.SetParamNames("id")
		c.SetParamValues(idParam)
	}
	return c, rec
}

// createTestAssetForFieldTest inserts a minimal computer asset with the
// given RAM (MB) in its subtype_payload, owned by ownerID (0 = unowned).
func createTestAssetForFieldTest(t *testing.T, h *Handler, tenantID, ownerID int64, ramMB int) int64 {
	t.Helper()
	ctx := context.Background()
	payload := map[string]interface{}{"hostname": "test-host", "ram_mb": ramMB}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	asset, err := h.db.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "uuid-" + strconv.Itoa(int(tenantID)) + "-" + strconv.Itoa(ramMB) + "-" + strconv.Itoa(int(ownerID)),
		TenantID:        tenantID,
		Subtype:         "computer",
		SubtypePayload:  string(encoded),
		EnrollmentState: "enrolled",
	})
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if ownerID != 0 {
		if err := h.assetSvc.SetOwner(ctx, asset.ID, ownerID); err != nil {
			t.Fatalf("set owner: %v", err)
		}
	}
	return asset.ID
}

// (a) Admin edits another user's email + title -> 200, DB reflects it.
func TestUserFieldUpdateAdminEditsOther(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_admin_edit_test.db", "field-admin-tenant")
	ctx := context.Background()
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "target-user")
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{
		Section: "identity",
		Fields:  map[string]string{"email": "new@example.com"},
	}
	c, rec := fieldUpdateContext(e, sess, body, strconv.FormatInt(target, 10), "")
	if err := h.UserFieldUpdate(c); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	updated, err := h.identitySvc.Get(ctx, target)
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if updated.Email != "new@example.com" {
		t.Errorf("email = %q, want new@example.com", updated.Email)
	}

	// Task 8 review Finding 3: UserFieldUpdate must also append an
	// activity_log row (h.logFieldUpdateActivity) so the edit shows up in
	// the target's Activity tab -- not just the field-update response.
	activity, err := h.db.Queries.ListActivityForEntity(ctx, db.ListActivityForEntityParams{
		TenantID:   tenantID,
		EntityType: "identity",
		EntityID:   target,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	found := false
	for _, a := range activity {
		if a.Event == "user_updated" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a %q activity_log row for identity %d, got %+v", "user_updated", target, activity)
	}

	// title lives in the "organization" section.
	body2 := FieldUpdateRequest{Section: "organization", Fields: map[string]string{"title": "Staff Engineer"}}
	c2, rec2 := fieldUpdateContext(e, sess, body2, strconv.FormatInt(target, 10), "")
	if err := h.UserFieldUpdate(c2); err != nil {
		t.Fatalf("title update failed: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec2.Code)
	}
	updated2, err := h.identitySvc.Get(ctx, target)
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if updated2.Title != "Staff Engineer" {
		t.Errorf("title = %q, want Staff Engineer", updated2.Title)
	}
}

// Hotfix 2026-07-09: editing role through the identity field API must be
// rejected -- role is managed exclusively via the Roles tab
// (identity.assign_roles), never as a text field. This is the service-level
// half of the guard shared with the UI via identities.NonEditableFieldKeys
// (web/templates/users.templ's userGeneralTab reads the same map to decide
// which fields even render as inputs).
func TestUserFieldUpdateRoleRejected(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_role_rejected_test.db", "field-role-tenant")
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "role-target-user")
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{Section: "identity", Fields: map[string]string{"role": "admin"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(target, 10), "")
	err := h.UserFieldUpdate(c)
	if err == nil {
		t.Fatal("expected validation error for role field update")
	}
	mustHTTPStatus(t, err, http.StatusBadRequest)
	if !strings.Contains(err.Error(), "role") {
		t.Errorf("error = %q, want it to name the rejected field %q", err.Error(), "role")
	}

	// display_name, in the same "identity" section, must still be editable.
	body2 := FieldUpdateRequest{Section: "identity", Fields: map[string]string{"display_name": "New Name"}}
	c2, rec2 := fieldUpdateContext(e, sess, body2, strconv.FormatInt(target, 10), "")
	if err := h.UserFieldUpdate(c2); err != nil {
		t.Fatalf("display_name update failed: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec2.Code, rec2.Body.String())
	}
}

// (b) User-template session edits OWN allowlisted field (phone_mobile) -> 200.
func TestUserFieldUpdateSelfServiceAllowlisted(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_self_allow_test.db", "field-self-tenant")
	ctx := context.Background()
	e := echo.New()

	selfID := createTestIdentityForPlurisTest(t, h, tenantID, "self-user")
	sess := &auth.UserSession{
		TenantID: tenantID, IdentityID: selfID, Role: identities.RoleUser,
		Grants: userSession(tenantID).Grants,
	}

	body := FieldUpdateRequest{Section: "contact", Fields: map[string]string{"phone_mobile": "555-1234"}}
	c, rec := fieldUpdateContext(e, sess, body, strconv.FormatInt(selfID, 10), "")
	if err := h.UserFieldUpdate(c); err != nil {
		t.Fatalf("self-service update failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	updated, err := h.identitySvc.Get(ctx, selfID)
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if updated.PhoneMobile != "555-1234" {
		t.Errorf("phone_mobile = %q, want 555-1234", updated.PhoneMobile)
	}
}

// (c) User edits own NON-allowlisted field (department) -> 403.
func TestUserFieldUpdateSelfServiceNonAllowlisted(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_self_deny_test.db", "field-self-deny-tenant")
	e := echo.New()

	selfID := createTestIdentityForPlurisTest(t, h, tenantID, "self-user2")
	sess := &auth.UserSession{
		TenantID: tenantID, IdentityID: selfID, Role: identities.RoleUser,
		Grants: userSession(tenantID).Grants,
	}

	body := FieldUpdateRequest{Section: "organization", Fields: map[string]string{"department": "Engineering"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(selfID, 10), "")
	err := h.UserFieldUpdate(c)
	if err == nil {
		t.Fatal("expected forbidden error for non-allowlisted self-service field")
	}
	mustHTTPStatus(t, err, http.StatusForbidden)
}

// (d) User edits ANOTHER user -> 403.
func TestUserFieldUpdateOtherUserForbidden(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_other_forbidden_test.db", "field-other-tenant")
	e := echo.New()

	selfID := createTestIdentityForPlurisTest(t, h, tenantID, "self-user3")
	other := createTestIdentityForPlurisTest(t, h, tenantID, "other-user")
	sess := &auth.UserSession{
		TenantID: tenantID, IdentityID: selfID, Role: identities.RoleUser,
		Grants: userSession(tenantID).Grants,
	}

	body := FieldUpdateRequest{Section: "contact", Fields: map[string]string{"phone_mobile": "555-0000"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(other, 10), "")
	err := h.UserFieldUpdate(c)
	if err == nil {
		t.Fatal("expected forbidden error editing another user")
	}
	mustHTTPStatus(t, err, http.StatusForbidden)
}

// (e) Unknown section -> 400.
func TestUserFieldUpdateUnknownSection(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_unknown_section_test.db", "field-unknown-section-tenant")
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "target-user2")
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{Section: "not-a-real-section", Fields: map[string]string{"email": "x@example.com"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(target, 10), "")
	err := h.UserFieldUpdate(c)
	if err == nil {
		t.Fatal("expected validation error for unknown section")
	}
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// (f) Key not in that section -> 400.
func TestUserFieldUpdateKeyNotInSection(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_key_not_in_section_test.db", "field-key-section-tenant")
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "target-user3")
	sess := adminSession(tenantID)

	// "email" belongs to "identity", not "contact".
	body := FieldUpdateRequest{Section: "contact", Fields: map[string]string{"email": "x@example.com"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(target, 10), "")
	err := h.UserFieldUpdate(c)
	if err == nil {
		t.Fatal("expected validation error for key not in section")
	}
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// (g) Cross-tenant target -> 404.
func TestUserFieldUpdateCrossTenant404(t *testing.T) {
	h, tenantAID := setupPlurisTestDB(t, "field_cross_tenant_test.db", "field-cross-a")
	ctx := context.Background()
	e := echo.New()

	tenantB, err := h.db.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "field-cross-b", Slug: "field-cross-b"})
	if err != nil {
		t.Fatalf("create tenant b: %v", err)
	}
	targetInA := createTestIdentityForPlurisTest(t, h, tenantAID, "cross-target")

	sess := &auth.UserSession{
		TenantID: tenantB.ID, IdentityID: 999, Role: identities.RoleAdmin,
		Grants: adminSession(tenantB.ID).Grants,
	}
	body := FieldUpdateRequest{Section: "identity", Fields: map[string]string{"email": "x@example.com"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(targetInA, 10), "")
	err = h.UserFieldUpdate(c)
	if err == nil {
		t.Fatal("expected not-found error for cross-tenant target")
	}
	mustHTTPStatus(t, err, http.StatusNotFound)
}

// (h) Asset: admin updates ram_mb -> 200, payload JSON reflects it.
func TestAssetFieldUpdateAdminUpdatesRAM(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_asset_ram_test.db", "field-asset-tenant")
	ctx := context.Background()
	e := echo.New()

	assetID := createTestAssetForFieldTest(t, h, tenantID, 0, 4096)
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{Section: "hardware", Fields: map[string]string{"ram_mb": "8192"}}
	c, rec := fieldUpdateContext(e, sess, body, strconv.FormatInt(assetID, 10), "computer")
	if err := h.AssetFieldUpdate(c); err != nil {
		t.Fatalf("asset field update failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	row, err := h.db.Queries.GetAsset(ctx, assetID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(row.SubtypePayload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	ram, ok := payload["ram_mb"].(float64)
	if !ok || int(ram) != 8192 {
		t.Errorf("payload ram_mb = %v, want 8192", payload["ram_mb"])
	}
}

// (i) Asset: bad int coercion (ram_mb = "notanumber") -> 400.
func TestAssetFieldUpdateBadIntCoercion(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_asset_badint_test.db", "field-asset-badint-tenant")
	e := echo.New()

	assetID := createTestAssetForFieldTest(t, h, tenantID, 0, 4096)
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{Section: "hardware", Fields: map[string]string{"ram_mb": "notanumber"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(assetID, 10), "computer")
	err := h.AssetFieldUpdate(c)
	if err == nil {
		t.Fatal("expected validation error for bad int coercion")
	}
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// Bool coercion failure ("notabool") on an identity bool param -> 400.
func TestUserFieldUpdateBadBoolCoercion(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_bad_bool_test.db", "field-bad-bool-tenant")
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "target-user4")
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{Section: "security", Fields: map[string]string{"account_enabled": "notabool"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(target, 10), "")
	err := h.UserFieldUpdate(c)
	if err == nil {
		t.Fatal("expected validation error for bad bool coercion")
	}
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// Task 8 review Finding 2 regression: the asset detail UI's "Lifecycle"
// tile renders location/vendor/purchase_date/warranty_expires/
// lifecycle_state with the exact same [data-editable] affordance as the
// "Hardware" tile, but UpdateFields used to hard-reject every section
// except "hardware" -- every one of those real saves 400ed. "location"
// is a genuine assets-table column (see db/schema/001_initial.sql), so
// a "lifecycle" section edit must now succeed and persist to that
// column via the new UpdateAssetEditableColumns query.
func TestAssetFieldUpdateLifecycleSectionColumnEdit(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_asset_lifecycle_test.db", "field-asset-lifecycle-tenant")
	ctx := context.Background()
	e := echo.New()

	assetID := createTestAssetForFieldTest(t, h, tenantID, 0, 4096)
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{Section: "lifecycle", Fields: map[string]string{"location": "HQ - Floor 3"}}
	c, rec := fieldUpdateContext(e, sess, body, strconv.FormatInt(assetID, 10), "computer")
	if err := h.AssetFieldUpdate(c); err != nil {
		t.Fatalf("lifecycle-section update failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	row, err := h.db.Queries.GetAsset(ctx, assetID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if !row.Location.Valid || row.Location.String != "HQ - Floor 3" {
		t.Errorf("location column = %+v, want %q", row.Location, "HQ - Floor 3")
	}
}

// Fix pass 2 regression: the printer subtype mounts "vendor" in its
// "hardware" section (catalog/params/schemas.go), but vendor reads always
// come from the assets-table column (web/lists/assets.go's
// getAssetParamValue). UpdateFields used to route on section=="hardware"
// before checking assetColumnBackedKeys, so a hardware-section vendor
// edit silently wrote into subtype_payload instead of the column: 200
// returned, but the column (and thus the UI) never changed. Routing
// column-backed keys first regardless of section fixes this.
func TestAssetFieldUpdatePrinterVendorRoutesToColumn(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_asset_printer_vendor_test.db", "field-asset-printer-vendor-tenant")
	ctx := context.Background()
	e := echo.New()

	payload := map[string]interface{}{"printer_model": "LaserJet"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	asset, err := h.db.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            "uuid-printer-vendor-test",
		TenantID:        tenantID,
		Subtype:         "printer",
		SubtypePayload:  string(encoded),
		EnrollmentState: "enrolled",
	})
	if err != nil {
		t.Fatalf("create printer asset: %v", err)
	}
	assetID := asset.ID
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{Section: "hardware", Fields: map[string]string{"vendor": "Acme Printers"}}
	c, rec := fieldUpdateContext(e, sess, body, strconv.FormatInt(assetID, 10), "printer")
	if err := h.AssetFieldUpdate(c); err != nil {
		t.Fatalf("printer vendor update failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	row, err := h.db.Queries.GetAsset(ctx, assetID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if !row.Vendor.Valid || row.Vendor.String != "Acme Printers" {
		t.Errorf("vendor column = %+v, want %q", row.Vendor, "Acme Printers")
	}

	var gotPayload map[string]interface{}
	if err := json.Unmarshal([]byte(row.SubtypePayload), &gotPayload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if _, ok := gotPayload["vendor"]; ok {
		t.Errorf("subtype_payload contains %q key, want it absent: %+v", "vendor", gotPayload)
	}
}

// Task 8 review Finding 2 regression: a computed/structural key (uuid)
// must still 400 even though the UI marks its span [data-editable] --
// it is neither a subtype_payload key nor one of the real editable
// assets-table columns.
func TestAssetFieldUpdateReadonlyKeyRejected(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "field_asset_readonly_test.db", "field-asset-readonly-tenant")
	e := echo.New()

	assetID := createTestAssetForFieldTest(t, h, tenantID, 0, 4096)
	sess := adminSession(tenantID)

	body := FieldUpdateRequest{Section: "identity", Fields: map[string]string{"uuid": "some-new-uuid"}}
	c, _ := fieldUpdateContext(e, sess, body, strconv.FormatInt(assetID, 10), "computer")
	err := h.AssetFieldUpdate(c)
	if err == nil {
		t.Fatal("expected validation error for readonly key uuid")
	}
	mustHTTPStatus(t, err, http.StatusBadRequest)
}
