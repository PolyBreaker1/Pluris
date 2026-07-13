# Dependency Groups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give policy modules a clear, interconnected applicability schema ("dependency groups" — Pluris's WMI-filter equivalent) so the platform can decide whether a module is a valid choice for a device.

**Architecture:** A `DependencyGroup` is an AND-set of conditions over canonical device-fact param keys. A policy module references groups in two roles — `platform` (match ANY) and `requirement` (match ALL). DB-backed (migration 004) with bundled builtin templates seeded per tenant. A pure evaluator (`catalog/dependencygroups`) computes eligible/ineligible/unknown and is unit-tested independent of any live agent.

**Tech Stack:** Go 1.25, Echo v4, Templ, sqlc (SQLite modernc, WAL), vanilla JS.

## Global Constraints

- Never touch or delete repo-root `pluris.db*` (owner's live GUI-testing DB). All tests use scratch paths (`t.TempDir`).
- `-buildvcs=false` on every `go` command.
- SQL comments in `.sql` files are ASCII only (no em-dashes, no apostrophes).
- Run `make gen` after editing any `.templ`; run `sqlc generate` after editing any `.sql`. Never hand-edit `*_templ.go` or `db/*.sql.go` generated files.
- Migrations are append-only. Migration 004 contains no PRAGMA, so it runs inside the tracker transaction.
- No new external Go dependencies.
- Owner manages all git operations. Do NOT `git commit` or `git push`. Where a task's final step says "Commit", instead STOP and report the task complete so the owner can review and commit.
- Suite green means done: `go test -buildvcs=false -count=1 ./...` exits 0 and `gofmt -l` is clean.
- Invariants: INV-CPP (canonical parameter paths), INV-L (list columns from `web/lists/`), DetailShell is the one detail-page layout. RBAC mutations gated to admin/super_admin via `requireRoleAdmin` (`console/handlers/roles.go`).

## Shared type & name reference (used across tasks)

**`catalog/dependencygroups` package (Task 3):**
```go
type Operator string
const ( OpIn Operator = "in"; OpNotIn Operator = "not_in"; OpExists Operator = "exists" )

type Role string
const ( RolePlatform Role = "platform"; RoleRequirement Role = "requirement" )

type Status string
const ( StatusEligible Status = "eligible"; StatusIneligible Status = "ineligible"; StatusUnknown Status = "unknown" )

type Condition struct { ParamPath string; Operator Operator; Values []string }
type Group struct { ID int64; Slug, Name, Description string; Builtin bool; Conditions []Condition }
type ModuleLink struct { GroupID int64; Role Role }
type GroupResult struct { GroupID int64; Slug, Name string; Role Role; Pass string; Reason string } // Pass: "pass"|"fail"|"unknown"
type Result struct { Status Status; Platforms, Requirements []GroupResult }

func Eligible(links []ModuleLink, groups map[int64]Group, facts map[string]string) Result
```
`facts` is keyed by **bare param key** (e.g. `os_family`, `os_package_family`, `disk_encryption`), not the full path. `Condition.ParamPath` stores the full canonical path (for display/validation and interconnection); the evaluator derives the key as the substring after the last `/`. This keeps the schema interconnected while matching is entity-agnostic (a fact shared by computer + server matches either).

**DB generated names (Task 1):** `db.DependencyGroup`, `db.DependencyGroupCondition`, `db.ModuleDependencyLink`; queries `CreateDependencyGroup`, `GetDependencyGroup`, `GetDependencyGroupBySlug`, `ListDependencyGroupsByTenant`, `UpdateDependencyGroup`, `DeleteDependencyGroup`, `CreateDependencyGroupCondition`, `ListConditionsForGroup`, `DeleteConditionsForGroup`, `DeleteDependencyGroupCondition`, `CreateModuleDependencyLink`, `DeleteModuleDependencyLink`, `ListLinksForModule`, `ListLinksForGroup`, `CountLinksForGroup`.

**Service (Task 4):** `services.DependencyGroupService`, `services.NewDependencyGroupService(db)`. Handler field `depGroupSvc`.

---

### Task 1: Migration 004 + queries + sqlc generation

**Files:**
- Create: `db/schema/004_dependency_groups.sql`
- Create: `db/queries/dependency_groups.sql`
- Modify: `pkg/database/database.go` (migrations slice, ~line 103-106)
- Test: `pkg/database/dependency_groups_test.go`

**Interfaces:**
- Produces: DB tables `dependency_groups`, `dependency_group_conditions`, `module_dependency_links`; generated `db.*` types & queries listed in the shared reference.

- [ ] **Step 1: Write the failing test**

`pkg/database/dependency_groups_test.go`:
```go
package database_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func TestDependencyGroupRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := database.New(filepath.Join(t.TempDir(), "t.db"))
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
		Description: sqlNull("RPM family"), IsBuiltin: true,
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

func sqlNull(s string) (n struct{ String string; Valid bool }) { return } // replaced below
```
Note: replace the `sqlNull` helper — use `sql.NullString{String: "RPM family", Valid: true}` inline and `import "database/sql"`. (Delete the stub; it exists only so this snippet is self-contained.) Confirm `CreateTenant`/`CreateTenantParams` exist by checking `db/tenants.sql.go`; if the tenant helper differs, create the tenant with the existing query.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/database/ -run TestDependencyGroupRoundTrip`
Expected: FAIL to compile — `undefined: db.CreateDependencyGroupParams`.

- [ ] **Step 3: Write the migration**

`db/schema/004_dependency_groups.sql`:
```sql
-- Migration 004: dependency groups (module applicability filters).
-- A dependency group is an AND set of conditions over device fact keys.
-- Modules link to groups in two roles: platform (match any) and
-- requirement (match all). Contains no PRAGMA, so it runs inside the
-- tracker transaction.

-- Named, reusable applicability filter, scoped per tenant.
CREATE TABLE IF NOT EXISTS dependency_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    is_builtin BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, slug)
);

-- One predicate inside a group. All conditions in a group are ANDed.
-- param_path is a canonical parameter path; value_json is a JSON array
-- of strings (empty array for the exists operator).
CREATE TABLE IF NOT EXISTS dependency_group_conditions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL REFERENCES dependency_groups(id) ON DELETE CASCADE,
    param_path TEXT NOT NULL,
    operator TEXT NOT NULL,
    value_json TEXT NOT NULL DEFAULT '[]',
    seq INTEGER NOT NULL DEFAULT 0
);

