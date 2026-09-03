package services

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pluris/pluris/db"
)

// .pmdl export (2026-07-17 spec, Part 4): <urn>.pmdl is a gzip'd tar
// generated from the structured columns — the DB stays the source of
// truth (INV-PMDL); the manifest is derived output, never read back as
// live state inside this console. Layout:
//
//	module.yaml
//	<version>/version.yaml
//	<version>/scripts/<phase>/<seq>_<filename>
//
// format_version gates import compatibility; the signature block is
// reserved (empty) until the agent work defines the trust chain.

const PmdlFormatVersion = 1

type pmdlModuleManifest struct {
	FormatVersion int    `yaml:"format_version"`
	URN           string `yaml:"urn"`
	Title         string `yaml:"title"`
	Description   string `yaml:"description"`
	Origin        string `yaml:"origin"`
}

type pmdlTest struct {
	Kind         string   `yaml:"kind"`
	ParamPath    string   `yaml:"param_path,omitempty"`
	ScriptSource string   `yaml:"script_source,omitempty"`
	ScriptRef    string   `yaml:"script_ref,omitempty"`
	RefSource    string   `yaml:"ref_source,omitempty"`
	Operator     string   `yaml:"operator"`
	Values       []string `yaml:"values"`
}

type pmdlDependency struct {
	Module     string `yaml:"module"`
	Constraint string `yaml:"constraint"`
}

type pmdlScriptEntry struct {
	Phase    string `yaml:"phase"`
	Seq      int64  `yaml:"seq"`
	Filename string `yaml:"filename"`
}

type pmdlVersionManifest struct {
	Version             string            `yaml:"version"`
	State               string            `yaml:"state"`
	TargetOS            []string          `yaml:"target_os"`
	Scope               string            `yaml:"scope"`
	Satisfies           []string          `yaml:"satisfies"`
	Parameters          string            `yaml:"parameters"`
	Sandbox             string            `yaml:"sandbox"`
	ReportSchema        string            `yaml:"report_schema"`
	ConditionsMatchMode string            `yaml:"conditions_match_mode"`
	Tests               []pmdlTest        `yaml:"tests"`
	DependsOn           []pmdlDependency  `yaml:"depends_on"`
	Conflicts           []string          `yaml:"conflicts"`
	Scripts             []pmdlScriptEntry `yaml:"scripts"`
	Signature           struct{}          `yaml:"signature"`
}

