package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/pluris/pluris/pkg/auth"
)

func runConfigGroupBulk(t *testing.T, h *Handler, sess *auth.UserSession, body ConfigGroupBulkRequest) (PolicyModuleBulkResponse, error) {
	t.Helper()
	req := jsonRequest(http.MethodPost, "/api/config-groups/bulk", body, sess)
	c, rec := newEchoCtx(req)
	if err := h.ConfigGroupBulk(c); err != nil {
		return PolicyModuleBulkResponse{}, err
	}
	var response PolicyModuleBulkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response, nil
}

func TestConfigGroupBulkDeleteRestoreAndPurge(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "config-group-bulk")
	sess := adminEditorSession(tenantID, actorID)
	group, err := h.configGroupSvc.Create(context.Background(), tenantID, "Bulk configuration group", "", true)
	if err != nil {
		t.Fatal(err)
	}
	rawID := strconv.FormatInt(group.ID, 10)
	for _, action := range []string{"delete", "restore", "delete", "purge"} {
		response, err := runConfigGroupBulk(t, h, sess, ConfigGroupBulkRequest{Action: action, IDs: []string{rawID}})
		if err != nil || len(response.OK) != 1 || len(response.Failed) != 0 {
			t.Fatalf("%s response = %+v, %v", action, response, err)
		}
	}
}

func TestConfigGroupsDeletedFilterRendersDeletedRowsAndActions(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "config-group-filter")
	sess := adminEditorSession(tenantID, actorID)
	group, err := h.configGroupSvc.Create(context.Background(), tenantID, "Deleted configuration group", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.configGroupSvc.Delete(context.Background(), tenantID, group.ID, actorID); err != nil {
		t.Fatal(err)
	}
	req := formRequest(http.MethodGet, "/policy/groups?state=deleted", nil, sess)
	c, rec := newEchoCtx(req)
	if err := h.PolicyGroups(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{`data-select-id="` + strconv.FormatInt(group.ID, 10) + `"`, `data-select-caps="restore,purge"`, `Selected configuration groups move to Deleted`, `state=deleted`} {
		if !strings.Contains(body, want) {
			t.Fatalf("deleted configuration groups page missing %q", want)
		}
	}
}
