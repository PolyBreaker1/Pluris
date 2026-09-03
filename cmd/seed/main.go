package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pluris/pluris/db"
	"github.com/pluris/pluris/pkg/database"
)

func main() {
	tenantFlag := flag.String("tenant", "", "tenant slug to seed into; default: the first existing tenant. Pass 'demo' to create-or-reuse a standalone demo tenant.")
	dbFlag := flag.String("db", "pluris.db", "database file to seed (point at a scratch file to test without touching the live database)")
	flag.Parse()

	if err := run(*dbFlag, *tenantFlag); err != nil {
		log.Fatalf("Seeding failed: %v", err)
	}
}

// isUniqueConstraintErr reports whether err is a SQLite UNIQUE-constraint
// violation — the only error the seeder treats as "already seeded".
// Anything else is a real failure and must abort the run. String matching
// keeps this file on database/sql-level surfaces (no direct mattn/go-sqlite3
// import just for the typed error).
func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// run seeds the database at dbPath with demo data for the tenant identified
// by tenantSlug (see resolveTenant for the resolution rules). It is safe to
// call twice against the same database: every insert either uses
// ON CONFLICT DO NOTHING semantics or is preceded by an existence check.
func run(dbPath, tenantSlug string) error {
	database, err := database.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	ctx := context.Background()

	log.Println("Seeding database with comprehensive test data...")

	tenant, err := resolveTenant(ctx, database, tenantSlug)
	if err != nil {
		return err
	}
	log.Printf("✓ Tenant: %s (ID: %d, Slug: %s)", tenant.Name, tenant.ID, tenant.Slug)

	// Create sites
	getOrCreateSite := func(name, slug string) (db.Site, error) {
		site, err := database.Queries.CreateSite(ctx, db.CreateSiteParams{
			TenantID: tenant.ID,
			Name:     name,
			Slug:     slug,
		})
		if err == nil {
			return site, nil
		}
		if !isUniqueConstraintErr(err) {
			return db.Site{}, fmt.Errorf("create site %s: %w", slug, err)
		}
		log.Printf("Site %s already present, reusing", slug)
		sites, listErr := database.Queries.ListSitesByTenant(ctx, tenant.ID)
		if listErr != nil {
			return db.Site{}, fmt.Errorf("failed to list sites for tenant %s: %w", tenant.Slug, listErr)
		}
		for _, s := range sites {
			if s.Slug == slug {
				return s, nil
			}
		}
		return db.Site{}, fmt.Errorf("failed to create or find site %s: %w", slug, err)
	}

	hqSite, err := getOrCreateSite("Headquarters", "hq")
	if err != nil {
		return err
	}
	log.Printf("✓ Site: %s (ID: %d, Slug: %s)", hqSite.Name, hqSite.ID, hqSite.Slug)

	remoteSite, err := getOrCreateSite("Remote Office", "remote")
	if err != nil {
		return err
	}
	log.Printf("✓ Site: %s (ID: %d, Slug: %s)", remoteSite.Name, remoteSite.ID, remoteSite.Slug)

	dcSite, err := getOrCreateSite("Data Center", "dc")
	if err != nil {
		return err
	}
	log.Printf("✓ Site: %s (ID: %d, Slug: %s)", dcSite.Name, dcSite.ID, dcSite.Slug)

	// Helper to generate human ID
	humanID := func(subtype, tenantSlug, siteSlug string, seq int) string {
		return fmt.Sprintf("%s.%s.%s.%04d", subtype, tenantSlug, siteSlug, seq)
	}

	// Deterministic per-tenant UUID: embedding the tenant ID lets the same
	// seeder run against multiple tenants in one database without tripping
	// the global UNIQUE(uuid) constraint, while staying stable across runs
	// for the same tenant (which is what makes re-runs skip cleanly).
	assetUUID := func(prefix string, i int) string {
		return fmt.Sprintf("%s-e29b-41d4-%04x-%012d", prefix, tenant.ID&0xffff, i)
	}

	// =========================================================================
	// COMPUTERS - 15 devices across 3 sites
	// =========================================================================
	computers := []struct {
		hostname        string
		osFamily        string
		osDistribution  string
		osVersion       string
		kernelVersion   string
		architecture    string
		cpuSummary      string
		ramMB           int64
		storageMB       int64
		enrollmentState string
		vendor          string
		siteSlug        string
		site            db.Site
		packageFamily   string
		diskEncryption  string
	}{
		// HQ - Development team
		{"dev-laptop-001", "linux", "Ubuntu", "24.04 LTS", "6.8.0-40-generic", "x86_64", "Intel Core i7-12700K @ 3.6 GHz (8 cores)", 16384, 512000, "enrolled", "Dell", "hq", hqSite, "deb", "luks"},
		{"dev-laptop-002", "linux", "Fedora", "40", "6.9.5-200.fc40.x86_64", "x86_64", "AMD Ryzen 7 5800X @ 3.8 GHz (8 cores)", 32768, 1024000, "enrolled", "Lenovo", "hq", hqSite, "rpm", "luks"},
		{"dev-workstation-001", "linux", "Arch Linux", "rolling", "6.9.7-arch1-1", "x86_64", "AMD Ryzen 9 7950X @ 4.5 GHz (16 cores)", 65536, 2048000, "enrolled", "Custom Build", "hq", hqSite, "arch", "none"},
		{"dev-laptop-003", "linux", "Pop!_OS", "22.04 LTS", "6.6.10-76060610-generic", "x86_64", "Intel Core i7-1365U @ 1.8 GHz (10 cores)", 32768, 512000, "enrolled", "System76", "hq", hqSite, "deb", "luks"},
		{"dev-laptop-004", "linux", "Linux Mint", "21.3", "6.5.0-44-generic", "x86_64", "AMD Ryzen 5 7640U @ 4.3 GHz (6 cores)", 16384, 512000, "enrolled", "Framework", "hq", hqSite, "deb", "none"},
		// HQ - QA and Management
		{"qa-laptop-001", "windows", "Windows 11 Pro", "23H2", "10.0.22631.3880", "x86_64", "Intel Core i5-13500 @ 2.5 GHz (6 cores)", 16384, 512000, "enrolled", "HP", "hq", hqSite, "other", "bitlocker"},
		{"qa-laptop-002", "windows", "Windows 11 Pro", "23H2", "10.0.22631.3880", "x86_64", "Intel Core i5-1335U @ 1.3 GHz (10 cores)", 16384, 256000, "enrolled", "Dell", "hq", hqSite, "other", "bitlocker"},
		{"admin-macbook-001", "macos", "macOS Sonoma", "14.5", "23.5.0", "arm64", "Apple M3 Pro (12 cores)", 36864, 512000, "enrolled", "Apple", "hq", hqSite, "other", "filevault"},
		{"exec-macbook-001", "macos", "macOS Sonoma", "14.5", "23.5.0", "arm64", "Apple M3 Max (16 cores)", 65536, 1024000, "enrolled", "Apple", "hq", hqSite, "other", "filevault"},
		// HQ - Pending/Approved devices
		{"new-laptop-001", "linux", "Ubuntu", "24.04 LTS", "6.8.0-40-generic", "x86_64", "Intel Core i5-1340P @ 1.9 GHz (12 cores)", 16384, 512000, "pending", "Dell", "hq", hqSite, "deb", "none"},
		{"new-laptop-002", "windows", "Windows 11 Pro", "23H2", "10.0.22631.3880", "x86_64", "AMD Ryzen 7 7840U @ 3.3 GHz (8 cores)", 32768, 512000, "approved", "Lenovo", "hq", hqSite, "other", "bitlocker"},
		// Remote Office
		{"remote-laptop-001", "linux", "Ubuntu", "22.04 LTS", "5.15.0-113-generic", "x86_64", "Intel Core i7-1265U @ 1.8 GHz (10 cores)", 16384, 512000, "enrolled", "Dell", "remote", remoteSite, "deb", "luks"},
		{"remote-laptop-002", "windows", "Windows 11 Pro", "23H2", "10.0.22631.3880", "x86_64", "Intel Core i5-1235U @ 1.3 GHz (10 cores)", 16384, 256000, "enrolled", "HP", "remote", remoteSite, "other", "bitlocker"},
		{"remote-macbook-001", "macos", "macOS Ventura", "13.6", "22.6.0", "arm64", "Apple M2 (8 cores)", 16384, 512000, "enrolled", "Apple", "remote", remoteSite, "other", "filevault"},
		{"remote-desktop-001", "linux", "Debian", "12", "6.1.0-21-amd64", "x86_64", "Intel Core i5-12400 @ 2.5 GHz (6 cores)", 32768, 1024000, "enrolled", "Custom Build", "remote", remoteSite, "deb", "luks"},
	}

	computerCount := 0
	for i, comp := range computers {
		payload := fmt.Sprintf(`{
			"hostname": "%s",
			"fqdn": "%s.%s.local",
			"os_family": "%s",
			"os_package_family": "%s",
			"disk_encryption": "%s",
			"os_distribution": "%s",
			"os_version": "%s",
			"kernel_version": "%s",
			"architecture": "%s",
			"cpu_summary": "%s",
			"ram_mb": %d,
			"serial_number": "SN-%s-%04d",
			"storage_mb": %d
		}`, comp.hostname, comp.hostname, tenant.Slug, comp.osFamily, comp.packageFamily, comp.diskEncryption, comp.osDistribution,
			comp.osVersion, comp.kernelVersion, comp.architecture, comp.cpuSummary, comp.ramMB,
			strings.ToUpper(comp.hostname[:3]), i+1, comp.storageMB)

		asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
			Uuid:            assetUUID("550e8400", i),
			TenantID:        tenant.ID,
			SiteID:          sql.NullInt64{Int64: comp.site.ID, Valid: true},
			Subtype:         "computer",
			SubtypePayload:  payload,
			EnrollmentState: comp.enrollmentState,
			AgentVersion:    sql.NullString{String: "1.2.0", Valid: true},
			Labels:          sql.NullString{String: `{"env":"dev","team":"engineering"}`, Valid: true},
			HumanID:         sql.NullString{String: humanID("comp", tenant.Slug, comp.siteSlug, i+1), Valid: true},
		})
		if err != nil {
			if isUniqueConstraintErr(err) {
				log.Printf("Computer %s already present, skipping", comp.hostname)
				continue
			}
			return fmt.Errorf("create computer %s: %w", comp.hostname, err)
		}

		// Update with CMDB fields. Raw SQL: no generated query covers this
		// seed-only combination of CMDB columns with relative date() math.
		_, err = database.Conn().Exec(`
			UPDATE assets
			SET vendor = ?,
				lifecycle_state = 'active',
				location = ?,
				purchase_date = date('now', '-' || ? || ' months'),
				warranty_expires_at = date('now', '+' || ? || ' months')
			WHERE id = ?
		`, comp.vendor, fmt.Sprintf("%s, Floor %d", comp.site.Name, (i%4)+1), (i*3)+6, 36-(i*2), asset.ID)
		if err != nil {
			log.Printf("Failed to update CMDB fields for %s: %v", comp.hostname, err)
		}

		log.Printf("✓ Computer: %s (ID: %s, State: %s, Vendor: %s)", comp.hostname, asset.HumanID.String, comp.enrollmentState, comp.vendor)
		computerCount++
	}

	// =========================================================================
	// SERVERS - 12 servers across HQ and DC
	// =========================================================================
	servers := []struct {
		hostname       string
		osDistribution string
		osVersion      string
		kernelVersion  string
		ramMB          int64
		role           string
		services       []string
		siteSlug       string
		site           db.Site
		packageFamily  string
		diskEncryption string
	}{
		// Data Center - Production
		{"prod-app-01", "Ubuntu Server", "24.04 LTS", "6.8.0-40-generic", 65536, "app", []string{"nginx", "gunicorn", "python3"}, "dc", dcSite, "deb", "luks"},
		{"prod-app-02", "Ubuntu Server", "24.04 LTS", "6.8.0-40-generic", 65536, "app", []string{"nginx", "gunicorn", "python3"}, "dc", dcSite, "deb", "luks"},
		{"prod-db-01", "Ubuntu Server", "24.04 LTS", "6.8.0-40-generic", 131072, "db", []string{"postgresql-16", "pgbouncer"}, "dc", dcSite, "deb", "luks"},
		{"prod-db-02", "Ubuntu Server", "24.04 LTS", "6.8.0-40-generic", 131072, "db", []string{"postgresql-16", "patroni"}, "dc", dcSite, "deb", "none"},
		{"prod-web-01", "Rocky Linux", "9.4", "5.14.0-427.22.1.el9_4.x86_64", 32768, "web", []string{"nginx", "certbot", "haproxy"}, "dc", dcSite, "rpm", "luks"},
		{"prod-web-02", "Rocky Linux", "9.4", "5.14.0-427.22.1.el9_4.x86_64", 32768, "web", []string{"nginx", "certbot", "haproxy"}, "dc", dcSite, "rpm", "none"},
		{"prod-cache-01", "Alpine Linux", "3.20", "6.6.32-0-lts", 65536, "other", []string{"redis", "sentinel"}, "dc", dcSite, "apk", "none"},
		{"prod-queue-01", "Debian", "12", "6.1.0-21-amd64", 32768, "other", []string{"rabbitmq", "erlang"}, "dc", dcSite, "deb", "luks"},
		// HQ - Development/Staging
		{"dev-all-in-one", "Ubuntu Server", "24.04 LTS", "6.8.0-40-generic", 65536, "app", []string{"docker", "postgresql", "redis", "nginx"}, "hq", hqSite, "deb", "none"},
		{"staging-app-01", "Ubuntu Server", "22.04 LTS", "5.15.0-113-generic", 32768, "app", []string{"docker", "nginx"}, "hq", hqSite, "deb", "none"},
		{"monitoring-01", "Debian", "12", "6.1.0-21-amd64", 16384, "other", []string{"prometheus", "grafana", "alertmanager"}, "hq", hqSite, "deb", "luks"},
		{"ci-runner-01", "Ubuntu Server", "24.04 LTS", "6.8.0-40-generic", 32768, "other", []string{"gitlab-runner", "docker"}, "hq", hqSite, "deb", "none"},
	}

	serverCount := 0
	for i, srv := range servers {
		servicesJSON := `["` + srv.services[0]
		for _, s := range srv.services[1:] {
			servicesJSON += `","` + s
		}
		servicesJSON += `"]`

		payload := fmt.Sprintf(`{
			"hostname": "%s",
			"fqdn": "%s.%s.local",
			"os_family": "linux",
			"os_package_family": "%s",
			"disk_encryption": "%s",
			"os_distribution": "%s",
			"os_version": "%s",
			"kernel_version": "%s",
			"architecture": "x86_64",
			"cpu_summary": "Intel Xeon Gold 6248R @ 3.0 GHz (24 cores)",
			"ram_mb": %d,
			"serial_number": "SRV-%s-%04d",
			"storage_mb": 4096000,
			"server_role": "%s",
			"services": %s,
			"uptime_since": "2026-04-15T08:30:00Z"
		}`, srv.hostname, srv.hostname, tenant.Slug, srv.packageFamily, srv.diskEncryption, srv.osDistribution,
			srv.osVersion, srv.kernelVersion, srv.ramMB, strings.ToUpper(srv.role), i+1, srv.role, servicesJSON)

		asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
			Uuid:            assetUUID("650e8400", i),
			TenantID:        tenant.ID,
			SiteID:          sql.NullInt64{Int64: srv.site.ID, Valid: true},
			Subtype:         "server",
			SubtypePayload:  payload,
			EnrollmentState: "enrolled",
			AgentVersion:    sql.NullString{String: "1.2.0", Valid: true},
			Labels:          sql.NullString{String: fmt.Sprintf(`{"env":"production","tier":"%s"}`, srv.role), Valid: true},
			HumanID:         sql.NullString{String: humanID("srv", tenant.Slug, srv.siteSlug, i+1), Valid: true},
		})
		if err != nil {
			if isUniqueConstraintErr(err) {
				log.Printf("Server %s already present, skipping", srv.hostname)
				continue
			}
			return fmt.Errorf("create server %s: %w", srv.hostname, err)
		}

		// Update with CMDB fields. Raw SQL: no generated query covers this
		// seed-only combination of CMDB columns with relative date() math.
		_, err = database.Conn().Exec(`
			UPDATE assets
			SET vendor = 'Dell',
				lifecycle_state = 'active',
				location = ?,
				purchase_date = date('now', '-' || ? || ' months'),
				warranty_expires_at = date('now', '+' || ? || ' months')
			WHERE id = ?
		`, fmt.Sprintf("%s, Rack %d", srv.site.Name, (i%8)+1), (i*2)+12, 48-(i*3), asset.ID)
		if err != nil {
			log.Printf("Failed to update CMDB fields for %s: %v", srv.hostname, err)
		}

		log.Printf("✓ Server: %s (ID: %s, Role: %s)", srv.hostname, asset.HumanID.String, srv.role)
		serverCount++
	}

	// =========================================================================
	// PRINTERS - 10 printers across sites
	// =========================================================================
	printers := []struct {
		model           string
		vendor          string
		ip              string
		queues          []string
		formats         []string
		siteSlug        string
		site            db.Site
		enrollmentState string
	}{
		// HQ Printers
		{"HP LaserJet Pro 4001dn", "HP", "192.168.1.100", []string{"hq-laser-bw", "hq-laser-duplex"}, []string{"PCL6", "PostScript", "PDF"}, "hq", hqSite, "enrolled"},
		{"HP Color LaserJet Pro MFP M479fdw", "HP", "192.168.1.101", []string{"hq-color-main", "hq-color-duplex"}, []string{"PCL6", "PostScript", "PDF"}, "hq", hqSite, "enrolled"},
		{"Epson EcoTank ET-4850", "Epson", "192.168.1.102", []string{"hq-inkjet-color"}, []string{"PCL", "PDF"}, "hq", hqSite, "enrolled"},
		{"Brother MFC-L8900CDW", "Brother", "192.168.1.103", []string{"hq-brother-color", "hq-brother-bw"}, []string{"PCL6", "PostScript", "PDF", "XPS"}, "hq", hqSite, "enrolled"},
		{"Canon imageCLASS MF753Cdw", "Canon", "192.168.1.104", []string{"hq-canon-main"}, []string{"PCL6", "PostScript", "PDF"}, "hq", hqSite, "enrolled"},
		{"Xerox VersaLink C7025", "Xerox", "192.168.1.105", []string{"hq-xerox-production"}, []string{"PCL6", "PostScript", "PDF", "XPS"}, "hq", hqSite, "enrolled"},
		// Remote Office Printers
		{"HP LaserJet Pro M404dn", "HP", "192.168.2.100", []string{"remote-laser-main"}, []string{"PCL6", "PostScript"}, "remote", remoteSite, "enrolled"},
		{"Brother HL-L3270CDW", "Brother", "192.168.2.101", []string{"remote-color"}, []string{"PCL6", "PDF"}, "remote", remoteSite, "enrolled"},
		// Pending/Problem printers
		{"HP LaserJet Enterprise M507dn", "HP", "192.168.1.110", []string{"hq-new-laser"}, []string{"PCL6", "PostScript", "PDF"}, "hq", hqSite, "pending"},
		{"Epson WorkForce Pro WF-C5790", "Epson", "192.168.1.111", []string{"hq-wf-color"}, []string{"PCL", "PDF"}, "hq", hqSite, "approved"},
	}

	printerCount := 0
	for i, prn := range printers {
		queuesJSON := `["` + prn.queues[0] + `"`
		for _, q := range prn.queues[1:] {
			queuesJSON += `,"` + q + `"`
		}
		queuesJSON += `]`

		formatsJSON := `["` + prn.formats[0] + `"`
		for _, f := range prn.formats[1:] {
			formatsJSON += `,"` + f + `"`
		}
		formatsJSON += `]`

		payload := fmt.Sprintf(`{
			"printer_model": "%s",
			"printer_ip": "%s",
			"queues": %s,
			"supported_formats": %s,
			"consumables": [
				{"kind": "toner_black", "level": %d},
				{"kind": "toner_cyan", "level": %d},
				{"kind": "toner_magenta", "level": %d},
				{"kind": "toner_yellow", "level": %d}
			]
		}`, prn.model, prn.ip, queuesJSON, formatsJSON,
			85-i*5, 72-i*4, 68-i*3, 79-i*6)

		asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
			Uuid:            assetUUID("750e8400", i),
			TenantID:        tenant.ID,
			SiteID:          sql.NullInt64{Int64: prn.site.ID, Valid: true},
			Subtype:         "printer",
			SubtypePayload:  payload,
			EnrollmentState: prn.enrollmentState,
			AgentVersion:    sql.NullString{String: "", Valid: false},
			Labels:          sql.NullString{String: fmt.Sprintf(`{"type":"network_printer","floor":"%d"}`, (i%4)+1), Valid: true},
			HumanID:         sql.NullString{String: humanID("prn", tenant.Slug, prn.siteSlug, i+1), Valid: true},
		})
		if err != nil {
			if isUniqueConstraintErr(err) {
				log.Printf("Printer %s already present, skipping", prn.model)
				continue
			}
			return fmt.Errorf("create printer %s: %w", prn.model, err)
		}

		// Update with CMDB fields. Raw SQL: no generated query covers this
		// seed-only combination of CMDB columns with relative date() math.
		_, err = database.Conn().Exec(`
			UPDATE assets
			SET vendor = ?,
				lifecycle_state = 'active',
				location = ?,
				purchase_date = date('now', '-' || ? || ' months'),
				warranty_expires_at = date('now', '+' || ? || ' months')
			WHERE id = ?
		`, prn.vendor, fmt.Sprintf("%s, Floor %d, Copy Room", prn.site.Name, (i%4)+1), (i*4)+8, 24-(i*2), asset.ID)
		if err != nil {
			log.Printf("Failed to update CMDB fields for %s: %v", prn.model, err)
		}

		log.Printf("✓ Printer: %s (ID: %s, IP: %s)", prn.model, asset.HumanID.String, prn.ip)
		printerCount++
	}

	// =========================================================================
	// DESKS - 12 hot-desking stations
	// =========================================================================
	desks := []struct {
		location     string
		dockModel    string
		monitorCount int
		siteSlug     string
		site         db.Site
	}{
		// HQ Floor 3 - Engineering
		{"Floor 3, Desk 01", "Dell WD19TBS", 2, "hq", hqSite},
		{"Floor 3, Desk 02", "Dell WD19TBS", 2, "hq", hqSite},
		{"Floor 3, Desk 03", "CalDigit TS4", 3, "hq", hqSite},
		{"Floor 3, Desk 04", "HP Thunderbolt Dock G4", 2, "hq", hqSite},
		{"Floor 3, Desk 05", "Dell WD19TBS", 2, "hq", hqSite},
		// HQ Floor 4 - Management
		{"Floor 4, Desk 01", "CalDigit TS4", 3, "hq", hqSite},
		{"Floor 4, Desk 02", "Belkin Thunderbolt 4 Dock", 2, "hq", hqSite},
		{"Floor 4, Desk 03", "Dell WD22TB4", 2, "hq", hqSite},
		// Remote Office
		{"Main Area, Desk 01", "Dell WD19TBS", 2, "remote", remoteSite},
		{"Main Area, Desk 02", "HP Thunderbolt Dock G4", 2, "remote", remoteSite},
		{"Main Area, Desk 03", "Anker 577", 1, "remote", remoteSite},
		{"Conference Room A", "CalDigit TS4", 2, "remote", remoteSite},
	}

	deskCount := 0
	for i, desk := range desks {
		payload := fmt.Sprintf(`{
			"desk_location": "%s",
			"dock_model": "%s",
			"monitor_count": %d,
			"guest_profile": "default-hotdesk"
		}`, desk.location, desk.dockModel, desk.monitorCount)

		asset, err := database.Queries.CreateAsset(ctx, db.CreateAssetParams{
			Uuid:            assetUUID("850e8400", i),
			TenantID:        tenant.ID,
			SiteID:          sql.NullInt64{Int64: desk.site.ID, Valid: true},
			Subtype:         "desk",
			SubtypePayload:  payload,
			EnrollmentState: "enrolled",
			AgentVersion:    sql.NullString{String: "", Valid: false},
			Labels:          sql.NullString{String: `{"type":"hot_desk","bookable":"true"}`, Valid: true},
			HumanID:         sql.NullString{String: humanID("desk", tenant.Slug, desk.siteSlug, i+1), Valid: true},
		})
		if err != nil {
			if isUniqueConstraintErr(err) {
				log.Printf("Desk %s already present, skipping", desk.location)
				continue
			}
			return fmt.Errorf("create desk %s: %w", desk.location, err)
		}

		// Update with CMDB fields - desks have different vendor (dock vendor).
		// Raw SQL below for the same reason as the other asset blocks: no
		// generated query covers this seed-only CMDB update with date() math.
		dockVendor := "Dell"
		if strings.Contains(desk.dockModel, "CalDigit") {
			dockVendor = "CalDigit"
		} else if strings.Contains(desk.dockModel, "HP") {
			dockVendor = "HP"
		} else if strings.Contains(desk.dockModel, "Belkin") {
			dockVendor = "Belkin"
		} else if strings.Contains(desk.dockModel, "Anker") {
			dockVendor = "Anker"
		}

		_, err = database.Conn().Exec(`
			UPDATE assets
			SET vendor = ?,
				lifecycle_state = 'active',
				location = ?,
				purchase_date = date('now', '-' || ? || ' months')
			WHERE id = ?
		`, dockVendor, fmt.Sprintf("%s, %s", desk.site.Name, desk.location), (i*2)+3, asset.ID)
		if err != nil {
			log.Printf("Failed to update CMDB fields for %s: %v", desk.location, err)
		}

		log.Printf("✓ Desk: %s (ID: %s, Dock: %s)", desk.location, asset.HumanID.String, desk.dockModel)
		deskCount++
	}

	// =========================================================================
	// IDENTITIES - 10 directory entries across 3 sites, varied roles/state
	// so the Users list has something to filter/sort/search against (role,
	// department, site, enabled, locked). Directory records only - no
	// password_hash, matching "Identity list - synced from Kanidm or
	// manually added" on the page: most demo staff never need console login.
	// =========================================================================
	identitySeeds := []struct {
		username, email, displayName, givenName, surname string
		title, department, employeeType                  string
		site                                             db.Site
		role                                             string
		phoneOffice, phoneMobile                         string
		managerUsername                                  string // "" = no manager
		locked, disabled                                 bool
	}{
		{"j.moreau", "j.moreau@" + tenant.Slug + ".local", "Julien Moreau", "Julien", "Moreau",
			"Engineering Manager", "Engineering", "full_time", hqSite, "admin",
			"+1-555-0101", "+1-555-7101", "", false, false},
		{"s.chen", "s.chen@" + tenant.Slug + ".local", "Sarah Chen", "Sarah", "Chen",
			"Senior Software Engineer", "Engineering", "full_time", hqSite, "user",
			"+1-555-0102", "+1-555-7102", "j.moreau", false, false},
		{"a.osei", "a.osei@" + tenant.Slug + ".local", "Ama Osei", "Ama", "Osei",
			"Software Engineer", "Engineering", "full_time", hqSite, "user",
			"+1-555-0103", "+1-555-7103", "j.moreau", false, false},
		{"r.kowalski", "r.kowalski@" + tenant.Slug + ".local", "Robert Kowalski", "Robert", "Kowalski",
			"IT Support Technician", "IT", "full_time", hqSite, "technician",
			"+1-555-0104", "+1-555-7104", "", false, false},
		{"n.patel", "n.patel@" + tenant.Slug + ".local", "Nisha Patel", "Nisha", "Patel",
			"IT Support Technician", "IT", "full_time", dcSite, "technician",
			"+1-555-0105", "+1-555-7105", "", false, false},
		{"d.oconnor", "d.oconnor@" + tenant.Slug + ".local", "Declan O'Connor", "Declan", "O'Connor",
			"QA Engineer", "Quality Assurance", "full_time", hqSite, "user",
			"+1-555-0106", "+1-555-7106", "j.moreau", false, false},
		{"m.tanaka", "m.tanaka@" + tenant.Slug + ".local", "Mika Tanaka", "Mika", "Tanaka",
			"Account Executive", "Sales", "full_time", remoteSite, "user",
			"+1-555-0107", "+1-555-7107", "", false, false},
		{"l.silva", "l.silva@" + tenant.Slug + ".local", "Lucas Silva", "Lucas", "Silva",
			"Customer Support Specialist", "Support", "full_time", remoteSite, "user",
			"+1-555-0108", "+1-555-7108", "", true, false}, // locked: too many bad logins
		{"e.novak", "e.novak@" + tenant.Slug + ".local", "Eva Novak", "Eva", "Novak",
			"Office Manager", "Operations", "full_time", hqSite, "user",
			"+1-555-0109", "+1-555-7109", "", false, false},
		{"t.andersson", "t.andersson@" + tenant.Slug + ".local", "Theo Andersson", "Theo", "Andersson",
			"Contract Developer", "Engineering", "contractor", hqSite, "user",
			"+1-555-0110", "+1-555-7110", "j.moreau", false, true}, // disabled: contract ended
	}

	identityIDByUsername := make(map[string]int64, len(identitySeeds))
	identityCount := 0
	for _, seed := range identitySeeds {
		var managerID sql.NullInt64
		if seed.managerUsername != "" {
			if id, ok := identityIDByUsername[seed.managerUsername]; ok {
				managerID = sql.NullInt64{Int64: id, Valid: true}
			}
		}

		ident, err := database.Queries.CreateIdentity(ctx, db.CreateIdentityParams{
			TenantID:          tenant.ID,
			SiteID:            sql.NullInt64{Int64: seed.site.ID, Valid: true},
			Username:          seed.username,
			UserPrincipalName: sql.NullString{String: seed.email, Valid: true},
			Email:             seed.email,
			DisplayName:       seed.displayName,
			GivenName:         sql.NullString{String: seed.givenName, Valid: true},
			Surname:           sql.NullString{String: seed.surname, Valid: true},
			Title:             sql.NullString{String: seed.title, Valid: true},
			Department:        sql.NullString{String: seed.department, Valid: true},
			Company:           sql.NullString{String: tenant.Name, Valid: true},
			EmployeeID:        sql.NullString{String: fmt.Sprintf("EMP-%04d", identityCount+1), Valid: true},
			EmployeeType:      sql.NullString{String: seed.employeeType, Valid: true},
			ManagerID:         managerID,
			PhoneOffice:       sql.NullString{String: seed.phoneOffice, Valid: true},
			PhoneMobile:       sql.NullString{String: seed.phoneMobile, Valid: true},
			Role:              seed.role,
		})
		if err != nil {
			if isUniqueConstraintErr(err) {
				log.Printf("Identity %s already present, skipping", seed.username)
				// Still needed below by any seeded report's managerUsername lookup.
				existing, getErr := database.Queries.GetIdentityByEmail(ctx, db.GetIdentityByEmailParams{
					TenantID: tenant.ID,
					Email:    seed.email,
				})
				if getErr == nil {
					identityIDByUsername[seed.username] = existing.ID
				}
				continue
			}
			return fmt.Errorf("create identity %s: %w", seed.username, err)
		}
		identityIDByUsername[seed.username] = ident.ID

		if seed.locked {
			if err := database.Queries.SetIdentityLocked(ctx, db.SetIdentityLockedParams{
				ID:            ident.ID,
				AccountLocked: true,
			}); err != nil {
				return fmt.Errorf("lock identity %s: %w", seed.username, err)
			}
		}
		if seed.disabled {
			if err := database.Queries.SetIdentityEnabled(ctx, db.SetIdentityEnabledParams{
				ID:             ident.ID,
				AccountEnabled: false,
			}); err != nil {
				return fmt.Errorf("disable identity %s: %w", seed.username, err)
			}
		}

		log.Printf("✓ Identity: %s (ID: %d, Role: %s, Dept: %s)", seed.displayName, ident.ID, seed.role, seed.department)
		identityCount++
	}

	// =========================================================================
	// GROUPS, MEMBERSHIPS, CONFIG GROUP, SOFTWARE, ASSET EXTRAS
	// =========================================================================

	// First computers of the tenant, in a deterministic order. Raw SQL because
	// ListAssetsBySubtype orders by last_seen_at (NULL for seeded assets),
	// which would make "first computer" unstable across runs.
	firstComputerIDs := func(limit int) ([]int64, error) {
		rows, err := database.Conn().Query(
			`SELECT id FROM assets WHERE tenant_id = ? AND subtype = 'computer' ORDER BY id LIMIT ?`,
			tenant.ID, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	}

	// --- Groups ---
	getOrCreateGroup := func(name, slug, category, scope string) (db.Group, error) {
		groups, err := database.Queries.ListGroupsByTenant(ctx, tenant.ID)
		if err != nil {
			return db.Group{}, fmt.Errorf("failed to list groups: %w", err)
		}
		for _, g := range groups {
			if g.Slug == slug {
				log.Printf("Group %s already present, skipping", slug)
				return g, nil
			}
		}
		group, err := database.Queries.CreateGroup(ctx, db.CreateGroupParams{
			TenantID: tenant.ID,
			SiteID:   sql.NullInt64{},
			Name:     name,
			Slug:     slug,
		})
		if err != nil {
			return db.Group{}, fmt.Errorf("failed to create group %s: %w", slug, err)
		}
		// CreateGroup does not accept the newer group_category/group_scope
		// columns yet; set them explicitly rather than relying on defaults.
		if _, err := database.Conn().Exec(
			`UPDATE groups SET group_category = ?, group_scope = ? WHERE id = ?`,
			category, scope, group.ID); err != nil {
			return db.Group{}, fmt.Errorf("failed to set category/scope for group %s: %w", slug, err)
		}
		log.Printf("✓ Group: %s (slug: %s, category: %s, scope: %s)", name, slug, category, scope)
		return group, nil
	}

	engGroup, err := getOrCreateGroup("Engineering-Laptops", "engineering-laptops", "security", "global")
	if err != nil {
		return err
	}
	staffGroup, err := getOrCreateGroup("HQ-All-Staff", "hq-all-staff", "security", "domain_local")
	if err != nil {
		return err
	}
	groupCount := 2

	// --- Memberships ---
	membershipCount := 0
	compIDs, err := firstComputerIDs(3)
	if err != nil {
		return fmt.Errorf("failed to query computers for group membership: %w", err)
	}
	for _, id := range compIDs {
		// AddAssetToGroup is ON CONFLICT DO NOTHING - idempotent by design.
		if err := database.Queries.AddAssetToGroup(ctx, db.AddAssetToGroupParams{
			GroupID: engGroup.ID,
			AssetID: sql.NullInt64{Int64: id, Valid: true},
		}); err != nil {
			return fmt.Errorf("failed to add asset %d to group %s: %w", id, engGroup.Slug, err)
		}
		membershipCount++
	}
	log.Printf("✓ Memberships: %d computers -> %s", len(compIDs), engGroup.Name)

	identities, err := database.Queries.ListIdentitiesByTenant(ctx, db.ListIdentitiesByTenantParams{
		TenantID: tenant.ID,
		Offset:   0,
		Limit:    2,
	})
	if err != nil {
		return fmt.Errorf("failed to list identities: %w", err)
	}
	for _, ident := range identities {
		if err := database.Queries.AddIdentityToGroup(ctx, db.AddIdentityToGroupParams{
			GroupID:    staffGroup.ID,
			IdentityID: sql.NullInt64{Int64: ident.ID, Valid: true},
		}); err != nil {
			return fmt.Errorf("failed to add identity %d to group %s: %w", ident.ID, staffGroup.Slug, err)
		}
		membershipCount++
	}
	if len(identities) == 0 {
		log.Printf("Tenant %s has no identities yet; skipping identity memberships", tenant.Slug)
	} else {
		log.Printf("✓ Memberships: %d identities -> %s", len(identities), staffGroup.Name)
	}

	// --- Configuration group with one binding and one assignment ---
	// Policy URN is a real bundled catalog entry: the first policy in
	// catalog/policies/catalog_computer.go ("Minimum password length").
	const baselinePolicyURN = "sec.account.password.min-length"

	cfgGroup, err := database.Queries.GetConfigurationGroupByName(ctx, db.GetConfigurationGroupByNameParams{
		TenantID: tenant.ID,
		Name:     "HQ-Baseline",
	})
	if err != nil {
		cfgGroup, err = database.Queries.CreateConfigurationGroup(ctx, db.CreateConfigurationGroupParams{
			TenantID:    tenant.ID,
			Name:        "HQ-Baseline",
			Description: sql.NullString{String: "Baseline security configuration for HQ devices", Valid: true},
			Scope:       "machine",
			Disabled:    false,
		})
		if err != nil {
			return fmt.Errorf("failed to create configuration group HQ-Baseline: %w", err)
		}
		log.Printf("✓ Configuration group: %s (ID: %d)", cfgGroup.Name, cfgGroup.ID)
	} else {
		log.Printf("Configuration group HQ-Baseline already present, skipping")
	}
	cfgGroupCount := 1

	bindingCount := 0
	if _, err := database.Queries.GetBindingByGroupAndPolicy(ctx, db.GetBindingByGroupAndPolicyParams{
		ConfigurationGroupID: cfgGroup.ID,
		PolicyUrn:            baselinePolicyURN,
	}); err != nil {
		if _, err := database.Queries.CreateConfigurationGroupBinding(ctx, db.CreateConfigurationGroupBindingParams{
			ConfigurationGroupID: cfgGroup.ID,
			PolicyUrn:            baselinePolicyURN,
			State:                "enabled",
			ParameterValues:      sql.NullString{String: `{"minimum_length":"14"}`, Valid: true},
		}); err != nil {
			return fmt.Errorf("failed to create binding for %s: %w", baselinePolicyURN, err)
		}
		log.Printf("✓ Binding: %s -> %s", cfgGroup.Name, baselinePolicyURN)
	} else {
		log.Printf("Binding %s already present, skipping", baselinePolicyURN)
	}
	bindingCount = 1

	assignmentCount := 0
	if len(compIDs) > 0 {
		firstComputer := compIDs[0]
		// Existence check first: the assignments table has a UNIQUE
		// constraint on (group, target_type, target_id) and no
		// ON CONFLICT clause in the sqlc insert.
		alreadyAssigned := false
		assignments, err := database.Queries.ListAssignmentsByGroup(ctx, cfgGroup.ID)
		if err != nil {
			return fmt.Errorf("failed to list assignments: %w", err)
		}
		for _, a := range assignments {
			if a.TargetType == "asset" && a.TargetID == firstComputer {
				alreadyAssigned = true
				break
			}
		}
		if alreadyAssigned {
			log.Printf("Assignment %s -> asset %d already present, skipping", cfgGroup.Name, firstComputer)
		} else {
			if _, err := database.Queries.CreateConfigurationGroupAssignment(ctx, db.CreateConfigurationGroupAssignmentParams{
				ConfigurationGroupID: cfgGroup.ID,
				TargetType:           "asset",
				TargetID:             firstComputer,
				Priority:             0,
				Enforced:             false,
			}); err != nil {
				return fmt.Errorf("failed to create assignment: %w", err)
			}
			log.Printf("✓ Assignment: %s -> asset %d (priority 0)", cfgGroup.Name, firstComputer)
		}
		assignmentCount = 1
	} else {
		log.Printf("Tenant %s has no computers; skipping configuration group assignment", tenant.Slug)
	}

	// --- Installed software on the first computer ---
	softwareCount := 0
	if len(compIDs) > 0 {
		firstComputer := compIDs[0]
		software := []struct {
			name      string
			version   string
			publisher string
			pkgType   string
			sizeMB    int64
			daysAgo   int
		}{
			{"Firefox", "128.0", "Mozilla", "deb", 210, 12},
			{"Slack", "4.36.0", "Slack Technologies", "appimage", 180, 30},
			{"Microsoft Office 365", "2024", "Microsoft", "wine", 3174, 45},
			{"Docker", "27.3", "Docker Inc.", "native", 450, 7},
		}

		existing, err := database.Queries.ListSoftwareForAsset(ctx, firstComputer)
		if err != nil {
			return fmt.Errorf("failed to list installed software: %w", err)
		}
		installed := make(map[string]bool, len(existing))
		for _, sw := range existing {
			installed[sw.Name] = true
		}

		for _, sw := range software {
			if installed[sw.name] {
				log.Printf("Software %s already present on asset %d, skipping", sw.name, firstComputer)
				softwareCount++
				continue
			}
			if _, err := database.Queries.CreateInstalledSoftware(ctx, db.CreateInstalledSoftwareParams{
				AssetID:     firstComputer,
				Name:        sw.name,
				Version:     sql.NullString{String: sw.version, Valid: true},
				Publisher:   sql.NullString{String: sw.publisher, Valid: true},
				PkgType:     sw.pkgType,
				InstalledAt: sql.NullTime{Time: time.Now().AddDate(0, 0, -sw.daysAgo), Valid: true},
				SizeMb:      sql.NullInt64{Int64: sw.sizeMB, Valid: true},
			}); err != nil {
				return fmt.Errorf("failed to create installed software %s: %w", sw.name, err)
			}
			log.Printf("✓ Software: %s %s (%s) on asset %d", sw.name, sw.version, sw.pkgType, firstComputer)
			softwareCount++
		}
	}

	// --- Asset extras: description + managed-by on the first 2 computers ---
	extrasCount := 0
	extraIDs, err := firstComputerIDs(2)
	if err != nil {
		return fmt.Errorf("failed to query computers for extras: %w", err)
	}
	var manager sql.NullInt64
	if len(identities) > 0 {
		manager = sql.NullInt64{Int64: identities[0].ID, Valid: true}
	} else {
		log.Printf("Tenant %s has no identities; leaving managed_by unset on seeded computers", tenant.Slug)
	}
	for _, id := range extraIDs {
		if err := database.Queries.SetAssetDescription(ctx, db.SetAssetDescriptionParams{
			Description: sql.NullString{String: "Engineering laptop", Valid: true},
			ID:          id,
		}); err != nil {
			return fmt.Errorf("failed to set description on asset %d: %w", id, err)
		}
		if manager.Valid {
			if err := database.Queries.SetAssetManagedBy(ctx, db.SetAssetManagedByParams{
				ManagedBy: manager,
				ID:        id,
			}); err != nil {
				return fmt.Errorf("failed to set managed_by on asset %d: %w", id, err)
			}
		}
		extrasCount++
	}
	log.Printf("✓ Asset extras: description/managed_by set on %d computers", extrasCount)

	// Summary
	log.Println("")
	log.Println("✅ Database seeded successfully!")
	log.Printf("   - Tenant: %s (%s)", tenant.Name, tenant.Slug)
	log.Printf("   - 3 sites (hq, remote, dc)")
	log.Printf("   - %d computers (various OS, vendors, states)", computerCount)
	log.Printf("   - %d servers (app/db/web/other roles)", serverCount)
	log.Printf("   - %d printers (HP, Epson, Brother, Canon, Xerox)", printerCount)
	log.Printf("   - %d desks (hot-desking stations)", deskCount)
	log.Printf("   - %d groups (Engineering-Laptops, HQ-All-Staff)", groupCount)
	log.Printf("   - %d group memberships (assets + identities)", membershipCount)
	log.Printf("   - %d configuration group (HQ-Baseline), %d binding, %d assignment", cfgGroupCount, bindingCount, assignmentCount)
	log.Printf("   - %d installed software rows on the first computer", softwareCount)
	log.Printf("   - %d computers with description + managed-by extras", extrasCount)
	log.Println("")
	log.Println("All assets have:")
	log.Println("   - Human-readable IDs (comp.demo.hq.0001, srv.demo.dc.0001, etc.)")
	log.Println("   - Comprehensive parameter coverage")
	log.Println("   - CMDB lifecycle fields (vendor, purchase date, warranty)")
	log.Println("   - Labels for filtering")
	return nil
}

