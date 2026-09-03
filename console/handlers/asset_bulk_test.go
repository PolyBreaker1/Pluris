package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
)

func runAssetBulk(t *testing.T, h *Handler, sess *auth.UserSession, body AssetBulkRequest) (PolicyModuleBulkResponse, error) {
	t.Helper()
	req := jsonRequest(http.MethodPost, "/api/assets/bulk", body, sess)
	c, rec := newEchoCtx(req)
	if err := h.AssetBulk(c); err != nil {
		return PolicyModuleBulkResponse{}, err
	}
	var response PolicyModuleBulkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response, nil
}

func TestAssetBulkDeleteRestoreAndPurge(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "asset-bulk")
	sess := adminEditorSession(tenantID, actorID)
	ctx := context.Background()
	asset, err := h.db.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid: "asset-bulk-uuid", TenantID: tenantID, Subtype: "computer",
		SubtypePayload: `{"hostname":"bulk-asset"}`, EnrollmentState: "enrolled",
		HumanID: sql.NullString{String: "comp.bulk.asset", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, action := range []string{"delete", "restore", "delete", "purge"} {
		response, err := runAssetBulk(t, h, sess, AssetBulkRequest{Action: action, IDs: []string{asset.HumanID.String}})
		if err != nil || len(response.OK) != 1 || len(response.Failed) != 0 {
			t.Fatalf("%s response = %+v, %v", action, response, err)
		}
	}
	if _, err := h.db.Queries.GetAssetForDeletion(ctx, db.GetAssetForDeletionParams{TenantID: tenantID, Identifier: asset.HumanID}); err == nil {
		t.Fatal("permanently deleted asset still exists")
	}
}

func TestAssetsDeletedFilterRendersDeletedRowsAndActions(t *testing.T) {
	h, _, tenantID := newModuleEditorTestDB(t)
	actorID := newEditorIdentity(t, h.db, tenantID, "asset-filter")
	sess := adminEditorSession(tenantID, actorID)
	ctx := context.Background()
	asset, err := h.db.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid: "asset-filter-uuid", TenantID: tenantID, Subtype: "computer",
		SubtypePayload: `{"hostname":"deleted-filter"}`, EnrollmentState: "enrolled",
		HumanID: sql.NullString{String: "comp.deleted.filter", Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.assetSvc.Delete(ctx, tenantID, asset.HumanID.String, actorID); err != nil {
		t.Fatal(err)
	}
	req := formRequest(http.MethodGet, "/assets/computers?state=deleted", nil, sess)
	c, rec := newEchoCtx(req)
	if err := h.AssetsComputers(c); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{`value="deleted" selected`, `data-select-id="comp.deleted.filter"`, `data-select-caps="restore,purge"`, `Selected assets move to Deleted`} {
		if !strings.Contains(body, want) {
			t.Fatalf("deleted assets page missing %q: %s", want, body)
		}
	}
}
