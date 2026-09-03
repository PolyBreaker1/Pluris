package services_test

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"

	"github.com/pluris/pluris/catalog/dependencygroups"
	"github.com/pluris/pluris/pkg/services"
)

func TestVersionConditionCRUDDraftGuards(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	pub := newTestIdentity(t, d, ten)

	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.cond-mod", "Cond Module", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AddVersionCondition(ctx, draft.ID, "param", "", "in", []string{"linux"}, "", ""); !errors.Is(err, services.ErrParamPathRequired) {
		t.Fatalf("empty path: want ErrParamPathRequired, got %v", err)
	}
	if _, err := svc.AddVersionCondition(ctx, draft.ID, "script", "", "contains", []string{"x"}, "src", "ref"); !errors.Is(err, services.ErrScriptSourceAmbiguous) {
		t.Fatalf("both source+ref: want ErrScriptSourceAmbiguous, got %v", err)
	}

	c1, err := svc.AddVersionCondition(ctx, draft.ID, "param", "computer/hardware/os_family", "in", []string{"linux"}, "", "")
	if err != nil {
		t.Fatalf("add param test: %v", err)
	}
	c2, err := svc.AddVersionCondition(ctx, draft.ID, "command", "", "contains", []string{"3"}, "uname -r", "")
	if err != nil {
		t.Fatalf("add command test: %v", err)
	}
	if _, err := svc.AddVersionCondition(ctx, draft.ID, "script", "", "contains", []string{"example"}, "", "custom-sh"); err != nil {
		t.Fatalf("add script-ref test: %v", err)
	}
	if c2.Seq <= c1.Seq {
		t.Fatalf("seq ordering: c1=%d c2=%d", c1.Seq, c2.Seq)
	}

	if err := svc.UpdateVersionCondition(ctx, draft.ID, c2.ID, "command", "", "equals", []string{"6.1"}, "uname -r", ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := svc.SetConditionsMatchMode(ctx, draft.ID, "any"); err != nil {
		t.Fatalf("match mode: %v", err)
	}
	if err := svc.SetConditionsMatchMode(ctx, draft.ID, "bogus"); !errors.Is(err, services.ErrInvalidMatchMode) {
		t.Fatalf("bad mode: want ErrInvalidMatchMode, got %v", err)
	}

	if _, err := svc.SetScript(ctx, draft.ID, "apply", "apply.sh", "#!/bin/bash\ntrue"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, draft.ID, pub); err != nil {
		t.Fatalf("publish: %v", err)
	}

	if _, err := svc.AddVersionCondition(ctx, draft.ID, "param", "computer/hardware/os_family", "in", []string{"linux"}, "", ""); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("add on published: want ErrVersionNotDraft, got %v", err)
	}
	if err := svc.UpdateVersionCondition(ctx, draft.ID, c1.ID, "param", "computer/hardware/os_family", "equals", []string{"x"}, "", ""); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("update on published: want ErrVersionNotDraft, got %v", err)
	}
	if err := svc.RemoveVersionCondition(ctx, draft.ID, c1.ID); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("remove on published: want ErrVersionNotDraft, got %v", err)
	}
	if err := svc.SetConditionsMatchMode(ctx, draft.ID, "all"); !errors.Is(err, services.ErrVersionNotDraft) {
		t.Fatalf("mode on published: want ErrVersionNotDraft, got %v", err)
	}
	if err := svc.RemoveVersionCondition(ctx, 999999, c1.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing version: want ErrNoRows, got %v", err)
	}

	conds, err := svc.ListVersionConditions(ctx, draft.ID)
	if err != nil || len(conds) != 3 {
		t.Fatalf("list: n=%d err=%v", len(conds), err)
	}
}

func TestVersionConditionsGroupEval(t *testing.T) {
	svc, _, ten := newModuleSvc(t)
	ctx := context.Background()
	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.eval-mod", "Eval Module", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionCondition(ctx, draft.ID, "param", "computer/hardware/os_family", "in", []string{"linux"}, "", ""); err != nil {
		t.Fatal(err)
	}
	cmd, err := svc.AddVersionCondition(ctx, draft.ID, "command", "", "contains", []string{"3"}, "uname -r", "")
	if err != nil {
		t.Fatal(err)
	}

	conds, err := svc.ListVersionConditions(ctx, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	g := services.VersionConditionsGroup("all", conds)

	facts := map[string]string{"os_family": "linux"}
	if got := dependencygroups.EvalGroup(g, facts); got != "unknown" {
		t.Fatalf("command unreported: want unknown, got %s", got)
	}
	facts["script_result/"+itoa(cmd.ID)] = "3.10.0"
	if got := dependencygroups.EvalGroup(g, facts); got != "pass" {
		t.Fatalf("all reported: want pass, got %s", got)
	}
	facts["script_result/"+itoa(cmd.ID)] = dependencygroups.ExitFailSentinel
	if got := dependencygroups.EvalGroup(g, facts); got != "fail" {
		t.Fatalf("exit fail: want fail, got %s", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestForkLatestPublishedCopiesTestsAndMatchMode(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	pub := newTestIdentity(t, d, ten)

	mod, err := svc.CreateModule(ctx, &ten, nil, "tenant.acme.fork-mod", "Fork Module", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{Version: "1.0.0", Scope: "machine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionCondition(ctx, draft.ID, "command", "", "contains", []string{"3"}, "uname -r", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetConditionsMatchMode(ctx, draft.ID, "any"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, draft.ID, "apply", "apply.sh", "true"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Publish(ctx, draft.ID, pub); err != nil {
		t.Fatal(err)
	}

	fork, err := svc.ForkLatestPublished(ctx, mod.ID, "1.1.0")
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	conds, err := svc.ListVersionConditions(ctx, fork.ID)
	if err != nil || len(conds) != 1 || conds[0].Kind != "command" || conds[0].ScriptSource != "uname -r" {
		t.Fatalf("fork lost tests: n=%d err=%v", len(conds), err)
	}
	frow, _ := d.Queries.GetPolicyModuleVersion(ctx, fork.ID)
	if frow.ConditionsMatchMode != "any" {
		t.Fatalf("fork lost match mode: %q", frow.ConditionsMatchMode)
	}
}
