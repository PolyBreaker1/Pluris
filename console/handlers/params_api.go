package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/pluris/pluris/catalog/params"
	"github.com/pluris/pluris/pkg/auth"
	"github.com/pluris/pluris/pkg/authz"
)

// Task 1.2 (parameter registry API): GET /api/params exposes the
// canonical parameter registry (catalog/params) as a permission-filtered
// JSON tree — the single feed for the module editor parameter tree, the
// dependency-condition builder, and future agents. The response is
// filtered per session grant via SubtypeSchema.VisibleDefs (the safe,
// EffectivePermission-stamped entry point — NEVER filter raw schema defs
// with FilterByGrants here, their Permission fields are empty and
// everything would leak), so route-level RBAC stays open to any
// authenticated console user (see pkg/auth/rbac.go's "/api/params" entry).

// ParamsAPIOperator mirrors params.Operator for the operator dropdown.
type ParamsAPIOperator struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	NeedsValue bool   `json:"needsValue"`
}

// ParamsAPIParam is one parameter the caller may see: everything a UI
// needs to render a param picker entry, operator dropdown, and typed
// value input. Internal-only ParamDef fields (Permission, Category —
// redundant with the enclosing section — and the list-view Sort/Mono
// display hints) are deliberately omitted.
type ParamsAPIParam struct {
	// Path is the canonical "entity/section/param" identifier (INV-CPP) —
	// the value conditions and module bindings store.
	Path        string `json:"path"`
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Unit        string `json:"unit,omitempty"`
	// EnumValues is null unless Type == "enum" (order = display order).
	EnumValues []string `json:"enumValues"`
	// Filter is the list-view FilterMode ("" = not filterable there);
	// condition builders use Operators instead, but the mode is part of
	// the definition surface so it is mirrored.
	Filter string `json:"filter,omitempty"`
	// LinkTarget is the target entity kind when Type == "link".
	LinkTarget string `json:"linkTarget,omitempty"`
	// CompoundFields lists sub-field keys when Type == "compound".
	CompoundFields []string `json:"compoundFields,omitempty"`
	// Operators are the valid comparison operators for this param's type,
	// in dropdown display order.
	Operators []ParamsAPIOperator `json:"operators"`
}

// ParamsAPISection groups params exactly like the schema section does.
type ParamsAPISection struct {
	Key    string           `json:"key"`
	Label  string           `json:"label"`
	Params []ParamsAPIParam `json:"params"`
}

// ParamsAPIEntity is one parameter source (entity namespace). Entity is
// the canonical path slug ("user" for the identity schema, per INV-CPP).
type ParamsAPIEntity struct {
	Entity      string             `json:"entity"`
	Label       string             `json:"label"`
	PluralLabel string             `json:"pluralLabel"`
	Sections    []ParamsAPISection `json:"sections"`
}

// ParamsAPIResponse is the GET /api/params body. Sources is named for the
// catalog/params Source abstraction: today it holds only the built-in
// entity schemas, later tasks blend in tenant/module namespaces.
type ParamsAPIResponse struct {
	Sources []ParamsAPIEntity `json:"sources"`
}

// ParamsAPI handles GET /api/params. Route-level RBAC only requires an
// authenticated session; the per-grant filtering below IS the
// authorization — every caller gets exactly the subtree their grants may
// see. Entities and sections left empty by filtering are dropped
// entirely. Output is deterministic: canonical schema order, section
// order, param order.
func (h *Handler) ParamsAPI(c echo.Context) error {
	ctx := c.Request().Context()
	sess := auth.FromContext(ctx)
	if sess == nil {
		// Defense in depth: RequireAuth normally redirects before we run.
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}
	has := func(permKey string) bool { return sess.Grants.Can(permKey) }

	resp := ParamsAPIResponse{Sources: []ParamsAPIEntity{}}

	// Task 4.4: ?module_id=<urn> appends that module's dynamic
	// module/input/<key> parameters (parsed from its latest version's
	// parameters_schema) as one extra source, authorized exactly like
	// the module editor page itself (moduleAccessInput +
	// authz.ModuleCanView). Invalid/denied module_id fails the WHOLE
	// request with 404/403 rather than silently omitting the source --
	// a caller that asked for a specific module's inputs and got an
	// empty tree back would have no way to tell "no inputs declared"
	// from "you don't have access" from "no such module".
	if moduleID := c.QueryParam("module_id"); moduleID != "" {
		modSource, err := h.moduleInputsSource(ctx, sess, moduleID)
		if err != nil {
			return err
		}
		if modSource != nil {
			resp.Sources = append(resp.Sources, *modSource)
		}
	}

	for _, schema := range params.OrderedSchemas() {
		visible := schema.VisibleDefs(has)
		if len(visible) == 0 {
			continue
		}
		ent := ParamsAPIEntity{
			Entity:      schema.PathEntity,
			Label:       schema.Label,
			PluralLabel: schema.PluralLabel,
			Sections:    []ParamsAPISection{},
		}
		for _, md := range visible {
			// VisibleDefs is section-ordered, so a section change is
			// always a new section (keys are unique within a schema).
			if n := len(ent.Sections); n == 0 || ent.Sections[n-1].Key != md.SectionKey {
				ent.Sections = append(ent.Sections, ParamsAPISection{
					Key:    md.SectionKey,
					Label:  md.SectionLabel,
					Params: []ParamsAPIParam{},
				})
			}
			sec := &ent.Sections[len(ent.Sections)-1]
			sec.Params = append(sec.Params, toParamsAPIParam(md))
		}
		resp.Sources = append(resp.Sources, ent)
	}
	return c.JSON(http.StatusOK, resp)
}

