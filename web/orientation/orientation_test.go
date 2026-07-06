package orientation

import (
	"strings"
	"testing"
)

// TestEveryConceptHasShape — orientation entries are read by three UI
// surfaces (sidebar, page banner, docs). A blank ADEquivalent or
// Summary would render an empty cell. Catch it at build time.
func TestEveryConceptHasShape(t *testing.T) {
	for _, c := range All {
		if strings.TrimSpace(c.Key) == "" {
			t.Errorf("empty Key in entry %+v", c)
		}
		if strings.TrimSpace(c.Title) == "" {
			t.Errorf("[%s] empty Title", c.Key)
		}
		if strings.TrimSpace(c.ADEquivalent) == "" {
			t.Errorf("[%s] empty ADEquivalent — say \"No direct equivalent — …\" if honest, never blank", c.Key)
		}
		if strings.TrimSpace(c.Summary) == "" {
			t.Errorf("[%s] empty Summary", c.Key)
		}
		if len(c.Summary) > 220 {
			t.Errorf("[%s] Summary is %d chars (max ~220 to fit the banner cleanly)", c.Key, len(c.Summary))
		}
		if len(c.SidebarHint) > 60 {
			t.Errorf("[%s] SidebarHint is %d chars (max ~40 for sidebar truncation)", c.Key, len(c.SidebarHint))
		}
		// Empty-state contract: if EmptyTitle is set, EmptyHelp must
		// be set too — otherwise the empty state shows a heading with
		// no explanation.
		if c.EmptyTitle != "" && strings.TrimSpace(c.EmptyHelp) == "" {
			t.Errorf("[%s] EmptyTitle set but EmptyHelp empty", c.Key)
		}
		// Create-action contract: label and href travel together.
		if (c.CreateLabel == "") != (c.CreateHref == "") {
			t.Errorf("[%s] CreateLabel and CreateHref must both be set or both empty", c.Key)
		}
	}
}

// TestUniqueKeys — duplicates would cause Lookup to return whichever
// came first; the silent precedence is the kind of bug that hides for
// months. Fail at build time.
func TestUniqueKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range All {
		if seen[c.Key] {
			t.Errorf("duplicate concept key: %q", c.Key)
		}
		seen[c.Key] = true
	}
}

// TestLookupZeroValue — Lookup of an unknown key returns Concept{}
// (Title empty) so callers using `if Found(key)` skip rendering. Any
// regression that returned a synthetic placeholder would silently
// surface a blank banner.
func TestLookupZeroValue(t *testing.T) {
	if got := Lookup("definitely-not-a-key"); got.Title != "" {
		t.Errorf("Lookup of unknown key should return zero Concept, got %+v", got)
	}
	if Found("definitely-not-a-key") {
		t.Errorf("Found should be false for unknown key")
	}
}