// resolveTenant maps the -tenant flag to a concrete tenant row:
//   - slug == "demo": create-or-reuse the standalone demo tenant
//   - slug set (anything else): must already exist, otherwise fail with the
//     list of available slugs
//   - slug empty: use the first existing tenant (the setup-wizard-created one
//     in a normal install); fail if the database has no tenants at all
func resolveTenant(ctx context.Context, database *database.Database, slug string) (db.Tenant, error) {
	switch {
	case slug == "demo":
		tenant, err := database.Queries.GetTenantBySlug(ctx, "demo")
		if err == nil {
			log.Printf("Demo tenant already present, reusing")
			return tenant, nil
		}
		tenant, err = database.Queries.CreateTenant(ctx, db.CreateTenantParams{
			Name: "Demo Organization",
			Slug: "demo",
		})
		if err != nil {
			return db.Tenant{}, fmt.Errorf("failed to create demo tenant: %w", err)
		}
		return tenant, nil

	case slug != "":
		tenant, err := database.Queries.GetTenantBySlug(ctx, slug)
		if err != nil {
			tenants, listErr := database.Queries.ListTenants(ctx)
			if listErr != nil {
				return db.Tenant{}, fmt.Errorf("tenant %q not found and listing tenants failed: %v", slug, listErr)
			}
			slugs := make([]string, 0, len(tenants))
			for _, t := range tenants {
				slugs = append(slugs, t.Slug)
			}
			return db.Tenant{}, fmt.Errorf("tenant %q not found; available tenant slugs: [%s] (or pass -tenant demo to create a standalone demo tenant)",
				slug, strings.Join(slugs, ", "))
		}
		return tenant, nil

	default:
		tenants, err := database.Queries.ListTenants(ctx)
		if err != nil {
			return db.Tenant{}, fmt.Errorf("failed to list tenants: %w", err)
		}
		if len(tenants) == 0 {
			return db.Tenant{}, fmt.Errorf("no tenants exist yet: run the setup wizard first (/setup), or pass -tenant demo to create a standalone demo tenant")
		}
		return tenants[0], nil
	}
}
