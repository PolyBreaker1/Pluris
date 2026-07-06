package glossary

import (
	"strings"
	"testing"
)

// TestEveryTermHasShape — INV-O3. Every glossary entry is rendered as
// a tooltip in the UI; blank fields surface as broken hover.
func TestEveryTermHasShape(t *testing.T) {
	allowedCategories := map[string]bool{
		"auth": true, "policy": true, "process": true, "service": true,
		"filesystem": true, "network": true, "package": true,
		"ui": true, "pluris": true,
	}
	for _, term := range All {
		if strings.TrimSpace(term.Key) == "" {
			t.Errorf("empty Key in entry %+v", term)
		}
		if strings.TrimSpace(term.OneLine) == "" {
			t.Errorf("[%s] empty OneLine", term.Key)
		}
		if strings.TrimSpace(term.ADEquivalent) == "" {
			t.Errorf("[%s] empty ADEquivalent — start with \"≈ \", \"= \", or \"No close equivalent — …\"", term.Key)
		}
		if !allowedCategories[term.Category] {
			t.Errorf("[%s] unknown category %q (allowed: auth, policy, process, service, filesystem, network, package, ui, pluris)", term.Key, term.Category)
		}
		// Audience reminder: if a Linux jargon definition leans on
		// another piece of Linux jargon without context, the L1
		// admin is no better off. We can't enforce English fully,
		// but flag the most common offenders.
		low := strings.ToLower(term.OneLine)
		for _, bad := range []string{" daemon ", " sysadmin", " kernel module"} {
			if strings.Contains(low, bad) {
				// Soft warning, not a failure — sometimes unavoidable.
				t.Logf("[%s] OneLine contains %q — consider rewording for a Windows L1 audience", term.Key, strings.TrimSpace(bad))
			}
		}
	}
}

// TestUniqueKeys — duplicates would silently shadow earlier entries.
func TestUniqueKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, term := range All {
		k := strings.ToLower(term.Key)
		if seen[k] {
			t.Errorf("duplicate term key (case-insensitive): %q", term.Key)
		}
		seen[k] = true
	}
}

// TestLookupCaseInsensitive — UI cells may carry the token in any
// case ("AppArmor", "apparmor", "APPARMOR"). All resolve.
func TestLookupCaseInsensitive(t *testing.T) {
	for _, variant := range []string{"apparmor", "AppArmor", "APPARMOR", " AppArmor "} {
		if !Found(variant) {
			t.Errorf("Lookup(%q) should resolve", variant)
		}
	}
}

// TestLookupUnknownReturnsZero — missing keys must degrade gracefully.
func TestLookupUnknownReturnsZero(t *testing.T) {
	if got := Lookup("not-a-real-term"); got.OneLine != "" {
		t.Errorf("Lookup of unknown should return zero Term, got %+v", got)
	}
	if Found("not-a-real-term") {
		t.Errorf("Found should be false for unknown key")
	}
}
