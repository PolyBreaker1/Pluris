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

func runDependencyGroupBulk(t *testing.T, h *Handler, sess *auth.UserSession, body DependencyGroupBulkRequest) (PolicyModuleBulkResponse, error) {
	t.Helper()
	req := jsonRequest(http.MethodPost, "/api/dependency-groups/bulk", body, sess)
	c, rec := newEchoCtx(req)
	if err := h.DependencyGroupBulk(c); err != nil {
		return PolicyModuleBulkResponse{}, err
	}
	var response PolicyModuleBulkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response, nil
}

func TestDependencyGroupBulkDeleteRestoreAndPurge(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "dependency-group-bulk")
	sess := adminEditorSession(tenantID, actorID)
	group, err := h.depGroupSvc.Create(context.Background(), tenantID, "Bulk dependency group", "")
	if err != nil {
		t.Fatal(err)
	}
	rawID := strconv.FormatInt(group.ID, 10)
	for _, action := range []string{"delete", "restore", "delete", "purge"} {
		response, err := runDependencyGroupBulk(t, h, sess, DependencyGroupBulkRequest{Action: action, IDs: []string{rawID}})
		if err != nil || len(response.OK) != 1 || len(response.Failed) != 0 {
			t.Fatalf("%s response = %+v, %v", action, response, err)
		}
	}
}

func TestDependencyGroupsDeletedFilterRendersDeletedRowsAndActions(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "dependency-group-filter")
	sess := adminEditorSession(tenantID, actorID)
	group, err := h.depGroupSvc.Create(context.Background(), tenantID, "Deleted dependency group", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.depGroupSvc.Delete(context.Background(), tenantID, group.ID, actorID); err != nil {
		t.Fatal(err)
	}
	req := formRequest(http.MethodGet, "/policy/dependency-groups?state=deleted", nil, sess)
	c, rec := newEchoCtx(req)
	if err := h.DependencyGroups(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{`data-select-id="` + strconv.FormatInt(group.ID, 10) + `"`, `data-select-caps="restore,purge"`, `Selected dependency groups move to Deleted`, `state=deleted`} {
		if !strings.Contains(body, want) {
			t.Fatalf("deleted dependency groups page missing %q", want)
		}
	}
}
