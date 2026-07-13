package services_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
)

func newDGSvc(t *testing.T) (*services.DependencyGroupService, *database.Database, int64) {
	t.Helper()
	d, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ten, err := d.Queries.CreateTenant(context.Background(), db.CreateTenantParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	return services.NewDependencyGroupService(d), d, ten.ID
}

func TestEnsureBuiltinsIdempotent(t *testing.T) {
	svc, _, ten := newDGSvc(t)
	ctx := context.Background()
	if err := svc.EnsureBuiltins(ctx, ten); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureBuiltins(ctx, ten); err != nil {
		t.Fatal(err)
	}
	gs, err := svc.ListByTenant(ctx, ten)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 8 {
		t.Fatalf("want 8 builtin groups, got %d", len(gs))
	}
	// Each builtin has at least one condition loaded.
	for _, g := range gs {
		if len(g.Conditions) == 0 {
			t.Fatalf("builtin %s has no conditions", g.Slug)
		}
	}
}

func TestCreateLinkAndDeleteGuard(t *testing.T) {
	svc, _, ten := newDGSvc(t)
	ctx := context.Background()
	if err := svc.EnsureBuiltins(ctx, ten); err != nil {
		t.Fatal(err)
	}
	// Builtin delete is refused.
	gs, _ := svc.ListByTenant(ctx, ten)
	if err := svc.Delete(ctx, gs[0].ID); !errors.Is(err, services.ErrBuiltinProtected) {
		t.Fatalf("want ErrBuiltinProtected, got %v", err)
	}
	// Custom group create + condition + link.
	g, err := svc.Create(ctx, ten, "Custom", "c")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AddCondition(ctx, g.ID, "computer/hardware/os_family", "in", []string{"linux"}, "param", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.LinkModule(ctx, ten, "pluris.mod.x", g.ID, "platform"); err != nil {
		t.Fatal(err)
	}
	links, err := svc.ListLinksForModule(ctx, ten, "pluris.mod.x")
	if err != nil || len(links) != 1 {
		t.Fatalf("links=%v err=%v", links, err)
	}
	// Custom group deletes fine.
	if err := svc.Delete(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
}

// TestEnsureBuiltinsTolerantOfDuplicateCreate simulates the losing side
// of the concurrent-first-request race: a bare row for one of the
// builtin slugs already exists (as if another writer's CreateDependencyGroup
// won the race) before EnsureBuiltins runs. EnsureBuiltins must not treat
// the resulting UNIQUE(tenant_id, slug) violation as a hard error — it
// should detect it, re-fetch the existing row, skip re-seeding its
// conditions, and continue seeding the rest without error.
func TestEnsureBuiltinsTolerantOfDuplicateCreate(t *testing.T) {
	svc, d, ten := newDGSvc(t)
	ctx := context.Background()

	// Pre-create a bare "rpm-based" row with no conditions, standing in
	// for a concurrent writer that already won the race for that slug.
	pre, err := d.Queries.CreateDependencyGroup(ctx, db.CreateDependencyGroupParams{
		TenantID: ten, Slug: "rpm-based", Name: "RPM-based OS",
		Description: sql.NullString{String: "pre-existing", Valid: true}, IsBuiltin: true,
	})
	if err != nil {
		t.Fatalf("pre-create rpm-based: %v", err)
	}

	if err := svc.EnsureBuiltins(ctx, ten); err != nil {
		t.Fatalf("EnsureBuiltins must tolerate a duplicate builtin row, got: %v", err)
	}

	gs, err := svc.ListByTenant(ctx, ten)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 8 {
		t.Fatalf("want 8 builtin groups, got %d", len(gs))
	}
	// The pre-existing bare row must still be the one present (idempotent
	// skip, not a duplicate or an overwrite), and it keeps zero
	// conditions since EnsureBuiltins must not re-seed it.
	found := false
	for _, g := range gs {
		if g.Slug == "rpm-based" {
			found = true
			if g.ID != pre.ID {
				t.Fatalf("rpm-based group id changed: got %d, want pre-existing %d", g.ID, pre.ID)
			}
			if len(g.Conditions) != 0 {
				t.Fatalf("rpm-based conditions should not be re-seeded onto the pre-existing row, got %d", len(g.Conditions))
			}
		}
	}
	if !found {
		t.Fatal("rpm-based builtin missing after EnsureBuiltins")
	}
}

// TestEnsureBuiltinsConcurrentFirstRequest runs EnsureBuiltins from two
// goroutines on a fresh tenant simultaneously (the real race this fix
// targets) and asserts neither returns an error. Run with -race to catch
// any data races in addition to the logical race.
func TestEnsureBuiltinsConcurrentFirstRequest(t *testing.T) {
	svc, _, ten := newDGSvc(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = svc.EnsureBuiltins(ctx, ten)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: EnsureBuiltins returned error: %v", i, err)
		}
	}

	gs, err := svc.ListByTenant(ctx, ten)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != 8 {
		t.Fatalf("want 8 builtin groups after concurrent EnsureBuiltins, got %d", len(gs))
	}
}
