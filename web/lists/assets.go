// Package lists — Assets column registry.
//
// IMPORTANT: This file derives ALL column definitions from the param
// registry (catalog/params). The param system is the single source of
// truth. Labels, sort types, groups all come from ParamDef and
// SubtypeSchema. No independent field declarations.
//
// The only asset-specific logic here is the cell renderer (how to
// extract and format a value from an assets.Asset for display).
package lists

import (
	"strconv"
	"strings"
	"time"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/catalog/params"
)

// init registers the Assets list with fields derived from the param registry.
func init() {
	groups, fields := buildAssetsFieldsFromParams()
	Register(ListIDAssets, "Assets", groups, fields)
}

// buildAssetsFieldsFromParams generates FieldGroup and FieldDef slices
// from the param schemas. It collects all unique params across all
// subtype schemas (computers+servers+printers+desks) so the unified
// assets table can show any parameter from any subtype.
func buildAssetsFieldsFromParams() ([]FieldGroup, []FieldDef) {
	// Collect unique sections in schema-declaration order.
	// Use the computer schema as primary ordering reference (largest subtype).
	primary := params.SchemaComputer
	sectionOrder := make([]params.SchemaSection, 0, len(primary.Sections))
	sectionSeen := map[string]bool{}

	// First: sections from computer schema
	for _, sec := range primary.Sections {
		sectionOrder = append(sectionOrder, sec)
		sectionSeen[sec.Key] = true
	}
	// Then: any sections from other schemas not already seen
	for _, schema := range []*params.SubtypeSchema{params.SchemaServer, params.SchemaPrinter, params.SchemaDesk} {
		for _, sec := range schema.Sections {
			if !sectionSeen[sec.Key] {
				sectionOrder = append(sectionOrder, sec)
				sectionSeen[sec.Key] = true
			}
		}
	}

	// Build groups from sections
	groups := make([]FieldGroup, 0, len(sectionOrder))
	for _, sec := range sectionOrder {
		groups = append(groups, FieldGroup{
			Key:   sec.Key,
			Label: sec.Label,
		})
	}

	// Collect all unique param keys across all schemas, preserving section grouping.
	// Each param becomes one FieldDef.
	paramSeen := map[string]bool{}
	fields := make([]FieldDef, 0, 40)

	// Collect default columns from ALL schemas for DefaultVisible
	defaultCols := map[string]bool{}
	for _, schema := range params.Schemas {
		for _, key := range schema.DefaultColumns {
			defaultCols[key] = true
		}
	}

	for _, sec := range sectionOrder {
		// Collect params from this section across all schemas
		var sectionParams []string
		seen := map[string]bool{}
		for _, schema := range []*params.SubtypeSchema{params.SchemaComputer, params.SchemaServer, params.SchemaPrinter, params.SchemaDesk} {
			for _, s := range schema.Sections {
				if s.Key != sec.Key {
					continue
				}
				for _, key := range s.Params {
					if !seen[key] && !paramSeen[key] {
						sectionParams = append(sectionParams, key)
						seen[key] = true
					}
				}
			}
		}

		for _, key := range sectionParams {
			paramSeen[key] = true
			def := params.DefByKey(key)
			if def == nil {
				continue
			}

			// Skip compound sub-fields (they're rendered as part of their parent)
			if def.Type == params.TypeCompound {
				continue
			}

			fields = append(fields, FieldDef{
				Key:            def.Key,
				Label:          def.Label,
				Description:    def.Description,
				Group:          sec.Key,
				Width:          paramWidth(def),
				Sort:           string(def.Sort),
				DefaultVisible: defaultCols[def.Key],
			})
		}
	}

	return groups, fields
}

// paramWidth assigns a CSS width hint based on parameter type and key.
func paramWidth(def *params.ParamDef) string {
	switch def.Type {
	case params.TypeInt, params.TypeFloat:
		return "8%"
	case params.TypeBool:
		return "6%"
	case params.TypeEnum:
		return "10%"
	case params.TypeTime, params.TypeDate:
		return "12%"
	case params.TypeList, params.TypeMap:
		return "14%"
	}
	// String/Link widths by convention
	switch {
	case def.Key == "name":
		return "18%"
	case def.Key == "id" || def.Key == "uuid":
		return "14%"
	case def.Mono:
		return "12%"
	}
	return "12%"
}

// =========================================================================
// Cell Renderer — extracts and formats values from assets.Asset
//
// Uses param keys directly. The renderer knows how to get each param's
// value from the Asset struct (which has a typed Payload per subtype).
// =========================================================================

// RenderAssetCell returns the HTML content for a table cell.
func RenderAssetCell(key string, a assets.Asset) string {
	val := getAssetParamValue(key, a)
	def := params.DefByKey(key)
	if def == nil {
		return esc(val)
	}
	return formatParamValue(val, def, a)
}

// RenderAssetCellSort returns the sortable value for data-sort attribute.
func RenderAssetCellSort(key string, a assets.Asset) string {
	return getAssetParamValue(key, a)
}