// moduleInputsSource loads moduleID (a module URN), authorizes sess to
// view it exactly like PolicyModuleDetail does (moduleAccessInput +
// authz.ModuleCanView), and parses its LATEST version's
// parameters_schema into a ParamsAPIEntity. "Latest version" is: the
// most-recently-created draft if one exists, else the most-recently-
// created version of any state (ListVersionsByModule is already
// ORDER BY created_at DESC) -- the same selection PolicyModuleDetail
// uses to decide which version tab is initially shown, since a draft is
// what the module editor's Parameters tab is actually editing. A module
// with zero versions, or whose selected version declares no
// parameters_schema, returns (nil, nil) -- not an error, just nothing to
// add.
//
// Module inputs carry no per-def Permission (see catalog/params/
// module_input.go's file header): visibility is all-or-nothing, gated
// here by ModuleCanView before any schema is even parsed.
func (h *Handler) moduleInputsSource(ctx context.Context, sess *auth.UserSession, moduleID string) (*ParamsAPIEntity, error) {
	row, err := h.resolveTenantModuleByURN(ctx, sess, moduleID)
	if err != nil {
		return nil, err // already a 404 echo.HTTPError
	}
	access, err := h.moduleAccessInput(ctx, sess, row)
	if err != nil {
		return nil, err
	}
	if !authz.ModuleCanView(access) {
		return nil, echo.NewHTTPError(http.StatusForbidden, "not allowed to view this module")
	}

	versions, err := h.db.Queries.ListVersionsByModule(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, nil
	}
	selected := versions[0]
	for _, v := range versions {
		if v.State == "draft" {
			selected = v
			break
		}
	}

	defs, err := params.ModuleInputDefs(selected.ParametersSchema.String)
	if err != nil {
		// A corrupt parameters_schema shouldn't fail the whole /api/params
		// request -- degrade to "no module inputs" the same way
		// ModuleInputDefs documents for its own malformed-input case.
		return nil, nil
	}
	if len(defs) == 0 {
		return nil, nil
	}

	sec := ParamsAPISection{Key: "input", Label: "Inputs", Params: make([]ParamsAPIParam, 0, len(defs))}
	for _, d := range defs {
		ops := params.OperatorsForParam(d.Def)
		outOps := make([]ParamsAPIOperator, len(ops))
		for i, op := range ops {
			outOps[i] = ParamsAPIOperator{Key: op.Key, Label: op.Label, NeedsValue: op.NeedsValue}
		}
		sec.Params = append(sec.Params, ParamsAPIParam{
			Path:       d.Path,
			Key:        d.Def.Key,
			Label:      d.Def.Label,
			Type:       string(d.Def.Type),
			EnumValues: d.Def.EnumValues,
			Operators:  outOps,
		})
	}

	return &ParamsAPIEntity{
		Entity:      "module",
		Label:       "Module inputs: " + row.Title,
		PluralLabel: "Module inputs: " + row.Title,
		Sections:    []ParamsAPISection{sec},
	}, nil
}

// toParamsAPIParam maps a permission-resolved MountedDef to its wire shape.
func toParamsAPIParam(md params.MountedDef) ParamsAPIParam {
	d := md.Def
	ops := params.OperatorsForParam(d)
	outOps := make([]ParamsAPIOperator, len(ops))
	for i, op := range ops {
		outOps[i] = ParamsAPIOperator{Key: op.Key, Label: op.Label, NeedsValue: op.NeedsValue}
	}
	return ParamsAPIParam{
		Path:           md.Path,
		Key:            d.Key,
		Label:          d.Label,
		Description:    d.Description,
		Type:           string(d.Type),
		Unit:           d.Unit,
		EnumValues:     d.EnumValues,
		Filter:         string(d.Filter),
		LinkTarget:     d.LinkTarget,
		CompoundFields: d.CompoundFields,
		Operators:      outOps,
	}
}
