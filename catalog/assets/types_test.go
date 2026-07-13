package assets

import "testing"

// ----------------------------------------------------------------------
// Subtype enum invariants — every registered Subtype must be valid;
// every Payload's Kind() must be a registered Subtype. Additions are
// detected at compile-time + here.
// ----------------------------------------------------------------------

func TestSubtypeIsValid(t *testing.T) {
	for _, s := range AllSubtypes() {
		if !s.IsValid() {
			t.Errorf("AllSubtypes() returned %q which fails IsValid()", s)
		}
		if s.Slug() != string(s) {
			t.Errorf("Subtype(%q).Slug() = %q, want %q", s, s.Slug(), s)
		}
		if s.Label() == "" || s.PluralLabel() == "" {
			t.Errorf("Subtype(%q) has empty Label or PluralLabel", s)
		}
	}
	if Subtype("nonsense").IsValid() {
		t.Errorf("unregistered subtype must not be IsValid()")
	}
}

func TestPayloadKindIsRegistered(t *testing.T) {
	cases := []SubtypePayload{
		ComputerPayload{},
		ServerPayload{},
		PrinterPayload{},
		DeskPayload{},
	}
	for _, p := range cases {
		if !p.Kind().IsValid() {
			t.Errorf("payload %T returned unregistered Kind %q", p, p.Kind())
		}
	}
}

// ----------------------------------------------------------------------
// EnrollmentState / LifecycleState predicates.
// ----------------------------------------------------------------------

func TestEnrollmentIsHealthy(t *testing.T) {
	healthy := map[EnrollmentState]bool{
		EnrollmentPending:  false,
		EnrollmentApproved: false,
		EnrollmentEnrolled: true,
		EnrollmentDisabled: false,
		EnrollmentRevoked:  false,
	}
	for s, want := range healthy {
		if got := s.IsHealthy(); got != want {
			t.Errorf("EnrollmentState(%q).IsHealthy() = %v, want %v", s, got, want)
		}
	}
}

func TestLifecycleStateIsValid(t *testing.T) {
	for _, l := range []LifecycleState{
		LifecycleActive, LifecycleInRepair,
		LifecycleDecommissioned, LifecycleDisposed,
	} {
		if !l.IsValid() {
			t.Errorf("LifecycleState(%q) must be IsValid()", l)
		}
	}
	if LifecycleState("nonsense").IsValid() {
		t.Error("unregistered LifecycleState must not be IsValid()")
	}
}

// ----------------------------------------------------------------------
// Validate — the cheap render-time invariant check. Every mock asset
// must validate; mismatched subtype/payload must fail.
// ----------------------------------------------------------------------

func TestValidateAcceptsAllMocks(t *testing.T) {
	for _, a := range AllAssets() {
		if err := a.Validate(); err != nil {
			t.Errorf("mock asset %q failed Validate: %v", a.ID, err)
		}
	}
}

func TestValidateRejectsMismatchedPayload(t *testing.T) {
	bad := Asset{
		ID:      "test.bad",
		Subtype: SubtypeComputer,
		Payload: ServerPayload{Hostname: "x"}, // kind mismatch
	}
	if err := bad.Validate(); err == nil {
		t.Error("Validate must reject a Subtype that disagrees with Payload.Kind()")
	}
}

func TestValidateRejectsMissingFields(t *testing.T) {
	for _, a := range []Asset{
		{}, // no ID
		{ID: "x", Subtype: "nonsense", Payload: ComputerPayload{}},
		{ID: "x", Subtype: SubtypeComputer}, // nil payload
		{ID: "x", Subtype: SubtypeComputer, Payload: ComputerPayload{}, EnrollmentState: "weird"},
	} {
		if err := a.Validate(); err == nil {
			t.Errorf("Validate did not catch invalid asset %+v", a)
		}
	}
}

// ----------------------------------------------------------------------
// AllOfSubtype — partition invariants. Every subtype tab must have
// at least one mock asset (so the empty state never shows in v1
// fixtures), and the union of all four partitions must equal AllAssets.
// ----------------------------------------------------------------------

func TestAllOfSubtypeIsNonEmpty(t *testing.T) {
	for _, s := range AllSubtypes() {
		got := AllOfSubtype(s)
		if len(got) == 0 {
			t.Errorf("AllOfSubtype(%q) is empty — every subtype must have at least one fixture", s)
		}
		for _, a := range got {
			if a.Subtype != s {
				t.Errorf("AllOfSubtype(%q) returned an asset with Subtype %q (id=%s)", s, a.Subtype, a.ID)
			}
		}
	}
}

func TestAllOfSubtypePartitionsAllAssets(t *testing.T) {
	total := len(AllAssets())
	sum := 0
	for _, s := range AllSubtypes() {
		sum += len(AllOfSubtype(s))
	}
	if sum != total {
		t.Errorf("sum(AllOfSubtype) = %d but len(AllAssets) = %d — subtype partitioning is leaky", sum, total)
	}
}

// ----------------------------------------------------------------------
// Coverage requirements from mock.go's selection criteria comment.
// ----------------------------------------------------------------------

func TestMocksCoverEveryEnrollmentState(t *testing.T) {
	seen := map[EnrollmentState]bool{}
	for _, a := range AllAssets() {
		seen[a.EnrollmentState] = true
	}
	for _, want := range []EnrollmentState{
		EnrollmentPending, EnrollmentApproved, EnrollmentEnrolled,
		EnrollmentDisabled,
	} {
		if !seen[want] {
			t.Errorf("no mock asset has EnrollmentState=%q — UI chip rendering is untested for this state", want)
		}
	}
	// Revoked is intentionally absent in mocks (terminal state); skipped here.
}

func TestMocksCoverDeskGuestProfile(t *testing.T) {
	for _, a := range AllOfSubtype(SubtypeDesk) {
		if p, ok := a.Payload.(DeskPayload); ok && p.GuestProfileID != "" {
			return
		}
	}
	t.Error("at least one Desk mock must set GuestProfileID — drives §III scope merge")
}
