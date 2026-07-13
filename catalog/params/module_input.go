package params

// Task 4.4 — module inputs are the ONE dynamic parameter namespace in
// Pluris: unlike the built-in entity registry (computer/server/...),
// module input parameters are defined per policy-module-version, in
// that version's parameters_schema JSON Schema column (see
// db/schema/008_module_scripts.sql). They cannot live in the static
// Schemas registry (paths.go) because they don't exist until a module
// author saves a schema in the module editor's Parameters tab.
//
// ModuleInputDefs is the single parser that turns that JSON Schema text
// into canonical params.ParamDefs under the reserved "module/input/<key>"
// namespace (see sources.go's namespace-reservation comment). Callers
// (console/handlers/params_api.go) mount the result as a per-request
// Source-shaped response section; the defs are NEVER registered globally
// via RegisterSource, because they're per-module, not tenant-wide or
// server-wide -- see the file header on sources.go for why "module/..."
// stays reserved-but-unimplemented as a package-level Source.
//
// Permission model: module input parameters carry NO per-def Permission
// (ParamDef.Permission stays ""). Visibility is all-or-nothing per
// module: whoever can view the module (authz.ModuleCanView) sees every
// input it declares. That's enforced by the API handler BEFORE it calls
// ModuleInputDefs, not by this parser or by EffectivePermission/
// FilterByGrants.

import (
	"encoding/json"
	"fmt"
	"sort"
)

// ModuleInputDef is one module input parameter, resolved from a module
// version's parameters_schema. Path is the canonical
// "module/input/<key>" mount point (INV-CPP-adjacent: module inputs are
// per-module dynamic parameters, not entries in the static Schemas
// registry, so they don't go through paths.go's buildPathIndex).
type ModuleInputDef struct {
	// Path is "module/input/<key>" -- what {{ param "..." }} tokens and
	// the /api/params tree use to address this input.
	Path string

	// Def is the canonical ParamDef -- Key, Label, Type, EnumValues.
	// Permission is always "" (see file header).
	Def ParamDef

	// Required mirrors the JSON Schema "required" array. Not part of
	// ParamDef (no built-in entity parameter is ever "required" in this
	// sense), surfaced separately for callers that care (e.g. a future
	// binding-time validation pass).
	Required bool

	// Default is the raw JSON Schema "default" value, re-marshaled to
	// its JSON text form ("" if the property declares none). Kept as
	// JSON text rather than typed so callers don't need a second type
	// switch on top of ParamDef.Type.
	Default string
}

// moduleInputSchema mirrors the JSON Schema subset the module editor's
// Parameters tab writes (see web/static/policy-module-editor.js's
// buildSchema): {"type":"object","properties":{...},"required":[...]}.
type moduleInputSchema struct {
	Type       string                        `json:"type"`
	Properties map[string]moduleInputPropDef `json:"properties"`
	Required   []string                      `json:"required"`
}

type moduleInputPropDef struct {
	Type    string          `json:"type"`
	Title   string          `json:"title"`
	Enum    []string        `json:"enum"`
	Default json.RawMessage `json:"default"`
}

// ModuleInputDefs parses a module version's parameters_schema column
// into ParamDefs under the "module/input/<key>" namespace. An empty
// schemaJSON (module declares no inputs -- the common case) returns a
// nil slice and no error. Malformed JSON, a non-object top-level shape,
// or a non-object "properties" value are reported as an error -- never
// a panic -- so a corrupt row degrades a caller to "no inputs shown",
// not a crash.
//
// Property key order in the source JSON is NOT preserved (encoding/json
// decodes objects into a Go map, which has no stable iteration order);
// output is sorted by key instead, which is what makes repeated calls
// -- and therefore the /api/params response for a given module version
// -- byte-identical, matching the determinism the built-in entity
// source already guarantees.
//
// Type mapping: JSON Schema "string"/"number"/"integer"/"boolean" map
// to TypeString/TypeFloat/TypeInt/TypeBool. A property with a non-empty
// "enum" is TypeEnum regardless of its declared "type" (the editor
// always writes enum properties as {"type":"string","enum":[...]}, per
// buildSchema's coerceDefault). Any other/unrecognized "type" value
// falls back to TypeString rather than erroring, so a hand-edited or
// future-versioned schema degrades gracefully instead of hiding the
// whole module's inputs over one unknown property.
func ModuleInputDefs(schemaJSON string) ([]ModuleInputDef, error) {
	if schemaJSON == "" {
		return nil, nil
	}
	var schema moduleInputSchema
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return nil, fmt.Errorf("params: malformed module parameters_schema: %w", err)
	}
	if len(schema.Properties) == 0 {
		return nil, nil
	}

	required := map[string]bool{}
	for _, k := range schema.Required {
		required[k] = true
	}

	keys := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]ModuleInputDef, 0, len(keys))
	for _, key := range keys {
		prop := schema.Properties[key]
		label := prop.Title
		if label == "" {
			label = key
		}
		typ := TypeString
		switch {
		case len(prop.Enum) > 0:
			typ = TypeEnum
		case prop.Type == "number":
			typ = TypeFloat
		case prop.Type == "integer":
			typ = TypeInt
		case prop.Type == "boolean":
			typ = TypeBool
		case prop.Type == "string" || prop.Type == "":
			typ = TypeString
		}
		def := ModuleInputDef{
			Path: "module/input/" + key,
			Def: ParamDef{
				Key:        key,
				Label:      label,
				Type:       typ,
				EnumValues: prop.Enum,
			},
			Required: required[key],
		}
		if len(prop.Default) > 0 {
			def.Default = string(prop.Default)
		}
		out = append(out, def)
	}
	return out, nil
}
