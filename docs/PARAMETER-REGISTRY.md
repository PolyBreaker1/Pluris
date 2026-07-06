# Adding New Parameters - Standardized Process

## TL;DR - It's Very Easy! ✅

**Adding a new parameter/column requires just 2 steps:**

1. **Define parameter** in `catalog/params/definitions.go` (one entry)
2. **Mount in schema** in `catalog/params/schemas.go` (add key to array)

**Everything else happens automatically:**
- ✅ Table columns generated
- ✅ Filter dropdowns updated
- ✅ Detail pages updated
- ✅ Data attributes added
- ✅ Column picker updated
- ✅ Sort/filter operators set
- ✅ Type-aware rendering

---

## The Parameter Registry System

### Single Source of Truth

```
catalog/params/
├── types.go          # Core types (ParamDef, SubtypeSchema, etc.)
├── definitions.go    # ALL parameter definitions (40+ params)
├── schemas.go        # Subtype schemas (which params each mounts)
├── operators.go      # Filter operators by type
└── filter_config.go  # Auto-generates filter JSON
```

**Key principle:** Define once, use everywhere.

---

## Step-by-Step: Adding a New Parameter

### Example: Add "Serial Number" to Computers

#### Step 1: Define Parameter

**File:** `catalog/params/definitions.go`

Add one entry to the `allDefs` array (alphabetically):

```go
var allDefs = []ParamDef{
    // ... existing parameters ...
    
    // Add your new parameter
    {
        Key:         "serial_number",
        Label:       "Serial Number",
        Description: "Manufacturer serial number for warranty tracking.",
        Category:    "hardware",
        Type:        TypeString,
        Filter:      FilterContains,
        Sort:        SortAlpha,
        Mono:        true,  // Display in monospace font
    },
    
    // ... more parameters ...
}
```

**Field explanations:**

| Field | Purpose | Options |
|-------|---------|---------|
| `Key` | Unique identifier, lowercase_underscore | Any string (must match DB field) |
| `Label` | Display name in UI | Short, human-readable |
| `Description` | Help text shown in column picker | 1-2 sentences |
| `Category` | Logical grouping | `identity`, `hardware`, `enrollment`, `lifecycle` |
| `Type` | Data type | `TypeString`, `TypeInt`, `TypeEnum`, `TypeTime`, `TypeBool` |
| `Filter` | Filter operator | `FilterContains`, `FilterEquals`, `FilterGte`, etc. |
| `Sort` | Sort algorithm | `SortAlpha`, `SortNumeric`, `SortDate` |
| `Mono` | Monospace font? | `true` for codes/IDs, `false` for names |

---

#### Step 2: Mount in Schema

**File:** `catalog/params/schemas.go`

Add the parameter key to the relevant section:

```go
var SchemaComputer = &SubtypeSchema{
    Subtype:     "computer",
    Label:       "Computer",
    PluralLabel: "Computers",
    Sections: []SchemaSection{
        {Key: "identity", Label: "Identity", Params: []string{
            "name", "id", "uuid", "tenant", "site", "owner", "groups", "labels",
        }},
        {Key: "hardware", Label: "Hardware", Params: []string{
            "hostname", "fqdn", "os_family", "os_distribution", "os_version",
            "architecture", "cpu", "ram_mb", "storage_mb",
            "serial_number",  // ← ADD HERE
        }},
        {Key: "enrollment", Label: "Enrollment", Params: []string{
            "enrollment_state", "enrolled_at", "last_seen_at", "agent_version",
        }},
        {Key: "lifecycle", Label: "Lifecycle", Params: []string{
            "lifecycle_state", "vendor", "location", "purchase_date", "warranty_expires",
        }},
    },
    DefaultColumns: []string{
        "name", "os_distribution", "site", "owner", "enrollment_state", "last_seen_at",
        // Optionally add to default visible columns:
        // "serial_number",
    },
}
```

