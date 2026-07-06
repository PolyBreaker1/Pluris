package templates

import (
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/web/lists"
)

// Helpers for the AssetsPage / AssetDetailPage templates. Kept in a
// plain .go file (not _templ.go) so they can be tested directly and so
// the template stays focused on rendering.

// subtypeFromSlug maps a URL slug ("computers", "servers", …) to the
// canonical Subtype enum. Unknown slugs fall back to SubtypeComputer
// so the page never blanks out.
func subtypeFromSlug(slug string) assets.Subtype {
	switch slug {
	case "computers":
		return assets.SubtypeComputer
	case "servers":
		return assets.SubtypeServer
	case "printers":
		return assets.SubtypePrinter
	case "desks":
		return assets.SubtypeDesk
	}
	return assets.SubtypeComputer
}

// assetSubtypeSlug maps the canonical Subtype enum (singular: "computer",
// "server", "printer", "desk") to the URL slug used by /assets/:subtype
// routes (plural: "computers", "servers", "printers", "desks"). Mirrors
// assetListNavigationScript's inline JS map ('computer':'computers', …)
// and is the inverse of subtypeFromSlug. Falls back to slug+"s" for any
// future subtype that isn't special-cased, matching the JS fallback.
func assetSubtypeSlug(s assets.Subtype) string {
	switch s {
	case assets.SubtypeComputer:
		return "computers"
	case assets.SubtypeServer:
		return "servers"
	case assets.SubtypePrinter:
		return "printers"
	case assets.SubtypeDesk:
		return "desks"
	}
	return string(s) + "s"
}

