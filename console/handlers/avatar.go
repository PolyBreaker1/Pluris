package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/labstack/echo/v4"
)

// AvatarDir is the on-disk directory avatar files are written to and the
// directory the /avatars static route serves from (see
// console/server/server.go). A package-level var so tests can point it at
// a t.TempDir() instead of the real "data/avatars" working-directory path.
var AvatarDir = "data/avatars"

// avatarMaxBytes is the upload size cap enforced on the multipart file
// part (spec: <=2MB).
const avatarMaxBytes = 2 * 1024 * 1024

// avatarExtByContentType maps the sniffed (not client-supplied) MIME type
// to the file extension the upload is persisted under. Only these three
// image types are accepted; anything else is rejected with 400.
var avatarExtByContentType = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
}

// UserAvatarUpload handles POST /api/users/:id/avatar: a multipart form
// upload (file field "avatar") that becomes the target identity's profile
// picture. Route-level RBAC only requires an authenticated session (see
// server.go's registration comment for the sibling field-update API);
// this handler carries the actual authorization via
// requirePermissionScoped(identity.update) -- "all" scope may set anyone's
// avatar, "own" scope only the caller's own.
func (h *Handler) UserAvatarUpload(c echo.Context) error {
	ctx := c.Request().Context()

	targetID, err := h.resolveTenantIdentity(c)
	if err != nil {
		return err
	}
	if err := requirePermissionScoped(c, "identity.update", targetID); err != nil {
		return err
	}

	fileHeader, err := c.FormFile("avatar")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "missing avatar file")
	}
	if fileHeader.Size > avatarMaxBytes {
		return echo.NewHTTPError(http.StatusBadRequest, "avatar file too large (max 2MB)")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read avatar file")
	}
	defer src.Close()

	// Read the whole file into memory to (a) sniff its real content type
	// off the actual bytes -- never trust the client-supplied
	// Content-Type header -- and (b) enforce the size cap against the
	// true byte count regardless of what Content-Length claimed.
	data, err := io.ReadAll(io.LimitReader(src, avatarMaxBytes+1))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "could not read avatar file")
	}
	if len(data) > avatarMaxBytes {
		return echo.NewHTTPError(http.StatusBadRequest, "avatar file too large (max 2MB)")
	}

	sniffLen := len(data)
	if sniffLen > 512 {
		sniffLen = 512
	}
	detected := http.DetectContentType(data[:sniffLen])
	ext, ok := avatarExtByContentType[detected]
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "unsupported image type: "+detected)
	}

	if err := os.MkdirAll(AvatarDir, 0o755); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not prepare avatar storage")
	}

	// Best-effort: remove any previously-stored avatar for this id under a
	// different extension, so switching from e.g. .png to .jpg doesn't
	// leave a stale file the static route could still be asked to serve
	// by an old cached URL.
	for _, oldExt := range []string{".png", ".jpg", ".webp"} {
		if oldExt == ext {
			continue
		}
		_ = os.Remove(filepath.Join(AvatarDir, fmt.Sprintf("%d%s", targetID, oldExt)))
	}

	destPath := filepath.Join(AvatarDir, fmt.Sprintf("%d%s", targetID, ext))
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not save avatar")
	}

	avatarURL := fmt.Sprintf("/avatars/%d%s", targetID, ext)

	identity, err := h.identitySvc.Get(ctx, targetID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "user not found")
	}
	identity.AvatarURL = avatarURL
	if _, err := h.identitySvc.Update(ctx, identity); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not save avatar")
	}

	h.logFieldUpdateActivity(c, "identity", targetID, "user_updated", "avatar")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"updated": []string{"avatar_url"},
		"url":     avatarURL,
	})
}
