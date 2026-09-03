package lists

import "testing"

func TestPolicyModuleMassActions(t *testing.T) {
	actions := MassActionsFor(ListIDPolicyModules)
	if len(actions) != 5 {
		t.Fatalf("got %d actions, want 5", len(actions))
	}
	for i, want := range []string{"duplicate", "revoke", "delete", "restore", "purge"} {
		if actions[i].Key != want {
			t.Errorf("action %d key = %q, want %q", i, actions[i].Key, want)
		}
		if actions[i].URL != "/api/modules/bulk" {
			t.Errorf("action %q URL = %q", actions[i].Key, actions[i].URL)
		}
	}
	if !actions[1].Danger {
		t.Error("revoke action must be dangerous")
	}
	if !actions[2].Danger {
		t.Error("delete action must be dangerous")
	}
}

func TestMassActionsForReturnsCopy(t *testing.T) {
	actions := MassActionsFor(ListIDPolicyModules)
	actions[0].Label = "changed"
	if got := MassActionsFor(ListIDPolicyModules)[0].Label; got != "Duplicate" {
		t.Fatalf("registry was mutated through returned slice: %q", got)
	}
}
