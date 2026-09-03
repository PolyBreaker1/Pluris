package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
)

func runGroupBulk(t *testing.T, h *Handler, sess *auth.UserSession, body GroupBulkRequest) (PolicyModuleBulkResponse, error) {
	t.Helper()
	req := jsonRequest(http.MethodPost, "/api/groups/bulk", body, sess)
	c, rec := newEchoCtx(req)
	if err := h.GroupBulk(c); err != nil {
		return PolicyModuleBulkResponse{}, err
	}
	var response PolicyModuleBulkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response, nil
}

func TestGroupBulkDeleteRestoreAndPurge(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "group-bulk")
	sess := adminEditorSession(tenantID, actorID)
	group, err := h.groupSvc.Create(context.Background(), tenantID, "Bulk group", "", services.MemberKindMixed, services.MembershipStatic, "security", "global")
	if err != nil {
		t.Fatal(err)
	}
	rawID := strconv.FormatInt(group.ID, 10)
	for _, action := range []string{"delete", "restore", "delete", "purge"} {
		response, err := runGroupBulk(t, h, sess, GroupBulkRequest{Action: action, IDs: []string{rawID}})
		if err != nil || len(response.OK) != 1 || len(response.Failed) != 0 {
			t.Fatalf("%s response = %+v, %v", action, response, err)
		}
	}
}

func TestGroupsDeletedFilterRendersDeletedRowsAndActions(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "group-filter")
	sess := adminEditorSession(tenantID, actorID)
	group, err := h.groupSvc.Create(context.Background(), tenantID, "Deleted group", "", services.MemberKindIdentity, services.MembershipStatic, "security", "global")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.groupSvc.Delete(context.Background(), tenantID, group.ID, actorID); err != nil {
		t.Fatal(err)
	}
	req := formRequest(http.MethodGet, "/groups?kind=identity&state=deleted", nil, sess)
	c, rec := newEchoCtx(req)
	if err := h.GroupsList(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{`data-select-id="` + strconv.FormatInt(group.ID, 10) + `"`, `data-select-caps="restore,purge"`, `Selected groups move to Deleted`, `state=deleted`} {
		if !strings.Contains(body, want) {
			t.Fatalf("deleted groups page missing %q", want)
		}
	}
}
