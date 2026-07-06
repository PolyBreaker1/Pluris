package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

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

	return assets.Asset{
		ID:                 dbAsset.Uuid, // Use UUID as ID
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
