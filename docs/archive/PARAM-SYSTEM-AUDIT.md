# Parameter System Audit

## Understanding the Strict Structure

### Core Architecture

**Three-Layer System:**
1. **ParamDef** (definitions.go) - Defines WHAT a parameter IS
   - Key, label, description
   - Type, unit, enum values
   - Filter mode, sort type
   - Single source of truth for each concept

2. **SubtypeSchema** (schemas.go) - Defines WHICH params each subtype mounts
   - Lists param keys per subtype
   - Grouped into sections
   - Defines default columns
   - Controls display order

3. **Consumers** - Everything reads from registry
   - Table columns (`web/lists/assets.go`)
   - Filter dropdowns (`catalog/params/filter_config.go`)
   - Detail pages (`web/templates/pages.templ`)
   - Row `data-*` attributes (`web/templates/assets_helpers.go`)

### Key Principles

**R1: Single Source of Truth**
- "RAM is RAM" - one definition, referenced everywhere
- No duplicate field declarations
- Parameter key IS the column key IS the data attribute key

**R2: Strict Parameter Flow**
```
ParamDef → SubtypeSchema → Table Columns
                         → Filter Config
                         → Detail Sections
                         → Row Attributes
```

**R3: Type System**
- 40 parameter definitions in registry
- 4 subtype schemas (computer, server, printer, desk)
- 4 link definitions (owner, site, groups, guest_profile)

## Database vs Parameter Alignment

### Current Database Schema

