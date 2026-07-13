package assets

import "time"

// Mock asset catalog — populates every Assets subtype tab and the
// dashboard tile drilldowns. Editing this file is the v1 way to add
// fixtures; once db/schema/asset.go lands in Increment 3 this file
// becomes seed data for tests + a developer-mode "Reset to demo
// fleet" button.
//
// Selection criteria so the UI exercises every code path:
//   - At least two assets per subtype.
//   - At least one of every EnrollmentState (so the chip renders).
//   - At least one with a non-empty Owner, one without.
//   - At least one with a populated Labels map.
//   - At least one of every LifecycleState across the catalog.
//   - At least one Desk with GuestProfileID set (drives §III merge).
//   - One Server in each major Role (db/web/file/app) so the role
//     filter looks plausible.

// must parses an ISO timestamp and panics on failure. Used only at
// init-time on hard-coded literals — failure means the file is wrong,
// not the runtime, so a panic is correct.
func must(ts string) time.Time {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		panic("assets: bad mock timestamp " + ts + ": " + err.Error())
	}
	return t
}

// mustDate parses YYYY-MM-DD.
func mustDate(d string) time.Time {
	t, err := time.Parse("2006-01-02", d)
	if err != nil {
		panic("assets: bad mock date " + d + ": " + err.Error())
	}
	return t
}

// AllAssets returns every mock asset across every subtype. Order is
// stable so list snapshots are deterministic; the UI re-sorts by
// (Subtype, PrimaryHostname).
func AllAssets() []Asset {
	out := []Asset{}
	out = append(out, mockComputers()...)
	out = append(out, mockServers()...)
	out = append(out, mockPrinters()...)
	out = append(out, mockDesks()...)
	return out
}

// AllOfSubtype is the subtype-filtered view used by /assets/<subtype>.
// Returns a freshly allocated slice; safe for the caller to mutate.
func AllOfSubtype(s Subtype) []Asset {
	all := AllAssets()
	out := make([]Asset, 0, len(all))
	for _, a := range all {
		if a.Subtype == s {
			out = append(out, a)
		}
	}
	return out
}

// FindByID looks up a single asset by its stable ID across all subtypes.
// Returns nil if no asset with that ID exists in the catalog.
func FindByID(id string) *Asset {
	for _, a := range AllAssets() {
		if a.ID == id {
			return &a
		}
	}
	return nil
}

// ----------------------------------------------------------------------
// Computers — the largest subtype population.
// ----------------------------------------------------------------------

func mockComputers() []Asset {
	return []Asset{
		{
			ID: "comp.acme.hq.lt-0142", UUID: "a8f5-…-0142", TenantID: "acme",
			Subtype: SubtypeComputer,
			Payload: ComputerPayload{
				Hostname: "lt-0142", FQDN: "lt-0142.acme.local",
				OSFamily: "linux", OSDistribution: "Ubuntu", OSVersion: "24.04 LTS",
				Architecture: "x86_64",
				CPUSummary:   "Intel i7-1365U @ 1.8GHz (10 cores)",
				RAMMB:        16384, StorageMB: 512000,
			},
			Site: "HQ — Floor 3", Groups: []string{"engineering", "vip-laptops"},
			Labels:             map[string]string{"team": "platform", "ring": "canary"},
			EnrollmentState:    EnrollmentEnrolled,
			EnrolledAt:         must("2026-02-14T09:12:00Z"),
			LastSeenAt:         must("2026-05-17T08:42:00Z"),
			AgentVersion:       "0.8.3",
			LifecycleState:     LifecycleActive,
			Location:           "HQ Floor 3 / Desk 14",
			OwnerIdentity:      "alice.chen@acme.local",
			Vendor:             "Lenovo",
			PurchaseDate:       mustDate("2026-01-10"),
			PurchasePriceCents: 142500,
			WarrantyExpiresAt:  mustDate("2029-01-10"),
		},
		{
			ID: "comp.acme.hq.lt-0143", UUID: "a8f5-…-0143", TenantID: "acme",
			Subtype: SubtypeComputer,
			Payload: ComputerPayload{
				Hostname: "lt-0143", FQDN: "lt-0143.acme.local",
				OSFamily: "windows", OSDistribution: "Windows 11 Pro", OSVersion: "23H2",
				Architecture: "x86_64",
				CPUSummary:   "AMD Ryzen 7 7840U (16 cores)",
				RAMMB:        32768, StorageMB: 1024000,
			},
			Site: "HQ — Floor 3", Groups: []string{"sales"},
			Labels:             map[string]string{"team": "sales"},
			EnrollmentState:    EnrollmentEnrolled,
			EnrolledAt:         must("2026-03-02T13:31:00Z"),
			LastSeenAt:         must("2026-05-17T08:39:00Z"),
			AgentVersion:       "0.8.3",
			LifecycleState:     LifecycleActive,
			OwnerIdentity:      "ben.kowalski@acme.local",
			Vendor:             "Dell",
			PurchaseDate:       mustDate("2026-02-20"),
			PurchasePriceCents: 168000,
			WarrantyExpiresAt:  mustDate("2029-02-20"),
		},
		{
			ID: "comp.acme.remote.lt-0211", TenantID: "acme",
			Subtype: SubtypeComputer,
			Payload: ComputerPayload{
				Hostname: "lt-0211", FQDN: "lt-0211.acme.local",
				OSFamily: "linux", OSDistribution: "Fedora", OSVersion: "41",
				Architecture: "arm64",
				CPUSummary:   "Apple M2 Pro (12 cores)",
				RAMMB:        16384, StorageMB: 512000,
			},
			Site: "Remote — EU-West", Groups: []string{"engineering"},
			EnrollmentState: EnrollmentPending,
			LifecycleState:  LifecycleActive,
			Vendor:          "Apple",
			PurchaseDate:    mustDate("2026-05-01"),
		},
		{
			ID: "comp.acme.hq.lt-old-0009", UUID: "old-…-0009", TenantID: "acme",
			Subtype: SubtypeComputer,
			Payload: ComputerPayload{
				Hostname: "lt-old-0009",
				OSFamily: "linux", OSDistribution: "Ubuntu", OSVersion: "20.04 LTS",
				Architecture: "x86_64", RAMMB: 8192, StorageMB: 256000,
			},
			Site:              "HQ — Storage",
			EnrollmentState:   EnrollmentDisabled,
			EnrolledAt:        must("2024-08-11T11:00:00Z"),
			LastSeenAt:        must("2026-04-30T17:01:00Z"),
			AgentVersion:      "0.7.1",
			LifecycleState:    LifecycleDecommissioned,
			Vendor:            "Lenovo",
			PurchaseDate:      mustDate("2024-08-01"),
			WarrantyExpiresAt: mustDate("2027-08-01"),
		},
	}
}