-- Link from a policy module (catalog mock slug, no FK) to a dependency
-- group, tagged with the role the group plays for that module.
CREATE TABLE IF NOT EXISTS module_dependency_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    module_id TEXT NOT NULL,
    group_id INTEGER NOT NULL REFERENCES dependency_groups(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    UNIQUE(tenant_id, module_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_depgroups_tenant ON dependency_groups(tenant_id);
CREATE INDEX IF NOT EXISTS idx_depconditions_group ON dependency_group_conditions(group_id);
CREATE INDEX IF NOT EXISTS idx_modulelinks_module ON module_dependency_links(tenant_id, module_id);
CREATE INDEX IF NOT EXISTS idx_modulelinks_group ON module_dependency_links(group_id);
```

- [ ] **Step 4: Register the migration**

In `pkg/database/database.go`, add to the `migrations` slice (after `003_roles_software_logs.sql`):
```go
		"db/schema/004_dependency_groups.sql",
```

- [ ] **Step 5: Write the queries**

`db/queries/dependency_groups.sql`:
```sql
-- Dependency group queries. Matches the tables in
-- db/schema/004_dependency_groups.sql. Groups are per tenant module
-- applicability filters; conditions AND within a group; module links
-- attach a group to a module in a role (platform or requirement).

-- name: CreateDependencyGroup :one
INSERT INTO dependency_groups (tenant_id, slug, name, description, is_builtin)
VALUES (@tenant_id, @slug, @name, @description, @is_builtin)
RETURNING *;

-- name: GetDependencyGroup :one
SELECT * FROM dependency_groups WHERE id = @id;

-- name: GetDependencyGroupBySlug :one
SELECT * FROM dependency_groups
WHERE tenant_id = @tenant_id AND slug = @slug
LIMIT 1;

-- name: ListDependencyGroupsByTenant :many
SELECT * FROM dependency_groups
WHERE tenant_id = @tenant_id
ORDER BY is_builtin DESC, name;

-- name: UpdateDependencyGroup :exec
UPDATE dependency_groups
SET name = @name, description = @description, updated_at = CURRENT_TIMESTAMP
WHERE id = @id;

-- name: DeleteDependencyGroup :exec
DELETE FROM dependency_groups WHERE id = @id;

-- name: CreateDependencyGroupCondition :one
INSERT INTO dependency_group_conditions (group_id, param_path, operator, value_json, seq)
VALUES (@group_id, @param_path, @operator, @value_json, @seq)
RETURNING *;

-- name: ListConditionsForGroup :many
SELECT * FROM dependency_group_conditions
WHERE group_id = @group_id
ORDER BY seq, id;

-- name: DeleteConditionsForGroup :exec
DELETE FROM dependency_group_conditions WHERE group_id = @group_id;

-- name: DeleteDependencyGroupCondition :exec
DELETE FROM dependency_group_conditions WHERE id = @id AND group_id = @group_id;

-- name: CreateModuleDependencyLink :exec
INSERT OR IGNORE INTO module_dependency_links (tenant_id, module_id, group_id, role)
VALUES (@tenant_id, @module_id, @group_id, @role);

-- name: DeleteModuleDependencyLink :exec
DELETE FROM module_dependency_links
WHERE tenant_id = @tenant_id AND module_id = @module_id AND group_id = @group_id;

-- name: ListLinksForModule :many
SELECT * FROM module_dependency_links
WHERE tenant_id = @tenant_id AND module_id = @module_id
ORDER BY role, group_id;

-- name: ListLinksForGroup :many
SELECT * FROM module_dependency_links
WHERE group_id = @group_id
ORDER BY module_id;

-- name: CountLinksForGroup :one
SELECT COUNT(*) FROM module_dependency_links WHERE group_id = @group_id;
```

- [ ] **Step 6: Generate code**

Run: `sqlc generate` then `go build -buildvcs=false ./...`
Expected: no errors; `db/dependency_groups.sql.go` created with the params/row types.

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test -buildvcs=false ./pkg/database/ -run TestDependencyGroupRoundTrip`
Expected: PASS. Then `go test -buildvcs=false -count=1 ./...` and `gofmt -l pkg/ db/` (empty output).

- [ ] **Step 8: Commit** — STOP and report; owner commits.

---

### Task 2: Device-fact parameters (os_package_family, disk_encryption)

**Files:**
- Modify: `catalog/params/definitions.go` (add two `ParamDef` entries near `os_family`, ~line 110)
- Modify: `catalog/params/schemas.go` (add the two keys to the computer + server `hardware` sections, lines ~38-41 and ~66-69)
- Test: `catalog/params/dependency_facts_test.go`

**Interfaces:**
- Produces: canonical paths `computer/hardware/os_package_family`, `computer/hardware/disk_encryption` (and `server/...`); enum param defs `os_package_family` and `disk_encryption`.

- [ ] **Step 1: Write the failing test**

`catalog/params/dependency_facts_test.go`:
```go
package params_test

import (
	"testing"

	"github.com/pluris/pluris/catalog/params"
)

func TestDependencyFactsRegistered(t *testing.T) {
	for _, key := range []string{"os_package_family", "disk_encryption"} {
		d := params.DefByKey(key)
		if d == nil {
			t.Fatalf("param %q not registered", key)
		}
		if len(d.EnumValues) == 0 {
			t.Fatalf("param %q has no enum values", key)
		}
	}
	if got := params.PathFor("computer", "os_package_family"); got != "computer/hardware/os_package_family" {
		t.Fatalf("computer os_package_family path = %q", got)
	}
	if got := params.PathFor("computer", "disk_encryption"); got != "computer/hardware/disk_encryption" {
		t.Fatalf("computer disk_encryption path = %q", got)
	}
	if got := params.PathFor("server", "disk_encryption"); got != "server/hardware/disk_encryption" {
		t.Fatalf("server disk_encryption path = %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./catalog/params/ -run TestDependencyFactsRegistered`
Expected: FAIL — `param "os_package_family" not registered`.

- [ ] **Step 3: Add the param definitions**

In `catalog/params/definitions.go`, immediately after the `os_family` entry (line ~110):
```go
		{Key: "os_package_family", Label: "Package format", Description: "OS package family (rpm/deb/arch/apk).", Category: "hardware", Type: TypeEnum, EnumValues: []string{"rpm", "deb", "arch", "apk", "other"}, Filter: FilterEquals, Sort: SortAlpha},
		{Key: "disk_encryption", Label: "Disk encryption", Description: "Primary disk encryption mechanism.", Category: "hardware", Type: TypeEnum, EnumValues: []string{"none", "bitlocker", "luks", "filevault", "other"}, Filter: FilterEquals, Sort: SortAlpha},
```

- [ ] **Step 4: Mount the keys on computer + server hardware sections**

In `catalog/params/schemas.go`, computer schema `hardware` section (line ~38-41), append the two keys to the `Params` slice:
```go
		{Key: "hardware", Label: "Hardware", Params: []string{
			"hostname", "fqdn", "os_family", "os_package_family", "disk_encryption", "os_distribution", "os_version", "kernel_version",
			"architecture", "cpu", "ram_mb", "serial_number", "storage_mb",
		}},
```
And the server schema `hardware` section (line ~66-69):
```go
		{Key: "hardware", Label: "Hardware", Params: []string{
			"hostname", "fqdn", "os_family", "os_package_family", "disk_encryption", "os_distribution", "os_version", "kernel_version",
			"architecture", "server_role", "services", "uptime_since", "ram_mb", "serial_number",
		}},
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -buildvcs=false ./catalog/params/`
Expected: PASS (all params tests). Then `go build -buildvcs=false ./...` — the params power the detail pages, so a build confirms the new keys render.

- [ ] **Step 6: Commit** — STOP and report; owner commits.

---

### Task 3: Dependency-group catalog types + pure evaluator

**Files:**
- Create: `catalog/dependencygroups/types.go`
- Create: `catalog/dependencygroups/eval.go`
- Test: `catalog/dependencygroups/eval_test.go`

**Interfaces:**
- Consumes: nothing (pure package).
- Produces: the types & `Eligible` from the shared reference.

- [ ] **Step 1: Write the failing test**

`catalog/dependencygroups/eval_test.go`:
```go
package dependencygroups

import "testing"

func groups() map[int64]Group {
	return map[int64]Group{
		1: {ID: 1, Slug: "any-linux", Name: "Any Linux", Conditions: []Condition{{ParamPath: "computer/hardware/os_family", Operator: OpIn, Values: []string{"linux"}}}},
		2: {ID: 2, Slug: "windows", Name: "Windows", Conditions: []Condition{{ParamPath: "computer/hardware/os_family", Operator: OpIn, Values: []string{"windows"}}}},
		3: {ID: 3, Slug: "rpm-based", Name: "RPM-based OS", Conditions: []Condition{{ParamPath: "computer/hardware/os_package_family", Operator: OpIn, Values: []string{"rpm"}}}},
		4: {ID: 4, Slug: "debian-based", Name: "Debian-based OS", Conditions: []Condition{{ParamPath: "computer/hardware/os_package_family", Operator: OpIn, Values: []string{"deb"}}}},
		5: {ID: 5, Slug: "disk-enc", Name: "Disk encryption active", Conditions: []Condition{{ParamPath: "computer/hardware/disk_encryption", Operator: OpNotIn, Values: []string{"none"}}}},
	}
}

func TestEligibleAgnosticNoPlatform(t *testing.T) {
	r := Eligible(nil, groups(), map[string]string{"os_family": "linux"})
	if r.Status != StatusEligible {
		t.Fatalf("want eligible, got %s", r.Status)
	}
}

func TestPlatformAnyPasses(t *testing.T) {
	links := []ModuleLink{{GroupID: 3, Role: RolePlatform}, {GroupID: 4, Role: RolePlatform}}
	r := Eligible(links, groups(), map[string]string{"os_package_family": "deb"})
	if r.Status != StatusEligible {
		t.Fatalf("want eligible (debian matches), got %s", r.Status)
	}
}

func TestPlatformFailsIneligible(t *testing.T) {
	links := []ModuleLink{{GroupID: 2, Role: RolePlatform}}
	r := Eligible(links, groups(), map[string]string{"os_family": "linux"})
	if r.Status != StatusIneligible {
		t.Fatalf("want ineligible (windows-only on linux), got %s", r.Status)
	}
}

func TestRequirementAllOneFails(t *testing.T) {
	links := []ModuleLink{{GroupID: 1, Role: RolePlatform}, {GroupID: 5, Role: RoleRequirement}}
	r := Eligible(links, groups(), map[string]string{"os_family": "linux", "disk_encryption": "none"})
	if r.Status != StatusIneligible {
		t.Fatalf("want ineligible (encryption none), got %s", r.Status)
	}
}

func TestUnknownFact(t *testing.T) {
	links := []ModuleLink{{GroupID: 5, Role: RoleRequirement}}
	r := Eligible(links, groups(), map[string]string{"os_family": "linux"}) // no disk_encryption fact
	if r.Status != StatusUnknown {
		t.Fatalf("want unknown (no encryption fact), got %s", r.Status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./catalog/dependencygroups/`
Expected: FAIL to compile — undefined `Group`, `Eligible`, etc.

- [ ] **Step 3: Write the types**

`catalog/dependencygroups/types.go`:
```go
// Package dependencygroups holds the pure applicability model for policy
// modules: a dependency group is an AND set of conditions over device
// fact keys, and a module links to groups in a platform (match any) or
// requirement (match all) role. Persistence lives in pkg/services; this
// package is deliberately dependency free and unit tested in isolation.
package dependencygroups

type Operator string

const (
	OpIn     Operator = "in"
	OpNotIn  Operator = "not_in"
	OpExists Operator = "exists"
)

type Role string

const (
	RolePlatform    Role = "platform"
	RoleRequirement Role = "requirement"
)

type Status string

const (
	StatusEligible   Status = "eligible"
	StatusIneligible Status = "ineligible"
	StatusUnknown    Status = "unknown"
)

// Condition is one predicate. ParamPath is a full canonical path (for
// display and interconnection); matching uses only its trailing key.
type Condition struct {
	ParamPath string
	Operator  Operator
	Values    []string
}

type Group struct {
	ID          int64
	Slug        string
	Name        string
	Description string
	Builtin     bool
	Conditions  []Condition
}

type ModuleLink struct {
	GroupID int64
	Role    Role
}

// GroupResult is one group's verdict; Pass is "pass" | "fail" | "unknown".
type GroupResult struct {
	GroupID int64
	Slug    string
	Name    string
	Role    Role
	Pass    string
	Reason  string
}

type Result struct {
	Status       Status
	Platforms    []GroupResult
	Requirements []GroupResult
}
```

- [ ] **Step 4: Write the evaluator**

`catalog/dependencygroups/eval.go`:
```go
package dependencygroups

import "strings"

// paramKey returns the trailing segment of a canonical path, which is the
// entity agnostic fact key used to look up device facts.
func paramKey(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func contains(vs []string, v string) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}

// evalCondition returns "pass", "fail", or "unknown". A fact absent from
// facts is always "unknown" (the device has not reported it), never a
// false pass or fail.
func evalCondition(c Condition, facts map[string]string) string {
	v, ok := facts[paramKey(c.ParamPath)]
	switch c.Operator {
	case OpExists:
		if !ok {
			return "unknown"
		}
		if v != "" {
			return "pass"
		}
		return "fail"
	case OpIn:
		if !ok {
			return "unknown"
		}
		if contains(c.Values, v) {
			return "pass"
		}
		return "fail"
	case OpNotIn:
		if !ok {
			return "unknown"
		}
		if !contains(c.Values, v) {
			return "pass"
		}
		return "fail"
	}
	return "unknown"
}

// evalGroup ANDs a group's conditions. A definitive fail dominates an
// unknown; all-pass is pass; otherwise unknown.
func evalGroup(g Group, facts map[string]string) string {
	verdict := "pass"
	for _, c := range g.Conditions {
		switch evalCondition(c, facts) {
		case "fail":
			return "fail"
		case "unknown":
			verdict = "unknown"
		}
	}
	return verdict
}

// Eligible evaluates a module's dependency links against device facts.
// Platform links: pass if ANY passes (none linked = agnostic pass).
// Requirement links: pass only if ALL pass. Overall: ineligible if either
// aggregate fails, eligible if both pass, otherwise unknown.
func Eligible(links []ModuleLink, groups map[int64]Group, facts map[string]string) Result {
	var res Result
	platAny, platUnknown, platCount := false, false, 0
	reqFail, reqUnknown, reqCount := false, false, 0

	for _, l := range links {
		g, ok := groups[l.GroupID]
		if !ok {
			continue
		}
		v := evalGroup(g, facts)
		gr := GroupResult{GroupID: g.ID, Slug: g.Slug, Name: g.Name, Role: l.Role, Pass: v, Reason: reasonFor(g, v)}
		switch l.Role {
		case RolePlatform:
			platCount++
			res.Platforms = append(res.Platforms, gr)
			if v == "pass" {
				platAny = true
			} else if v == "unknown" {
				platUnknown = true
			}
		case RoleRequirement:
			reqCount++
			res.Requirements = append(res.Requirements, gr)
			if v == "fail" {
				reqFail = true
			} else if v == "unknown" {
				reqUnknown = true
			}
		}
	}

	platOK := "pass"
	if platCount > 0 {
		switch {
		case platAny:
			platOK = "pass"
		case platUnknown:
			platOK = "unknown"
		default:
			platOK = "fail"
		}
	}
	reqOK := "pass"
	if reqCount > 0 {
		switch {
		case reqFail:
			reqOK = "fail"
		case reqUnknown:
			reqOK = "unknown"
		default:
			reqOK = "pass"
		}
	}

	switch {
	case platOK == "fail" || reqOK == "fail":
		res.Status = StatusIneligible
	case platOK == "pass" && reqOK == "pass":
		res.Status = StatusEligible
	default:
		res.Status = StatusUnknown
	}
	return res
}

func reasonFor(g Group, v string) string {
	switch v {
	case "pass":
		return g.Name + " matched"
	case "fail":
		return g.Name + " did not match"
	default:
		return g.Name + " needs agent inventory"
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -buildvcs=false ./catalog/dependencygroups/`
Expected: PASS (all five cases).

- [ ] **Step 6: Commit** — STOP and report; owner commits.

---

### Task 4: DependencyGroupService (persistence + builtins + Evaluate)

**Files:**
- Create: `pkg/services/dependencygroups.go`
- Test: `pkg/services/dependencygroups_test.go`

**Interfaces:**
- Consumes: `db.*` queries (Task 1); `dependencygroups.*` types + `Eligible` (Task 3).
- Produces: `services.DependencyGroupService` with methods:
  `NewDependencyGroupService(db) *DependencyGroupService`,
  `EnsureBuiltins(ctx, tenantID) error`,
  `ListByTenant(ctx, tenantID) ([]dependencygroups.Group, error)` (each with conditions loaded),
  `Get(ctx, id) (dependencygroups.Group, error)`,
  `Create(ctx, tenantID, name, description string) (dependencygroups.Group, error)`,
  `Update(ctx, id int64, name, description string) error`,
  `Delete(ctx, id int64) error` (returns `ErrBuiltinProtected` when builtin),
  `AddCondition(ctx, groupID int64, paramPath, operator string, values []string) error`,
  `RemoveCondition(ctx, groupID, condID int64) error`,
  `LinkModule(ctx, tenantID int64, moduleID string, groupID int64, role string) error`,
  `UnlinkModule(ctx, tenantID int64, moduleID string, groupID int64) error`,
  `ListLinksForModule(ctx, tenantID int64, moduleID string) ([]dependencygroups.ModuleLink, error)`,
  `CountLinks(ctx, groupID int64) (int64, error)`,
  `Evaluate(ctx, tenantID int64, moduleID string, facts map[string]string) (dependencygroups.Result, error)`.

- [ ] **Step 1: Write the failing test**

`pkg/services/dependencygroups_test.go`:
```go
package services_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
	"github.com/pluris/pluris/pkg/services"
)

func newDGSvc(t *testing.T) (*services.DependencyGroupService, *database.Database, int64) {
	t.Helper()
	d, err := database.New(filepath.Join(t.TempDir(), "t.db"))
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
	if err := svc.AddCondition(ctx, g.ID, "computer/hardware/os_family", "in", []string{"linux"}); err != nil {
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -buildvcs=false ./pkg/services/ -run TestEnsureBuiltins`
Expected: FAIL to compile — `undefined: services.NewDependencyGroupService`.

- [ ] **Step 3: Write the service**

`pkg/services/dependencygroups.go`:
```go
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

// ErrBuiltinProtected is returned when a caller tries to delete a builtin
// dependency group. Builtins are editable but not deletable.
var ErrBuiltinProtected = errors.New("builtin dependency group cannot be deleted")

type DependencyGroupService struct {
	db *database.Database
}

func NewDependencyGroupService(db *database.Database) *DependencyGroupService {
	return &DependencyGroupService{db: db}
}

// builtinGroup is one seed template.
type builtinCond struct {
	Path string
	Op   string
	Vals []string
}
type builtinGroup struct {
	Slug, Name, Desc string
	Conds            []builtinCond
}

var builtinGroups = []builtinGroup{
	{"rpm-based", "RPM-based OS", "Fedora, RHEL, openSUSE and other RPM package systems.", []builtinCond{{"computer/hardware/os_package_family", "in", []string{"rpm"}}}},
	{"debian-based", "Debian-based OS", "Debian, Ubuntu and other deb package systems.", []builtinCond{{"computer/hardware/os_package_family", "in", []string{"deb"}}}},
	{"arch-based", "Arch-based OS", "Arch and derivatives using pacman.", []builtinCond{{"computer/hardware/os_package_family", "in", []string{"arch"}}}},
	{"any-linux", "Any Linux", "Any Linux operating system.", []builtinCond{{"computer/hardware/os_family", "in", []string{"linux"}}}},
	{"windows", "Windows", "Any Windows operating system.", []builtinCond{{"computer/hardware/os_family", "in", []string{"windows"}}}},
	{"disk-encryption-active", "Disk encryption active", "Primary disk uses any encryption mechanism.", []builtinCond{{"computer/hardware/disk_encryption", "not_in", []string{"none"}}}},
	{"bitlocker", "BitLocker enabled", "Primary disk encrypted with BitLocker.", []builtinCond{{"computer/hardware/disk_encryption", "in", []string{"bitlocker"}}}},
	{"luks", "LUKS enabled", "Primary disk encrypted with LUKS.", []builtinCond{{"computer/hardware/disk_encryption", "in", []string{"luks"}}}},
}

// builtinModuleLinks seeds default module to group links for bundled
// modules. moduleID must match a catalog/policymodules mock slug.
var builtinModuleLinks = []struct {
	ModuleID, Slug, Role string
}{
	{"pluris.sshd.password-auth-disable", "any-linux", "platform"},
}

func (s *DependencyGroupService) EnsureBuiltins(ctx context.Context, tenantID int64) error {
	for _, b := range builtinGroups {
		existing, err := s.db.Queries.GetDependencyGroupBySlug(ctx, db.GetDependencyGroupBySlugParams{TenantID: tenantID, Slug: b.Slug})
		if err == nil {
			// Already seeded; leave user edits intact.
			_ = existing
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		g, err := s.db.Queries.CreateDependencyGroup(ctx, db.CreateDependencyGroupParams{
			TenantID: tenantID, Slug: b.Slug, Name: b.Name,
			Description: sql.NullString{String: b.Desc, Valid: true}, IsBuiltin: true,
		})
		if err != nil {
			return err
		}
		for i, c := range b.Conds {
			vals, _ := json.Marshal(c.Vals)
			if _, err := s.db.Queries.CreateDependencyGroupCondition(ctx, db.CreateDependencyGroupConditionParams{
				GroupID: g.ID, ParamPath: c.Path, Operator: c.Op, ValueJson: string(vals), Seq: int64(i),
			}); err != nil {
				return err
			}
		}
	}
	// Default module links (idempotent via INSERT OR IGNORE).
	for _, l := range builtinModuleLinks {
		g, err := s.db.Queries.GetDependencyGroupBySlug(ctx, db.GetDependencyGroupBySlugParams{TenantID: tenantID, Slug: l.Slug})
		if err != nil {
			continue
		}
		_ = s.db.Queries.CreateModuleDependencyLink(ctx, db.CreateModuleDependencyLinkParams{
			TenantID: tenantID, ModuleID: l.ModuleID, GroupID: g.ID, Role: l.Role,
		})
	}
	return nil
}

func (s *DependencyGroupService) toGroup(ctx context.Context, row db.DependencyGroup) (dependencygroups.Group, error) {
	conds, err := s.db.Queries.ListConditionsForGroup(ctx, row.ID)
	if err != nil {
		return dependencygroups.Group{}, err
	}
	g := dependencygroups.Group{ID: row.ID, Slug: row.Slug, Name: row.Name, Builtin: row.IsBuiltin}
	if row.Description.Valid {
		g.Description = row.Description.String
	}
	for _, c := range conds {
		var vals []string
		_ = json.Unmarshal([]byte(c.ValueJson), &vals)
		g.Conditions = append(g.Conditions, dependencygroups.Condition{
			ParamPath: c.ParamPath, Operator: dependencygroups.Operator(c.Operator), Values: vals,
		})
	}
	return g, nil
}

func (s *DependencyGroupService) ListByTenant(ctx context.Context, tenantID int64) ([]dependencygroups.Group, error) {
	rows, err := s.db.Queries.ListDependencyGroupsByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]dependencygroups.Group, 0, len(rows))
	for _, r := range rows {
		g, err := s.toGroup(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (s *DependencyGroupService) Get(ctx context.Context, id int64) (dependencygroups.Group, error) {
	row, err := s.db.Queries.GetDependencyGroup(ctx, id)
	if err != nil {
		return dependencygroups.Group{}, err
	}
	return s.toGroup(ctx, row)
}

func (s *DependencyGroupService) Create(ctx context.Context, tenantID int64, name, description string) (dependencygroups.Group, error) {
	row, err := s.db.Queries.CreateDependencyGroup(ctx, db.CreateDependencyGroupParams{
		TenantID: tenantID, Slug: slugify(name), Name: name,
		Description: sql.NullString{String: description, Valid: description != ""}, IsBuiltin: false,
	})
	if err != nil {
		return dependencygroups.Group{}, err
	}
	return s.toGroup(ctx, row)
}

func (s *DependencyGroupService) Update(ctx context.Context, id int64, name, description string) error {
	return s.db.Queries.UpdateDependencyGroup(ctx, db.UpdateDependencyGroupParams{
		ID: id, Name: name, Description: sql.NullString{String: description, Valid: description != ""},
	})
}

func (s *DependencyGroupService) Delete(ctx context.Context, id int64) error {
	row, err := s.db.Queries.GetDependencyGroup(ctx, id)
	if err != nil {
		return err
	}
	if row.IsBuiltin {
		return ErrBuiltinProtected
	}
	return s.db.Queries.DeleteDependencyGroup(ctx, id)
}

func (s *DependencyGroupService) AddCondition(ctx context.Context, groupID int64, paramPath, operator string, values []string) error {
	conds, err := s.db.Queries.ListConditionsForGroup(ctx, groupID)
	if err != nil {
		return err
	}
	vals, _ := json.Marshal(values)
	_, err = s.db.Queries.CreateDependencyGroupCondition(ctx, db.CreateDependencyGroupConditionParams{
		GroupID: groupID, ParamPath: paramPath, Operator: operator, ValueJson: string(vals), Seq: int64(len(conds)),
	})
	return err
}

func (s *DependencyGroupService) RemoveCondition(ctx context.Context, groupID, condID int64) error {
	return s.db.Queries.DeleteDependencyGroupCondition(ctx, db.DeleteDependencyGroupConditionParams{ID: condID, GroupID: groupID})
}

func (s *DependencyGroupService) LinkModule(ctx context.Context, tenantID int64, moduleID string, groupID int64, role string) error {
	return s.db.Queries.CreateModuleDependencyLink(ctx, db.CreateModuleDependencyLinkParams{
		TenantID: tenantID, ModuleID: moduleID, GroupID: groupID, Role: role,
	})
}

func (s *DependencyGroupService) UnlinkModule(ctx context.Context, tenantID int64, moduleID string, groupID int64) error {
	return s.db.Queries.DeleteModuleDependencyLink(ctx, db.DeleteModuleDependencyLinkParams{
		TenantID: tenantID, ModuleID: moduleID, GroupID: groupID,
	})
}

func (s *DependencyGroupService) ListLinksForModule(ctx context.Context, tenantID int64, moduleID string) ([]dependencygroups.ModuleLink, error) {
	rows, err := s.db.Queries.ListLinksForModule(ctx, db.ListLinksForModuleParams{TenantID: tenantID, ModuleID: moduleID})
	if err != nil {
		return nil, err
	}
	out := make([]dependencygroups.ModuleLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, dependencygroups.ModuleLink{GroupID: r.GroupID, Role: dependencygroups.Role(r.Role)})
	}
	return out, nil
}

func (s *DependencyGroupService) CountLinks(ctx context.Context, groupID int64) (int64, error) {
	return s.db.Queries.CountLinksForGroup(ctx, groupID)
}

func (s *DependencyGroupService) Evaluate(ctx context.Context, tenantID int64, moduleID string, facts map[string]string) (dependencygroups.Result, error) {
	links, err := s.ListLinksForModule(ctx, tenantID, moduleID)
	if err != nil {
		return dependencygroups.Result{}, err
	}
	groups, err := s.ListByTenant(ctx, tenantID)
	if err != nil {
		return dependencygroups.Result{}, err
	}
	byID := make(map[int64]dependencygroups.Group, len(groups))
	for _, g := range groups {
		byID[g.ID] = g
	}
	return dependencygroups.Eligible(links, byID, facts), nil
}
```
Note: `slugify` — check `pkg/services/` for an existing slug helper (groups/config groups likely have one). If none exists, add a small unexported `slugify(string) string` in this file that lowercases, replaces non-alphanumerics with `-`, and trims. Keep it ASCII.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -buildvcs=false ./pkg/services/ -run 'TestEnsureBuiltins|TestCreateLinkAndDeleteGuard'`
Expected: PASS.

- [ ] **Step 5: Commit** — STOP and report; owner commits.

---

### Task 5: Dependency Groups list page + sidebar

**Files:**
- Create: `web/lists/dependency_groups.go`
- Create: `web/templates/dependency_groups.templ`
- Modify: `web/templates/menu.go` (Policy children ~line 48-51; label switch ~line 107-111)
- Modify: `console/handlers/handlers.go` (Handler struct + `New` — add `depGroupSvc`; add `DependencyGroups` handler)
- Modify: `console/server/server.go` (route, after `/policy/modules/sources` ~line 144)
- Test: `console/server/server_test.go` (add a route smoke case)

**Interfaces:**
- Consumes: `services.DependencyGroupService` (Task 4).
- Produces: page at `/policy/dependency-groups` with `data-testid="page-dependency-groups"`; `templates.DependencyGroupsPage(groups []DependencyGroupRow)`; list id `lists.ListIDDependencyGroups = "dependency-groups"`.

- [ ] **Step 1: Write the failing test**

Add to `console/server/server_test.go` `tests` slice (near the policy-groups case ~line 149):
```go
	{name: "dependency-groups", path: "/policy/dependency-groups", expectStatus: 200, expectTestID: `data-testid="page-dependency-groups"`},
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test -buildvcs=false ./console/server/ -run TestRoutes` (use the actual test function name in that file; grep it if unsure)
Expected: FAIL — 404 or missing testid.

- [ ] **Step 3: Register the list columns**

`web/lists/dependency_groups.go`:
```go
package lists

const ListIDDependencyGroups = "dependency-groups"

func init() {
	Register(ListIDDependencyGroups, "Dependency Groups", detailTabGroups(), []FieldDef{
		{Key: "name", Label: "Name", Group: "main"},
		{Key: "conditions", Label: "Conditions", Group: "main"},
		{Key: "used_by", Label: "Used by", Group: "main"},
		{Key: "type", Label: "Type", Group: "main"},
	})
}
```
Check `web/lists/detail_tabs.go` for the exact `FieldDef` field names (`Key`, `Label`, `Group`) and `Register` signature; match them exactly. `detailTabGroups()` already exists there.

- [ ] **Step 4: Add the view model + handler wiring**

In `console/handlers/handlers.go`: add field to the `Handler` struct and constructor:
```go
	depGroupSvc   *services.DependencyGroupService
```
```go
		depGroupSvc:   services.NewDependencyGroupService(db),
```
Add the handler (new file `console/handlers/dependency_groups.go` is fine, or in handlers.go). The list handler seeds builtins so a fresh tenant shows templates:
```go
func (h *Handler) DependencyGroups(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	if err := h.depGroupSvc.EnsureBuiltins(ctx, sess.TenantID); err != nil {
		return err
	}
	groups, err := h.depGroupSvc.ListByTenant(ctx, sess.TenantID)
	if err != nil {
		return err
	}
	rows := make([]templates.DependencyGroupRow, 0, len(groups))
	for _, g := range groups {
		count, _ := h.depGroupSvc.CountLinks(ctx, g.ID)
		rows = append(rows, templates.DependencyGroupRow{
			ID: g.ID, Name: g.Name, Slug: g.Slug, Builtin: g.Builtin,
			ConditionSummary: templates.DependencyConditionSummary(g), UsedBy: count,
		})
	}
	return render(c, templates.DependencyGroupsPage(rows))
}
```

- [ ] **Step 5: Write the list template**

`web/templates/dependency_groups.templ`:
```go
package templates

import (
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/dependencygroups"
)

// DependencyGroupRow is the list-view model for one dependency group.
type DependencyGroupRow struct {
	ID               int64
	Name             string
	Slug             string
	Builtin          bool
	ConditionSummary string
	UsedBy           int64
}

// DependencyConditionSummary renders a group's conditions as a short
// human string, e.g. "Package format is one of rpm".
func DependencyConditionSummary(g dependencygroups.Group) string {
	parts := make([]string, 0, len(g.Conditions))
	for _, c := range g.Conditions {
		key := c.ParamPath
		if i := strings.LastIndex(key, "/"); i >= 0 {
			key = key[i+1:]
		}
		verb := "is one of"
		switch c.Operator {
		case dependencygroups.OpNotIn:
			verb = "is not"
		case dependencygroups.OpExists:
			verb = "is set"
		}
		parts = append(parts, key+" "+verb+" "+strings.Join(c.Values, ", "))
	}
	if len(parts) == 0 {
		return "No conditions"
	}
	return strings.Join(parts, " and ")
}

templ DependencyGroupsPage(rows []DependencyGroupRow) {
	@Page("policy-dependency-groups", "Dependency Groups") {
		<div data-testid="page-dependency-groups" class="page-body">
			<div class="page-header">
				<h1>Dependency Groups</h1>
				<a href="/policy/dependency-groups/new" class="btn btn-primary">+ New group</a>
			</div>
			<p class="page-intro">Reusable applicability filters that decide which policy modules are valid for a device. Like Windows WMI filters.</p>
			<table class="policy-table">
				<thead><tr><th>Name</th><th>Conditions</th><th>Used by</th><th>Type</th></tr></thead>
				<tbody>
					for _, r := range rows {
						<tr data-policy-id={ strconv.FormatInt(r.ID, 10) } onclick={ templ.ComponentScript{Call: "location.href='/policy/dependency-groups/" + strconv.FormatInt(r.ID, 10) + "'"} }>
							<td>{ r.Name }</td>
							<td>{ r.ConditionSummary }</td>
							<td>{ strconv.FormatInt(r.UsedBy, 10) } modules</td>
							<td>
								if r.Builtin {
									<span class="asset-chip">Builtin</span>
								} else {
									<span class="asset-chip">Custom</span>
								}
							</td>
						</tr>
					}
				</tbody>
			</table>
		</div>
	}
}
```
IMPORTANT: `@Page(...)` and `.policy-table`/`.asset-chip` — confirm the real page-wrapper component name and classes by reading `web/templates/pages.templ` (the Configuration Groups / Policy Catalog pages). Match whatever wrapper those pages use (it may be `@Shell`, `@AppLayout`, or a bespoke wrapper) and the same table classes so styling is consistent. The `onclick` navigation mirrors `policyCatalogNavigationScript`; if a row-click helper already exists, reuse it instead of inline script.

- [ ] **Step 6: Add the sidebar item + label**

In `web/templates/menu.go`, Policy `Children` (after the Modules item):
```go
			{Label: "Dependency Groups", Href: "/policy/dependency-groups", Key: "policy-dependency-groups"},
```
And in the title switch (~line 107-111) add:
```go
	case "policy-dependency-groups":
		return "Dependency Groups"
```

- [ ] **Step 7: Add the route**

In `console/server/server.go` after line ~144:
```go
	e.GET("/policy/dependency-groups", h.DependencyGroups)
```

- [ ] **Step 8: Generate + test**

Run: `make gen` then `go test -buildvcs=false ./console/... ./web/...`
Expected: PASS incl. the new route smoke test.

- [ ] **Step 9: Commit** — STOP and report; owner commits.

---

### Task 6: Dependency group detail + editor (CRUD)

**Files:**
- Modify: `web/templates/dependency_groups.templ` (add `DependencyGroupDetailPage` + editor components)
- Modify: `console/handlers/dependency_groups.go` (detail/new/create/update/delete + condition add/remove)
- Modify: `console/server/server.go` (routes)
- Test: `console/handlers/dependency_groups_test.go`

**Interfaces:**
- Consumes: Task 4 service, Task 5 page wrapper.
- Produces: `/policy/dependency-groups/:id` (DetailShell), `/new`, POST create/update/delete/conditions; handlers `DependencyGroupDetail`, `DependencyGroupNew`, `DependencyGroupCreate`, `DependencyGroupUpdate`, `DependencyGroupDelete`, `DependencyGroupConditionAdd`, `DependencyGroupConditionRemove`.

- [ ] **Step 1: Write the failing test**

`console/handlers/dependency_groups_test.go` — model it on `console/handlers/roles_test.go` (same `auth.WithSession` injection + `echo.NewContext`). Cover: (a) create writes a row; (b) a technician session gets 403 on create; (c) deleting a builtin returns an error status. Skeleton:
```go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	// ... same imports as roles_test.go: echo, auth, database, services, identities
)

func TestDependencyGroupCreateAndRBAC(t *testing.T) {
	h, d, tenantID := newTestHandler(t) // reuse the roles_test helper pattern; if none, build like roles_test.go
	_ = d
	e := echo.New()

	// Technician is forbidden.
	form := url.Values{"name": {"My Group"}, "description": {"x"}}
	req := httptest.NewRequest(http.MethodPost, "/policy/dependency-groups", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetRequest(req.WithContext(auth.WithSession(req.Context(), &auth.Session{TenantID: tenantID, IdentityID: 2, Role: identities.RoleTechnician})))
	if err := h.DependencyGroupCreate(c); err == nil {
		t.Fatal("technician create should be forbidden")
	}

	// Admin succeeds.
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.SetRequest(req.WithContext(auth.WithSession(req.Context(), &auth.Session{TenantID: tenantID, IdentityID: 1, Role: identities.RoleAdmin})))
	if err := h.DependencyGroupCreate(c); err != nil {
		t.Fatalf("admin create failed: %v", err)
	}
	groups, _ := h.depGroupSvc.ListByTenant(req.Context(), tenantID)
	found := false
	for _, g := range groups {
		if g.Name == "My Group" {
			found = true
		}
	}
	if !found {
		t.Fatal("group not created")
	}
}
```
If no `newTestHandler` helper exists, construct exactly as `roles_test.go` does (open scratch DB via `database.New(t.TempDir()...)`, `handlers.New(db)` — note: `New` returns `*Handler`; the test is in package `handlers` so it can read `h.depGroupSvc`). Create a tenant first.

- [ ] **Step 2: Run to verify it fails**

Run: `go test -buildvcs=false ./console/handlers/ -run TestDependencyGroupCreateAndRBAC`
Expected: FAIL to compile — `h.DependencyGroupCreate` undefined.

- [ ] **Step 3: Write the CRUD handlers**

Append to `console/handlers/dependency_groups.go`:
```go
func (h *Handler) DependencyGroupDetail(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	g, err := h.depGroupSvc.Get(ctx, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "dependency group not found")
	}
	return render(c, templates.DependencyGroupDetailPage(g, csrfTokenFrom(c)))
}

func (h *Handler) DependencyGroupNew(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	return render(c, templates.DependencyGroupNewPage(csrfTokenFrom(c)))
}

func (h *Handler) DependencyGroupCreate(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	name := c.FormValue("name")
	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	g, err := h.depGroupSvc.Create(ctx, sess.TenantID, name, c.FormValue("description"))
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(g.ID, 10))
}