// GetAssetRawValue returns the raw string value of a param for clipboard/display.
func GetAssetRawValue(key string, a assets.Asset) string {
	return getAssetParamValue(key, a)
}

// getAssetParamValue extracts the raw string value of a param from an Asset.
// This is the single function that maps param keys to struct fields.
func getAssetParamValue(key string, a assets.Asset) string {
	// Identity params (common to all subtypes)
	switch key {
	case "name":
		return a.PrimaryHostname()
	case "id":
		return a.ID
	case "uuid":
		return a.UUID
	case "tenant":
		return a.TenantID
	case "site":
		return a.Site
	case "owner":
		return a.OwnerIdentity
	case "description":
		return a.Description
	case "managed_by":
		return a.ManagedBy
	case "groups":
		return strings.Join(a.Groups, ",")
	case "labels":
		parts := make([]string, 0, len(a.Labels))
		for k, v := range a.Labels {
			parts = append(parts, k+"="+v)
		}
		return strings.Join(parts, " ")
	// Enrollment params
	case "enrollment_state":
		return string(a.EnrollmentState)
	case "enrolled_at":
		if a.EnrolledAt.IsZero() {
			return ""
		}
		return a.EnrolledAt.UTC().Format(time.RFC3339)
	case "last_seen_at":
		if a.LastSeenAt.IsZero() {
			return ""
		}
		return a.LastSeenAt.UTC().Format(time.RFC3339)
	case "agent_version":
		return a.AgentVersion
	// Lifecycle params
	case "lifecycle_state":
		return string(a.LifecycleState)
	case "vendor":
		return a.Vendor
	case "location":
		return a.Location
	case "purchase_date":
		if a.PurchaseDate.IsZero() {
			return ""
		}
		return a.PurchaseDate.Format("2006-01-02")
	case "warranty_expires":
		if a.WarrantyExpiresAt.IsZero() {
			return ""
		}
		return a.WarrantyExpiresAt.Format("2006-01-02")
	}

	// Subtype-specific params — dispatched by payload type
	switch p := a.Payload.(type) {
	case assets.ComputerPayload:
		return getComputerParam(key, p)
	case assets.ServerPayload:
		return getServerParam(key, p)
	case assets.PrinterPayload:
		return getPrinterParam(key, p)
	case assets.DeskPayload:
		return getDeskParam(key, p)
	}
	return ""
}

func getComputerParam(key string, p assets.ComputerPayload) string {
	switch key {
	case "hostname":
		return p.Hostname
	case "fqdn":
		return p.FQDN
	case "os_family":
		return osFamily(p.OSDistribution)
	case "os_distribution":
		return p.OSDistribution
	case "os_version":
		return p.OSVersion
	case "kernel_version":
		return p.KernelVersion
	case "architecture":
		return p.Architecture
	case "cpu":
		return p.CPUSummary
	case "ram_mb":
		if p.RAMMB > 0 {
			return strconv.Itoa(p.RAMMB)
		}
		return ""
	case "storage_mb":
		if p.StorageMB > 0 {
			return strconv.Itoa(p.StorageMB)
		}
		return ""
	}
	return ""
}

func getServerParam(key string, p assets.ServerPayload) string {
	switch key {
	case "hostname":
		return p.Hostname
	case "fqdn":
		return p.FQDN
	case "os_family":
		return osFamily(p.OSDistribution)
	case "os_distribution":
		return p.OSDistribution
	case "os_version":
		return p.OSVersion
	case "kernel_version":
		return p.KernelVersion
	case "architecture":
		return p.Architecture
	case "server_role":
		return string(p.Role)
	case "services":
		names := make([]string, 0, len(p.Services))
		for _, svc := range p.Services {
			names = append(names, svc.Name)
		}
		return strings.Join(names, ",")
	case "uptime_since":
		if p.UptimeStartedAt.IsZero() {
			return ""
		}
		return p.UptimeStartedAt.UTC().Format(time.RFC3339)
	case "ram_mb":
		// Servers don't have RAMMB in the current mock, but the param is mounted
		return ""
	}
	return ""
}

func getPrinterParam(key string, p assets.PrinterPayload) string {
	switch key {
	case "printer_model":
		return p.Model
	case "printer_ip":
		return p.IP
	case "queues":
		return strings.Join(p.Queues, ",")
	case "supported_formats":
		return strings.Join(p.SupportedFormats, ",")
	case "consumables":
		parts := make([]string, 0, len(p.CurrentConsumable))
		for _, c := range p.CurrentConsumable {
			parts = append(parts, c.Kind+":"+strconv.Itoa(c.LevelPct)+"%")
		}
		return strings.Join(parts, ",")
	}
	return ""
}

func getDeskParam(key string, p assets.DeskPayload) string {
	switch key {
	case "desk_location":
		return p.LocationLabel
	case "dock_model":
		return p.DockModel
	case "monitor_count":
		if p.MonitorCount > 0 {
			return strconv.Itoa(p.MonitorCount)
		}
		return ""
	case "guest_profile":
		return p.GuestProfileID
	}
	return ""
}

