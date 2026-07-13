# Parameters

**What:** The canonical parameter registry (`catalog/params`) that single-sources every field definition consumed by detail pages, list columns, filters, dependency-group conditions, and field-update validation.

**Related:** [[overview]], [[data-model]], [[identity-assets]]

## Canonical parameter paths (INV-CPP)

Every parameter mounted by a schema is addressable as **`<entity>/<section>/<param>`** (lowercase snake, forward slashes) — e.g. `computer/hardware/ram_mb`, `user/identity/email`. Paths are *derived*, never stored in parallel: `entity` = `SubtypeSchema.PathEntity`, `section` = `SchemaSection.Key`, `param` = `ParamDef.Key`. A shared `ParamDef` (e.g. `ram_mb` mounted by both `computer` and `server`) gets one distinct path per mounting entity, all resolving back to the same underlying definition.

The path index (`catalog/params/paths.go`) is built lazily on first use (`sync.Once`) from the immutable `Schemas` registry and never rebuilt — mutating `Schemas` after that point would silently desynchronize the index, so schema registration must happen at package-init time, not at runtime.

Canonical paths are the addressing scheme `dependency_group_conditions.param_path` uses (see [[data-model]] and "Dependency-group conditions" below).

## The registry

`catalog/params/types.go` defines the shapes; `catalog/params/definitions.go` holds the actual parameter list.

**`ParamDef`** fields: `Key` (stable identifier, `lowercase_underscore`, unique registry-wide), `Label`, `Description`, `Category` (display grouping — `identity`/`hardware`/`enrollment`/`lifecycle`/etc.), `Type`, `Unit`, `EnumValues` (when `Type == TypeEnum`), `Filter` (primary filter mode), `Sort`, `Mono` (monospace rendering), `LinkTarget` (when `Type == TypeLink`), `CompoundFields` (when `Type == TypeCompound`).

