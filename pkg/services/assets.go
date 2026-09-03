package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// timeParseDate parses a "YYYY-MM-DD" date string, the format
// coerceParamValue leaves TypeDate values in and web/lists/assets.go's
// getAssetParamValue renders them back as.
func timeParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// AssetService handles asset operations
type AssetService struct {
	db *database.Database
}

// NewAssetService creates a new asset service
func NewAssetService(db *database.Database) *AssetService {
	return &AssetService{db: db}
}

// ListSoftware returns the installed-software inventory rows for an
// asset, identified by its numeric DB id (see ResolveDBID).
func (s *AssetService) ListSoftware(ctx context.Context, assetID int64) ([]db.InstalledSoftware, error) {
	return s.db.Queries.ListSoftwareForAsset(ctx, assetID)
}

// ListActivity returns the most recent activity-log rows for an asset
// (entity_type "asset"), newest first, capped at limit.
func (s *AssetService) ListActivity(ctx context.Context, tenantID, assetID, limit int64) ([]db.ActivityLog, error) {
	return s.db.Queries.ListActivityForEntity(ctx, db.ListActivityForEntityParams{
		TenantID:   tenantID,
		EntityType: "asset",
		EntityID:   assetID,
		Limit:      limit,
	})
}

// ListBySubtype fetches all assets of a given subtype for tenant
func (s *AssetService) ListBySubtype(ctx context.Context, tenantID int64, subtype string) ([]assets.Asset, error) {
	dbAssets, err := s.db.Queries.ListAssetsBySubtype(ctx, db.ListAssetsBySubtypeParams{
		TenantID: tenantID,
		Subtype:  subtype,
		Limit:    10000, // Reasonable upper limit
		Offset:   0,
	})
	if err != nil {
		return nil, err
	}

	// Convert db rows to catalog assets.Asset
	result := make([]assets.Asset, len(dbAssets))
	for i, row := range dbAssets {
		result[i] = s.convertRowToAsset(row)
	}

	return result, nil
}

// ListDeletedBySubtype is the explicit recycle-bin view. Normal asset reads
// remain active-only.
func (s *AssetService) ListDeletedBySubtype(ctx context.Context, tenantID int64, subtype string) ([]assets.Asset, error) {
	rows, err := s.db.Queries.ListDeletedAssetsBySubtype(ctx, db.ListDeletedAssetsBySubtypeParams{
		TenantID: tenantID, Subtype: subtype, Limit: 10000, Offset: 0,
	})
	if err != nil {
		return nil, err
	}
	result := make([]assets.Asset, len(rows))
	for i, row := range rows {
		result[i] = convertDBAssetToAsset(row)
	}
	return result, nil
}

// Delete follows the asset retention setting.
func (s *AssetService) Delete(ctx context.Context, tenantID int64, identifier string, actorID int64) error {
	row, err := s.db.Queries.GetAssetForDeletion(ctx, db.GetAssetForDeletionParams{TenantID: tenantID, Identifier: sql.NullString{String: identifier, Valid: true}})
	if err != nil {
		return err
	}
	setting, err := s.db.Queries.GetRetentionSetting(ctx, EntityKindAsset)
	if err != nil {
		return err
	}
	if setting.Mode == RetentionModeImmediate {
		return s.db.Queries.DeleteAsset(ctx, row.ID)
	}
	_, err = s.db.Queries.SoftDeleteAsset(ctx, db.SoftDeleteAssetParams{DeletedBy: actorID, ID: row.ID, TenantID: tenantID})
	return err
}

func (s *AssetService) Restore(ctx context.Context, tenantID int64, identifier string) error {
	row, err := s.db.Queries.GetAssetForDeletion(ctx, db.GetAssetForDeletionParams{TenantID: tenantID, Identifier: sql.NullString{String: identifier, Valid: true}})
	if err != nil {
		return err
	}
	_, err = s.db.Queries.RestoreAsset(ctx, db.RestoreAssetParams{ID: row.ID, TenantID: tenantID})
	return err
}

