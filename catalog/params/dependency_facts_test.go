package params_test

import (
	"testing"

	"github.com/pluris/pluris/catalog/params"
)

func TestDependencyFactsRegistered(t *testing.T) {
	for _, key := range []string{"os_package_family", "disk_encryption"} {
		d := params.DefByKey(key)
		if d == nil {
			t.Fatalf("param %q not registered", key)
		}
		if len(d.EnumValues) == 0 {
			t.Fatalf("param %q has no enum values", key)
		}
	}
	if got := params.PathFor("computer", "os_package_family"); got != "computer/hardware/os_package_family" {
		t.Fatalf("computer os_package_family path = %q", got)
	}
	if got := params.PathFor("computer", "disk_encryption"); got != "computer/hardware/disk_encryption" {
		t.Fatalf("computer disk_encryption path = %q", got)
	}
	if got := params.PathFor("server", "disk_encryption"); got != "server/hardware/disk_encryption" {
		t.Fatalf("server disk_encryption path = %q", got)
	}
}