**Choose the right section:**
- `identity` - Names, IDs, ownership
- `hardware` - Physical specs, OS, network
- `enrollment` - Agent status, connectivity
- `lifecycle` - Procurement, warranty, disposal

---

#### Step 3: That's It! ✅

Rebuild and restart:

```bash
make build
./scripts/restart-dev.sh
```

**Automatically happens:**

1. **Table column appears** in column picker (hidden by default unless in `DefaultColumns`)
2. **Filter dropdown includes it** with appropriate operators
3. **Detail page shows it** in the correct section
4. **Data attribute added** to table rows: `data-serial-number="..."`
5. **Column picker search** finds it by name
6. **Sorting works** using the specified `Sort` algorithm
7. **Type-aware rendering** applies (monospace if `Mono: true`)

---

## Advanced Examples

### Example 1: Add an Enum Field

```go
// 1. Define in definitions.go
{
    Key:         "warranty_status",
    Label:       "Warranty",
    Description: "Current warranty coverage status.",
    Category:    "lifecycle",
    Type:        TypeEnum,
    EnumValues:  []string{"active", "expired", "extended", "n/a"},
    Filter:      FilterEquals,
    Sort:        SortAlpha,
},

// 2. Mount in schemas.go (lifecycle section)
{Key: "lifecycle", Label: "Lifecycle", Params: []string{
    "lifecycle_state", "vendor", "location", 
    "purchase_date", "warranty_expires",
    "warranty_status",  // ← ADD HERE
}},
```

**Result:**
- Filter dropdown shows: `Warranty` → `equals` → `[active, expired, extended, n/a]`
- Enum values appear as dropdown in filter
- Detail page shows as badge/chip

---

### Example 2: Add a Numeric Field

```go
// 1. Define in definitions.go
{
    Key:         "disk_count",
    Label:       "Disk Count",
    Description: "Number of physical or virtual disks attached.",
    Category:    "hardware",
    Type:        TypeInt,
    Filter:      FilterGte,  // "Greater than or equal"
    Sort:        SortNumeric,
},

// 2. Mount in schemas.go (hardware section)
{Key: "hardware", Label: "Hardware", Params: []string{
    "hostname", "fqdn", "cpu", "ram_mb", "storage_mb",
    "disk_count",  // ← ADD HERE
}},
```

**Result:**
- Filter shows: `Disk Count` → `≥ / ≤ / = / ≠` → `[number input]`
- Table sorts numerically (1, 2, 10, not "1", "10", "2")
- Rendered as plain number

---

### Example 3: Add a Date Field

```go
// 1. Define in definitions.go
{
    Key:         "last_maintenance",
    Label:       "Last Maintenance",
    Description: "Date of most recent hardware maintenance.",
    Category:    "lifecycle",
    Type:        TypeTime,
    Filter:      FilterDateGte,
    Sort:        SortDate,
},

// 2. Mount in schemas.go (lifecycle section)
{Key: "lifecycle", Label: "Lifecycle", Params: []string{
    "lifecycle_state", "vendor", "location", 
    "purchase_date", "warranty_expires",
    "last_maintenance",  // ← ADD HERE
}},
```

**Result:**
- Filter shows: `Last Maintenance` → `after / before / on` → `[date picker]`
- Table sorts chronologically
- Rendered as relative time ("2 days ago") or formatted date

---

### Example 4: Add to Multiple Subtypes

If a parameter applies to multiple asset types:

```go
// 1. Define ONCE in definitions.go
{
    Key:         "ip_address",
    Label:       "IP Address",
    Description: "Primary IPv4 or IPv6 address.",
    Category:    "network",
    Type:        TypeString,
    Filter:      FilterContains,
    Sort:        SortAlpha,
    Mono:        true,
},

// 2. Mount in MULTIPLE schemas in schemas.go

// Add to computers
var SchemaComputer = &SubtypeSchema{
    Sections: []SchemaSection{
        {Key: "hardware", Label: "Hardware", Params: []string{
            "hostname", "fqdn", "ip_address",  // ← ADD HERE
        }},
    },
}

// Add to servers
var SchemaServer = &SubtypeSchema{
    Sections: []SchemaSection{
        {Key: "hardware", Label: "Hardware", Params: []string{
            "hostname", "fqdn", "ip_address",  // ← ADD HERE
        }},
    },
}

// Add to printers
var SchemaPrinter = &SubtypeSchema{
    Sections: []SchemaSection{
        {Key: "network", Label: "Network", Params: []string{
            "printer_ip", "ip_address",  // ← ADD HERE
        }},
    },
}
```