func (s *AssetService) PermanentlyDelete(ctx context.Context, tenantID int64, identifier string) error {
	row, err := s.db.Queries.GetAssetForDeletion(ctx, db.GetAssetForDeletionParams{TenantID: tenantID, Identifier: sql.NullString{String: identifier, Valid: true}})
	if err != nil {
		return err
	}
	return s.db.Queries.DeleteAsset(ctx, row.ID)
}

// GetByUUID fetches a single asset by UUID
func (s *AssetService) GetByUUID(ctx context.Context, uuid string) (*assets.Asset, error) {
	dbAsset, err := s.db.Queries.GetAssetByUUID(ctx, uuid)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	asset := convertDBAssetToAsset(dbAsset)
	return &asset, nil
}

// GetByID fetches a single asset by human ID or UUID
func (s *AssetService) GetByID(ctx context.Context, id string) (*assets.Asset, error) {
	// Try by human_id first
	row, err := s.db.Queries.GetAssetByHumanID(ctx, sql.NullString{String: id, Valid: true})
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if err == nil {
		// Found by human_id - convert GetAssetByHumanIDRow to asset
		asset := s.convertHumanIDRowToAsset(row)
		return &asset, nil
	}

	// Try by UUID as fallback
	dbAsset, err := s.db.Queries.GetAssetByUUID(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found
		}
		return nil, err
	}

	// For UUID lookup, we don't have the JOIN data, so convert with empty values
	asset := convertDBAssetToAsset(dbAsset)
	return &asset, nil
}

// SetOwner assigns ownerID as the owning identity of assetID.
func (s *AssetService) SetOwner(ctx context.Context, assetID, ownerID int64) error {
	if err := s.db.Queries.SetAssetOwner(ctx, db.SetAssetOwnerParams{
		ID:      assetID,
		OwnerID: sql.NullInt64{Int64: ownerID, Valid: true},
	}); err != nil {
		return fmt.Errorf("set owner on asset %d: %w", assetID, err)
	}
	return nil
}

// ClearOwner removes any owning identity from assetID.
func (s *AssetService) ClearOwner(ctx context.Context, assetID int64) error {
	if err := s.db.Queries.ClearAssetOwner(ctx, assetID); err != nil {
		return fmt.Errorf("clear owner on asset %d: %w", assetID, err)
	}
	return nil
}