// ----------------------------------------------------------------------
// Servers — small population, exercises every Role.
// ----------------------------------------------------------------------

func mockServers() []Asset {
	return []Asset{
		{
			ID: "srv.acme.hq.db-01", UUID: "srv-…-db01", TenantID: "acme",
			Subtype: SubtypeServer,
			Payload: ServerPayload{
				Hostname: "db-01", FQDN: "db-01.acme.local",
				OSFamily: "linux", OSDistribution: "Debian", OSVersion: "12",
				Architecture: "x86_64",
				Services: []ServerService{
					{Name: "postgresql", Port: 5432},
					{Name: "pgbouncer", Port: 6432},
				},
				UptimeStartedAt: must("2026-04-22T03:00:00Z"),
				Role:            ServerRoleDB,
			},
			Site: "HQ — DC", Groups: []string{"production", "tier-1"},
			Labels:             map[string]string{"env": "prod", "criticality": "tier-1"},
			EnrollmentState:    EnrollmentEnrolled,
			EnrolledAt:         must("2025-11-03T08:00:00Z"),
			LastSeenAt:         must("2026-05-17T08:43:00Z"),
			AgentVersion:       "0.8.3",
			LifecycleState:     LifecycleActive,
			Vendor:             "Supermicro",
			PurchaseDate:       mustDate("2025-10-01"),
			PurchasePriceCents: 950000,
			WarrantyExpiresAt:  mustDate("2030-10-01"),
		},
		{
			ID: "srv.acme.hq.web-01", UUID: "srv-…-web01", TenantID: "acme",
			Subtype: SubtypeServer,
			Payload: ServerPayload{
				Hostname: "web-01", FQDN: "web-01.acme.local",
				OSFamily: "linux", OSDistribution: "Ubuntu", OSVersion: "24.04 LTS",
				Architecture: "x86_64",
				Services: []ServerService{
					{Name: "nginx", Port: 443},
					{Name: "nginx", Port: 80},
				},
				UptimeStartedAt: must("2026-05-12T14:20:00Z"),
				Role:            ServerRoleWeb,
			},
			Site: "HQ — DC", Groups: []string{"production"},
			EnrollmentState: EnrollmentEnrolled,
			EnrolledAt:      must("2025-09-22T09:00:00Z"),
			LastSeenAt:      must("2026-05-17T08:42:00Z"),
			AgentVersion:    "0.8.3",
			LifecycleState:  LifecycleActive,
			Vendor:          "Dell",
		},
		{
			ID: "srv.acme.hq.file-01", TenantID: "acme",
			Subtype: SubtypeServer,
			Payload: ServerPayload{
				Hostname: "file-01", FQDN: "file-01.acme.local",
				OSFamily: "linux", OSDistribution: "Ubuntu", OSVersion: "22.04 LTS",
				Architecture: "x86_64",
				Services:     []ServerService{{Name: "smbd", Port: 445}, {Name: "nfsd", Port: 2049}},
				Role:         ServerRoleFile,
			},
			Site:            "HQ — DC",
			EnrollmentState: EnrollmentApproved,
			LifecycleState:  LifecycleInRepair,
		},
		{
			ID: "srv.acme.eu.app-01", TenantID: "acme",
			Subtype: SubtypeServer,
			Payload: ServerPayload{
				Hostname: "app-01", FQDN: "app-01.acme.eu",
				OSFamily: "linux", OSDistribution: "RHEL", OSVersion: "9.4",
				Architecture: "x86_64",
				Services:     []ServerService{{Name: "podman", Port: 0}},
				Role:         ServerRoleApp,
			},
			Site:            "Remote — EU-West",
			EnrollmentState: EnrollmentEnrolled,
			EnrolledAt:      must("2026-01-15T11:00:00Z"),
			LastSeenAt:      must("2026-05-17T08:30:00Z"),
			AgentVersion:    "0.8.2",
			LifecycleState:  LifecycleActive,
		},
	}
}

