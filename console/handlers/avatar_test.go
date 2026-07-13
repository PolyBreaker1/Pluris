package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/pkg/auth"
)

// tinyPNGBase64 is the canonical 67-byte 1x1 transparent PNG, used across
// these tests as a minimal valid image upload.
const tinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

func tinyPNGBytes(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(tinyPNGBase64)
	if err != nil {
		t.Fatalf("decode tiny png: %v", err)
	}
	return data
}

// newAvatarUploadReq builds a multipart POST request with a single
// "avatar" file part named filename, containing data, targeting the
// avatar-upload endpoint for the given identity id.
func newAvatarUploadReq(t *testing.T, id int64, filename string, data []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("avatar", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	target := "/api/users/" + strconv.FormatInt(id, 10) + "/avatar"
	req := httptest.NewRequest(http.MethodPost, target, &buf)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	return req
}

// avatarUploadContext wires sess into the request context and sets the
// :id route param, mirroring fieldUpdateContext in field_api_test.go.
func avatarUploadContext(e *echo.Echo, req *http.Request, sess *auth.UserSession, id int64) (echo.Context, *httptest.ResponseRecorder) {
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(id, 10))
	return c, rec
}

// (a) A user-template session uploads their OWN avatar -> 200, file
// written to AvatarDir, identity.avatar_url set to the expected path.
func TestUserAvatarUploadOwnSuccess(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "avatar_own_test.db", "avatar-own-tenant")
	AvatarDir = t.TempDir()
	ctx := context.Background()
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "self-user")
	sess := &auth.UserSession{
		TenantID: tenantID, IdentityID: target, Role: "user",
		Grants: userSession(tenantID).Grants,
	}

	req := newAvatarUploadReq(t, target, "avatar.png", tinyPNGBytes(t))
	c, rec := avatarUploadContext(e, req, sess, target)

	if err := h.UserAvatarUpload(c); err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "\"url\":\"/avatars/"+strconv.FormatInt(target, 10)+".png\"") {
		t.Errorf("response body missing expected url: %s", rec.Body.String())
	}

	updated, err := h.identitySvc.Get(ctx, target)
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	wantURL := "/avatars/" + strconv.FormatInt(target, 10) + ".png"
	if updated.AvatarURL != wantURL {
		t.Errorf("avatar_url = %q, want %q", updated.AvatarURL, wantURL)
	}
}

// (b) Non-image content (text/plain-ish bytes) -> 400.
func TestUserAvatarUploadWrongContentType(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "avatar_badtype_test.db", "avatar-badtype-tenant")
	AvatarDir = t.TempDir()
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "badtype-user")
	sess := adminSession(tenantID)

	req := newAvatarUploadReq(t, target, "notes.txt", []byte("this is definitely not an image, just plain ASCII text"))
	c, rec := avatarUploadContext(e, req, sess, target)

	err := h.UserAvatarUpload(c)
	if err == nil {
		t.Fatalf("expected error, got 200: %s", rec.Body.String())
	}
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// (c) Oversized upload (>2MB) -> 400.
func TestUserAvatarUploadTooLarge(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "avatar_toolarge_test.db", "avatar-toolarge-tenant")
	AvatarDir = t.TempDir()
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "toolarge-user")
	sess := adminSession(tenantID)

	big := bytes.Repeat([]byte{0xFF}, avatarMaxBytes+1024)
	req := newAvatarUploadReq(t, target, "big.png", big)
	c, rec := avatarUploadContext(e, req, sess, target)

	err := h.UserAvatarUpload(c)
	if err == nil {
		t.Fatalf("expected error, got 200: %s", rec.Body.String())
	}
	mustHTTPStatus(t, err, http.StatusBadRequest)
}

// (d) A user-template session targets ANOTHER user's avatar -> 403.
func TestUserAvatarUploadOtherUserForbidden(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "avatar_other_forbidden_test.db", "avatar-other-tenant")
	AvatarDir = t.TempDir()
	e := echo.New()

	actor := createTestIdentityForPlurisTest(t, h, tenantID, "actor-user")
	target := createTestIdentityForPlurisTest(t, h, tenantID, "other-user")
	sess := &auth.UserSession{
		TenantID: tenantID, IdentityID: actor, Role: "user",
		Grants: userSession(tenantID).Grants,
	}

	req := newAvatarUploadReq(t, target, "avatar.png", tinyPNGBytes(t))
	c, rec := avatarUploadContext(e, req, sess, target)

	err := h.UserAvatarUpload(c)
	if err == nil {
		t.Fatalf("expected error, got 200: %s", rec.Body.String())
	}
	mustHTTPStatus(t, err, http.StatusForbidden)
	_ = rec
}

// (e) An admin (all-scope) uploads an avatar for another user -> 200.
func TestUserAvatarUploadAdminForOther(t *testing.T) {
	h, tenantID := setupPlurisTestDB(t, "avatar_admin_other_test.db", "avatar-admin-tenant")
	AvatarDir = t.TempDir()
	ctx := context.Background()
	e := echo.New()

	target := createTestIdentityForPlurisTest(t, h, tenantID, "target-for-admin")
	sess := adminSession(tenantID)

	req := newAvatarUploadReq(t, target, "avatar.png", tinyPNGBytes(t))
	c, rec := avatarUploadContext(e, req, sess, target)

	if err := h.UserAvatarUpload(c); err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	updated, err := h.identitySvc.Get(ctx, target)
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	wantURL := "/avatars/" + strconv.FormatInt(target, 10) + ".png"
	if updated.AvatarURL != wantURL {
		t.Errorf("avatar_url = %q, want %q", updated.AvatarURL, wantURL)
	}
}
