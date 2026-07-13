package dependencygroups

import "testing"

func groups() map[int64]Group {
	return map[int64]Group{
		1: {ID: 1, Slug: "any-linux", Name: "Any Linux", Conditions: []Condition{{ParamPath: "computer/hardware/os_family", Operator: OpIn, Values: []string{"linux"}}}},
		2: {ID: 2, Slug: "windows", Name: "Windows", Conditions: []Condition{{ParamPath: "computer/hardware/os_family", Operator: OpIn, Values: []string{"windows"}}}},
		3: {ID: 3, Slug: "rpm-based", Name: "RPM-based OS", Conditions: []Condition{{ParamPath: "computer/hardware/os_package_family", Operator: OpIn, Values: []string{"rpm"}}}},
		4: {ID: 4, Slug: "debian-based", Name: "Debian-based OS", Conditions: []Condition{{ParamPath: "computer/hardware/os_package_family", Operator: OpIn, Values: []string{"deb"}}}},
		5: {ID: 5, Slug: "disk-enc", Name: "Disk encryption active", Conditions: []Condition{{ParamPath: "computer/hardware/disk_encryption", Operator: OpNotIn, Values: []string{"none"}}}},
	}
}

func TestEligibleAgnosticNoPlatform(t *testing.T) {
	r := Eligible(nil, groups(), map[string]string{"os_family": "linux"})
	if r.Status != StatusEligible {
		t.Fatalf("want eligible, got %s", r.Status)
	}
}

func TestPlatformAnyPasses(t *testing.T) {
	links := []ModuleLink{{GroupID: 3, Role: RolePlatform}, {GroupID: 4, Role: RolePlatform}}
	r := Eligible(links, groups(), map[string]string{"os_package_family": "deb"})
	if r.Status != StatusEligible {
		t.Fatalf("want eligible (debian matches), got %s", r.Status)
	}
}

func TestPlatformFailsIneligible(t *testing.T) {
	links := []ModuleLink{{GroupID: 2, Role: RolePlatform}}
	r := Eligible(links, groups(), map[string]string{"os_family": "linux"})
	if r.Status != StatusIneligible {
		t.Fatalf("want ineligible (windows-only on linux), got %s", r.Status)
	}
}

func TestRequirementAllOneFails(t *testing.T) {
	links := []ModuleLink{{GroupID: 1, Role: RolePlatform}, {GroupID: 5, Role: RoleRequirement}}
	r := Eligible(links, groups(), map[string]string{"os_family": "linux", "disk_encryption": "none"})
	if r.Status != StatusIneligible {
		t.Fatalf("want ineligible (encryption none), got %s", r.Status)
	}
}

func TestUnknownFact(t *testing.T) {
	links := []ModuleLink{{GroupID: 5, Role: RoleRequirement}}
	r := Eligible(links, groups(), map[string]string{"os_family": "linux"}) // no disk_encryption fact
	if r.Status != StatusUnknown {
		t.Fatalf("want unknown (no encryption fact), got %s", r.Status)
	}
}
