package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/pkg/auth"
)

func runIdentityBulk(t *testing.T, h *Handler, sess *auth.UserSession, body IdentityBulkRequest) (PolicyModuleBulkResponse, error) {
	t.Helper()
	req := jsonRequest(http.MethodPost, "/api/users/bulk", body, sess)
	c, rec := newEchoCtx(req)
	if err := h.IdentityBulk(c); err != nil {
		return PolicyModuleBulkResponse{}, err
	}
	var response PolicyModuleBulkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response, nil
}

func TestIdentityBulkDeleteRestoreAndPurge(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "identity-actor")
	targetID := newEditorIdentity(t, h.db, tenantID, "identity-target")
	sess := adminEditorSession(tenantID, actorID)
	rawID := strconv.FormatInt(targetID, 10)
	for _, action := range []string{"delete", "restore", "delete", "purge"} {
		response, err := runIdentityBulk(t, h, sess, IdentityBulkRequest{Action: action, IDs: []string{rawID}})
		if err != nil || len(response.OK) != 1 || len(response.Failed) != 0 {
			t.Fatalf("%s response = %+v, %v", action, response, err)
		}
	}
}

func TestUsersDeletedFilterRendersDeletedRowsAndActions(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "identity-filter-actor")
	targetID := newEditorIdentity(t, h.db, tenantID, "identity-filter-target")
	sess := adminEditorSession(tenantID, actorID)
	if err := h.identitySvc.Delete(context.Background(), tenantID, targetID, actorID); err != nil {
		t.Fatal(err)
	}
	req := formRequest(http.MethodGet, "/users?state=deleted", nil, sess)
	c, rec := newEchoCtx(req)
	if err := h.Users(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{`value="deleted" selected`, `data-select-id="` + strconv.FormatInt(targetID, 10) + `"`, `data-select-caps="restore,purge"`, `Selected users move to Deleted`} {
		if !strings.Contains(body, want) {
			t.Fatalf("deleted users page missing %q", want)
		}
	}
}

func TestIdentityBulkCannotDeleteSelf(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "identity-self")
	sess := &auth.UserSession{TenantID: tenantID, IdentityID: actorID, Role: identities.RoleAdmin, Grants: adminEditorSession(tenantID, actorID).Grants}
	response, err := runIdentityBulk(t, h, sess, IdentityBulkRequest{Action: "delete", IDs: []string{strconv.FormatInt(actorID, 10)}})
	if err != nil || len(response.Failed) != 1 {
		t.Fatalf("self-delete response = %+v, %v", response, err)
	}
}
