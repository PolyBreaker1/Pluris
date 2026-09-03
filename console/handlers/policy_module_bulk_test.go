package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/permissions"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
	"github.com/pluris/pluris/pkg/services"
)

func runModuleBulk(t *testing.T, h *Handler, sess *auth.UserSession, reqBody PolicyModuleBulkRequest) (PolicyModuleBulkResponse, error) {
	t.Helper()
	req := jsonRequest(http.MethodPost, "/api/modules/bulk", reqBody, sess)
	c, rec := newEchoCtx(req)
	err := h.PolicyModuleBulk(c)
	if err != nil {
		return PolicyModuleBulkResponse{}, err
	}
	var resp PolicyModuleBulkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return resp, nil
}

func createPublishedModuleForBulk(t *testing.T, h *Handler, tenantID, ownerID int64, urn string) db.PolicyModuleVersion {
	t.Helper()
	ctx := context.Background()
	mod, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, urn, urn, "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := h.moduleSvc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.moduleSvc.SetScript(ctx, draft.ID, "apply", "apply.sh", "#!/bin/sh\ntrue\n"); err != nil {
		t.Fatal(err)
	}
	if err := h.moduleSvc.Publish(ctx, draft.ID, ownerID); err != nil {
		t.Fatal(err)
	}
	return draft
}

func TestPolicyModuleBulkCloneHappyPath(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "bulk-cloner")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	if err := h.moduleSvc.SeedBundled(ctx); err != nil {
		t.Fatal(err)
	}
	const urn = "pluris.sshd.password-auth-disable"

	resp, err := runModuleBulk(t, h, sess, PolicyModuleBulkRequest{Action: "clone", IDs: []string{urn}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.OK) != 1 || resp.OK[0] != urn || len(resp.Failed) != 0 {
		t.Fatalf("response = %+v", resp)
	}
	mods, err := h.moduleSvc.ListModules(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) < 4 {
		t.Fatalf("clone was not added; visible modules = %d", len(mods))
	}
}

func TestPolicyModuleBulkRevokeHappyPath(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "bulk-revoker")
	sess := adminEditorSession(tenantID, ownerID)
	draft := createPublishedModuleForBulk(t, h, tenantID, ownerID, "tenant.acme.revoke")

	resp, err := runModuleBulk(t, h, sess, PolicyModuleBulkRequest{Action: "revoke", IDs: []string{"tenant.acme.revoke"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.OK) != 1 || len(resp.Failed) != 0 {
		t.Fatalf("response = %+v", resp)
	}
	version, err := h.db.Queries.GetPolicyModuleVersion(context.Background(), draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if version.State != "revoked" {
		t.Fatalf("state = %q, want revoked", version.State)
	}
}

func TestPolicyModuleBulkDeleteMixedEligibility(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "bulk-deleter")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	if err := h.moduleSvc.SeedBundled(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.delete", "Delete", ""); err != nil {
		t.Fatal(err)
	}

	resp, err := runModuleBulk(t, h, sess, PolicyModuleBulkRequest{
		Action: "delete", IDs: []string{"pluris.sshd.password-auth-disable", "tenant.acme.delete"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.OK) != 1 || resp.OK[0] != "tenant.acme.delete" {
		t.Fatalf("ok = %+v", resp.OK)
	}
	if len(resp.Failed) != 1 || resp.Failed[0].ID != "pluris.sshd.password-auth-disable" || resp.Failed[0].Reason == "" {
		t.Fatalf("failed = %+v", resp.Failed)
	}
	if _, err := h.moduleSvc.GetModuleRow(ctx, "tenant.acme.delete"); err == nil {
		t.Fatal("eligible tenant module still exists")
	}
}

func TestPolicyModuleBulkDeleteReferencedSoftDeletes(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "bulk-ref")
	sess := adminEditorSession(tenantID, ownerID)
	ctx := context.Background()
	if _, err := h.moduleSvc.CreateModule(ctx, &tenantID, &ownerID, "tenant.acme.refbulk", "Referenced", ""); err != nil {
		t.Fatal(err)
	}
	group, err := h.depGroupSvc.Create(ctx, tenantID, "Bulk blocker", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.depGroupSvc.LinkModule(ctx, tenantID, "tenant.acme.refbulk", group.ID, "requirement"); err != nil {
		t.Fatal(err)
	}

	resp, err := runModuleBulk(t, h, sess, PolicyModuleBulkRequest{Action: "delete", IDs: []string{"tenant.acme.refbulk"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.OK) != 1 || len(resp.Failed) != 0 {
		t.Fatalf("response = %+v", resp)
	}
	row, err := h.db.Queries.GetPolicyModuleByURNIncludingDeleted(ctx, "tenant.acme.refbulk")
	if err != nil || row.DeletedAt == nil {
		t.Fatalf("referenced module was not soft deleted: %+v, %v", row, err)
	}
}

func TestPolicyModuleBulkRejectsUnauthorizedAndMalformedAction(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	ownerID := newEditorIdentity(t, h.db, tenantID, "bulk-owner")
	noGrantID := newEditorIdentity(t, h.db, tenantID, "bulk-no-grant")
	if _, err := h.moduleSvc.CreateModule(context.Background(), &tenantID, &ownerID, "tenant.acme.privatebulk", "Private", ""); err != nil {
		t.Fatal(err)
	}
	noGrant := &auth.UserSession{
		TenantID: tenantID, IdentityID: noGrantID, Role: identities.RoleUser,
		Grants: authz.Grants(permissions.TemplateGrants("user")),
	}
	_, err := runModuleBulk(t, h, noGrant, PolicyModuleBulkRequest{Action: "delete", IDs: []string{"tenant.acme.privatebulk"}})
	assertHTTPErrorStatus(t, err, http.StatusForbidden)

	admin := adminEditorSession(tenantID, ownerID)
	_, err = runModuleBulk(t, h, admin, PolicyModuleBulkRequest{Action: "explode", IDs: []string{"tenant.acme.privatebulk"}})
	assertHTTPErrorStatus(t, err, http.StatusBadRequest)
}

func assertHTTPErrorStatus(t *testing.T, err error, want int) {
	t.Helper()
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != want {
		t.Fatalf("error = %#v, want HTTP %d", err, want)
	}
}