// assetSearchBlob — single lowercased string the lists.js engine
// matches against in `contains` mode (data-searchable attribute).
// Mirrors policySearchBlob's shape.
func assetSearchBlob(a assets.Asset) string {
	parts := []string{
		a.ID, a.UUID, a.PrimaryHostname(), a.Site, a.OwnerIdentity,
		a.Vendor, a.Location, string(a.EnrollmentState), string(a.LifecycleState),
	}
	parts = append(parts, a.Groups...)
	for k, v := range a.Labels {
		parts = append(parts, k+"="+v)
	}
	switch p := a.Payload.(type) {
	case assets.ComputerPayload:
		parts = append(parts, p.Hostname, p.FQDN, p.OSDistribution, p.OSVersion, p.Architecture)
	case assets.ServerPayload:
		parts = append(parts, p.Hostname, p.FQDN, p.OSDistribution, p.OSVersion, string(p.Role))
		for _, svc := range p.Services {
			parts = append(parts, svc.Name)
		}
	case assets.PrinterPayload:
		parts = append(parts, p.Model, p.Vendor, p.IP)
		parts = append(parts, p.Queues...)
	case assets.DeskPayload:
		parts = append(parts, p.LocationLabel, p.DockModel, p.GuestProfileID)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// osFamily returns "windows", "macos", or "linux" based on the OS distribution string.
func osFamily(distro string) string {
	d := strings.ToLower(distro)
	if strings.Contains(d, "windows") {
		return "windows"
	}
	if strings.Contains(d, "macos") || strings.Contains(d, "mac os") || strings.Contains(d, "mac") {
		return "macos"
	}
	return "linux"
}

// assetFilterAttrs returns HTML data attributes for each param registry key.
// These are emitted on each <tr> so the advanced filter's getVal() can
// look up values by param key directly via row.dataset[camelCase].
//
// Keys use the param registry format (lowercase_underscores). The browser
// converts data-enrollment-state="enrolled" to dataset.enrollmentState.
func assetFilterAttrs(a assets.Asset) string {
	var b strings.Builder
	// Identity
	w := func(key, val string) {
		if val == "" {
			return
		}
		b.WriteString(` data-`)
		b.WriteString(strings.ReplaceAll(key, "_", "-"))
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(val))
		b.WriteString(`"`)
	}
	w("name", a.PrimaryHostname())
	w("id", a.ID)
	w("uuid", a.UUID)
	w("tenant", a.TenantID)
	w("site", a.Site)
	w("owner", a.OwnerIdentity)
	w("groups", strings.Join(a.Groups, ","))
	// Labels as key=val pairs
	var labels []string
	for k, v := range a.Labels {
		labels = append(labels, k+"="+v)
	}
	w("labels", strings.Join(labels, " "))
	// Enrollment
	w("enrollment-state", string(a.EnrollmentState))
	if !a.EnrolledAt.IsZero() {
		w("enrolled-at", a.EnrolledAt.UTC().Format("2006-01-02"))
	}
	if !a.LastSeenAt.IsZero() {
		w("last-seen-at", a.LastSeenAt.UTC().Format("2006-01-02T15:04:05Z"))
	}
	w("agent-version", a.AgentVersion)
	// Lifecycle
	w("lifecycle-state", string(a.LifecycleState))
	w("vendor", a.Vendor)
	w("location", a.Location)
	if !a.PurchaseDate.IsZero() {
		w("purchase-date", a.PurchaseDate.Format("2006-01-02"))
	}
	if !a.WarrantyExpiresAt.IsZero() {
		w("warranty-expires", a.WarrantyExpiresAt.Format("2006-01-02"))
	}
	// Subtype-specific
	switch p := a.Payload.(type) {
	case assets.ComputerPayload:
		w("hostname", p.Hostname)
		w("fqdn", p.FQDN)
		w("os-distribution", p.OSDistribution)
		w("os-version", p.OSVersion)
		w("os-family", osFamily(p.OSDistribution))
		w("architecture", p.Architecture)
		w("cpu", p.CPUSummary)
		if p.RAMMB > 0 {
			w("ram-mb", strconv.Itoa(p.RAMMB))
		}
		if p.StorageMB > 0 {
			w("storage-mb", strconv.Itoa(p.StorageMB))
		}
	case assets.ServerPayload:
		w("hostname", p.Hostname)
		w("fqdn", p.FQDN)
		w("os-distribution", p.OSDistribution)
		w("os-version", p.OSVersion)
		w("os-family", osFamily(p.OSDistribution))
		w("architecture", p.Architecture)
		w("server-role", string(p.Role))
		var svcNames []string
		for _, svc := range p.Services {
			svcNames = append(svcNames, svc.Name)
		}
		w("services", strings.Join(svcNames, ","))
	case assets.PrinterPayload:
		w("printer-model", p.Model)
		w("printer-ip", p.IP)
		w("queues", strings.Join(p.Queues, ","))
		w("supported-formats", strings.Join(p.SupportedFormats, ","))
	case assets.DeskPayload:
		w("desk-location", p.LocationLabel)
		w("dock-model", p.DockModel)
		w("monitor-count", strconv.Itoa(p.MonitorCount))
		w("guest-profile", p.GuestProfileID)
	}
	return b.String()
}

// assetSearchBlobEscaped — HTML-escaped version for embedding in raw attribute strings.
func assetSearchBlobEscaped(a assets.Asset) string {
	return html.EscapeString(assetSearchBlob(a))
}

// =========================================================================
// Detail page — param-driven section builder
//
// The detail page renders sections and fields from the schema, using
// ParamDef.Label for <dt> text and lists.RenderAssetCell for values.
// No hard-coded labels — everything derives from the param registry.
// =========================================================================

// DetailField represents one parameter's display in the detail page.
type DetailField struct {
	Key      string // param key
	Label    string // from ParamDef.Label
	Value    string // rendered HTML value
	RawValue string // plain text value for clipboard
	Mono     bool   // from ParamDef.Mono
}

// DetailSection represents one schema section in the detail page.
type DetailSection struct {
	Key    string        // section key
	Label  string        // section label
	Fields []DetailField // non-empty fields in this section
}

// buildDetailSections builds the detail page content from the subtype
// schema. Only sections/fields with non-empty values are included.
func buildDetailSections(a *assets.Asset) []DetailSection {
	schema := params.SchemaBySubtype(string(a.Subtype))
	if schema == nil {
		return nil
	}

	sections := make([]DetailSection, 0, len(schema.Sections))
	for _, sec := range schema.Sections {
		ds := DetailSection{Key: sec.Key, Label: sec.Label}
		for _, paramKey := range sec.Params {
			def := params.DefByKey(paramKey)
			if def == nil {
				continue
			}
			// Skip compound type (rendered via sub-fields)
			if def.Type == params.TypeCompound {
				continue
			}
			val := lists.RenderAssetCell(paramKey, *a)
			rawVal := lists.GetAssetRawValue(paramKey, *a)
			// Skip if value is the muted dash (empty)
			if val == "" || val == `<span class="policy-muted">—</span>` {
				continue
			}
			ds.Fields = append(ds.Fields, DetailField{
				Key:      paramKey,
				Label:    def.Label,
				Value:    formatDetailValue(val, def),
				RawValue: rawVal,
				Mono:     def.Mono,
			})
		}
		if len(ds.Fields) > 0 {
			sections = append(sections, ds)
		}
	}
	return sections
}

// formatDetailValue converts table-cell HTML to detail-page HTML.
// Strips all table-specific wrappers for clean, consistent plain text display.
func formatDetailValue(html string, def *params.ParamDef) string {
	// No HTML - return as-is
	if !strings.Contains(html, "<") {
		return html
	}

	// Extract plain text content from any HTML wrapper
	// Handles: <span class="...">text</span>, <div class="...">text</div>, <code>text</code>, etc.
	return stripHTMLTags(html)
}

// stripHTMLTags removes all HTML tags and returns plain text content.
// Handles nested tags and multiple elements.
func stripHTMLTags(html string) string {
	var result strings.Builder
	inTag := false

	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}

	return strings.TrimSpace(result.String())
}

