package database_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func TestDependencyGroupRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	// A tenant to own the group.
	ten, err := d.Queries.CreateTenant(ctx, db.CreateTenantParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := d.Queries.CreateDependencyGroup(ctx, db.CreateDependencyGroupParams{
		TenantID: ten.ID, Slug: "rpm-based", Name: "RPM-based OS",
		Description: sql.NullString{String: "RPM family", Valid: true}, IsBuiltin: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Queries.CreateDependencyGroupCondition(ctx, db.CreateDependencyGroupConditionParams{
		GroupID: g.ID, ParamPath: "computer/hardware/os_package_family", Operator: "in", ValueJson: `["rpm"]`, Seq: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.Queries.CreateModuleDependencyLink(ctx, db.CreateModuleDependencyLinkParams{
		TenantID: ten.ID, ModuleID: "pluris.sshd.password-auth-disable", GroupID: g.ID, Role: "platform",
	}); err != nil {
		t.Fatal(err)
	}
	conds, err := d.Queries.ListConditionsForGroup(ctx, g.ID)
	if err != nil || len(conds) != 1 || conds[0].Operator != "in" {
		t.Fatalf("conditions = %+v err=%v", conds, err)
	}
	links, err := d.Queries.ListLinksForModule(ctx, db.ListLinksForModuleParams{TenantID: ten.ID, ModuleID: "pluris.sshd.password-auth-disable"})
	if err != nil || len(links) != 1 || links[0].Role != "platform" {
		t.Fatalf("links = %+v err=%v", links, err)
	}
}