// ----------------------------------------------------------------------
// Printers — exercises consumables UI and offline state.
// ----------------------------------------------------------------------

func mockPrinters() []Asset {
	return []Asset{
		{
			ID: "prn.acme.hq.fl3-mfp", TenantID: "acme",
			Subtype: SubtypePrinter,
			Payload: PrinterPayload{
				Model:            "MFC-L8900CDW",
				Vendor:           "Brother",
				IP:               "10.10.30.41",
				Queues:           []string{"hq-fl3-color", "hq-fl3-mono"},
				SupportedFormats: []string{"PCL6", "PostScript", "PDF"},
				CurrentConsumable: []PrinterConsumable{
					{Kind: "toner.black", LevelPct: 62},
					{Kind: "toner.cyan", LevelPct: 18},
					{Kind: "toner.magenta", LevelPct: 71},
					{Kind: "toner.yellow", LevelPct: 44},
					{Kind: "drum", LevelPct: 80},
				},
			},
			Site:              "HQ — Floor 3",
			EnrollmentState:   EnrollmentEnrolled,
			LastSeenAt:        must("2026-05-17T08:35:00Z"),
			LifecycleState:    LifecycleActive,
			Vendor:            "Brother",
			PurchaseDate:      mustDate("2025-06-12"),
			WarrantyExpiresAt: mustDate("2027-06-12"),
		},
		{
			ID: "prn.acme.hq.fl1-laser", TenantID: "acme",
			Subtype: SubtypePrinter,
			Payload: PrinterPayload{
				Model:            "LaserJet M404n",
				Vendor:           "HP",
				IP:               "10.10.10.18",
				Queues:           []string{"hq-fl1-mono"},
				SupportedFormats: []string{"PCL6", "PostScript"},
				CurrentConsumable: []PrinterConsumable{
					{Kind: "toner.black", LevelPct: 8},
				},
			},
			Site:            "HQ — Floor 1",
			EnrollmentState: EnrollmentEnrolled,
			LastSeenAt:      must("2026-05-17T07:14:00Z"),
			LifecycleState:  LifecycleActive,
			Vendor:          "HP",
		},
	}
}

// ----------------------------------------------------------------------
// Desks — exercises GuestProfileID and the "no agent" path.
// ----------------------------------------------------------------------

func mockDesks() []Asset {
	return []Asset{
		{
			ID: "desk.acme.hq.fl3.07", TenantID: "acme",
			Subtype: SubtypeDesk,
			Payload: DeskPayload{
				LocationLabel:  "HQ Floor 3 / Desk 07",
				DockModel:      "Lenovo ThinkPad Universal USB-C Dock",
				MonitorCount:   2,
				GuestProfileID: "profile.guest.kiosk",
			},
			Site:           "HQ — Floor 3",
			LifecycleState: LifecycleActive,
			Location:       "HQ Floor 3 / Desk 07",
		},
		{
			ID: "desk.acme.hq.fl3.08", TenantID: "acme",
			Subtype: SubtypeDesk,
			Payload: DeskPayload{
				LocationLabel: "HQ Floor 3 / Desk 08",
				DockModel:     "Dell WD19S",
				MonitorCount:  1,
			},
			Site:           "HQ — Floor 3",
			LifecycleState: LifecycleActive,
			Location:       "HQ Floor 3 / Desk 08",
		},
		{
			ID: "desk.acme.eu.hotdesk-a", TenantID: "acme",
			Subtype: SubtypeDesk,
			Payload: DeskPayload{
				LocationLabel:  "EU-West Hotdesk A",
				DockModel:      "CalDigit TS4",
				MonitorCount:   2,
				GuestProfileID: "profile.guest.hotdesk",
			},
			Site:           "Remote — EU-West",
			LifecycleState: LifecycleActive,
			Location:       "EU-West / Hotdesk A",
		},
	}
}
