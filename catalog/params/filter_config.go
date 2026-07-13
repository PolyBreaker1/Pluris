package params

import "encoding/json"

// FilterConfig is the JSON structure served to the filter builder JS.
// It contains everything the client needs to render the filter UI for
// a given subtype: available parameters (grouped by section), their
// types, valid operators, enum values, and units.
type FilterConfig struct {
	Subtype  string                 `json:"subtype"`
	Sections []FilterSection        `json:"sections"`
	Params   map[string]FilterParam `json:"params"`
}

// FilterSection is a group of parameters in the filter builder dropdown.
type FilterSection struct {
	Key    string   `json:"key"`
	Label  string   `json:"label"`
	Params []string `json:"params"`
}

// FilterParam is the client-facing definition of a single filterable parameter.
type FilterParam struct {
	Key        string           `json:"key"`
	Label      string           `json:"label"`
	Type       string           `json:"type"`
	Unit       string           `json:"unit,omitempty"`
	Mono       bool             `json:"mono,omitempty"`
	EnumValues []string         `json:"enumValues,omitempty"`
	Operators  []FilterOperator `json:"operators"`
}

// FilterOperator is one operator option in the filter builder.
type FilterOperator struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	NeedsValue bool   `json:"needsValue"`
}

// BuildFilterConfig produces the FilterConfig for a given subtype schema.
// This is the single source the JS filter builder reads — no parallel
// definitions in the frontend.
func BuildFilterConfig(schema *SubtypeSchema) FilterConfig {
	cfg := FilterConfig{
		Subtype: schema.Subtype,
		Params:  make(map[string]FilterParam),
	}

	for _, sec := range schema.Sections {
		// Collect only filterable params for this section.
		var filterable []string
		for _, key := range sec.Params {
			def := DefByKey(key)
			if def == nil {
				continue
			}
			filterable = append(filterable, key)

			// Build the param entry if not already done (a param could
			// theoretically appear in multiple sections, though we avoid that).
			if _, exists := cfg.Params[key]; !exists {
				ops := OperatorsForParam(*def)
				fops := make([]FilterOperator, 0, len(ops))
				for _, op := range ops {
					fops = append(fops, FilterOperator{
						Key:        op.Key,
						Label:      op.Label,
						NeedsValue: op.NeedsValue,
					})
				}
				cfg.Params[key] = FilterParam{
					Key:        key,
					Label:      def.Label,
					Type:       string(def.Type),
					Unit:       def.Unit,
					Mono:       def.Mono,
					EnumValues: def.EnumValues,
					Operators:  fops,
				}
			}
		}
		cfg.Sections = append(cfg.Sections, FilterSection{
			Key:    sec.Key,
			Label:  sec.Label,
			Params: filterable,
		})
	}
	return cfg
}

// FilterConfigJSON returns the JSON-encoded filter config for a subtype.
// Safe to embed in a <script type="application/json"> data island.
func FilterConfigJSON(subtype string) string {
	schema := SchemaBySubtype(subtype)
	if schema == nil {
		return "{}"
	}
	cfg := BuildFilterConfig(schema)
	b, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(b)
}
