package services

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pluris/pluris/catalog/policymodules"
	"github.com/pluris/pluris/db"
)

// .pmdl import (2026-07-17 spec, Part 4.3). Parses a .pmdl archive into
// the structured columns only (INV-PMDL): module origin='imported',
// owner = importer, every version lands as a DRAFT regardless of its
// exported state (publish stays an explicit local decision, keeping
// INV-M3's publish gate). Security guards: size caps, entry-count cap,
// tar-slip path rejection, format_version gate.

const (
	pmdlMaxArchiveBytes = 16 << 20
	pmdlMaxFileBytes    = 4 << 20
	pmdlMaxEntries      = 512
)

var (
	// ErrPmdlInvalid wraps any structural parse failure (bad gzip/tar,
	// missing module.yaml, malformed manifest, unsafe path).
	ErrPmdlInvalid = errors.New("invalid .pmdl archive")
	// ErrPmdlTooLarge is returned when the archive exceeds the size or
	// entry-count caps.
	ErrPmdlTooLarge = errors.New(".pmdl archive exceeds size limits")
	// ErrPmdlUnsupportedVersion is returned for an unknown
	// format_version major.
	ErrPmdlUnsupportedVersion = errors.New("unsupported .pmdl format version")
	// ErrPmdlURNConflict is returned when the archive's module URN
	// already exists and as-copy was not requested.
	ErrPmdlURNConflict = errors.New("a module with this URN already exists")
)

type pmdlParsed struct {
	Module   pmdlModuleManifest
	Versions []pmdlParsedVersion
}

type pmdlParsedVersion struct {
	Manifest pmdlVersionManifest
	Scripts  map[string]string
}

// ParsePmdl reads and validates a .pmdl archive without touching the
// database.
func ParsePmdl(r io.Reader) (*pmdlParsed, error) {
	limited := io.LimitReader(r, pmdlMaxArchiveBytes+1)
	gz, err := gzip.NewReader(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: not a gzip archive", ErrPmdlInvalid)
	}
	tr := tar.NewReader(gz)

	var moduleYAML []byte
	versionYAML := map[string][]byte{}
	scripts := map[string]map[string]string{}
	entries := 0
	totalBytes := int64(0)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPmdlInvalid, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		entries++
		if entries > pmdlMaxEntries {
			return nil, ErrPmdlTooLarge
		}
		name := path.Clean(hdr.Name)
		if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "..") || strings.Contains(name, "/../") {
			return nil, fmt.Errorf("%w: unsafe path %q", ErrPmdlInvalid, hdr.Name)
		}
		if hdr.Size > pmdlMaxFileBytes {
			return nil, ErrPmdlTooLarge
		}
		content, err := io.ReadAll(io.LimitReader(tr, pmdlMaxFileBytes+1))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPmdlInvalid, err)
		}
		if int64(len(content)) > pmdlMaxFileBytes {
			return nil, ErrPmdlTooLarge
		}
		totalBytes += int64(len(content))
		if totalBytes > pmdlMaxArchiveBytes {
			return nil, ErrPmdlTooLarge
		}

		switch {
		case name == "module.yaml":
			moduleYAML = content
		case strings.HasSuffix(name, "/version.yaml") && strings.Count(name, "/") == 1:
			versionYAML[strings.TrimSuffix(name, "/version.yaml")] = content
		case strings.Contains(name, "/scripts/"):
			parts := strings.SplitN(name, "/scripts/", 2)
			if len(parts) == 2 {
				if scripts[parts[0]] == nil {
					scripts[parts[0]] = map[string]string{}
				}
				scripts[parts[0]][parts[1]] = string(content)
			}
		}
	}

	if moduleYAML == nil {
		return nil, fmt.Errorf("%w: missing module.yaml", ErrPmdlInvalid)
	}
	var mod pmdlModuleManifest
	if err := yaml.Unmarshal(moduleYAML, &mod); err != nil {
		return nil, fmt.Errorf("%w: malformed module.yaml", ErrPmdlInvalid)
	}
	if mod.FormatVersion > PmdlFormatVersion || mod.FormatVersion < 1 {
		return nil, ErrPmdlUnsupportedVersion
	}
	if strings.TrimSpace(mod.URN) == "" || strings.TrimSpace(mod.Title) == "" {
		return nil, fmt.Errorf("%w: module.yaml requires urn and title", ErrPmdlInvalid)
	}
	if len(versionYAML) == 0 {
		return nil, fmt.Errorf("%w: no versions in archive", ErrPmdlInvalid)
	}

	parsed := &pmdlParsed{Module: mod}
	dirs := make([]string, 0, len(versionYAML))
	for dir := range versionYAML {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		var vm pmdlVersionManifest
		if err := yaml.Unmarshal(versionYAML[dir], &vm); err != nil {
			return nil, fmt.Errorf("%w: malformed version.yaml in %s", ErrPmdlInvalid, dir)
		}
		if strings.TrimSpace(vm.Version) == "" {
			vm.Version = dir
		}
		parsed.Versions = append(parsed.Versions, pmdlParsedVersion{Manifest: vm, Scripts: scripts[dir]})
	}
	return parsed, nil
}