// =========================================================================
// Value Formatting — renders raw param values as HTML for table cells
// =========================================================================

func formatParamValue(val string, def *params.ParamDef, a assets.Asset) string {
	if val == "" {
		return mutedDash()
	}

	switch def.Type {
	case params.TypeEnum:
		return formatEnum(val, def)
	case params.TypeTime:
		return formatTime(val)
	case params.TypeDate:
		return esc(val)
	case params.TypeInt:
		return formatInt(val, def)
	case params.TypeList:
		return formatList(val)
	case params.TypeMap:
		return formatMap(val)
	case params.TypeLink:
		return formatLink(val, def)
	}

	// String type
	if def.Key == "name" {
		// Special: name cell shows hostname + ID
		return `<div class="policy-name">` + esc(val) + `</div>` +
			`<div class="policy-id font-mono">` + esc(a.ID) + `</div>`
	}
	if def.Mono {
		return `<span class="font-mono">` + esc(val) + `</span>`
	}
	return esc(val)
}

func formatEnum(val string, def *params.ParamDef) string {
	// Enrollment and lifecycle get colored chips
	switch def.Key {
	case "enrollment_state":
		return `<span class="pm-origin-chip pm-enroll-` + esc(val) + `">` + esc(val) + `</span>`
	case "lifecycle_state":
		return `<span class="pm-origin-chip pm-lifecycle-` + esc(val) + `">` + esc(val) + `</span>`
	}
	return `<span class="pm-origin-chip">` + esc(val) + `</span>`
}

func formatTime(val string) string {
	t, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return esc(val)
	}
	return esc(relativeTimeString(t))
}

func formatInt(val string, def *params.ParamDef) string {
	// Handle size units with auto-scaling
	if def.Unit == "MB" {
		return formatSizeMB(val)
	}
	if def.Unit != "" {
		return esc(val) + ` <span class="policy-muted">` + esc(def.Unit) + `</span>`
	}
	return esc(val)
}

// formatSizeMB converts MB to appropriate unit (MB/GB/TB/PB)
func formatSizeMB(val string) string {
	mb, err := strconv.ParseFloat(val, 64)
	if err != nil || mb <= 0 {
		return esc(val) + ` <span class="policy-muted">MB</span>`
	}

	const (
		KB = 1.0 / 1024
		GB = 1024.0
		TB = 1024.0 * 1024
		PB = 1024.0 * 1024 * 1024
	)

	var size float64
	var unit string

	switch {
	case mb >= PB:
		size = mb / PB
		unit = "PB"
	case mb >= TB:
		size = mb / TB
		unit = "TB"
	case mb >= GB:
		size = mb / GB
		unit = "GB"
	default:
		size = mb
		unit = "MB"
	}

	// Format nicely - no decimals for whole numbers, 1 decimal otherwise
	var formatted string
	if size == float64(int(size)) {
		formatted = strconv.Itoa(int(size))
	} else {
		formatted = strconv.FormatFloat(size, 'f', 1, 64)
	}

	return esc(formatted) + ` <span class="policy-muted">` + unit + `</span>`
}

func formatList(val string) string {
	items := strings.Split(val, ",")
	if len(items) == 0 || (len(items) == 1 && items[0] == "") {
		return mutedDash()
	}
	out := ""
	for _, item := range items {
		out += `<span class="pm-urn-chip">` + esc(strings.TrimSpace(item)) + `</span> `
	}
	return out
}

func formatMap(val string) string {
	if val == "" {
		return mutedDash()
	}
	parts := strings.Fields(val)
	out := ""
	for _, p := range parts {
		out += `<span class="pm-urn-chip">` + esc(p) + `</span> `
	}
	return out
}

func formatLink(val string, def *params.ParamDef) string {
	if val == "" {
		return mutedDash()
	}
	// Links with cardinality "many" are comma-separated
	if strings.Contains(val, ",") {
		items := strings.Split(val, ",")
		out := ""
		for _, item := range items {
			out += `<span class="pm-urn-chip">` + esc(strings.TrimSpace(item)) + `</span> `
		}
		return out
	}
	return esc(val)
}

// =========================================================================
// Helpers
// =========================================================================

func mutedDash() string { return `<span class="policy-muted">—</span>` }

func osFamily(distro string) string {
	d := strings.ToLower(distro)
	switch {
	case strings.Contains(d, "windows"):
		return "windows"
	case strings.Contains(d, "macos") || strings.Contains(d, "mac os") || strings.Contains(d, "darwin"):
		return "macos"
	default:
		return "linux"
	}
}

func relativeTimeString(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h ago"
	case d < 30*24*time.Hour:
		return strconv.Itoa(int(d/(24*time.Hour))) + "d ago"
	}
	return t.Format("2006-01-02")
}