// ResolveDBID looks up the numeric database primary key for an asset
// identified by its human-readable ID or UUID (the same two lookup
// paths GetByID already tries). Needed because catalog/assets.Asset.ID
// is a string (human ID or UUID), but SetOwner/ClearOwner operate on
// the numeric DB primary key.
func (s *AssetService) ResolveDBID(ctx context.Context, humanOrUUID string) (int64, error) {
	row, err := s.db.Queries.GetAssetByHumanID(ctx, sql.NullString{String: humanOrUUID, Valid: true})
	if err == nil {
		return row.ID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	dbAsset, err := s.db.Queries.GetAssetByUUID(ctx, humanOrUUID)
	if err != nil {
		return 0, fmt.Errorf("resolve asset db id for %q: %w", humanOrUUID, err)
	}
	return dbAsset.ID, nil
}

// assetColumnBackedKeys is the set of mounted asset param keys backed by
// a real assets-table column rather than the subtype_payload JSON blob
// (see UpdateFields doc comment). Membership here, not the param's Type,
// is what makes a non-hardware-section key editable: TypeString alone
// would also match structural/computed keys like "uuid" or "vendor" on
// subtypes where it happens to live in the hardware section, so routing
// keys off an explicit table keeps identity/enrollment's readonly and
// TypeLink keys (id, uuid, tenant, site, owner, groups, managed_by,
// enrollment_state, enrolled_at, last_seen_at, agent_version) rejected.
var assetColumnBackedKeys = map[string]bool{
	"description":      true,
	"lifecycle_state":  true,
	"vendor":           true,
	"location":         true,
	"purchase_date":    true,
	"warranty_expires": true,
}

// UpdateFields applies a partial set of param-keyed field updates to
// assetID, scoped to tenantID and subtype (a mismatch on either --
// wrong tenant or wrong subtype for this asset's route -- fails closed
// with ErrFieldNotFound, same as an unknown id). Any mounted section of
// the subtype schema may be targeted; each submitted key is routed by
// where it actually lives:
//   - The subtype schema's "hardware" section merges into the
//     subtype_payload JSON blob (TypeLink and TypeCompound params are
//     never editable there -- links resolve through other entities;
//     compounds have no single scalar value to coerce).
//   - assetColumnBackedKeys (description, lifecycle_state, vendor,
//     location, purchase_date, warranty_expires) are real assets-table
//     columns, applied via UpdateAssetEditableColumns.
//   - Everything else (id, uuid, tenant, site, owner, groups,
//     managed_by, enrollment_state, enrolled_at, last_seen_at,
//     agent_version, ...) is computed, a link, or agent-controlled, and
//     is rejected with a 400 naming the key -- the detail-page UI marks
//     these spans data-editable for uniform click-to-copy styling, but
//     they were never meant to be user-editable through this endpoint.
//
// On success, changed payload keys are persisted via UpdateAssetPayload
// and changed column keys via UpdateAssetEditableColumns (each fetched
// from the current row first, so a request touching only one column
// cannot clobber the others), and the list of applied keys is returned.
func (s *AssetService) UpdateFields(ctx context.Context, tenantID, assetID int64, subtype, section string, fields map[string]string) ([]string, error) {
	row, err := s.db.Queries.GetAsset(ctx, assetID)
	if err != nil {
		return nil, ErrFieldNotFound
	}
	if row.TenantID != tenantID || row.Subtype != subtype {
		return nil, ErrFieldNotFound
	}

	schema := params.SchemaBySubtype(subtype)
	if schema == nil {
		return nil, fmt.Errorf("%w: unknown subtype %q", ErrFieldValidation, subtype)
	}
	sec := sectionByKey(schema, section)
	if sec == nil {
		return nil, fmt.Errorf("%w: unknown section %q", ErrFieldValidation, section)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(row.SubtypePayload), &payload); err != nil || payload == nil {
		payload = make(map[string]interface{})
	}
	payloadTouched := false

	cols := db.UpdateAssetEditableColumnsParams{
		ID:                assetID,
		Description:       row.Description,
		LifecycleState:    row.LifecycleState,
		Vendor:            row.Vendor,
		Location:          row.Location,
		PurchaseDate:      row.PurchaseDate,
		WarrantyExpiresAt: row.WarrantyExpiresAt,
	}
	columnsTouched := false

	updated := make([]string, 0, len(fields))
	for key, raw := range fields {
		if !sectionHasParam(sec, key) {
			return nil, fieldErr(key, "not a field of section %q", section)
		}
		def := params.DefByKey(key)
		if def == nil {
			return nil, fieldErr(key, "unknown parameter")
		}

		switch {
		case assetColumnBackedKeys[key]:
			val, err := coerceParamValue(def, raw)
			if err != nil {
				return nil, fieldErr(key, "%s", err)
			}
			if err := applyAssetColumnField(&cols, key, val); err != nil {
				return nil, fieldErr(key, "%s", err)
			}
			columnsTouched = true
		case section == "hardware":
			if def.Type == params.TypeLink || def.Type == params.TypeCompound {
				return nil, fieldErr(key, "not editable")
			}
			val, err := coerceParamValue(def, raw)
			if err != nil {
				return nil, fieldErr(key, "%s", err)
			}
			payload[key] = val
			payloadTouched = true
		default:
			return nil, fieldErr(key, "not editable")
		}
		updated = append(updated, key)
	}

	if payloadTouched {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal asset %d payload: %w", assetID, err)
		}
		if err := s.db.Queries.UpdateAssetPayload(ctx, db.UpdateAssetPayloadParams{
			Payload: string(encoded),
			ID:      assetID,
		}); err != nil {
			return nil, fmt.Errorf("update asset %d payload: %w", assetID, err)
		}
	}
	if columnsTouched {
		if err := s.db.Queries.UpdateAssetEditableColumns(ctx, cols); err != nil {
			return nil, fmt.Errorf("update asset %d columns: %w", assetID, err)
		}
	}
	return updated, nil
}

