package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/services"
	"github.com/pluris/pluris/web/templates"
)

const manageDataPermission = "server_admin.manage_data"

func (h *Handler) DataManagement(c echo.Context) error {
	if err := requirePermission(c, manageDataPermission); err != nil {
		return err
	}
	settings, err := h.retentionSvc.ListSettings(c.Request().Context())
	if err != nil {
		return err
	}
	return render(c, templates.DataManagementPage(settings, csrfTokenFrom(c), c.QueryParam("message"), ""))
}

func (h *Handler) DataManagementSave(c echo.Context) error {
	if err := requirePermission(c, manageDataPermission); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	kind := strings.TrimSpace(c.FormValue("entity_kind"))
	mode := strings.TrimSpace(c.FormValue("mode"))
	var days *int64
	if raw := strings.TrimSpace(c.FormValue("purge_after_days")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			return h.renderDataManagementError(c, "Retention days must be zero or greater, or blank for never.")
		}
		days = &parsed
	}
	if _, err := h.retentionSvc.UpdateSetting(ctx, kind, mode, days, sess.IdentityID); err != nil {
		if err == services.ErrInvalidEntityKind || err == services.ErrInvalidRetentionMode || err == services.ErrInvalidRetentionDays {
			return h.renderDataManagementError(c, err.Error())
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/server-admin/data?message=Saved")
}

func (h *Handler) renderDataManagementError(c echo.Context, message string) error {
	settings, err := h.retentionSvc.ListSettings(c.Request().Context())
	if err != nil {
		return err
	}
	c.Response().WriteHeader(http.StatusBadRequest)
	return render(c, templates.DataManagementPage(settings, csrfTokenFrom(c), "", message))
}
