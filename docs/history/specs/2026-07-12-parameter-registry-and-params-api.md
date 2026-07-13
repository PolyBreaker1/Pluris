# Parameter Registry & `/api/params` — Design Record

**Date:** 2026-07-12
**Author:** Claude (Opus 4.8) + Peter (owner)
**Status:** Shipped (Phase 1 of the 2026-07 overhaul)
**Builds on:** `catalog/params/` (INV-CPP, pre-existing) — see [[parameters]] for the base registry model.

---

## Goal

Before this work, parameter/operator lists were duplicated ad hoc across the condition builder, filter UIs, and (once it existed) the module editor's parameter tree — each consumer re-derived "what params exist and what can I do with them" independently, and permission-scoping a param for a given session had no single answer. This record documents the single-source model that replaced that: `catalog/params/` is now THE registry for every parameter a Pluris session can see, entity or module, and `GET /api/params` is the one JSON contract every UI consumer reads instead of re-deriving.

## The registry model

- **`ParamDef.Permission`** — a new field on every `ParamDef`, the permission key (if any) gating that param's visibility. Empty string = no extra gate beyond the caller's general access to the entity.
- **`SubtypeSchema.DefaultPermission`** — a schema-level fallback permission applied to any mounted param that doesn't set its own `Permission`, so a whole schema can be gated once instead of per-field.
- **`EffectivePermission(path string, ...) string`** — resolves the permission key for a canonical path: entity paths resolve via the mounting `SubtypeSchema` + its `DefaultPermission` fallback; non-entity paths (module inputs — see below) fall through to `Source.Resolve(path).Permission` with no schema-default fallback (module sources have no `SubtypeSchema`).
- **`FilterByGrants(defs []ParamDef, grants ...) []ParamDef`**, **`VisibleDefs`**, **`EffectiveDefs`** — the one permission-filtering path. Every caller that needs "which params can this session see" calls through these, never hand-rolls a field blocklist. This closes the gap the old ad hoc per-page filtering left open.
- **`Source` interface gained `Resolve(path string) (ParamDef, bool)`** (`catalog/params/sources.go`). `entitySource.Resolve` adapts trivially onto the pre-existing `ResolvePath`. This closed an asymmetry: `AllPaths()` could already enumerate every registered source's paths, but there was no `Source`-generic way to resolve a path string back to a `ParamDef` — only the entity-specific `ResolvePath`. New `ResolveDef(path string) (ParamDef, bool)` (`catalog/params/paths.go`) tries `ResolvePath` first (unchanged error surface for entity paths), then walks registered non-entity sources in order.

## Module input namespace

A module's inputs are defined exactly once, as a JSON Schema in `policy_module_versions.parameters_schema` (the module editor's Parameters tab is the one place that writes it — see the module-persistence spec). They are exposed as parameters under the path prefix **`module/input/<key>`**, parsed by `catalog/params/module_input.go`'s `ModuleInputDefs(schemaJSON string) ([]ModuleInputDef, error)`:

- `ModuleInputDef{ Path, Def ParamDef, Required, Default }` — `Def` uses the same `ParamDef` shape as entity params (type/label/enum/operators), so a module input renders through the exact same UI code as `computer/hardware/ram_mb` would.
- **Deliberately NOT registered globally** via `RegisterSource` — module inputs don't exist independent of a specific module version being loaded, so they're resolved per-request (`?module_id=`), not baked into the package-level source list every other entity uses.
- Property order is sorted by key (JSON object → Go map decode does not preserve source order) — this is what makes the module source of an `/api/params` response byte-identical across repeated calls, matching the determinism guarantee entity sources already had.
- Unknown/unrecognized `"type"` values degrade to `TypeString` (forward-compat) rather than erroring; only genuinely malformed JSON / non-object schema shapes return an error.
- **Permission model**: a module input's `ParamDef.Permission` is always `""` — visibility is all-or-nothing per module, gated entirely by `authz.ModuleCanView` in the API handler BEFORE any schema is parsed, not per-field. There is no finer per-input permission today.

## `GET /api/params` — the JSON contract

`console/handlers/params_api.go`.

- **No `module_id`**: returns every entity source (computer/server/printer/desk/user today), permission-filtered for the calling session via `VisibleDefs`/`FilterByGrants`. Shape: a list of `{ entity, label, sections: [{ key, label, params: [{ key, path, label, type, ..., operators: [...] }] }] }`.
- **`?module_id=<urn>`**: additionally resolves the named module (via `resolveTenantModuleByURN`, cross-tenant → 404 same shape as the module editor), checks `authz.ModuleCanView` (denied → 403), and — if the check passes — appends ONE module source: `{ entity: "module", label: "Module inputs: <title>", sections: [{ key: "input", label: "Inputs", params: [...] }] }`, built from `ModuleInputDefs` against the resolved version's `parameters_schema`, using `OperatorsForParam` per input exactly like entity sources do.
- **"Latest version" for a module's inputs** = the most-recently-created DRAFT if one exists, else the most-recently-created version of any state (`ListVersionsByModule` is `ORDER BY created_at DESC`; prefer a draft). This mirrors what the module editor's own detail page does to pick its default-open version tab — a draft is what the Parameters tab is actually editing.
- A corrupt `parameters_schema` (should never happen — the editor validates JSON before saving) degrades to "no module source" rather than 500ing the whole response; an unknown/denied `module_id` fails the WHOLE request (404/403) rather than silently omitting the module source — the brief's requirement that a caller distinguish "no inputs" from "no access" from "no such module."
- Consumers: the filter-builder UI, the condition builder's parameter dropdown, and the module editor's parameter tree + `{{ param "..." }}` autocomplete (`web/static/policy-module-editor.js`'s `loadParamTree`, which appends `?module_id=<urn>` using the scripts-root's `data-module-urn` attribute already present for script-save calls).

## Single-source rule (INV-PS)

- Entity/tenant parameters: `catalog/params/` only. No hardcoded param or operator list survives anywhere else — every consumer reads the registry or `/api/params`.
- Module input parameters: `policy_module_versions.parameters_schema` only. The Parameters tab writes it; every other surface (condition builder's module-scoped dropdown, param tree, autocomplete) reads it back through `ModuleInputDefs`/`/api/params?module_id=`. There is no second, hand-maintained list of a module's inputs.

## Tests

`catalog/params/module_input_test.go` — empty schema, type/enum/required/default parsing, malformed-schema error (not panic), unknown-type fallback. `catalog/params/sources_test.go` — `ResolveDef`/`EffectivePermission` consult registered sources (fake source via `RegisterSource`+`t.Cleanup`), and every `AllPaths()` entry resolves identically via `ResolveDef` and the pre-existing `ResolvePath` (no regression). `console/handlers/params_api_test.go` — module-id happy path, stranger 403, unknown module 404, and a byte-equal check that the no-`module_id` response is unchanged from before this work.