// suppress unused import warning (time is used by formatAssetTime)
var _ = time.Now

// ----------------------------------------------------------------------
// Tiny formatters shared by templates and helpers.
// ----------------------------------------------------------------------

func formatAssetTime(t interface {
	IsZero() bool
	Format(string) string
}) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04 MST")
}

func formatAssetDate(t interface {
	IsZero() bool
	Format(string) string
}) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func formatAssetMB(mb int) string {
	if mb <= 0 {
		return ""
	}
	if mb < 1024 {
		return strconv.Itoa(mb) + " MB"
	}
	gb := float64(mb) / 1024.0
	return strconv.FormatFloat(gb, 'f', 1, 64) + " GB"
}

// assetServicesString — formats a []ServerService into a display string
// for the asset detail page. Reuses the same "name:port" convention.
func assetServicesString(svcs []assets.ServerService) string {
	out := make([]string, 0, len(svcs))
	for _, s := range svcs {
		if s.Port != 0 {
			out = append(out, s.Name+":"+strconv.Itoa(s.Port))
		} else {
			out = append(out, s.Name)
		}
	}
	return strings.Join(out, ", ")
}

// assetConsumablesString — formats printer consumables for display.
// Reuses the same "kind level%" convention.
func assetConsumablesString(cons []assets.PrinterConsumable) string {
	out := make([]string, 0, len(cons))
	for _, c := range cons {
		out = append(out, c.Kind+" "+strconv.Itoa(c.LevelPct)+"%")
	}
	return strings.Join(out, "  ·  ")
}

// assetOSIconURL returns a URL to load the distro/OS icon from
// cdn.simpleicons.org based on the asset's OS distribution field.
// Returns empty string if no icon can be determined.
// Source: https://simpleicons.org — CDN backed by Cloudflare, widely used.
func assetOSIconURL(a assets.Asset) string {
	var distro string
	switch p := a.Payload.(type) {
	case assets.ComputerPayload:
		distro = p.OSDistribution
	case assets.ServerPayload:
		distro = p.OSDistribution
	default:
		return ""
	}
	slug := osDistroToIconSlug(distro)
	if slug == "" {
		return ""
	}
	// White color variant for dark backgrounds.
	return "https://cdn.simpleicons.org/" + slug + "/ffffff"
}