**Result:** Parameter appears in all 3 subtypes, with consistent behavior across all.

---

## Parameter Types Reference

### TypeString
```go
{
    Type:   TypeString,
    Filter: FilterContains,  // or FilterEquals
    Sort:   SortAlpha,
    Mono:   false,  // or true for IDs/codes
}
```
**Use for:** Names, descriptions, codes, IDs

---

### TypeInt
```go
{
    Type:   TypeInt,
    Filter: FilterGte,  // ≥ ≤ = ≠
    Sort:   SortNumeric,
}
```
**Use for:** Counts, quantities, sizes (in units)

---

### TypeEnum
```go
{
    Type:       TypeEnum,
    EnumValues: []string{"option1", "option2", "option3"},
    Filter:     FilterEquals,
    Sort:       SortAlpha,
}
```
**Use for:** Status fields, categories, predefined options

---

### TypeTime
```go
{
    Type:   TypeTime,
    Filter: FilterDateGte,  // after / before / on
    Sort:   SortDate,
}
```
**Use for:** Timestamps, dates, deadlines

---

### TypeBool
```go
{
    Type:   TypeBool,
    Filter: FilterEquals,
    Sort:   SortAlpha,
}
```
**Use for:** Yes/no flags, toggles

---

### TypeLink
```go
{
    Type:       TypeLink,
    LinkTarget: "site",  // or "user", "group"
    Filter:     FilterEquals,
    Sort:       SortAlpha,
}
```
**Use for:** References to other entities (sites, users, groups)

---

## Filter Operators Reference

| Operator | Label | Use For |
|----------|-------|---------|
| `FilterContains` | "contains" | String search (partial match) |
| `FilterEquals` | "equals" | Exact match (strings, enums) |
| `FilterNotEquals` | "not equals" | Exclusion |
| `FilterGte` | "≥" | Numeric comparisons |
| `FilterLte` | "≤" | Numeric comparisons |
| `FilterDateGte` | "after" | Date comparisons |
| `FilterDateLte` | "before" | Date comparisons |
| `FilterHas` | "has" | Array/map membership |

---

## Categories Reference

Logical grouping for organization:

| Category | Purpose | Examples |
|----------|---------|----------|
| `identity` | Who/what it is | Name, ID, owner, site |
| `hardware` | Physical/technical | CPU, RAM, OS, network |
| `enrollment` | Agent status | Enrollment state, last seen |
| `lifecycle` | Business process | Purchase date, warranty, vendor |
| `network` | Connectivity | IP, MAC, hostname, FQDN |
| `custom` | Tenant-specific | Any custom fields |

---

## What Happens Automatically

### 1. Table Columns ✅

**File:** `web/lists/assets.go`

```go
// Automatically generates FieldDefs from parameter registry
func buildAssetsFieldsFromParams() []FieldDef {
    for _, param := range params.Definitions {
        fields = append(fields, FieldDef{
            Key:         param.Key,
            Label:       param.Label,
            Description: param.Description,
            Group:       param.Category,
            Sort:        param.Sort,
            Width:       inferWidth(param),
            // ... more auto-config
        })
    }
}
```

**Result:** Your new column appears in column picker, with correct label, description, sorting.

---

### 2. Filter Dropdowns ✅

**File:** `catalog/params/filter_config.go`

