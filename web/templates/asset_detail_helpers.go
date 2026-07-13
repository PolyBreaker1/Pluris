package templates

import (
	"database/sql"
	"strconv"

	"github.com/a-h/templ"

	"github.com/pluris/pluris/catalog/assets"
	"github.com/pluris/pluris/catalog/identities"
	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/services"
)

// Helpers for the standardized asset detail page (Task 7): hero spec,
// tab wiring and null-column rendering for the Software/Logs tabs.
// Kept in a plain .go file so they can be unit-tested directly.

// assetFieldPathAttrs returns the data-path attribute carrying the
// field's canonical parameter path (INV-CPP), or no attributes when the
// entity does not mount the key (derived fields like "id" or "tenant").
func assetFieldPathAttrs(subtype assets.Subtype, key string) templ.Attributes {
	if p := params.PathFor(string(subtype), key); p != "" {
		return templ.Attributes{"data-path": p}
	}
	return templ.Attributes{}
}

// assetDetailHero builds the HeroSpec for an asset detail page. subtype
// is the URL slug ("computers"); the owner picker stays the single hero
// action, unchanged from the pre-shell page.
func assetDetailHero(subtype string, a assets.Asset, owners []identities.Identity, csrfToken string) HeroSpec {
	chips := []Chip{{
		Label: string(a.EnrollmentState),
		Class: "asset-chip-enroll-" + string(a.EnrollmentState),
	}}
	if a.LifecycleState != "" {
		chips = append(chips, Chip{Label: string(a.LifecycleState), Class: "asset-chip-lifecycle"})
	}

	defs := []HeroDef{
		{Label: "Hostname", Value: a.PrimaryHostname(), IconSVG: heroIconMonitor},
	}
	if os := a.OSSummary(); os != "" {
		defs = append(defs, HeroDef{Label: "OS", Value: os, IconSVG: heroIconTerminal})
	}
	if a.Site != "" {
		defs = append(defs, HeroDef{Label: "Site", Value: a.Site, IconSVG: heroIconMapPin})
	}
	if a.ManagedBy != "" {
		defs = append(defs, HeroDef{Label: "Managed by", Value: a.ManagedBy, IconSVG: heroIconUsers})
	}
	if a.AgentVersion != "" {
		defs = append(defs, HeroDef{Label: "Agent", Value: a.AgentVersion, IconSVG: heroIconRadio})
	}

	return HeroSpec{
		Crumbs: []Crumb{
			{Label: "Assets", Href: "/assets/" + subtype},
			{Label: a.PrimaryHostname()},
		},
		Name:   a.PrimaryHostname(),
		ID:     a.ID,
		Chips:  chips,
		Defs:   defs,
		Action: assetOwnerPicker(subtype, a, owners, csrfToken),
	}
}

// assetSubtypeTabLabel maps a URL slug to its display label; falls back
// to the raw slug for unknown values so the crumb never blanks out.
func assetSubtypeTabLabel(slug string) string {
	switch slug {
	case "computers":
		return "Computers"
	case "servers":
		return "Servers"
	case "printers":
		return "Printers"
	case "desks":
		return "Desks"
	}
	return slug
}

// assetDetailTabs wires the 8 standardized tabs (spec §3). Slugs are
// stable API for detail.js hash deep-links and the server test.
func assetDetailTabs(subtype string, a assets.Asset, software []db.InstalledSoftware, logs []db.ActivityLog, groups []services.GroupRow, allGroups []db.Group, csrfToken string, applied []services.AppliedPolicy) []TabSpec {
	return []TabSpec{
		{Slug: "general", Label: "General", Body: assetGeneralTab(a)},
		{Slug: "groups", Label: "Groups", Body: assetGroupsTab(subtype, a, groups, allGroups, csrfToken)},
		{Slug: "policies", Label: "Applied Policies", Body: assetPoliciesTab(subtype, a, applied)},
		{Slug: "configuration", Label: "Configuration", Body: assetConfigurationTab(applied)},
		{Slug: "software", Label: "Software", Body: assetSoftwareTab(software)},
		{Slug: "wine-groups", Label: "Wine Groups", Body: assetWineGroupsTab()},
		{Slug: "scripts", Label: "Scripts", Body: assetScriptsTab()},
		{Slug: "logs", Label: "Logs", Body: assetLogsTab(logs)},
	}
}

// nullStringOrDash renders a nullable text column, dash when absent.
func nullStringOrDash(s sql.NullString) string {
	if s.Valid && s.String != "" {
		return s.String
	}
	return "—"
}

// nullDateOrDash renders a nullable timestamp as a date, dash when absent.
func nullDateOrDash(t sql.NullTime) string {
	if t.Valid {
		return t.Time.Format("2006-01-02")
	}
	return "—"
}

// softwareSizeOrDash renders installed_software.size_mb, dash when absent.
func softwareSizeOrDash(n sql.NullInt64) string {
	if n.Valid {
		return strconv.FormatInt(n.Int64, 10) + " MB"
	}
	return "—"
}

// logActorLabel renders activity_log.actor_identity_id; NULL means the
// system itself acted (agent check-in, migration, seed).
func logActorLabel(id sql.NullInt64) string {
	if id.Valid {
		return "#" + strconv.FormatInt(id.Int64, 10)
	}
	return "System"
}

// configurationValueCount counts bound values across applied policies
// (drives the Configuration tab empty state).
func configurationValueCount(applied []services.AppliedPolicy) int {
	n := 0
	for _, ap := range applied {
		n += len(ap.Values)
	}
	return n
}
