package authz

import "testing"

func TestGrantsCan(t *testing.T) {
	g := Grants{
		"identity.create": "yes",
		"identity.delete": "no",
		"identity.view":   "own",
		"asset.view":      "all",
		"asset.update":    "none",
	}
	cases := []struct {
		key  string
		want bool
	}{
		{"identity.create", true},
		{"identity.delete", false},
		{"identity.view", true},
		{"asset.view", true},
		{"asset.update", false},
		{"missing.key", false},
	}
	for _, c := range cases {
		if got := g.Can(c.key); got != c.want {
			t.Errorf("Can(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestGrantsCanBypass(t *testing.T) {
	g := Grants{BypassKey: "yes"}
	if !g.Can("anything.at.all") {
		t.Error("bypass marker should make Can true for any key")
	}
	if !g.CanScoped("anything.at.all", 1, 2) {
		t.Error("bypass marker should make CanScoped true regardless of owner/self")
	}
}

func TestGrantsCanScoped(t *testing.T) {
	g := Grants{
		"identity.view":   "own",
		"asset.view":      "all",
		"asset.update":    "none",
		"identity.delete": "no",
	}
	cases := []struct {
		name            string
		key             string
		ownerID, selfID int64
		want            bool
	}{
		{"all always true", "asset.view", 5, 9, true},
		{"own matches self", "identity.view", 7, 7, true},
		{"own does not match other", "identity.view", 7, 8, false},
		{"none is false", "asset.update", 1, 1, false},
		{"missing is false", "asset.create", 1, 1, false},
		{"unscoped no is false", "identity.delete", 1, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := g.CanScoped(c.key, c.ownerID, c.selfID); got != c.want {
				t.Errorf("CanScoped(%q, %d, %d) = %v, want %v", c.key, c.ownerID, c.selfID, got, c.want)
			}
		})
	}
}

func TestGrantsScopeOf(t *testing.T) {
	g := Grants{"identity.view": "own"}
	if got := g.ScopeOf("identity.view"); got != "own" {
		t.Errorf("ScopeOf(identity.view) = %q, want own", got)
	}
	if got := g.ScopeOf("missing.key"); got != "" {
		t.Errorf("ScopeOf(missing.key) = %q, want empty string", got)
	}
}

func TestUnionScopedRanking(t *testing.T) {
	a := Grants{"identity.view": "own"}
	b := Grants{"identity.view": "all"}
	c := Grants{"identity.view": "none"}
	got := Union(a, b, c)
	if got["identity.view"] != "all" {
		t.Errorf("Union scoped ranking: got %q, want all", got["identity.view"])
	}
}

func TestUnionUnscopedRanking(t *testing.T) {
	a := Grants{"identity.create": "no"}
	b := Grants{"identity.create": "yes"}
	got := Union(a, b)
	if got["identity.create"] != "yes" {
		t.Errorf("Union unscoped ranking: got %q, want yes", got["identity.create"])
	}
}

func TestUnionDisjointKeysMerge(t *testing.T) {
	a := Grants{"identity.view": "own"}
	b := Grants{"asset.view": "all"}
	got := Union(a, b)
	if got["identity.view"] != "own" || got["asset.view"] != "all" {
		t.Errorf("Union disjoint merge failed: %+v", got)
	}
}

func TestParseInvalidJSON(t *testing.T) {
	g := Parse("{bad")
	if len(g) != 0 {
		t.Errorf("Parse of invalid JSON should be empty, got %+v", g)
	}
}

func TestParseKeepsUnknownKeys(t *testing.T) {
	g := Parse(`{"totally.unknown.key":"yes","identity.view":"own"}`)
	if g["totally.unknown.key"] != "yes" {
		t.Errorf("Parse should keep unknown keys, got %+v", g)
	}
	if g["identity.view"] != "own" {
		t.Errorf("Parse should keep known keys, got %+v", g)
	}
}