```go
// Automatically generates filter JSON config
func FilterConfigJSON(subtype string) string {
    schema := SchemaBySubtype(subtype)
    for _, paramKey := range schema.AllParamKeys() {
        param := DefByKey(paramKey)
        config = append(config, FilterParam{
            Key:       param.Key,
            Label:     param.Label,
            Type:      param.Type,
            Operators: operatorsForType(param.Type),
            // ... more auto-config
        })
    }
}
```

**Result:** Your parameter appears in "Add Filter" dropdown with appropriate operators.

---

### 3. Detail Pages ✅

**File:** `web/templates/assets_helpers.go`

```go
// Automatically builds detail page sections from schema
func buildDetailSections(asset Asset, schema *params.SubtypeSchema) []DetailSection {
    for _, section := range schema.Sections {
        for _, paramKey := range section.Params {
            param := params.DefByKey(paramKey)
            items = append(items, DetailItem{
                Label: param.Label,
                Value: getAssetParamValue(asset, paramKey),
            })
        }
    }
}
```

**Result:** Your parameter appears in detail page in the correct section.

---

### 4. Data Attributes ✅

**File:** `web/templates/assets_helpers.go`

```go
// Automatically generates data-* attributes for filtering
func assetFilterAttrs(a Asset) string {
    for _, paramKey := range schema.AllParamKeys() {
        value := getAssetParamValue(a, paramKey)
        attrs += fmt.Sprintf(` data-%s="%s"`, paramKey, value)
    }
}
```

**Result:** Table rows get `data-serial-number="XYZ123"` for JavaScript filtering.

---

### 5. Column Picker ✅

**Template already generates items from fields**, so your new column appears automatically with:
- Search index (label is searchable)
- Checkbox toggle
- Description tooltip
- Group hierarchy

---

### 6. Type-Aware Rendering ✅

**File:** `web/lists/lists.go`

```go
func RenderAssetCell(key string, asset Asset) string {
    param := params.DefByKey(key)
    value := getAssetParamValue(asset, key)
    
    switch param.Type {
    case params.TypeTime:
        return formatTime(value)
    case params.TypeInt:
        return formatNumber(value)
    case params.TypeEnum:
        return formatBadge(value)
    case params.TypeString:
        if param.Mono {
            return fmt.Sprintf(`<code>%s</code>`, value)
        }
        return escapeHTML(value)
    }
}
```

**Result:** Values render appropriately (dates as relative time, numbers formatted, codes in monospace, etc.)

---

## Adding to Database

If your parameter needs database persistence (not just displayed from JSON):

### Option 1: JSON Payload (Recommended)

Most parameters live in the `subtype_payload` JSONB column. No schema change needed!

**In seed script:**
```go
payload := map[string]interface{}{
    "serial_number": "XYZ123ABC",  // ← Just add to map
    "ram_mb":        16384,
    // ... other params
}
```

### Option 2: Dedicated Column (For Core Fields)

Only for frequently-queried or indexed fields (like `name`, `enrollment_state`):

```sql
-- db/schema/003_add_serial_column.sql
ALTER TABLE assets ADD COLUMN serial_number TEXT;
CREATE INDEX idx_assets_serial ON assets(serial_number);
```

**Then update queries:**
```sql
-- db/queries/assets.sql
-- name: GetAssetBySerial :one
SELECT * FROM assets WHERE serial_number = $1;
```

**Most parameters don't need this** - JSON payload is fine!

---

## Testing Your New Parameter

### 1. Add Test Data

**File:** `cmd/seed/main.go`

```go
assets := []db.CreateAssetParams{
    {
        // ... existing fields
        SubtypePayload: map[string]interface{}{
            "serial_number": "SN123456",  // ← Add your parameter
            "ram_mb":        16384,
            // ... other params
        },
    },
}
```

### 2. Rebuild & Reseed

```bash
rm pluris.db  # Delete old database
make build
./bin/pluris-console  # Starts server, auto-creates DB, runs seed
```

### 3. Verify