// ImportModule creates an imported module (origin='imported') with every
// archive version as a draft. asCopy suffixes the URN on collision;
// without it a collision returns ErrPmdlURNConflict.
func (s *PolicyModuleService) ImportModule(ctx context.Context, parsed *pmdlParsed, tenantID, ownerID int64, asCopy bool) (db.PolicyModule, error) {
	urn := parsed.Module.URN
	if _, err := s.db.Queries.GetPolicyModuleByURNIncludingDeleted(ctx, urn); err == nil {
		if !asCopy {
			return db.PolicyModule{}, ErrPmdlURNConflict
		}
		for n := 1; ; n++ {
			candidate := urn + "-imported-" + strconv.Itoa(n)
			if _, err := s.db.Queries.GetPolicyModuleByURNIncludingDeleted(ctx, candidate); err != nil {
				urn = candidate
				break
			}
		}
	}

	mod, err := s.CreateModule(ctx, &tenantID, &ownerID, urn, parsed.Module.Title, parsed.Module.Description)
	if err != nil {
		return db.PolicyModule{}, err
	}
	if err := s.db.Queries.SetModuleOrigin(ctx, db.SetModuleOriginParams{ID: mod.ID, Origin: "imported"}); err != nil {
		return db.PolicyModule{}, err
	}

	for _, pv := range parsed.Versions {
		fields := versionFieldsFromManifest(pv.Manifest)
		draft, err := s.CreateDraft(ctx, mod.ID, fields)
		if err != nil {
			return db.PolicyModule{}, fmt.Errorf("version %s: %w", pv.Manifest.Version, err)
		}
		for _, entry := range pv.Manifest.Scripts {
			key := entry.Phase + "/" + fmt.Sprintf("%02d_%s", entry.Seq, entry.Filename)
			source, ok := pv.Scripts[key]
			if !ok {
				continue
			}
			if _, err := s.SetScript(ctx, draft.ID, policymodules.LifecyclePhase(entry.Phase), entry.Filename, source); err != nil {
				return db.PolicyModule{}, fmt.Errorf("version %s script %s: %w", pv.Manifest.Version, key, err)
			}
		}
		for _, test := range pv.Manifest.Tests {
			scriptSource := test.ScriptSource
			scriptRef := test.ScriptRef
			if scriptRef != "" && test.RefSource != "" {
				// Portability fallback: the target console has no Scripts
				// library yet, so a ref-with-embedded-source imports as
				// inline source.
				scriptSource = test.RefSource
				scriptRef = ""
			}
			if _, err := s.AddVersionCondition(ctx, draft.ID, test.Kind, test.ParamPath, test.Operator, test.Values, scriptSource, scriptRef); err != nil {
				return db.PolicyModule{}, fmt.Errorf("version %s test: %w", pv.Manifest.Version, err)
			}
		}
		if pv.Manifest.ConditionsMatchMode == "any" {
			if err := s.SetConditionsMatchMode(ctx, draft.ID, "any"); err != nil {
				return db.PolicyModule{}, err
			}
		}
	}
	return s.db.Queries.GetPolicyModule(ctx, mod.ID)
}

func versionFieldsFromManifest(m pmdlVersionManifest) ModuleVersionFields {
	fields := ModuleVersionFields{
		Version:          m.Version,
		Scope:            m.Scope,
		Satisfies:        m.Satisfies,
		ParametersSchema: m.Parameters,
		ReportSchema:     m.ReportSchema,
		Conflicts:        m.Conflicts,
	}
	if fields.Scope == "" {
		fields.Scope = "machine"
	}
	for _, o := range m.TargetOS {
		fields.TargetOS = append(fields.TargetOS, policymodules.TargetOS(o))
	}
	for _, dep := range m.DependsOn {
		constraint := dep.Constraint
		if constraint == "" {
			constraint = "*"
		}
		fields.DependsOn = append(fields.DependsOn, policymodules.Dependency{ModuleID: dep.Module, VersionConstraint: constraint})
	}
	if m.Sandbox != "" && json.Valid([]byte(m.Sandbox)) {
		_ = json.Unmarshal([]byte(m.Sandbox), &fields.SandboxProfile)
	}
	return fields
}