// ExportModule writes <urn>.pmdl content for the given version rows to
// w. Pure with respect to its inputs: rows in, tarball out. Also
// persists each exported version's manifest into manifest_yaml as a
// cache/audit artifact (nothing reads it back).
func (s *PolicyModuleService) ExportModule(ctx context.Context, row db.PolicyModule, versionIDs []int64, w io.Writer) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	now := time.Now()

	writeFile := func(name string, content []byte) error {
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Mode: 0o644, Size: int64(len(content)), ModTime: now,
		}); err != nil {
			return err
		}
		_, err := tw.Write(content)
		return err
	}

	moduleManifest, err := yaml.Marshal(pmdlModuleManifest{
		FormatVersion: PmdlFormatVersion,
		URN:           row.ModuleUrn,
		Title:         row.Title,
		Description:   row.Description.String,
		Origin:        moduleOrigin(row),
	})
	if err != nil {
		return err
	}
	if err := writeFile("module.yaml", moduleManifest); err != nil {
		return err
	}

	for _, vid := range versionIDs {
		vrow, err := s.db.Queries.GetPolicyModuleVersion(ctx, vid)
		if err != nil {
			return err
		}
		if vrow.ModuleID != row.ID {
			return fmt.Errorf("version %d does not belong to module %s", vid, row.ModuleUrn)
		}
		manifest, scripts, err := s.buildVersionManifest(ctx, vrow)
		if err != nil {
			return err
		}
		manifestYAML, err := yaml.Marshal(manifest)
		if err != nil {
			return err
		}
		dir := vrow.Version + "/"
		if err := writeFile(dir+"version.yaml", manifestYAML); err != nil {
			return err
		}
		for _, sc := range scripts {
			name := dir + "scripts/" + sc.Name + "/" + fmt.Sprintf("%02d_%s", sc.Seq, sanitizeFilename(sc.Name, sc.Name))
			if err := writeFile(name, []byte(sc.Source)); err != nil {
				return err
			}
		}
		_ = s.db.Queries.CacheVersionManifest(ctx, db.CacheVersionManifestParams{
			ID: vrow.ID, ManifestYaml: string(manifestYAML),
		})
	}

	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func (s *PolicyModuleService) buildVersionManifest(ctx context.Context, vrow db.PolicyModuleVersion) (pmdlVersionManifest, []db.PolicyModuleScript, error) {
	fields := FieldsFromVersionRow(vrow)
	m := pmdlVersionManifest{
		Version:             vrow.Version,
		State:               vrow.State,
		Scope:               vrow.Scope,
		Satisfies:           emptyIfNil(fields.Satisfies),
		Parameters:          fields.ParametersSchema,
		Sandbox:             vrow.SandboxProfile,
		ReportSchema:        vrow.ReportSchema,
		ConditionsMatchMode: vrow.ConditionsMatchMode,
		Conflicts:           emptyIfNil(fields.Conflicts),
	}
	m.TargetOS = make([]string, 0, len(fields.TargetOS))
	for _, o := range fields.TargetOS {
		m.TargetOS = append(m.TargetOS, string(o))
	}
	m.DependsOn = make([]pmdlDependency, 0, len(fields.DependsOn))
	for _, dep := range fields.DependsOn {
		m.DependsOn = append(m.DependsOn, pmdlDependency{Module: dep.ModuleID, Constraint: dep.VersionConstraint})
	}

	conds, err := s.db.Queries.ListVersionConditions(ctx, vrow.ID)
	if err != nil {
		return m, nil, err
	}
	m.Tests = make([]pmdlTest, 0, len(conds))
	for _, c := range conds {
		var vals []string
		_ = json.Unmarshal([]byte(c.ValueJson), &vals)
		test := pmdlTest{
			Kind: c.Kind, ParamPath: c.ParamPath, Operator: c.Operator,
			Values: emptyIfNil(vals), ScriptSource: c.ScriptSource, ScriptRef: c.ScriptRef,
		}
		// A script_ref test embeds the referenced source for portability
		// (the importing console may not have the library script). The
		// Scripts library is not built yet, so resolution currently
		// yields nothing — the field stays reserved in the format.
		m.Tests = append(m.Tests, test)
	}

	scripts, err := s.db.Queries.ListScriptsForVersion(ctx, vrow.ID)
	if err != nil {
		return m, nil, err
	}
	sort.Slice(scripts, func(i, j int) bool {
		if scripts[i].Name != scripts[j].Name {
			return scripts[i].Name < scripts[j].Name
		}
		return scripts[i].Seq < scripts[j].Seq
	})
	m.Scripts = make([]pmdlScriptEntry, 0, len(scripts))
	for _, sc := range scripts {
		m.Scripts = append(m.Scripts, pmdlScriptEntry{Phase: sc.Name, Seq: sc.Seq, Filename: sanitizeFilename(sc.Name, sc.Name)})
	}
	return m, scripts, nil
}

// sanitizeFilename strips path separators so a stored filename can
// never escape its scripts/<phase>/ directory in the archive.
func sanitizeFilename(name, phase string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	if name == "" || name == "." || name == ".." {
		return phase + ".script"
	}
	return name
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ExportVersionIDs resolves which versions ?version=/?all= select:
// explicit id, else every non-revoked version when all is set, else the
// latest published version, else the most recent version of any state.
func (s *PolicyModuleService) ExportVersionIDs(ctx context.Context, moduleID int64, explicitVersionID int64, all bool) ([]int64, error) {
	rows, err := s.db.Queries.ListVersionsByModule(ctx, moduleID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNoExportableVersion
	}
	if explicitVersionID != 0 {
		for _, r := range rows {
			if r.ID == explicitVersionID {
				return []int64{r.ID}, nil
			}
		}
		return nil, ErrNoExportableVersion
	}
	if all {
		var ids []int64
		for _, r := range rows {
			if r.State != "revoked" {
				ids = append(ids, r.ID)
			}
		}
		if len(ids) == 0 {
			return nil, ErrNoExportableVersion
		}
		return ids, nil
	}
	for _, r := range rows {
		if r.State == "published" {
			return []int64{r.ID}, nil
		}
	}
	return []int64{rows[0].ID}, nil
}

// ExportModuleBytes is ExportModule into a buffer (convenience for
// handlers and tests).
func (s *PolicyModuleService) ExportModuleBytes(ctx context.Context, row db.PolicyModule, versionIDs []int64) ([]byte, error) {
	var buf bytes.Buffer
	if err := s.ExportModule(ctx, row, versionIDs, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
