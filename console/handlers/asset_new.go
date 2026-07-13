package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/web/templates"
)

// assetNewSubtypeSlugs maps the plural route segment used by
// /assets/:subtype/... to the singular schema subtype value stored in
// assets.subtype, and the short human_id prefix used by cmd/seed's
// humanID() helper (comp/srv/prn/desk) so freshly-enrolled rows sort and
// read consistently alongside seeded ones.
var assetNewSubtypeSlugs = map[string]struct {
	subtype string
	prefix  string
}{
	"computers": {"computer", "comp"},
	"servers":   {"server", "srv"},
	"printers":  {"printer", "prn"},
	"desks":     {"desk", "desk"},
}

// AssetNewShow renders the minimal "create then edit inline" enrollment
// form for the given asset subtype. No wizard: the owner's design intent
// is a fast create that lands on the real (working) detail page, where
// every other field is filled in via the existing inline per-section edit.
func (h *Handler) AssetNewShow(c echo.Context) error {
	if err := requirePermission(c, "asset.create"); err != nil {
		return err
	}
	slug := c.Param("subtype")
	if _, ok := assetNewSubtypeSlugs[slug]; !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown asset subtype")
	}
	return render(c, templates.AssetNewPage(slug, csrfTokenFrom(c), ""))
}

// AssetCreateSubmit creates a minimal asset row from the enrollment form
// and redirects straight to its detail page, mirroring the seeder's
// human_id / uuid / subtype_payload shape (cmd/seed/main.go) so new rows
// are indistinguishable from seeded ones.
func (h *Handler) AssetCreateSubmit(c echo.Context) error {
	if err := requirePermission(c, "asset.create"); err != nil {
		return err
	}
	slug := c.Param("subtype")
	info, ok := assetNewSubtypeSlugs[slug]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown asset subtype")
	}

	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)

	name := c.FormValue("name")
	hostname := c.FormValue("hostname")
	if name == "" {
		return render(c, templates.AssetNewPage(slug, csrfTokenFrom(c), "Name is required."))
	}

	tenant, err := h.db.Queries.GetTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}

	seq, err := h.db.Queries.CountAssetsBySubtype(ctx, db.CountAssetsBySubtypeParams{
		TenantID: sess.TenantID,
		Subtype:  info.subtype,
	})
	if err != nil {
		return err
	}
	humanID := fmt.Sprintf("%s.%s.web.%04d", info.prefix, tenant.Slug, seq+1)

	payload, err := assetNewMinimalPayload(info.subtype, name, hostname)
	if err != nil {
		return err
	}

	asset, err := h.db.Queries.CreateAsset(ctx, db.CreateAssetParams{
		Uuid:            newAssetUUID(),
		TenantID:        sess.TenantID,
		Subtype:         info.subtype,
		SubtypePayload:  payload,
		EnrollmentState: "pending",
		Labels:          sql.NullString{String: "{}", Valid: true},
		HumanID:         sql.NullString{String: humanID, Valid: true},
	})
	if err != nil {
		log.Printf("asset create failed: %v", err)
		return render(c, templates.AssetNewPage(slug, csrfTokenFrom(c), "Could not create asset. Please try again."))
	}

	if sess != nil {
		_ = h.db.Queries.InsertActivity(ctx, db.InsertActivityParams{
			TenantID:        sess.TenantID,
			EntityType:      "asset",
			EntityID:        asset.ID,
			Event:           "asset_created",
			Detail:          sql.NullString{String: humanID, Valid: true},
			ActorIdentityID: sql.NullInt64{Int64: sess.IdentityID, Valid: true},
		})
	}

	return c.Redirect(http.StatusFound, "/assets/"+slug+"/"+humanID)
}

// assetNewMinimalPayload builds the smallest subtype_payload JSON that
// keeps the asset's primary display field (Asset.PrimaryHostname, see
// catalog/assets/types.go) non-empty: hostname for computer/server,
// model for printer, location_label for desk. Every other field is left
// unset and filled in later via the working inline section editor.
func assetNewMinimalPayload(subtype, name, hostname string) (string, error) {
	var payload map[string]any
	switch subtype {
	case "computer", "server":
		h := hostname
		if h == "" {
			h = name
		}
		payload = map[string]any{"hostname": h, "os_family": ""}
	case "printer":
		payload = map[string]any{"model": name}
	case "desk":
		payload = map[string]any{"location_label": name}
	default:
		payload = map[string]any{}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// newAssetUUID returns a random RFC 4122 v4 UUID string. Not security
// sensitive — it only needs to satisfy assets.uuid's UNIQUE constraint —
// so crypto/rand is used purely for convenient uniform byte output, same
// rationale as console/handlers/auth.go's randomSuffix.
func newAssetUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read failing is effectively unrecoverable on any
		// real system; fall back to a fixed-but-still-unique-enough
		// value rather than panicking a request handler.
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", 0)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]))
}
