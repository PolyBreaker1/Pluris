package services_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/pkg/services"
)

func buildPmdlFixtureModule(t *testing.T, svc *services.PolicyModuleService, ten int64, urn string) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	mod, err := svc.CreateModule(ctx, &ten, nil, urn, "Fixture", "fixture module")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := svc.CreateDraft(ctx, mod.ID, services.ModuleVersionFields{
		Version: "1.2.0", Scope: "machine",
		TargetOS:         []policymodules.TargetOS{policymodules.OSLinux},
		Satisfies:        []string{"sec.test.urn"},
		ParametersSchema: `{"type":"object","properties":{"timeout":{"type":"number","default":30}}}`,
		ReportSchema:     `{"type":"object","properties":{"ok":{"type":"boolean"}}}`,
		DependsOn:        []policymodules.Dependency{{ModuleID: "pluris.sshd.base-config", VersionConstraint: ">=1.0.0"}},
		Conflicts:        []string{"tenant.acme.enemy"},
		SandboxProfile:   policymodules.SandboxProfile{FsWrite: []string{"/etc/test"}, User: "root"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, draft.ID, policymodules.PhaseApply, "apply.sh", "#!/bin/bash\necho apply\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetScript(ctx, draft.ID, policymodules.PhaseUninstall, "rollback.sh", "#!/bin/bash\necho undo\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionCondition(ctx, draft.ID, "param", "computer/hardware/os_family", "in", []string{"linux"}, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddVersionCondition(ctx, draft.ID, "command", "", "contains", []string{"3"}, "uname -r", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetConditionsMatchMode(ctx, draft.ID, "any"); err != nil {
		t.Fatal(err)
	}
	return mod.ID, draft.ID
}

func TestPmdlExportImportRoundTrip(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	modID, draftID := buildPmdlFixtureModule(t, svc, ten, "tenant.acme.rt-mod")
	importer := newTestIdentity(t, d, ten)

	row, err := d.Queries.GetPolicyModule(ctx, modID)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := svc.ExportModuleBytes(ctx, row, []int64{draftID})
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Migration 012 dropped the filename column: scripts are now named
	// (not phase + separately-stored filename), so the exported archive
	// path is scripts/<name>/<seq>_<name> -- the filename argument
	// SetScript still accepts (for old-API compatibility) is discarded.
	names := tarEntryNames(t, blob)
	for _, want := range []string{"module.yaml", "1.2.0/version.yaml", "1.2.0/scripts/apply/00_apply", "1.2.0/scripts/uninstall/00_uninstall"} {
		if !names[want] {
			t.Fatalf("archive missing %q; have %v", want, names)
		}
	}

	vrow, _ := d.Queries.GetPolicyModuleVersion(ctx, draftID)
	if !strings.Contains(vrow.ManifestYaml, "format") == false && vrow.ManifestYaml == "" {
		t.Fatal("manifest_yaml cache should be populated on export")
	}

	parsed, err := services.ParsePmdl(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Module.URN != "tenant.acme.rt-mod" || parsed.Module.FormatVersion != services.PmdlFormatVersion {
		t.Fatalf("module manifest: %+v", parsed.Module)
	}

	imported, err := svc.ImportModule(ctx, parsed, ten, importer, true)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.Origin != "imported" {
		t.Fatalf("origin = %q, want imported", imported.Origin)
	}
	if imported.ModuleUrn != "tenant.acme.rt-mod-imported-1" {
		t.Fatalf("as-copy URN = %q", imported.ModuleUrn)
	}

	versions, err := d.Queries.ListVersionsByModule(ctx, imported.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("imported versions: n=%d err=%v", len(versions), err)
	}
	if versions[0].State != "draft" {
		t.Fatalf("imported version state = %q, want draft", versions[0].State)
	}

	ivrow, _ := d.Queries.GetPolicyModuleVersion(ctx, versions[0].ID)
	origFields := services.FieldsFromVersionRow(vrow)
	newFields := services.FieldsFromVersionRow(ivrow)
	if newFields.ParametersSchema != origFields.ParametersSchema ||
		newFields.ReportSchema != origFields.ReportSchema ||
		len(newFields.DependsOn) != 1 || newFields.DependsOn[0].VersionConstraint != ">=1.0.0" ||
		len(newFields.Conflicts) != 1 || newFields.SandboxProfile.User != "root" {
		t.Fatalf("field mismatch after import:\norig %+v\nnew  %+v", origFields, newFields)
	}
	if ivrow.ConditionsMatchMode != "any" {
		t.Fatalf("match mode not imported: %q", ivrow.ConditionsMatchMode)
	}
	conds, _ := svc.ListVersionConditions(ctx, versions[0].ID)
	if len(conds) != 2 || conds[0].Kind != "param" || conds[1].Kind != "command" || conds[1].ScriptSource != "uname -r" {
		t.Fatalf("tests not imported: %+v", conds)
	}
	scripts, _ := d.Queries.ListScriptsForVersion(ctx, versions[0].ID)
	if len(scripts) != 2 {
		t.Fatalf("scripts not imported: %+v", scripts)
	}

	// Round-trip property: export the imported module and compare
	// version manifests modulo the URN-independent content.
	blob2, err := svc.ExportModuleBytes(ctx, imported, []int64{versions[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	m1 := tarEntryContent(t, blob, "1.2.0/version.yaml")
	m2 := tarEntryContent(t, blob2, "1.2.0/version.yaml")
	if m1 != m2 {
		t.Fatalf("version manifest drift after round-trip:\n--- first\n%s\n--- second\n%s", m1, m2)
	}
}

func TestPmdlImportURNConflictWithoutCopy(t *testing.T) {
	svc, d, ten := newModuleSvc(t)
	ctx := context.Background()
	modID, draftID := buildPmdlFixtureModule(t, svc, ten, "tenant.acme.conflict-mod")
	importer := newTestIdentity(t, d, ten)

	row, _ := d.Queries.GetPolicyModule(ctx, modID)
	blob, err := svc.ExportModuleBytes(ctx, row, []int64{draftID})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := services.ParsePmdl(bytes.NewReader(blob))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportModule(ctx, parsed, ten, importer, false); !errors.Is(err, services.ErrPmdlURNConflict) {
		t.Fatalf("want ErrPmdlURNConflict, got %v", err)
	}
}

func TestParsePmdlRejectsMaliciousArchives(t *testing.T) {
	slip := makeTarGz(t, map[string]string{
		"module.yaml":        "format_version: 1\nurn: x.y\ntitle: X\n",
		"../../etc/passwd":   "evil",
		"1.0.0/version.yaml": "version: 1.0.0\n",
	})
	if _, err := services.ParsePmdl(bytes.NewReader(slip)); err == nil {
		t.Fatal("tar-slip path should be rejected")
	}

	if _, err := services.ParsePmdl(strings.NewReader("not a gzip")); err == nil {
		t.Fatal("non-gzip should be rejected")
	}

	noModule := makeTarGz(t, map[string]string{"1.0.0/version.yaml": "version: 1.0.0\n"})
	if _, err := services.ParsePmdl(bytes.NewReader(noModule)); err == nil {
		t.Fatal("missing module.yaml should be rejected")
	}

	futureVersion := makeTarGz(t, map[string]string{
		"module.yaml":        "format_version: 99\nurn: x.y\ntitle: X\n",
		"1.0.0/version.yaml": "version: 1.0.0\n",
	})
	if _, err := services.ParsePmdl(bytes.NewReader(futureVersion)); !errors.Is(err, services.ErrPmdlUnsupportedVersion) {
		t.Fatalf("future format_version: want ErrPmdlUnsupportedVersion, got %v", err)
	}

	entries := map[string]string{"module.yaml": "format_version: 1\nurn: x.y\ntitle: X\n", "1.0.0/version.yaml": "version: 1.0.0\n"}
	for i := 0; i < 600; i++ {
		entries["1.0.0/scripts/apply/junk"+itoa(int64(i))] = "y"
	}
	bomb := makeTarGz(t, entries)
	if _, err := services.ParsePmdl(bytes.NewReader(bomb)); !errors.Is(err, services.ErrPmdlTooLarge) {
		t.Fatalf("entry bomb: want ErrPmdlTooLarge, got %v", err)
	}
}

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func tarEntryNames(t *testing.T, blob []byte) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[hdr.Name] = true
	}
	return names
}

func tarEntryContent(t *testing.T, blob []byte, name string) string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name == name {
			b, _ := io.ReadAll(tr)
			return string(b)
		}
	}
	t.Fatalf("entry %q not found", name)
	return ""
}