// assetOSIconID returns the embedded SVG icon ID for the OS.
// Used for reliable local icon display without external CDN dependency.
func assetOSIconID(a assets.Asset) string {
	var distro string
	switch p := a.Payload.(type) {
	case assets.ComputerPayload:
		distro = p.OSDistribution
	case assets.ServerPayload:
		distro = p.OSDistribution
	default:
		return "icon-os-linux"
	}
	return osDistroToEmbeddedIconID(distro)
}

// osDistroToEmbeddedIconID maps OS distribution to embedded SVG icon ID
func osDistroToEmbeddedIconID(distro string) string {
	d := strings.ToLower(strings.TrimSpace(distro))
	if d == "" {
		return "icon-os-linux"
	}
	switch {
	case strings.Contains(d, "ubuntu"):
		return "icon-os-ubuntu"
	case strings.Contains(d, "fedora"):
		return "icon-os-fedora"
	case strings.Contains(d, "debian"):
		return "icon-os-debian"
	case strings.Contains(d, "arch"):
		return "icon-os-arch"
	case strings.Contains(d, "rocky"):
		return "icon-os-rocky"
	case strings.Contains(d, "windows"):
		return "icon-os-windows"
	case strings.Contains(d, "macos"), strings.Contains(d, "mac os"):
		return "icon-os-apple"
	default:
		return "icon-os-linux"
	}
}

// cpuBrandIconID returns the icon ID for the CPU brand
func cpuBrandIconID(cpuSummary string) string {
	c := strings.ToLower(cpuSummary)
	switch {
	case strings.Contains(c, "intel"):
		return "icon-cpu-intel"
	case strings.Contains(c, "amd"), strings.Contains(c, "ryzen"):
		return "icon-cpu-amd"
	case strings.Contains(c, "apple"), strings.Contains(c, "m1"), strings.Contains(c, "m2"), strings.Contains(c, "m3"):
		return "icon-cpu-apple"
	default:
		return ""
	}
}

// osDistroToIconSlug maps known OS distribution names to Simple Icons slugs.
// Case-insensitive prefix matching handles variations like "Ubuntu 24.04 LTS",
// "Windows 11 Pro", "RHEL 9.4", etc.
func osDistroToIconSlug(distro string) string {
	d := strings.ToLower(strings.TrimSpace(distro))
	if d == "" {
		return "linux"
	}
	// Ordered by specificity — more specific prefixes first.
	prefixes := []struct {
		prefix string
		slug   string
	}{
		{"ubuntu", "ubuntu"},
		{"debian", "debian"},
		{"fedora", "fedora"},
		{"rhel", "redhat"},
		{"red hat", "redhat"},
		{"centos", "centos"},
		{"rocky", "rockylinux"},
		{"arch", "archlinux"},
		{"manjaro", "manjaro"},
		{"opensuse", "opensuse"},
		{"suse", "suse"},
		{"mint", "linuxmint"},
		{"elementary", "elementary"},
		{"pop!_os", "popos"},
		{"pop os", "popos"},
		{"nixos", "nixos"},
		{"gentoo", "gentoo"},
		{"alpine", "alpinelinux"},
		{"kali", "kalilinux"},
		{"macos", "apple"},
		{"mac os", "apple"},
	}
	for _, p := range prefixes {
		if strings.HasPrefix(d, p.prefix) {
			return p.slug
		}
	}
	// Windows not available on SimpleIcons - return empty to use embedded SVG
	if strings.Contains(d, "windows") {
		return ""
	}
	// Fallback: generic Linux penguin.
	return "linux"
}

// IsWindowsOS returns true if the asset is running Windows
func IsWindowsOS(a assets.Asset) bool {
	var distro string
	switch p := a.Payload.(type) {
	case assets.ComputerPayload:
		distro = p.OSDistribution
	case assets.ServerPayload:
		distro = p.OSDistribution
	default:
		return false
	}
	return strings.Contains(strings.ToLower(distro), "windows")
}