func (h *Handler) DependencyGroupUpdate(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := h.depGroupSvc.Update(ctx, id, c.FormValue("name"), c.FormValue("description")); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(id, 10))
}

func (h *Handler) DependencyGroupDelete(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := h.depGroupSvc.Delete(ctx, id); err != nil {
		if errors.Is(err, services.ErrBuiltinProtected) {
			return echo.NewHTTPError(http.StatusBadRequest, "builtin groups cannot be deleted")
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups")
}

func (h *Handler) DependencyGroupConditionAdd(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	values := strings.Split(c.FormValue("values"), ",")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	if err := h.depGroupSvc.AddCondition(ctx, id, c.FormValue("param_path"), c.FormValue("operator"), values); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(id, 10)+"#conditions")
}

func (h *Handler) DependencyGroupConditionRemove(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	condID, err := strconv.ParseInt(c.Param("condID"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid condition id")
	}
	if err := h.depGroupSvc.RemoveCondition(ctx, id, condID); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/dependency-groups/"+strconv.FormatInt(id, 10)+"#conditions")
}
```
Add imports `errors`, `strings`, and `services` (`github.com/pluris/pluris/pkg/services`) to the file as needed.

- [ ] **Step 4: Write the detail/editor templates**

Add to `web/templates/dependency_groups.templ`, using `DetailShell` exactly as `policy_detail.templ` does (hero + tabs). Provide General (editable name/description with an Apply button), Conditions (existing rows + add row with param-path/operator/values inputs and a remove button per row), Modules (linked modules — read-only list for now; module linking is Task 7). Model the tab construction and `@DetailShell(...)` call on `web/templates/policy_detail.templ`:
```go
templ DependencyGroupDetailPage(g dependencygroups.Group, csrf string) {
	@DetailShell(
		"policy-dependency-groups",
		g.Name+" — Dependency Group",
		templ.Attributes{"data-testid": "page-dependency-group-detail", "data-policy-id": strconv.FormatInt(g.ID, 10)},
		dependencyGroupHero(g, csrf),
		dependencyGroupTabs(g, csrf),
	)
}
```
Write `dependencyGroupHero` (title + Delete form POSTing to `.../delete`, disabled with a tooltip when `g.Builtin`) and `dependencyGroupTabs` returning `[]TabSpec` with General/Conditions/Modules (check `TabSpec`/`HeroSpec` shapes in `web/templates/detail_shell.templ` and mirror `asset_detail_helpers.go`). For `DependencyGroupNewPage`, a simple form POSTing name+description to `/policy/dependency-groups`. Operator `<select>` options: `in`, `not_in`, `exists`. Param-path `<select>` options: iterate a curated list — reuse `params.Definitions` filtered to the applicability facts, or hardcode the three canonical computer paths (`computer/hardware/os_family`, `computer/hardware/os_package_family`, `computer/hardware/disk_encryption`) for v1.

- [ ] **Step 5: Add the routes**

In `console/server/server.go` after the list route:
```go
	e.GET("/policy/dependency-groups/new", h.DependencyGroupNew)
	e.POST("/policy/dependency-groups", h.DependencyGroupCreate)
	e.GET("/policy/dependency-groups/:id", h.DependencyGroupDetail)
	e.POST("/policy/dependency-groups/:id", h.DependencyGroupUpdate)
	e.POST("/policy/dependency-groups/:id/delete", h.DependencyGroupDelete)
	e.POST("/policy/dependency-groups/:id/conditions", h.DependencyGroupConditionAdd)
	e.POST("/policy/dependency-groups/:id/conditions/:condID/remove", h.DependencyGroupConditionRemove)
```
Order matters: register `/new` before `/:id` so it is not captured as an id.

- [ ] **Step 6: Generate + test**

Run: `make gen` then `go test -buildvcs=false ./console/... ./web/...`
Expected: PASS incl. `TestDependencyGroupCreateAndRBAC`.

- [ ] **Step 7: Commit** — STOP and report; owner commits.

---

### Task 7: Module ↔ dependency-group links + expandable modules

**Files:**
- Modify: `console/handlers/dependency_groups.go` (`ModuleDependencyAdd`, `ModuleDependencyRemove`)
- Modify: `console/handlers/handlers.go` (`PolicyModules` handler — pass per-module links to the template)
- Modify: `console/server/server.go` (routes)
- Modify: the modules template (find it: grep `PolicyModulesPage` / `func (h *Handler) PolicyModules` and the templ that renders module rows — likely in `web/templates/pages.templ`)
- Test: `console/handlers/dependency_groups_test.go` (add link round-trip)

**Interfaces:**
- Consumes: Task 4 service, Task 6 handlers.
- Produces: POST `/policy/modules/:moduleID/dependencies` (+`/:groupID/remove`); modules page shows each module's Platforms/Requires and (admin) a manage control.

- [ ] **Step 1: Write the failing test**

Add to `console/handlers/dependency_groups_test.go`:
```go
func TestModuleDependencyLinkRoundTrip(t *testing.T) {
	h, _, tenantID := newTestHandler(t)
	ctx := context.Background()
	if err := h.depGroupSvc.EnsureBuiltins(ctx, tenantID); err != nil {
		t.Fatal(err)
	}
	groups, _ := h.depGroupSvc.ListByTenant(ctx, tenantID)
	var anyLinux int64
	for _, g := range groups {
		if g.Slug == "any-linux" {
			anyLinux = g.ID
		}
	}
	if err := h.depGroupSvc.LinkModule(ctx, tenantID, "pluris.test.mod", anyLinux, "platform"); err != nil {
		t.Fatal(err)
	}
	links, _ := h.depGroupSvc.ListLinksForModule(ctx, tenantID, "pluris.test.mod")
	if len(links) != 1 || links[0].Role != "platform" {
		t.Fatalf("links=%+v", links)
	}
	if err := h.depGroupSvc.UnlinkModule(ctx, tenantID, "pluris.test.mod", anyLinux); err != nil {
		t.Fatal(err)
	}
	links, _ = h.depGroupSvc.ListLinksForModule(ctx, tenantID, "pluris.test.mod")
	if len(links) != 0 {
		t.Fatalf("expected 0 links after unlink, got %d", len(links))
	}
}
```

- [ ] **Step 2: Run to verify it fails / passes-service-only**

Run: `go test -buildvcs=false ./console/handlers/ -run TestModuleDependencyLinkRoundTrip`
Expected: PASS (this exercises Task 4 through the handler struct; it guards the wiring). If `newTestHandler` differs, adapt.

- [ ] **Step 3: Add the link handlers**

Append to `console/handlers/dependency_groups.go`:
```go
func (h *Handler) ModuleDependencyAdd(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	moduleID := c.Param("moduleID")
	groupID, err := strconv.ParseInt(c.FormValue("group_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid group id")
	}
	role := c.FormValue("role")
	if role != "platform" && role != "requirement" {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid role")
	}
	if err := h.depGroupSvc.LinkModule(ctx, sess.TenantID, moduleID, groupID, role); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/modules")
}

func (h *Handler) ModuleDependencyRemove(c echo.Context) error {
	if err := requireRoleAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid group id")
	}
	if err := h.depGroupSvc.UnlinkModule(ctx, sess.TenantID, c.Param("moduleID"), groupID); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/policy/modules")
}
```

- [ ] **Step 4: Feed per-module links into the modules page**

Read `console/handlers/handlers.go` `PolicyModules` and the templ that renders the module list. For each module in the page's view model, attach its resolved dependency groups grouped by role. Build a helper on the handler:
```go
// moduleDependencyView returns platform + requirement group names for one module.
func (h *Handler) moduleDependencyView(ctx context.Context, tenantID int64, moduleID string) (platforms, requirements []string) {
	links, err := h.depGroupSvc.ListLinksForModule(ctx, tenantID, moduleID)
	if err != nil {
		return nil, nil
	}
	groups, _ := h.depGroupSvc.ListByTenant(ctx, tenantID)
	byID := map[int64]string{}
	for _, g := range groups {
		byID[g.ID] = g.Name
	}
	for _, l := range links {
		name := byID[l.GroupID]
		if l.Role == "platform" {
			platforms = append(platforms, name)
		} else {
			requirements = append(requirements, name)
		}
	}
	return platforms, requirements
}
```
Ensure `PolicyModules` calls `h.depGroupSvc.EnsureBuiltins(ctx, sess.TenantID)` first (so the default sshd link exists), then attaches `platforms`/`requirements` to each module row's view model. Add those two `[]string` (or a small struct) fields to whatever module row struct the modules template consumes.

- [ ] **Step 5: Render expandable rows**

In the modules templ, make each module row expandable (a `<details>`/`<summary>` block or a toggle consistent with existing expandable UI — check if the codebase already has an expandable pattern before inventing one) revealing:
```
Platforms (match any): <chips of platforms, or "Any (no platform filter)">
Requires (match all):  <chips of requirements, or "None">
```
Plus, gated on admin, a small form to add a link (group `<select>` populated from the tenant's dependency groups + role `<select>` of platform/requirement, POST to `/policy/modules/{id}/dependencies`) and a remove control per existing link (POST to `/policy/modules/{id}/dependencies/{groupID}/remove`). Keep markup minimal and consistent with the page's existing table classes.

- [ ] **Step 6: Add routes**

`console/server/server.go`:
```go
	e.POST("/policy/modules/:moduleID/dependencies", h.ModuleDependencyAdd)
	e.POST("/policy/modules/:moduleID/dependencies/:groupID/remove", h.ModuleDependencyRemove)
```

- [ ] **Step 7: Generate + test**

Run: `make gen` then `go test -buildvcs=false ./...`
Expected: PASS.

- [ ] **Step 8: Commit** — STOP and report; owner commits.

---

### Task 8: Seeder facts + setup wiring + end-to-end verification

**Files:**
- Modify: `cmd/seed/*` (emit `os_package_family` + `disk_encryption` in the demo computer/server payloads)
- Modify: `console/handlers/auth.go` (`SetupSubmit` — call `h.depGroupSvc.EnsureBuiltins`)
- Test: full suite + headless e2e

**Interfaces:**
- Consumes: everything above.

- [ ] **Step 1: Seed the new facts**

In `cmd/seed`, find where computer/server subtype payloads are built (grep `os_family` under `cmd/seed`). Add both keys to the demo JSON payloads so seeded devices carry values, e.g. a computer: `"os_package_family":"deb","disk_encryption":"luks"`; a Windows-style demo asset if present: `"os_package_family":"other","disk_encryption":"bitlocker"`. Keep values consistent with each asset's existing `os_family`/`os_distribution`.

- [ ] **Step 2: Seed builtins at setup**

In `console/handlers/auth.go` `SetupSubmit`, after the existing `roleSvc.EnsureBuiltins` best-effort call, add (best-effort, do not fail setup):
```go
	_ = h.depGroupSvc.EnsureBuiltins(ctx, sess.TenantID)
```
Use whatever `ctx`/`sess`/tenant id is in scope at that point (mirror the roles call exactly).

- [ ] **Step 3: Full regen, build, suite**

Run:
```
make gen
go build -buildvcs=false ./...
go test -buildvcs=false -count=1 ./...
gofmt -l .
```
Expected: build ok, all packages `ok`, `gofmt -l` prints nothing.

- [ ] **Step 4: Headless end-to-end smoke**

Write a scratch script (in the scratchpad dir, NOT the repo) that: builds `cmd/console`, runs it with `PLURIS_HTTP_ADDR=:8091` against a fresh scratch DB (`PLURIS_DB` or CWD copy — never the repo `pluris.db`), does the CSRF setup POST, then curls:
- `GET /policy/dependency-groups` → 200, contains `page-dependency-groups` and "RPM-based OS".
- `GET /policy/dependency-groups/1` → 200.
- `POST /policy/dependency-groups` with an admin session cookie creating a group → 302, then the new group appears in the list.
- `GET /policy/modules` → 200, a module shows a Platforms/Requires block.
Confirm zero 5xx across the run. (Model the script on the Task 16 e2e from the previous plan; reuse its setup/login helper.)

- [ ] **Step 5: Update docs**

Update `docs/agent/HANDOFF.md`: add a Dependency Groups section marking the 8 tasks done with dates, file pointers, and test names. Update `README.md` "Planned next" if appropriate (dependency groups now exist). Do NOT edit `docs/funding/` for this.

- [ ] **Step 6: Commit** — STOP and report; owner commits, then does a manual browser walkthrough on :8081.

---

## Self-review notes (author)

- **Spec coverage:** typed platform-ANY/requirement-ALL model (Task 3 evaluator + Task 4 links) ✓; DB persistence + CRUD (Tasks 1,4,6) ✓; device facts as canonical params, no asset migration (Task 2) ✓; 8 default templates + default module links seeded idempotently (Task 4) ✓; operators in/not_in/exists (Task 3) ✓; pure tested evaluator (Task 3) ✓; sidebar item + list page (Task 5) ✓; DetailShell editor (Task 6) ✓; expandable modules + manage dependencies (Task 7) ✓; RBAC admin-gated (Tasks 5-7) ✓; route smoke + service + handler tests (Tasks 1,3,4,5,6,7) ✓; seeder + setup wiring + e2e (Task 8) ✓.
- **Type consistency:** `ValueJson` (sqlc camelCases `value_json`), `IsBuiltin` (from `is_builtin`), `ParamPath`, `GroupID`, `Role` used consistently across tasks. Facts map keyed by bare param key in both evaluator and tests.
- **Known verification points for implementers (flagged in-task, not placeholders):** exact `Register`/`FieldDef` field names in `web/lists/detail_tabs.go`; the real page-wrapper component + table classes in `web/templates/pages.templ`; `TabSpec`/`HeroSpec` shapes in `detail_shell.templ`; the modules template location + its row struct; the `cmd/seed` payload builder; `SetupSubmit`'s in-scope tenant id; presence of an existing `slugify` helper. Each names the file to read and the pattern to copy.
