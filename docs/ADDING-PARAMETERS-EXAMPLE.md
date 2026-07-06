# Adding Parameters - Quick Start Example

## Real Example: Serial Number

We just added "Serial Number" to demonstrate how easy it is. Here's exactly what was done:

---

## Step 1: Define Parameter (5 lines)

**File:** `catalog/params/definitions.go` (line 118)

```go
{
    Key:         "serial_number",
    Label:       "Serial Number",
    Description: "Manufacturer serial number for warranty tracking.",
    Category:    "hardware",
    Type:        TypeString,
    Filter:      FilterContains,
    Sort:        SortAlpha,
    Mono:        true,
},
```

**Total:** 1 struct entry

---

## Step 2: Mount in Schemas (2 places)

### Computers

**File:** `catalog/params/schemas.go` (line 36)

```go
{Key: "hardware", Label: "Hardware", Params: []string{
    "hostname", "fqdn", "os_family", "os_distribution", "os_version",
    "architecture", "cpu", "ram_mb", "serial_number", "storage_mb",
    //                                 ^^^^^^^^^^^^^^^ ADD HERE
}},
```

### Servers

**File:** `catalog/params/schemas.go` (line 63)

```go
{Key: "hardware", Label: "Hardware", Params: []string{
    "hostname", "fqdn", "os_family", "os_distribution", "os_version",
    "architecture", "server_role", "services", "uptime_since", "ram_mb", "serial_number",
    //                                                                     ^^^^^^^^^^^^^^^ ADD HERE
}},
```

**Total:** 2 array additions

---

## Step 3: Add Test Data (2 places)

### Computers

**File:** `cmd/seed/main.go` (line 109)

```go
payload := fmt.Sprintf(`{
    ...
    "ram_mb": %d,
    "serial_number": "SN-%s-%04d",  // ← ADD THIS LINE
    "storage_mb": %d
}`, ..., strings.ToUpper(comp.hostname[:3]), i+1, ...)
```

### Servers

**File:** `cmd/seed/main.go` (line 179)

```go
payload := fmt.Sprintf(`{
    ...
    "ram_mb": %d,
    "serial_number": "SRV-%s-%04d",  // ← ADD THIS LINE
    "storage_mb": 4096000,
}`, ..., strings.ToUpper(srv.role), i+1, ...)
```

**Total:** 2 JSON additions

---

## Step 4: Rebuild & Test

```bash
rm pluris.db*                  # Delete old database
make build                     # Rebuild with new parameter
./scripts/restart-dev.sh       # Restart server
```

---

## What Happened Automatically

### ✅ Table Column Added

**Column picker now shows:**
```
HARDWARE SPECIFICATIONS
  ☐ CPU
  ☐ RAM
  ☐ Serial Number          ← NEW!
  ☐ Storage
```

Search "serial" → Finds it!

---

### ✅ Filter Dropdown Updated

**Advanced filter shows:**
```
Add Filter
├─ Name (contains)
├─ OS Family (equals)
├─ RAM (≥)
├─ Serial Number (contains)  ← NEW!
└─ ...
```

Can filter: `Serial Number contains "SN-DEV"`

---

### ✅ Detail Page Updated

**Click any computer → Hardware section shows:**
```
HARDWARE
├─ Hostname: dev-laptop-001
├─ OS Family: linux
├─ CPU: AMD Ryzen 7 5800X
├─ RAM: 16 GB
├─ Serial Number: SN-DEV-0001  ← NEW! (in monospace)
└─ Storage: 512 GB
```

---

### ✅ Data Attribute Added

**Table rows have:**
```html
<tr data-serial-number="SN-DEV-0001" ...>
```

JavaScript can filter by this!

---

### ✅ Sort & Filter Work

- Click column header → Sorts alphabetically
- Filter by "SN-APP" → Shows only matching rows
- Monospace font applied automatically

---

## Test It Live

**URL:** https://YOUR-DEV-HOST.example.com/assets/computers

**Try these:**

1. **Column Picker**
   - Click "Columns" button
   - Search "serial"
   - See "Serial Number" option
   - Check it → Column appears

2. **Filter**
   - Click "Advanced"
   - Select "Serial Number"
   - Select "contains"
   - Type "DEV"
   - See filtered results

3. **Detail Page**
   - Click any computer row
   - Scroll to Hardware section
   - See "Serial Number: SN-DEV-0001"
   - Note monospace font

4. **Sort**
   - Make Serial Number visible
   - Click column header
   - Rows sort by serial number

---

## Summary

### Code Added
- **1 parameter definition** (5 lines)
- **2 schema mounts** (1 line each)
- **2 test data additions** (1 line each)

**Total:** ~10 lines of code

### Features Gained
- ✅ Table column with sort
- ✅ Filter dropdown with operators
- ✅ Detail page field
- ✅ Data attributes for JS
- ✅ Column picker entry
- ✅ Search indexing
- ✅ Type-aware rendering

### Time Required
- **Write code:** 2 minutes
- **Rebuild & test:** 1 minute
- **Total:** 3 minutes

---

## Next Steps

Want to add more parameters? Follow the same pattern:

1. **Define** in `definitions.go`
2. **Mount** in `schemas.go`
3. **Test data** in `seed/main.go` (optional)
4. **Rebuild** and verify

That's it! 🚀

---

## Documentation

**Full guide:** `docs/PARAMETER-REGISTRY.md`

Includes:
- Complete type system reference
- Filter operator guide
- Category best practices
- 10+ real examples
- Advanced patterns

**Status:** Production-ready, fully standardized! ✨
