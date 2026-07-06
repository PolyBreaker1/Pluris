# Filter Functionality Test Guide

## ✅ Database Structure Verification

The filtering system tests the database structure by reading `data-*` attributes from table rows.

### Database → HTML Flow

```
┌──────────────────────────────────────────────────────────┐
│  DATABASE (SQLite)                                       │
│  ┌────────────────────────────────────────────────────┐ │
│  │ assets table                                       │ │
│  │                                                    │ │
│  │ human_id: comp.demo.hq.0001                       │ │
│  │ subtype_payload: {                                │ │
│  │   "hostname": "dev-laptop-001",                   │ │
│  │   "os_family": "linux",                           │ │
│  │   "os_distribution": "Ubuntu",                    │ │
│  │   "ram_mb": 16384                                 │ │
│  │ }                                                 │ │
│  │ enrollment_state: "enrolled"                      │ │
│  │ vendor: "Dell"                                    │ │
│  └────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
                     ▼ SQL JOIN + JSON parse
┌──────────────────────────────────────────────────────────┐
│  SERVICE LAYER (Go)                                      │
│  ┌────────────────────────────────────────────────────┐ │
│  │ assets.Asset struct                                │ │
│  │                                                    │ │
│  │ ID: "comp.demo.hq.0001"                           │ │
│  │ Payload.Hostname: "dev-laptop-001"                │ │
│  │ Payload.OSFamily: "linux"                         │ │
│  │ Payload.OSDistribution: "Ubuntu"                  │ │
│  │ Payload.RAMMB: 16384                              │ │
│  │ EnrollmentState: "enrolled"                       │ │
│  │ Vendor: "Dell"                                    │ │
│  └────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
                     ▼ assetFilterAttrs()
┌──────────────────────────────────────────────────────────┐
│  HTML <tr> ATTRIBUTES                                    │
│  ┌────────────────────────────────────────────────────┐ │
│  │ <tr data-detail-id="comp.demo.hq.0001"            │ │
│  │     data-enrollment-state="enrolled"              │ │
│  │     data-hostname="dev-laptop-001"                │ │
│  │     data-os-family="linux"                        │ │
│  │     data-os-distribution="Ubuntu"                 │ │
│  │     data-ram-mb="16384"                           │ │
│  │     data-vendor="Dell">                           │ │
│  │   ... table cells ...                             │ │
│  │ </tr>                                             │ │
│  └────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
                     ▼ filter-builder.js reads
┌──────────────────────────────────────────────────────────┐
│  JAVASCRIPT FILTER ENGINE                                │
│  ┌────────────────────────────────────────────────────┐ │
│  │ getVal(row, "os_family")                          │ │
│  │ → row.getAttribute("data-os-family")              │ │
│  │ → "linux"                                         │ │
│  │                                                    │ │
│  │ match(row, {param: "os_family", op: "equals",    │ │
│  │             value: "linux"})                      │ │
│  │ → true                                            │ │
│  │                                                    │ │
│  │ row.classList.toggle('pf-hidden', !show)          │ │
│  └────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

## 🧪 Test Cases

### Test 1: Search by Hostname (Simple Mode - Default)
1. Open: https://YOUR-DEV-HOST.example.com/assets/computers
2. Type in search box: `dev-laptop`
3. **Expected**: Only rows with "dev-laptop" in hostname should be visible
4. **Verifies**: 
   - Database → data-hostname attribute
   - Filter engine reads data-hostname
   - Contains operator works

### Test 2: Filter by OS Family
1. Click "Advanced" button
2. Click "Add filter"
3. Select "OS family" from dropdown
4. Select operator: "equals"
5. Type value: `linux`
6. Click "Apply"
7. **Expected**: Only Linux machines visible
8. **Verifies**:
   - Database → data-os-family attribute
   - Parameter registry key mapping
   - Equals operator

### Test 3: Filter by RAM (Numeric)
1. In Advanced mode, add another filter
2. Select "RAM" from dropdown
3. Select operator: "greater than or equal"
4. Type value: `32768`
5. Click "Apply"
6. **Expected**: Only machines with 32GB+ RAM visible
7. **Verifies**:
   - Database → data-ram-mb attribute
   - Numeric comparison
   - Integer parameter type

### Test 4: Multi-Criteria (AND logic)
1. Keep the OS family filter: `os_family equals linux`
2. Add RAM filter: `ram_mb >= 16384`
3. Add enrollment filter: `enrollment_state equals enrolled`
4. Click "Apply"
5. **Expected**: Only enrolled Linux machines with 16GB+ RAM
6. **Verifies**:
   - Multiple criteria evaluation
   - AND logic
   - Complex filtering

### Test 5: OR Logic
1. Advanced mode
2. First filter: `os_family equals linux`
3. Second filter: Change logic to "OR"
4. Second filter: `os_family equals windows`
5. Click "Apply"
6. **Expected**: All Linux OR Windows machines
7. **Verifies**:
   - OR logic operator
   - Same parameter, different values

## 📊 Current Database State

**Computers** (5 total):
1. `comp.demo.hq.0001` - Ubuntu, 16GB RAM, enrolled
2. `comp.demo.hq.0002` - Fedora, 32GB RAM, enrolled
3. `comp.demo.hq.0003` - Arch, 16GB RAM, enrolled
4. `comp.demo.hq.0004` - Windows 11, 16GB RAM, approved
5. `comp.demo.hq.0005` - macOS, 16GB RAM, enrolled

**Servers** (3 total):
1. `srv.demo.hq.0001` - Ubuntu, 64GB RAM, application role
2. `srv.demo.hq.0002` - Ubuntu, 128GB RAM, database role
3. `srv.demo.hq.0003` - CentOS, 64GB RAM, web role

**Printers** (2 total):
1. `prn.demo.hq.0001` - HP LaserJet, 10.0.1.50
2. `prn.demo.hq.0002` - Epson EcoTank, 10.0.1.51

**Desks** (3 total):
1. `desk.demo.hq.0001` - Conference Room A
2. `desk.demo.hq.0002` - Hot Desk 15
3. `desk.demo.hq.0003` - CEO Office

## 🔍 Verification Commands

```bash
# Check if filter UI exists
curl -s http://localhost:8080/assets/computers | grep -o 'data-filter-builder="assets"'
# Expected: data-filter-builder="assets"