1. **Table column:** Click "Columns" → Check "Serial Number" → Should appear
2. **Filter:** Click "Advanced" → Select "Serial Number" → Should have operators
3. **Detail page:** Click any asset → Should show "Serial Number: SN123456"
4. **Search:** Open column picker → Type "serial" → Should find it
5. **Sorting:** Click column header → Should sort correctly

---

## Best Practices

### ✅ DO

- **Use descriptive labels:** "Serial Number" not "SN"
- **Add helpful descriptions:** "Manufacturer serial number for warranty tracking"
- **Choose correct type:** `TypeInt` for numbers, not `TypeString`
- **Set appropriate operators:** `FilterGte` for numbers, `FilterContains` for searchable text
- **Use categories consistently:** `hardware` for specs, `lifecycle` for business
- **Set `Mono: true`** for codes, IDs, UUIDs, MAC addresses
- **Test with real data:** Add to seed script

### ❌ DON'T

- **Don't duplicate keys:** Each `Key` must be unique
- **Don't skip description:** Help text is important for users
- **Don't use wrong type:** `TypeString` for a number breaks sorting/filtering
- **Don't add to wrong section:** Keep logical grouping consistent
- **Don't forget to mount:** Defining without mounting = invisible parameter

---

## Common Patterns

### Pattern 1: Hardware Spec
```go
{Key: "gpu_model", Label: "GPU", Description: "Graphics processor model.", 
 Category: "hardware", Type: TypeString, Filter: FilterContains, Sort: SortAlpha}
```

### Pattern 2: Count/Quantity
```go
{Key: "cpu_cores", Label: "CPU Cores", Description: "Number of processor cores.", 
 Category: "hardware", Type: TypeInt, Filter: FilterGte, Sort: SortNumeric}
```

### Pattern 3: Status/State
```go
{Key: "backup_status", Label: "Backup", Description: "Backup completion status.", 
 Category: "lifecycle", Type: TypeEnum, EnumValues: []string{"ok", "warning", "error", "none"}, 
 Filter: FilterEquals, Sort: SortAlpha}
```

### Pattern 4: Identifier
```go
{Key: "asset_tag", Label: "Asset Tag", Description: "Physical asset tracking label.", 
 Category: "identity", Type: TypeString, Filter: FilterEquals, Sort: SortAlpha, Mono: true}
```

### Pattern 5: Timestamp
```go
{Key: "deployed_at", Label: "Deployed", Description: "Date device was deployed to user.", 
 Category: "lifecycle", Type: TypeTime, Filter: FilterDateGte, Sort: SortDate}
```

---

## Summary

### How Easy Is It?

**Extremely easy!** ✅

1. Add one struct entry (5 lines)
2. Add one string to array (1 line)
3. Rebuild

**Total:** ~6 lines of code, everything else automatic.

### What's Standardized?

✅ Parameter definition format  
✅ Type system (String, Int, Enum, Time, Bool, Link)  
✅ Filter operators by type  
✅ Sort algorithms by type  
✅ Rendering logic by type  
✅ Schema mounting process  
✅ JSON payload storage  

### What Gets Generated?

✅ Table columns  
✅ Column picker items  
✅ Filter dropdowns  
✅ Detail page sections  
✅ Data attributes  
✅ Search indexes  
✅ Sort handlers  
✅ Type-aware rendering  

---

## Quick Reference Card

```
1. Edit: catalog/params/definitions.go
   → Add ParamDef to allDefs array

2. Edit: catalog/params/schemas.go  
   → Add key to relevant section's Params array

3. Build: make build

4. Test: Column picker, filter, detail page

Done! ✨
```

---

**Questions? Check:**
- `catalog/params/types.go` - Core type definitions
- `catalog/params/definitions.go` - All 40+ parameter examples
- `catalog/params/schemas.go` - All 4 subtype schema examples
- `docs/PARAMETER-REGISTRY.md` - (this doc)

---

**Status:** Fully standardized, production-ready! 🚀
