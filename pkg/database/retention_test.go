package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSoftDeleteMigrationSeedsRetentionSettings(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	settings, err := database.Queries.ListRetentionSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 6 {
		t.Fatalf("retention settings = %d, want 6", len(settings))
	}
	for _, setting := range settings {
		if setting.Mode != "soft" || setting.PurgeAfterDays != nil {
			t.Fatalf("unexpected default for %s: %+v", setting.EntityKind, setting)
		}
	}
}