// applyAssetColumnField sets the field on cols corresponding to key,
// given val already coerced by coerceParamValue to key's ParamDef type
// (a string for every column-backed key -- see coerceParamValue and
// assetColumnBackedKeys). An empty raw value clears the column (NULL)
// rather than erroring, matching the inline-edit UI's "clear and save"
// affordance. Date keys are parsed as YYYY-MM-DD, the same format
// web/lists/assets.go's getAssetParamValue renders them in.
func applyAssetColumnField(cols *db.UpdateAssetEditableColumnsParams, key string, val interface{}) error {
	s, _ := val.(string)
	switch key {
	case "description":
		cols.Description = sql.NullString{String: s, Valid: s != ""}
	case "lifecycle_state":
		cols.LifecycleState = sql.NullString{String: s, Valid: s != ""}
	case "vendor":
		cols.Vendor = sql.NullString{String: s, Valid: s != ""}
	case "location":
		cols.Location = sql.NullString{String: s, Valid: s != ""}
	case "purchase_date":
		if s == "" {
			cols.PurchaseDate = sql.NullTime{}
			return nil
		}
		t, err := timeParseDate(s)
		if err != nil {
			return fmt.Errorf("expected a date (YYYY-MM-DD), got %q", s)
		}
		cols.PurchaseDate = sql.NullTime{Time: t, Valid: true}
	case "warranty_expires":
		if s == "" {
			cols.WarrantyExpiresAt = sql.NullTime{}
			return nil
		}
		t, err := timeParseDate(s)
		if err != nil {
			return fmt.Errorf("expected a date (YYYY-MM-DD), got %q", s)
		}
		cols.WarrantyExpiresAt = sql.NullTime{Time: t, Valid: true}
	default:
		return fmt.Errorf("unhandled column-backed key %q", key)
	}
	return nil
}

// convertHumanIDRowToAsset converts db.GetAssetByHumanIDRow to catalog assets.Asset
func (s *AssetService) convertHumanIDRowToAsset(row db.GetAssetByHumanIDRow) assets.Asset {
	// Parse JSON payload
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(row.SubtypePayload), &payload); err != nil {
		payload = make(map[string]interface{})
	}

	// Build subtype-specific payload
	var typedPayload assets.SubtypePayload

	switch row.Subtype {
	case "computer":
		typedPayload = buildComputerPayload(payload)
	case "server":
		typedPayload = buildServerPayload(payload)
	case "printer":
		typedPayload = buildPrinterPayload(payload)
	case "desk":
		typedPayload = buildDeskPayload(payload)
	default:
		typedPayload = assets.ComputerPayload{}
	}

	// Parse labels
	labels := make(map[string]string)
	if row.Labels.Valid && row.Labels.String != "" {
		json.Unmarshal([]byte(row.Labels.String), &labels)
	}

	// Use human_id as primary ID
	id := row.HumanID.String
	if id == "" {
		id = row.Uuid
	}

	// Build base asset
	asset := assets.Asset{
		ID:                 id,
		UUID:               row.Uuid,
		TenantID:           row.TenantSlug.String, // From JOIN
		Subtype:            assets.Subtype(row.Subtype),
		Payload:            typedPayload,
		Site:               row.SiteName.String, // From JOIN
		Groups:             []string{},          // TODO: populate from group_memberships
		Labels:             labels,
		EnrollmentState:    assets.EnrollmentState(row.EnrollmentState),
		EnrolledAt:         row.EnrolledAt.Time,
		LastSeenAt:         row.LastSeenAt.Time,
		AgentVersion:       row.AgentVersion.String,
		LifecycleState:     assets.LifecycleState(row.LifecycleState.String),
		Location:           row.Location.String,
		OwnerIdentity:      row.OwnerName.String, // From JOIN
		Description:        row.Description.String,
		ManagedBy:          row.ManagedByName.String, // From JOIN
		Vendor:             row.Vendor.String,
		PurchaseDate:       row.PurchaseDate.Time,
		PurchasePriceCents: int64(row.PurchasePrice.Float64 * 100),
		WarrantyExpiresAt:  row.WarrantyExpiresAt.Time,
	}

	return asset
}

