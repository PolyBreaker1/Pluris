package database_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func openTestDB(t *testing.T) (*database.Database, int64) {
	t.Helper()
	d, err := database.Open(filepath.Join(t.TempDir(), "mvc.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	tenant, err := d.Queries.CreateTenant(context.Background(), db.CreateTenantParams{Name: "T", Slug: "t"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return d, tenant.ID
}

func createModuleWithVersion(t *testing.T, d *database.Database, tenantID int64, urn, state string) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	m, err := d.Queries.CreatePolicyModule(ctx, db.CreatePolicyModuleParams{
		ModuleUrn: urn, TenantID: sql.NullInt64{Int64: tenantID, Valid: true},
		Title: "T", Description: sql.NullString{String: "", Valid: true}, IsBundled: false,
	})
	if err != nil {
		t.Fatalf("create module: %v", err)
	}
	v, err := d.Queries.CreatePolicyModuleVersion(ctx, db.CreatePolicyModuleVersionParams{
		ModuleID: m.ID, Version: "1.0.0", State: "draft", Scope: "machine",
		TargetOs: "[]", Satisfies: "[]",
		ParametersSchema: sql.NullString{String: "", Valid: true},
		DependsOn:        sql.NullString{String: "[]", Valid: true},
		Conflicts:        sql.NullString{String: "[]", Valid: true},
		SandboxProfile:   "{}", ReportSchema: "", ManifestYaml: "",
	})
	if err != nil {
		t.Fatalf("create version: %v", err)
	}
	if state != "draft" {
		if _, err := d.Conn().Exec(`UPDATE policy_module_versions SET state = ? WHERE id = ?`, state, v.ID); err != nil {
			t.Fatalf("set state: %v", err)
		}
	}
	return m.ID, v.ID
}

func TestVersionConditionsCRUDAndDraftGuard(t *testing.T) {
	d, tenantID := openTestDB(t)
	ctx := context.Background()
	_, draftID := createModuleWithVersion(t, d, tenantID, "tenant.t.mvc-a", "draft")
	_, pubID := createModuleWithVersion(t, d, tenantID, "tenant.t.mvc-b", "published")

	cond, err := d.Queries.CreateVersionConditionGuarded(ctx, db.CreateVersionConditionGuardedParams{
		VersionID: draftID, Kind: "command", ParamPath: "", Operator: "contains",
		ValueJson: `["3"]`, ScriptSource: "uname -r", ScriptRef: "", Seq: 0,
	})
	if err != nil {
		t.Fatalf("create on draft: %v", err)
	}
	if cond.Kind != "command" || cond.ScriptSource != "uname -r" {
		t.Fatalf("round-trip mismatch: %+v", cond)
	}

	_, err = d.Queries.CreateVersionConditionGuarded(ctx, db.CreateVersionConditionGuardedParams{
		VersionID: pubID, Kind: "param", ParamPath: "computer/hardware/os_family",
		Operator: "in", ValueJson: `["linux"]`, Seq: 0,
	})
	if err != sql.ErrNoRows {
		t.Fatalf("create on published: want ErrNoRows, got %v", err)
	}

	rows, err := d.Queries.UpdateVersionConditionGuarded(ctx, db.UpdateVersionConditionGuardedParams{
		ID: cond.ID, VersionID: draftID, Kind: "command", ParamPath: "",
		Operator: "equals", ValueJson: `["6.1"]`, ScriptSource: "uname -r", ScriptRef: "",
	})
	if err != nil || rows != 1 {
		t.Fatalf("update on draft: rows=%d err=%v", rows, err)
	}

	if _, err := d.Conn().Exec(`UPDATE policy_module_versions SET state = 'published' WHERE id = ?`, draftID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	rows, err = d.Queries.UpdateVersionConditionGuarded(ctx, db.UpdateVersionConditionGuardedParams{
		ID: cond.ID, VersionID: draftID, Kind: "command", ParamPath: "",
		Operator: "equals", ValueJson: `["x"]`, ScriptSource: "uname -r", ScriptRef: "",
	})
	if err != nil || rows != 0 {
		t.Fatalf("update on published: want 0 rows, got rows=%d err=%v", rows, err)
	}
	rows, err = d.Queries.DeleteVersionConditionGuarded(ctx, db.DeleteVersionConditionGuardedParams{ID: cond.ID, VersionID: draftID})
	if err != nil || rows != 0 {
		t.Fatalf("delete on published: want 0 rows, got rows=%d err=%v", rows, err)
	}
}

func TestVersionConditionsCascadeAndMatchMode(t *testing.T) {
	d, tenantID := openTestDB(t)
	ctx := context.Background()
	_, vID := createModuleWithVersion(t, d, tenantID, "tenant.t.mvc-c", "draft")

	if _, err := d.Queries.CreateVersionConditionGuarded(ctx, db.CreateVersionConditionGuardedParams{
		VersionID: vID, Kind: "param", ParamPath: "computer/hardware/os_family",
		Operator: "in", ValueJson: `["linux"]`, Seq: 0,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	seq, err := d.Queries.MaxVersionConditionSeq(ctx, vID)
	if err != nil || seq.(int64) != 0 {
		t.Fatalf("max seq: %v %v", seq, err)
	}

	rows, err := d.Queries.UpdateVersionConditionsMatchMode(ctx, db.UpdateVersionConditionsMatchModeParams{
		ID: vID, ConditionsMatchMode: "any",
	})
	if err != nil || rows != 1 {
		t.Fatalf("match mode on draft: rows=%d err=%v", rows, err)
	}

	if err := d.Queries.DeletePolicyModuleVersion(ctx, vID); err != nil {
		t.Fatalf("delete version: %v", err)
	}
	conds, err := d.Queries.ListVersionConditions(ctx, vID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(conds) != 0 {
		t.Fatalf("cascade failed: %d conditions survive version delete", len(conds))
	}
}

func TestMigration011LegacyScriptExpectRewriteAndOrigin(t *testing.T) {
	d, tenantID := openTestDB(t)
	ctx := context.Background()

	var origin string
	if err := d.Conn().QueryRow(`SELECT origin FROM policy_modules WHERE is_bundled = TRUE LIMIT 1`).Scan(&origin); err == nil && origin != "bundled" {
		t.Fatalf("bundled origin backfill: got %q", origin)
	}
	m, err := d.Queries.CreatePolicyModule(ctx, db.CreatePolicyModuleParams{
		ModuleUrn: "tenant.t.origin", TenantID: sql.NullInt64{Int64: tenantID, Valid: true},
		Title: "T", Description: sql.NullString{Valid: true}, IsBundled: false,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := d.Queries.SetModuleOrigin(ctx, db.SetModuleOriginParams{ID: m.ID, Origin: "imported"}); err != nil {
		t.Fatalf("set origin: %v", err)
	}
	row, err := d.Queries.GetPolicyModule(ctx, m.ID)
	if err != nil || row.Origin != "imported" {
		t.Fatalf("origin round-trip: %q err=%v", row.Origin, err)
	}

	// Simulate a pre-011 legacy script condition, then re-run the 011
	// rewrite statements to prove the mapping (the migration itself ran
	// on an empty fresh DB above).
	res, err := d.Conn().Exec(`INSERT INTO dependency_groups (tenant_id, slug, name, description, is_builtin) VALUES (?, 'lg', 'LG', '', FALSE)`, tenantID)
	if err != nil {
		t.Fatalf("group: %v", err)
	}
	gid, _ := res.LastInsertId()
	if _, err := d.Conn().Exec(`INSERT INTO dependency_group_conditions (group_id, param_path, operator, value_json, seq, kind, script_source, script_expect)
		VALUES (?, '', 'in', '[]', 0, 'script', 'echo hi', '{"exit_code":0,"output_equals":"hi"}'),
		       (?, '', 'in', '[]', 1, 'script', 'true', '{"exit_code":0}')`, gid, gid); err != nil {
		t.Fatalf("legacy rows: %v", err)
	}
	for _, stmt := range []string{
		`UPDATE dependency_group_conditions SET operator='equals', value_json=json_array(json_extract(script_expect,'$.output_equals')) WHERE kind='script' AND json_valid(script_expect) AND json_extract(script_expect,'$.output_equals') IS NOT NULL`,
		`UPDATE dependency_group_conditions SET operator='exists', value_json='[]' WHERE kind='script' AND (NOT json_valid(script_expect) OR json_extract(script_expect,'$.output_equals') IS NULL)`,
	} {
		if _, err := d.Conn().Exec(stmt); err != nil {
			t.Fatalf("rewrite: %v", err)
		}
	}
	var op, vals string
	if err := d.Conn().QueryRow(`SELECT operator, value_json FROM dependency_group_conditions WHERE group_id = ? AND seq = 0`, gid).Scan(&op, &vals); err != nil {
		t.Fatalf("read: %v", err)
	}
	if op != "equals" || vals != `["hi"]` {
		t.Fatalf("output_equals mapping: op=%q vals=%q", op, vals)
	}
	if err := d.Conn().QueryRow(`SELECT operator, value_json FROM dependency_group_conditions WHERE group_id = ? AND seq = 1`, gid).Scan(&op, &vals); err != nil {
		t.Fatalf("read: %v", err)
	}
	if op != "exists" || vals != `[]` {
		t.Fatalf("exit-code-only mapping: op=%q vals=%q", op, vals)
	}
}