**`ParamType`** values: `TypeString`, `TypeInt`, `TypeFloat`, `TypeEnum`, `TypeBool`, `TypeTime`, `TypeDate`, `TypeList`, `TypeMap`, `TypeLink` (reference to another entity — owner/site/groups), `TypeCompound` (structured sub-fields, e.g. a printer consumable's `{kind, level}`).

**`FilterMode`** values: `FilterNone`, `FilterContains`, `FilterEquals`, `FilterPrefix`, `FilterNumGte`, `FilterNumLte`, `FilterDateGte`, `FilterDateLte`, `FilterHas`. This is the *primary* mode recorded on the `ParamDef`; the filter-builder UI actually offers a richer, type-derived operator set (see "Filters" below) — the two are related but not 1:1.

**`SortType`** values: `SortNone`, `SortAlpha`, `SortNum`, `SortDate`.

`catalog/params/definitions.go`'s `Definitions map[string]ParamDef` and `Links map[string]LinkDef` are populated once at `init()` from the `allDefs`/`allLinks` slices. `DefByKey(key)` is the lookup every consumer uses; `DefsForKeys(keys)` resolves an ordered list, silently skipping unknown keys; `SharedWith(paramKey)` answers "which subtypes also mount this param" for cross-subtype queries.

## Subtype schemas + sections

`catalog/params/schemas.go`'s `Schemas map[string]*SubtypeSchema` registers one schema per subtype: `computer`, `server`, `printer`, `desk`, plus `identity` (the Users directory, reusing the shared `tenant`/`site` `ParamDef`s per the Tenant → Site → Group → (Asset | Identity) hierarchy). `SchemaBySubtype(subtype)` looks one up by its `Subtype` slug.

A `SubtypeSchema` has: `Subtype` (asset/entity slug), `Label`/`PluralLabel`, `PathEntity` (the INV-CPP path prefix — differs from `Subtype` only for identity, whose paths use `user`), `Sections []SchemaSection` (ordered display grouping — each section has a `Key`, `Label`, and ordered `Params []string`), and `DefaultColumns []string` (which param keys a list view shows by default, in order).

Concretely: `SchemaComputer` mounts `identity`/`hardware`/`enrollment`/`lifecycle` sections; `SchemaIdentity` mounts `identity`/`organization`/`contact`/`location`/`profile`/`security`/`preferences`/`metadata` sections (see [[identity-assets]] for the identity field groups in AD terms).

## `PathFor` / `ResolvePath`

- `PathFor(entity, key string) string` — canonical path for a param mounted by `entity` (e.g. `PathFor("user", "email")` → `"user/identity/email"`), or `""` if not mounted.
- `SchemaByPathEntity(entity string) *SubtypeSchema` — schema registered under a path-entity slug (`"user"` → `SchemaIdentity`).
- `ResolvePath(path string) (*SubtypeSchema, *SchemaSection, *ParamDef, error)` — parses `"entity/section/param"` and fails closed (returns an error) on any malformed shape or unknown segment. This is the function dependency-group condition evaluation and any future path-driven UI use to go from a stored string back to live registry objects.
- `AllPaths() []string` — every registered canonical path, sorted; useful for path-picker UIs and tests that assert full coverage.

## How parameters drive the UI

- **Detail sections** — a subtype's detail page renders one section per `SchemaSection`, each showing its `Params` in order via `DefByKey` lookups. There is exactly one schema per subtype, so there is exactly one section layout regardless of entry point (per the Single-Source-of-Truth UI rule — see [[decisions]] ADR-004).
- **List columns** — `SubtypeSchema.DefaultColumns` seeds which columns a list view shows by default; the full column picker offers every mounted param across all sections. (Column-picker/data-attribute auto-generation as described in the older parameter registry docs, condensed from now-deleted documents — full text in git history — e.g. a `buildAssetsFieldsFromParams()` helper that walks `params.Definitions` — does not exist verbatim in current `web/lists` code; treat those old code samples as illustrative intent, not literal current implementation. See list-rendering invariants (INV-L series) for the authoritative list-view contract.)
- **Filters** — `catalog/params/filter_config.go`'s `BuildFilterConfig(schema)` walks a schema's sections and, for each mounted param, calls `OperatorsForType`/`OperatorsForParam` (`catalog/params/operators.go`) to attach the full type-appropriate operator set (e.g. strings get `contains`/`not_contains`/`equals`/`starts_with`/`is_empty`/…; numerics get `gt`/`gte`/`lt`/`lte`/`between`; enums get `equals`/`in`/`not_in`). The resulting `FilterConfig` JSON is what `web/static/filters-modern.js` renders as the filter-builder dropdown.
- **Dependency-group conditions** — `catalog/dependencygroups.Condition.ParamPath` is a full canonical path (`computer/hardware/os_package_family`); matching (`OpIn`/`OpNotIn`/`OpExists`) uses the path's trailing key against the asset's resolved fact value. See [[data-model]]'s "Dependency groups / conditions / module links" section.
- **Field-update validation** — see below.

## Field-update validation

`pkg/services/field_update.go` is the shared validation core both `AssetService.UpdateFields` and `IdentityService.UpdateFields` call:

1. `sectionByKey(schema, section)` — reject unknown sections.
2. `sectionHasParam(sec, key)` — reject a key that isn't mounted in the named section (a key from a different section, or an unknown key, is rejected even if it exists elsewhere in the schema).
3. `coerceParamValue(def, raw)` — type-coerces the raw string form-value per `def.Type`: `TypeBool` → `strconv.ParseBool`, `TypeInt` → `strconv.ParseInt`, `TypeList` → comma-split/trim/drop-empty, `TypeEnum` → validated against `def.EnumValues`, everything else passed through unchanged.
4. Editability check (entity-specific — see below).
5. On success, the coerced value is applied to the in-memory struct and the whole entity is persisted via `Update`.

Failures return `ErrFieldNotFound` (entity doesn't exist or belongs to another tenant → handlers map to 404) or `ErrFieldValidation` (unknown section/key/non-editable/coercion failure, wrapped with the offending key → handlers map to 400).

### `NonEditableFieldKeys` / `SelfServiceEditableKeys` (identity)

Both live in `catalog/identities/types.go`, the single source of truth shared by the service (`identityFieldEditable`) and the detail UI (`users.templ`'s `userGeneralTab`) so the two can never diverge:

- **`NonEditableFieldKeys`** — always read-only regardless of caller: `id`, `tenant`, `site`, `username`, `role` (managed on the Roles tab, never as a text field), `avatar_url`, `groups`, `manager`, and system-maintained audit fields (`password_last_set_at`, `last_logon_at`, `logon_count`, `bad_password_count`). `identityFieldEditable` additionally rejects every `TypeLink` param as a belt-and-braces guard for future link params not yet named here.
- **`SelfServiceEditableKeys`** — the narrower allowlist an identity holding only `own` scope on `identity.update` may set on *their own* record via `/api/users/:id/fields`: contact/personal fields only (`display_name`, `given_name`, `surname`, `initials`, `email`, phone/fax/office/address fields, `locale`, `timezone`). Never role, employment, site, or security fields — those require `all` scope. Enforced in `console/handlers/field_api.go`'s `UserFieldUpdate`, not in the service layer.

### `NonEditablePayloadKeys` (asset)

`catalog/assets/types.go`'s `NonEditablePayloadKeys` mirrors the identity list for assets: `id`, `uuid`, `tenant`, `site`, `owner`, `groups`, `managed_by`, `enrollment_state`, `enrolled_at`, `last_seen_at`, `agent_version`. This list is hand-kept in sync with `pkg/services/assets.go`'s `assetColumnBackedKeys` (the two can't share code directly: one lives in `catalog`, which the service layer imports, and the other in `pkg/services`, which `catalog` must not import back).

## Adding a parameter (current-code how-to)

1. **Define it once** in `catalog/params/definitions.go`: add a `ParamDef` entry to `allDefs` (key, label, description, category, type, filter, sort, mono/unit/enum values as applicable).
2. **Mount it** in the relevant `SubtypeSchema`(s) in `catalog/params/schemas.go`: append the key to the appropriate `SchemaSection.Params` slice (or to `DefaultColumns` if it should show by default in list views). A parameter shared across subtypes (e.g. `ip_address` on both computer and server) is defined once and mounted in each schema's section list.
3. **If it needs to be user-editable** via the inline field-update API: confirm it isn't accidentally excluded by `NonEditableFieldKeys`/`NonEditablePayloadKeys`, and add the corresponding `case` to `applyIdentityField` (`pkg/services/identities.go`) or the asset equivalent in `pkg/services/assets.go` so `UpdateFields` knows how to write the coerced value back onto the struct. Forgetting this step means the field mounts and displays but `UpdateFields` rejects it with "mounted but not supported."
4. **If it's a self-service-editable identity field**, also add it to `SelfServiceEditableKeys`.
5. **If it needs to gate dependency groups**, its canonical path (`PathFor(entity, key)`) becomes usable as a `dependency_group_conditions.param_path` value once mounted — no separate registration needed.
6. Run `go build ./...` / `go test ./catalog/params/...` — the path-index builder panics on duplicate paths, duplicate `PathEntity`s, or a schema mounting the same key twice, so structural mistakes fail fast.

There is no code-generation step for parameters (unlike templ/sqlc) — registry changes take effect on next build since `Definitions`/`Schemas`/the path index are all built from Go literals at package init.
