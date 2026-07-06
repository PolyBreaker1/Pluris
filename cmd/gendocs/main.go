// Command gendocs regenerates the markdown surfaces that mirror the
// in-product orientation + glossary data.
//
// Invocation:
//
//	go run ./cmd/gendocs
//
// Outputs (overwrites in place):
//
//	docs/for-windows-admins/concepts.md   ← from web/orientation/All
//	docs/for-windows-admins/glossary.md   ← from web/glossary/All
//
// Hand-written docs that this tool MUST NOT touch:
//
//	docs/for-windows-admins/README.md
//	docs/for-windows-admins/cheatsheet.md
//	docs/for-windows-admins/concepts/<slug>.md  (if added later)
//
// Lock as INV-O3: every concept added to web/orientation and every
// term added to web/glossary requires re-running this tool. The CI
// step `go run ./cmd/gendocs && git diff --exit-code docs/` enforces
// freshness.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pluris/pluris/web/glossary"
	"github.com/pluris/pluris/web/orientation"
)

const docsRoot = "docs/for-windows-admins"

func main() {
	if err := writeConcepts(); err != nil {
		fmt.Fprintf(os.Stderr, "concepts: %v\n", err)
		os.Exit(1)
	}
	if err := writeGlossary(); err != nil {
		fmt.Fprintf(os.Stderr, "glossary: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("gendocs: ok")
}

// writeConcepts emits one concise markdown table with one row per
// concept, plus an anchor section per concept for deeper reference.
// Editorial guidance: tables first, prose only where strictly needed.
func writeConcepts() error {
	var b strings.Builder
	b.WriteString("# Pluris concepts — what each screen IS\n\n")
	b.WriteString("Each row is one menu item or tab in the admin console. Click into the screen to act on it; this page tells you what you're looking at, the AD/GP analogue, and the Linux mechanism behind it.\n\n")
	b.WriteString("> AUTO-GENERATED from `web/orientation/orientation.go`. Edit there, then `go run ./cmd/gendocs`.\n\n")
	b.WriteString("## Top-level concepts\n\n")
	b.WriteString("| Concept | In Active Directory | One-line description |\n")
	b.WriteString("|---|---|---|\n")
	for _, c := range orientation.All {
		b.WriteString("| [")
		b.WriteString(c.Title)
		b.WriteString("](#")
		b.WriteString(slug(c.Key))
		b.WriteString(") | ")
		b.WriteString(mdEscape(c.ADEquivalent))
		b.WriteString(" | ")
		b.WriteString(mdEscape(c.Summary))
		b.WriteString(" |\n")
	}
	b.WriteString("\n---\n\n## Per-concept reference\n\n")
	for _, c := range orientation.All {
		b.WriteString("### ")
		b.WriteString(c.Title)
		b.WriteString("\n\n")
		b.WriteString("<a id=\"")
		b.WriteString(slug(c.Key))
		b.WriteString("\"></a>\n\n")
		b.WriteString("| | |\n|---|---|\n")
		b.WriteString("| **In Active Directory** | ")
		b.WriteString(mdEscape(c.ADEquivalent))
		b.WriteString(" |\n")
		if c.LinuxMechanism != "" {
			b.WriteString("| **On Linux** | ")
			b.WriteString(mdEscape(c.LinuxMechanism))
			b.WriteString(" |\n")
		}
		b.WriteString("| **Summary** | ")
		b.WriteString(mdEscape(c.Summary))
		b.WriteString(" |\n")
		if c.SidebarHint != "" {
			b.WriteString("| **Sidebar hint** | ")
			b.WriteString(mdEscape(c.SidebarHint))
			b.WriteString(" |\n")
		}
		if c.CreateLabel != "" {
			b.WriteString("| **Create action** | `")
			b.WriteString(mdEscape(c.CreateLabel))
			b.WriteString("` ")
			if c.CreateHref != "" {
				b.WriteString("→ `")
				b.WriteString(c.CreateHref)
				b.WriteString("`")
			}
			b.WriteString(" |\n")
		}
		if c.EmptyTitle != "" {
			b.WriteString("| **Empty state** | _\"")
			b.WriteString(mdEscape(c.EmptyTitle))
			b.WriteString("\"_ — ")
			b.WriteString(mdEscape(c.EmptyHelp))
			b.WriteString(" |\n")
		}
		b.WriteString("\n")
	}
	return writeFile("concepts.md", b.String())
}

// writeGlossary emits one alphabetical table per category, in
// declaration order of categories (auth → policy → … → pluris).
func writeGlossary() error {
	var b strings.Builder
	b.WriteString("# Pluris glossary — Linux & Pluris terms for Windows admins\n\n")
	b.WriteString("Every term you'll meet in the console, with its Active-Directory or Windows-stack analogue. Hover the same term in the UI to see the same gloss inline.\n\n")
	b.WriteString("> AUTO-GENERATED from `web/glossary/glossary.go`. Edit there, then `go run ./cmd/gendocs`.\n\n")

	// Preserve declaration order of categories.
	seen := map[string]bool{}
	var order []string
	for _, t := range glossary.All {
		if !seen[t.Category] {
			seen[t.Category] = true
			order = append(order, t.Category)
		}
	}
	heads := map[string]string{
		"auth":       "Authentication & identity",
		"policy":     "Policy & enforcement",
		"process":    "Process & execution",
		"service":    "Services & scheduling",
		"filesystem": "Filesystem & storage",
		"network":    "Networking",
		"package":    "Package management",
		"ui":         "Desktop & UI",
		"pluris":     "Pluris-specific concepts",
	}
	by := glossary.ByCategory()
	for _, cat := range order {
		head := heads[cat]
		if head == "" {
			head = titleASCII(cat)
		}
		b.WriteString("## ")
		b.WriteString(head)
		b.WriteString("\n\n")
		b.WriteString("| Term | One-liner | Windows / AD equivalent |\n")
		b.WriteString("|---|---|---|\n")
		for _, t := range by[cat] {
			b.WriteString("| `")
			b.WriteString(t.Key)
			b.WriteString("` | ")
			b.WriteString(mdEscape(t.OneLine))
			b.WriteString(" | ")
			b.WriteString(mdEscape(t.ADEquivalent))
			b.WriteString(" |\n")
		}
		b.WriteString("\n")
	}
	return writeFile("glossary.md", b.String())
}

func writeFile(name, content string) error {
	if err := os.MkdirAll(docsRoot, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(docsRoot, name), []byte(content), 0o644)
}

// mdEscape — light escape for table cells. We never want a stray pipe
// to break the row, and backticks should pass through (they're often
// the most readable way to mark a token).
func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

// titleASCII — upper-case the first ASCII letter of s. Used as a
// safety net for category keys not in the heads map. ASCII-only by
// design (category keys are short lowercase slugs); avoids the
// strings.Title deprecation without pulling in golang.org/x/text.
func titleASCII(s string) string {
	if s == "" {
		return s
	}
	if c := s[0]; c >= 'a' && c <= 'z' {
		return string(c-32) + s[1:]
	}
	return s
}

// slug — turn a key like "scripts-policy-modules" into a stable HTML
// id. Already kebab-cased; this just normalises and strips anything
// odd.
func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '/':
			b.WriteRune('-')
		}
	}
	return b.String()
}