# Check if search input exists
curl -s http://localhost:8080/assets/computers | grep -o 'id="fb-search"'
# Expected: id="fb-search"

# Check data attributes on rows
curl -s http://localhost:8080/assets/computers | grep -o 'data-os-family="[^"]*"' | head -5
# Expected: data-os-family="linux" (multiple times)

# Check enrollment state attributes
curl -s http://localhost:8080/assets/computers | grep -o 'data-enrollment-state="[^"]*"' | head -5
# Expected: data-enrollment-state="enrolled" and data-enrollment-state="approved"

# Check numeric RAM attributes
curl -s http://localhost:8080/assets/computers | grep -o 'data-ram-mb="[^"]*"' | head -5
# Expected: data-ram-mb="16384", data-ram-mb="32768"

# Count total rows
curl -s http://localhost:8080/assets/computers | grep -o 'data-detail-id="comp' | wc -l
# Expected: 5
```

## ✅ Success Criteria

1. **Filter UI renders** - Search box, template button, Advanced button visible
2. **Simple mode works** - Typing filters rows immediately
3. **Advanced mode works** - Can add multiple criteria
4. **Data attributes present** - All parameter keys have data-* attributes
5. **Filtering works** - Rows hide/show based on criteria
6. **Parameter registry aligned** - Filter dropdown shows all 26 computer parameters
7. **Database values correct** - Filters match database values exactly

## 🐛 Debugging

If filtering doesn't work:

1. **Open browser console** (F12)
2. **Check for JavaScript errors**
3. **Verify filter config loaded**:
   ```javascript
   // In browser console
   document.querySelector('[data-filter-config]').textContent
   // Should return JSON with params, sections, operators
   ```
4. **Check if filter-builder.js initialized**:
   ```javascript
   // Type in search box and check console
   // Should see no errors
   ```
5. **Manually test getVal function**:
   ```javascript
   var row = document.querySelector('tbody tr');
   row.getAttribute('data-os-family');
   // Should return "linux", "windows", etc.
   ```

## 📝 Filter Config Structure

The filter config JSON contains:
- **subtype**: "computer", "server", "printer", "desk"
- **sections**: Groups of parameters (Identity, Hardware, Enrollment, Lifecycle)
- **params**: Each parameter definition with:
  - `key`: Parameter key (e.g., "os_family")
  - `label`: Display label (e.g., "OS family")
  - `type`: Data type (string, int, float, date, time)
  - `operators`: Available operators (equals, contains, gt, lt, etc.)
  - `enumValues`: Predefined values for enums

All keys match database column names or JSON payload fields!