// convertRowToAsset converts db.ListAssetsBySubtypeRow to catalog assets.Asset
// This handles JOIN query results with site_name, owner_name, tenant_slug
func (s *AssetService) convertRowToAsset(row db.ListAssetsBySubtypeRow) assets.Asset {
	// Parse JSON payload
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(row.SubtypePayload), &payload); err != nil {
		payload = make(map[string]interface{})
	}

	// Build subtype-specific payload
	var typedPayload assets.SubtypePayload

	switch row.Subtype {
	case "computer":
		typedPayload = buildComputerPayload(payload)
	case "server":
		typedPayload = buildServerPayload(payload)
	case "printer":
		typedPayload = buildPrinterPayload(payload)
	case "desk":
		typedPayload = buildDeskPayload(payload)
	default:
		typedPayload = assets.ComputerPayload{}
	}

	// Parse labels JSON
	labels := make(map[string]string)
	if row.Labels.Valid && row.Labels.String != "" {
		json.Unmarshal([]byte(row.Labels.String), &labels)
	}

	// Resolve groups (TODO: implement separate query)
	groups := []string{}

	// Use human_id if available, otherwise fallback to UUID
	id := row.HumanID.String
	if !row.HumanID.Valid || id == "" {
		id = row.Uuid
	}

	return assets.Asset{
		ID:                 id,
		UUID:               row.Uuid,
		TenantID:           row.TenantSlug.String, // From JOIN
		Subtype:            assets.Subtype(row.Subtype),
		Payload:            typedPayload,
		Site:               row.SiteName.String, // From JOIN
		Groups:             groups,
		Labels:             labels,
		EnrollmentState:    assets.EnrollmentState(row.EnrollmentState),
		EnrolledAt:         row.EnrolledAt.Time,
		LastSeenAt:         row.LastSeenAt.Time,
		AgentVersion:       row.AgentVersion.String,
		LifecycleState:     assets.LifecycleState(row.LifecycleState.String),
		Location:           row.Location.String,
		OwnerIdentity:      row.OwnerName.String, // From JOIN
		Description:        row.Description.String,
		ManagedBy:          row.ManagedByName.String, // From JOIN
		Vendor:             row.Vendor.String,
		PurchaseDate:       row.PurchaseDate.Time,
		PurchasePriceCents: int64(row.PurchasePrice.Float64 * 100),
		WarrantyExpiresAt:  row.WarrantyExpiresAt.Time,
	}
}

// convertDBAssetToAsset converts db.Asset to catalog assets.Asset (for single
// asset queries). Package-level (not a method) so other services in this
// package — e.g. IdentityService.ListAssignedAssets — can reuse it without
// needing an AssetService instance.
func convertDBAssetToAsset(dbAsset db.Asset) assets.Asset {
	// Parse JSON payload
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(dbAsset.SubtypePayload), &payload); err != nil {
		payload = make(map[string]interface{})
	}

	// Build subtype-specific payload
	var typedPayload assets.SubtypePayload

	switch dbAsset.Subtype {
	case "computer":
		typedPayload = buildComputerPayload(payload)
	case "server":
		typedPayload = buildServerPayload(payload)
	case "printer":
		typedPayload = buildPrinterPayload(payload)
	case "desk":
		typedPayload = buildDeskPayload(payload)
	default:
		// Fallback to empty computer payload
		typedPayload = assets.ComputerPayload{}
	}

	// Parse labels JSON
	labels := make(map[string]string)
	if dbAsset.Labels.Valid && dbAsset.Labels.String != "" {
		json.Unmarshal([]byte(dbAsset.Labels.String), &labels)
	}

	id := dbAsset.Uuid
	if dbAsset.HumanID.Valid && dbAsset.HumanID.String != "" {
		id = dbAsset.HumanID.String
	}
	return assets.Asset{
		ID:                 id,
		UUID:               dbAsset.Uuid,
		TenantID:           fmt.Sprintf("%d", dbAsset.TenantID),
		Subtype:            assets.Subtype(dbAsset.Subtype),
		Payload:            typedPayload,
		Site:               "",         // TODO: join with sites table
		Groups:             []string{}, // TODO: join with groups table
		Labels:             labels,
		EnrollmentState:    assets.EnrollmentState(dbAsset.EnrollmentState),
		EnrolledAt:         dbAsset.EnrolledAt.Time,
		LastSeenAt:         dbAsset.LastSeenAt.Time,
		AgentVersion:       dbAsset.AgentVersion.String,
		LifecycleState:     assets.LifecycleState(dbAsset.LifecycleState.String),
		Location:           dbAsset.Location.String,
		OwnerIdentity:      "", // TODO: join with identities table
		Description:        dbAsset.Description.String,
		ManagedBy:          "", // TODO: join with identities table
		Vendor:             dbAsset.Vendor.String,
		PurchaseDate:       dbAsset.PurchaseDate.Time,
		PurchasePriceCents: int64(dbAsset.PurchasePrice.Float64 * 100), // Convert to cents
		WarrantyExpiresAt:  dbAsset.WarrantyExpiresAt.Time,
	}
}

