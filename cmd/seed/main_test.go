package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/pkg/database"
)

// TestSeedIsIdempotent runs the seeder twice against a scratch database:
// the first run creates the demo tenant and all seed data, the second run
// (with default tenant resolution -> first tenant) must succeed without
// touching anything, and the row counts must be identical after both runs.
func TestSeedIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "seed_test.db")

	if err := run(dbPath, "demo"); err != nil {
		t.Fatalf("first seed run failed: %v", err)
	}
	firstCounts := seedCounts(t, dbPath)

	// Second run with an empty tenant flag: default resolution picks the
	// first (demo) tenant. Must exit cleanly and change nothing.
	if err := run(dbPath, ""); err != nil {
		t.Fatalf("second seed run failed: %v", err)
	}
	secondCounts := seedCounts(t, dbPath)

	if firstCounts != secondCounts {
		t.Errorf("second run changed row counts: first=%+v second=%+v", firstCounts, secondCounts)
	}

	want := counts{
		Tenants:     1,
		Sites:       3,
		Assets:      15 + 12 + 10 + 12,
		Groups:      2,
		Memberships: 3, // 3 computers; demo tenant has no identities
		CfgGroups:   1,
		Bindings:    1,
		Assignments: 1,
		Software:    4,
		Described:   2,
	}
	if firstCounts != want {
		t.Errorf("unexpected seed counts: got %+v want %+v", firstCounts, want)
	}
}

func TestSeedFailsWithoutTenants(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	if err := run(dbPath, ""); err == nil {
		t.Fatal("expected error seeding an empty database with no -tenant flag")
	}
	if err := run(dbPath, "does-not-exist"); err == nil {
		t.Fatal("expected error seeding into a nonexistent tenant slug")
	}
}

type counts struct {
	Tenants     int
	Sites       int
	Assets      int
	Groups      int
	Memberships int
	CfgGroups   int
	Bindings    int
	Assignments int
	Software    int
	Described   int
}

func seedCounts(t *testing.T, dbPath string) counts {
	t.Helper()
	d, err := database.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open %s: %v", dbPath, err)
	}
	defer d.Close()

	ctx := context.Background()
	count := func(query string) int {
		var n int
		if err := d.Conn().QueryRowContext(ctx, query).Scan(&n); err != nil {
			t.Fatalf("count query %q failed: %v", query, err)
		}
		return n
	}

	return counts{
		Tenants:     count(`SELECT COUNT(*) FROM tenants`),
		Sites:       count(`SELECT COUNT(*) FROM sites`),
		Assets:      count(`SELECT COUNT(*) FROM assets`),
		Groups:      count(`SELECT COUNT(*) FROM groups`),
		Memberships: count(`SELECT COUNT(*) FROM group_memberships`),
		CfgGroups:   count(`SELECT COUNT(*) FROM configuration_groups`),
		Bindings:    count(`SELECT COUNT(*) FROM configuration_group_bindings`),
		Assignments: count(`SELECT COUNT(*) FROM configuration_group_assignments`),
		Software:    count(`SELECT COUNT(*) FROM installed_software`),
		Described:   count(`SELECT COUNT(*) FROM assets WHERE description IS NOT NULL`),
	}
}
