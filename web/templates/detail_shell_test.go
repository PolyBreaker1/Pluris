package templates

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/pluris/pluris/web/lists"
)

func renderToString(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestDetailShellRendersTabsAndPanels(t *testing.T) {
	hero := HeroSpec{
		Crumbs: []Crumb{{Label: "Assets", Href: "/assets/computers"}, {Label: "dev-1", Href: ""}},
		Name:   "dev-1",
		ID:     "comp.acme.hq.0001",
		Chips:  []Chip{{Label: "enrolled", Class: "asset-chip-enroll-enrolled"}},
		Defs:   []HeroDef{{Label: "Site", Value: "HQ"}},
	}
	tabs := []TabSpec{
		{Slug: "general", Label: "General", Body: templ.NopComponent},
		{Slug: "groups", Label: "Groups", Body: templ.NopComponent},
	}
	html := renderToString(t, DetailShell("assets-computers", "dev-1", templ.Attributes{"data-testid": "page-test"}, hero, tabs))

	for _, want := range []string{
		`data-tab="general"`, `data-tab="groups"`,
		`data-panel="general"`, `data-panel="groups"`,
		"asset-detail-hero", "dev-1", "comp.acme.hq.0001",
		`class="app-header-back"`, `href="/assets/computers"`,
		`class="asset-detail-crumb"`,
		"asset-detail-tab is-active", // first tab active
		`data-testid="page-test"`,    // spread attrs land on the wrapper
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in shell html", want)
		}
	}
	// exactly one active tab and one active panel
	if got := strings.Count(html, "asset-detail-tab is-active"); got != 1 {
		t.Fatalf("expected exactly 1 active tab, got %d", got)
	}
	if got := strings.Count(html, "detail-tab-panel is-active"); got != 1 {
		t.Fatalf("expected exactly 1 active panel, got %d", got)
	}
}

func TestDetailBackHrefUsesNearestLinkedCrumb(t *testing.T) {
	crumbs := []Crumb{
		{Label: "Policy", Href: "/policy/catalog"},
		{Label: "Catalog", Href: "/policy/catalog?scope=user"},
		{Label: "Change the system time"},
	}
	if got := detailBackHref(crumbs); got != "/policy/catalog?scope=user" {
		t.Fatalf("detailBackHref() = %q, want nearest linked crumb", got)
	}
	if got := detailBackHref([]Crumb{{Label: "Only"}}); got != "" {
		t.Fatalf("detailBackHref() without links = %q, want empty", got)
	}
}

func TestDetailShellNilAttrsRenders(t *testing.T) {
	tabs := []TabSpec{{Slug: "general", Label: "General", Body: templ.NopComponent}}
	html := renderToString(t, DetailShell("assets-computers", "dev-1", nil, HeroSpec{Name: "dev-1"}, tabs))
	if !strings.Contains(html, `data-tab="general"`) || strings.Contains(html, "data-testid") {
		t.Fatalf("nil attrs render broken: %s", html)
	}
}

func TestDetailEmptyRowRendersColspanAndMessage(t *testing.T) {
	html := renderToString(t, DetailEmptyRow(5, "No rows yet"))
	if !strings.Contains(html, `colspan="5"`) || !strings.Contains(html, "No rows yet") {
		t.Fatalf("bad empty row: %s", html)
	}
}

func TestDetailTableFrameRendersRegisteredColumns(t *testing.T) {
	frame := DetailTableFrame(lists.ListIDIdentities, nil)
	html := renderToString(t, templ.Component(frame))
	if !strings.Contains(html, "pm-table") {
		t.Fatalf("missing pm-table class: %s", html)
	}
	if !strings.Contains(html, "<thead>") {
		t.Fatalf("missing thead")
	}
	if !strings.Contains(html, "Email") {
		t.Fatalf("expected registered column label Email in thead, got: %s", html)
	}
	// Every registered field renders a th.
	fields := lists.FieldsFor(lists.ListIDIdentities)
	if got := strings.Count(html, "</th>"); got != len(fields) {
		t.Fatalf("expected %d th cells, got %d", len(fields), got)
	}
}