func buildComputerPayload(payload map[string]interface{}) assets.ComputerPayload {
	hostname, _ := payload["hostname"].(string)
	fqdn, _ := payload["fqdn"].(string)
	osFamily, _ := payload["os_family"].(string)
	osDistribution, _ := payload["os_distribution"].(string)
	osVersion, _ := payload["os_version"].(string)
	kernelVersion, _ := payload["kernel_version"].(string)
	architecture, _ := payload["architecture"].(string)
	cpuSummary, _ := payload["cpu_summary"].(string)

	ramMB := 0
	if v, ok := payload["ram_mb"].(float64); ok {
		ramMB = int(v)
	}

	storageMB := 0
	if v, ok := payload["storage_mb"].(float64); ok {
		storageMB = int(v)
	}

	return assets.ComputerPayload{
		Hostname:       hostname,
		FQDN:           fqdn,
		OSFamily:       osFamily,
		OSDistribution: osDistribution,
		OSVersion:      osVersion,
		KernelVersion:  kernelVersion,
		Architecture:   architecture,
		CPUSummary:     cpuSummary,
		RAMMB:          ramMB,
		StorageMB:      storageMB,
	}
}

func buildServerPayload(payload map[string]interface{}) assets.ServerPayload {
	hostname, _ := payload["hostname"].(string)
	fqdn, _ := payload["fqdn"].(string)
	osFamily, _ := payload["os_family"].(string)
	osDistribution, _ := payload["os_distribution"].(string)
	osVersion, _ := payload["os_version"].(string)
	kernelVersion, _ := payload["kernel_version"].(string)
	architecture, _ := payload["architecture"].(string)
	role, _ := payload["server_role"].(string)

	return assets.ServerPayload{
		Hostname:       hostname,
		FQDN:           fqdn,
		OSFamily:       osFamily,
		OSDistribution: osDistribution,
		OSVersion:      osVersion,
		KernelVersion:  kernelVersion,
		Architecture:   architecture,
		Role:           assets.ServerRole(role),
		Services:       []assets.ServerService{}, // TODO: parse from JSON
	}
}

func buildPrinterPayload(payload map[string]interface{}) assets.PrinterPayload {
	model, _ := payload["model"].(string)
	vendor, _ := payload["vendor"].(string)
	ip, _ := payload["ip"].(string)

	return assets.PrinterPayload{
		Model:  model,
		Vendor: vendor,
		IP:     ip,
		Queues: []string{}, // TODO: parse from JSON
	}
}

func buildDeskPayload(payload map[string]interface{}) assets.DeskPayload {
	locationLabel, _ := payload["location_label"].(string)
	dockModel, _ := payload["dock_model"].(string)
	monitorCount := 0
	if v, ok := payload["monitor_count"].(float64); ok {
		monitorCount = int(v)
	}

	return assets.DeskPayload{
		LocationLabel: locationLabel,
		DockModel:     dockModel,
		MonitorCount:  monitorCount,
	}
}