**Assets table structure:**
```sql
CREATE TABLE assets (
    id INTEGER PRIMARY KEY,
    uuid TEXT NOT NULL UNIQUE,
    tenant_id INTEGER NOT NULL,
    site_id INTEGER,
    subtype TEXT NOT NULL,
    subtype_payload TEXT NOT NULL DEFAULT '{}',  -- JSON
    
    -- Common fields
    enrollment_state TEXT NOT NULL DEFAULT 'pending',
    enrolled_at TIMESTAMP,
    last_seen_at TIMESTAMP,
    agent_version TEXT,
    labels TEXT DEFAULT '{}',
    
    -- CMDB fields
    lifecycle_state TEXT DEFAULT 'active',
    location TEXT,
    owner_identity_id INTEGER,
    vendor TEXT,
    purchase_date DATE,
    purchase_price REAL,
    warranty_expires_at DATE,
    
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

### Alignment Analysis

**✅ Correctly Aligned:**
- `uuid` → param `uuid`
- `enrollment_state` → param `enrollment_state`
- `enrolled_at` → param `enrolled_at`
- `last_seen_at` → param `last_seen_at`
- `agent_version` → param `agent_version`
- `lifecycle_state` → param `lifecycle_state`
- `location` → param `location`
- `vendor` → param `vendor`
- `purchase_date` → param `purchase_date`
- `warranty_expires_at` → param `warranty_expires`

**✅ JSON Payload (subtype-specific):**
- `hostname` → param `hostname`
- `fqdn` → param `fqdn`
- `os_family` → param `os_family`
- `os_distribution` → param `os_distribution`
- `os_version` → param `os_version`
- `architecture` → param `architecture`
- `cpu` → param `cpu`
- `ram_mb` → param `ram_mb`
- `storage_mb` → param `storage_mb`
- Server: `server_role`, `services`, `uptime_since`
- Printer: `printer_model`, `printer_ip`, `queues`, `supported_formats`, `consumables`
- Desk: `desk_location`, `dock_model`, `monitor_count`, `guest_profile`

**✅ Link Parameters (via foreign keys):**
- `site_id` → param `site` (TypeLink)
- `owner_identity_id` → param `owner` (TypeLink)
- Groups via `group_memberships` table → param `groups` (TypeLink)

**⚠️ Missing in Database:**
- `id` param expects human-readable ID (like "comp.acme.hq.lt-0142")
  - Currently using UUID as ID in Asset service
- `tenant` param - should be displayed but comes from tenant_id FK
- `name` param - primary display name, derived from payload fields

**⚠️ Name Mismatches:**
- DB: `warranty_expires_at` vs Param: `warranty_expires`
- DB: `purchase_price` (cents) vs Param: doesn't have price param defined

## Mock Data Sources Found

### Assets (catalog/assets/mock.go)
- ❌ **Still in use via old imports**
- Functions: `AllAssets()`, `AllOfSubtype()`, `mockComputers()`, `mockServers()`, `mockPrinters()`, `mockDesks()`
- Status: **REPLACED** in web handlers, but functions still exist

### Other Mock Sources (NOT in scope yet)
- `catalog/policies/catalog_custom.go` - Custom policy mock
- `catalog/configgroups/mock.go` - Configuration groups mock
- `catalog/policymodules/mock.go` - Policy modules mock
- These are separate entities, handle later

## Issues to Fix

### 1. Human-Readable Asset ID
**Problem:** Param `id` expects "comp.acme.hq.lt-0142", but we use UUID

**Solution Options:**
a) Generate human-readable IDs on asset creation
b) Use a composite pattern: `{subtype}.{tenant}.{site}.{seq}`
c) Store both UUID and human ID in database

**Decision:** Add `human_id` column to assets table, auto-generate on insert

### 2. Name Parameter Derivation
**Problem:** `name` param needs to derive from payload

**Current:**
- Computer: hostname
- Server: hostname  
- Printer: model
- Desk: desk_location

**Solution:** Service layer already handles this via `PrimaryHostname()` accessor

### 3. Link Resolution
**Problem:** Asset service returns empty strings for `site`, `owner`, `groups`

**Solution:** Add JOIN queries to fetch related entity names

### 4. Missing Parameters in Seed Data
**Problem:** Seed data doesn't populate all schema parameters

**Solution:** Update seed script to include all parameters per schema

## Action Plan

### Phase 1: Database Schema Updates
1. Add `human_id` column to assets table
2. Create migration for existing data
3. Add unique constraint on `human_id`

### Phase 2: Service Layer Enhancement  
1. Implement link resolution (JOINs for site, owner, groups)
2. Generate human IDs on asset creation
3. Ensure all param keys map correctly

### Phase 3: Verify Parameter Flow
1. Audit table columns use param registry
2. Verify filter config uses param registry  
3. Check detail page uses param registry
4. Confirm row attributes use param registry

### Phase 4: Update Seed Data
1. Add missing parameters to seed payloads
2. Ensure comprehensive coverage of all schemas
3. Test all four subtypes render correctly

### Phase 5: Remove Mock Dependencies
1. Verify no code imports `catalog/assets.AllAssets()`
2. Mark mock functions as deprecated
3. Document migration path

## Parameter Registry Coverage

### Computer Schema (26 params mounted)
**Identity:** name, id, uuid, tenant, site, owner, groups, labels (8)
**Hardware:** hostname, fqdn, os_family, os_distribution, os_version, architecture, cpu, ram_mb, storage_mb (9)
**Enrollment:** enrollment_state, enrolled_at, last_seen_at, agent_version (4)
**Lifecycle:** lifecycle_state, vendor, location, purchase_date, warranty_expires (5)

**Total: 26 parameters**

### Server Schema (25 params mounted)
**Identity:** name, id, uuid, tenant, site, owner, groups, labels (8)
**Hardware:** hostname, fqdn, os_family, os_distribution, os_version, architecture, server_role, services, uptime_since, ram_mb (10)
**Enrollment:** enrollment_state, enrolled_at, last_seen_at, agent_version (4)
**Lifecycle:** lifecycle_state, vendor, location, purchase_date, warranty_expires (5)

**Total: 27 parameters**

### Printer Schema (19 params mounted)
**Identity:** name, id, tenant, site, owner, groups, labels (7)
**Hardware:** printer_model, vendor, printer_ip, queues, supported_formats, consumables (6)
**Enrollment:** enrollment_state, last_seen_at (2)
**Lifecycle:** lifecycle_state, location, purchase_date, warranty_expires (4)

**Total: 19 parameters**

### Desk Schema (15 params mounted)
**Identity:** name, id, tenant, site, groups, labels (6)
**Hardware:** desk_location, dock_model, monitor_count, guest_profile (4)
**Lifecycle:** lifecycle_state, location, purchase_date, warranty_expires (4)

**Total: 14 parameters**

## Verification Checklist

- [x] Parameter registry structure understood
- [x] Database schema reviewed for alignment
- [x] Mock data sources identified
- [ ] Human ID generation implemented
- [ ] Link resolution via JOINs implemented
- [ ] Seed data updated with all parameters
- [ ] Table columns verified using param registry
- [ ] Filter config verified using param registry
- [ ] Detail pages verified using param registry
- [ ] Row attributes verified using param registry
- [ ] All four subtypes render correctly from database
- [ ] Mock functions deprecated/removed

## Next Steps

1. **Immediate:** Add human_id column and generator
2. **Immediate:** Implement JOIN queries for links
3. **Immediate:** Update seed data completeness
4. **Short-term:** Verify strict parameter flow end-to-end
5. **Short-term:** Remove/deprecate mock functions
