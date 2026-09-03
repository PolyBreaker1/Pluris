package dependencygroups

import "testing"

// TestEvalConditionOperators is a table-driven sweep over every operator
// AllOperators() lists, covering string, numeric, missing-fact, and
// unparseable-value cases (Task 2.1).
func TestEvalConditionOperators(t *testing.T) {
	cases := []struct {
		name  string
		cond  Condition
		facts map[string]string
		want  string
	}{
		// --- existing operators (byte-compatible sanity, not just via eval_test.go) ---
		{"in-pass", Condition{ParamPath: "x/k", Operator: OpIn, Values: []string{"a", "b"}}, map[string]string{"k": "a"}, "pass"},
		{"in-fail", Condition{ParamPath: "x/k", Operator: OpIn, Values: []string{"a", "b"}}, map[string]string{"k": "c"}, "fail"},
		{"in-missing", Condition{ParamPath: "x/k", Operator: OpIn, Values: []string{"a"}}, map[string]string{}, "unknown"},
		{"not_in-pass", Condition{ParamPath: "x/k", Operator: OpNotIn, Values: []string{"a"}}, map[string]string{"k": "c"}, "pass"},
		{"not_in-fail", Condition{ParamPath: "x/k", Operator: OpNotIn, Values: []string{"a"}}, map[string]string{"k": "a"}, "fail"},
		{"not_in-missing", Condition{ParamPath: "x/k", Operator: OpNotIn, Values: []string{"a"}}, map[string]string{}, "unknown"},
		{"exists-pass", Condition{ParamPath: "x/k", Operator: OpExists}, map[string]string{"k": "v"}, "pass"},
		{"exists-fail-empty", Condition{ParamPath: "x/k", Operator: OpExists}, map[string]string{"k": ""}, "fail"},
		{"exists-missing", Condition{ParamPath: "x/k", Operator: OpExists}, map[string]string{}, "unknown"},

		// --- string operators ---
		{"equals-pass", Condition{ParamPath: "x/k", Operator: OpEquals, Values: []string{"abc"}}, map[string]string{"k": "abc"}, "pass"},
		{"equals-fail", Condition{ParamPath: "x/k", Operator: OpEquals, Values: []string{"abc"}}, map[string]string{"k": "abd"}, "fail"},
		{"equals-case-sensitive", Condition{ParamPath: "x/k", Operator: OpEquals, Values: []string{"ABC"}}, map[string]string{"k": "abc"}, "fail"},
		{"equals-missing", Condition{ParamPath: "x/k", Operator: OpEquals, Values: []string{"abc"}}, map[string]string{}, "unknown"},
		{"equals-no-operand", Condition{ParamPath: "x/k", Operator: OpEquals}, map[string]string{"k": "abc"}, "unknown"},

		{"not_equals-pass", Condition{ParamPath: "x/k", Operator: OpNotEquals, Values: []string{"abc"}}, map[string]string{"k": "xyz"}, "pass"},
		{"not_equals-fail", Condition{ParamPath: "x/k", Operator: OpNotEquals, Values: []string{"abc"}}, map[string]string{"k": "abc"}, "fail"},
		{"not_equals-missing", Condition{ParamPath: "x/k", Operator: OpNotEquals, Values: []string{"abc"}}, map[string]string{}, "unknown"},

		{"contains-pass", Condition{ParamPath: "x/k", Operator: OpContains, Values: []string{"bc"}}, map[string]string{"k": "abcd"}, "pass"},
		{"contains-fail", Condition{ParamPath: "x/k", Operator: OpContains, Values: []string{"zz"}}, map[string]string{"k": "abcd"}, "fail"},
		{"contains-missing", Condition{ParamPath: "x/k", Operator: OpContains, Values: []string{"bc"}}, map[string]string{}, "unknown"},

		{"not_contains-pass", Condition{ParamPath: "x/k", Operator: OpNotContains, Values: []string{"zz"}}, map[string]string{"k": "abcd"}, "pass"},
		{"not_contains-fail", Condition{ParamPath: "x/k", Operator: OpNotContains, Values: []string{"bc"}}, map[string]string{"k": "abcd"}, "fail"},

		{"starts_with-pass", Condition{ParamPath: "x/k", Operator: OpStartsWith, Values: []string{"ab"}}, map[string]string{"k": "abcd"}, "pass"},
		{"starts_with-fail", Condition{ParamPath: "x/k", Operator: OpStartsWith, Values: []string{"cd"}}, map[string]string{"k": "abcd"}, "fail"},
		{"starts_with-missing", Condition{ParamPath: "x/k", Operator: OpStartsWith, Values: []string{"ab"}}, map[string]string{}, "unknown"},

		{"ends_with-pass", Condition{ParamPath: "x/k", Operator: OpEndsWith, Values: []string{"cd"}}, map[string]string{"k": "abcd"}, "pass"},
		{"ends_with-fail", Condition{ParamPath: "x/k", Operator: OpEndsWith, Values: []string{"ab"}}, map[string]string{"k": "abcd"}, "fail"},
		{"ends_with-missing", Condition{ParamPath: "x/k", Operator: OpEndsWith, Values: []string{"cd"}}, map[string]string{}, "unknown"},

		// --- numeric operators ---
		{"gt-pass", Condition{ParamPath: "x/k", Operator: OpGT, Values: []string{"5"}}, map[string]string{"k": "10"}, "pass"},
		{"gt-fail", Condition{ParamPath: "x/k", Operator: OpGT, Values: []string{"5"}}, map[string]string{"k": "3"}, "fail"},
		{"gt-equal-fails", Condition{ParamPath: "x/k", Operator: OpGT, Values: []string{"5"}}, map[string]string{"k": "5"}, "fail"},
		{"gt-unparseable-fact", Condition{ParamPath: "x/k", Operator: OpGT, Values: []string{"5"}}, map[string]string{"k": "notanumber"}, "unknown"},
		{"gt-unparseable-operand", Condition{ParamPath: "x/k", Operator: OpGT, Values: []string{"notanumber"}}, map[string]string{"k": "10"}, "unknown"},
		{"gt-missing", Condition{ParamPath: "x/k", Operator: OpGT, Values: []string{"5"}}, map[string]string{}, "unknown"},

		{"gte-pass-equal", Condition{ParamPath: "x/k", Operator: OpGTE, Values: []string{"5"}}, map[string]string{"k": "5"}, "pass"},
		{"gte-pass-greater", Condition{ParamPath: "x/k", Operator: OpGTE, Values: []string{"5"}}, map[string]string{"k": "6"}, "pass"},
		{"gte-fail", Condition{ParamPath: "x/k", Operator: OpGTE, Values: []string{"5"}}, map[string]string{"k": "4"}, "fail"},

		{"lt-pass", Condition{ParamPath: "x/k", Operator: OpLT, Values: []string{"5"}}, map[string]string{"k": "3"}, "pass"},
		{"lt-fail", Condition{ParamPath: "x/k", Operator: OpLT, Values: []string{"5"}}, map[string]string{"k": "10"}, "fail"},
		{"lt-equal-fails", Condition{ParamPath: "x/k", Operator: OpLT, Values: []string{"5"}}, map[string]string{"k": "5"}, "fail"},

		{"lte-pass-equal", Condition{ParamPath: "x/k", Operator: OpLTE, Values: []string{"5"}}, map[string]string{"k": "5"}, "pass"},
		{"lte-pass-less", Condition{ParamPath: "x/k", Operator: OpLTE, Values: []string{"5"}}, map[string]string{"k": "3"}, "pass"},
		{"lte-fail", Condition{ParamPath: "x/k", Operator: OpLTE, Values: []string{"5"}}, map[string]string{"k": "10"}, "fail"},

		// float parsing
		{"gt-float-pass", Condition{ParamPath: "x/k", Operator: OpGT, Values: []string{"1.5"}}, map[string]string{"k": "2.75"}, "pass"},

		// --- regex ---
		{"matches-pass", Condition{ParamPath: "x/k", Operator: OpMatches, Values: []string{"^ab.*d$"}}, map[string]string{"k": "abcd"}, "pass"},
		{"matches-fail", Condition{ParamPath: "x/k", Operator: OpMatches, Values: []string{"^zz"}}, map[string]string{"k": "abcd"}, "fail"},
		{"matches-invalid-regex", Condition{ParamPath: "x/k", Operator: OpMatches, Values: []string{"("}}, map[string]string{"k": "abcd"}, "unknown"},
		{"matches-empty-pattern-is-unknown", Condition{ParamPath: "x/k", Operator: OpMatches, Values: []string{""}}, map[string]string{"k": "abcd"}, "unknown"},
		{"matches-missing", Condition{ParamPath: "x/k", Operator: OpMatches, Values: []string{"a"}}, map[string]string{}, "unknown"},
		{"matches-no-operand", Condition{ParamPath: "x/k", Operator: OpMatches}, map[string]string{"k": "abcd"}, "unknown"},

		// --- unknown operator (defensive) ---
		{"unknown-operator", Condition{ParamPath: "x/k", Operator: "bogus"}, map[string]string{"k": "v"}, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := evalCondition(tc.cond, tc.facts)
			if got != tc.want {
				t.Fatalf("evalCondition(%+v, %v) = %q, want %q", tc.cond, tc.facts, got, tc.want)
			}
		})
	}
}

// TestAllOperatorsHaveLabels guards against silently adding an operator
// constant without a matching Label() switch case: every operator
// AllOperators() lists must have an explicit (non-empty, human-readable)
// label — "contains" happens to be spelled the same as its own key, so
// this checks non-emptiness rather than inequality with the raw key.
func TestAllOperatorsHaveLabels(t *testing.T) {
	seen := map[string]bool{}
	for _, op := range AllOperators() {
		if op.Label() == "" {
			t.Errorf("operator %q has an empty Label()", op)
		}
		seen[string(op)] = true
	}
	// Every operator the eval engine's evalCondition switch handles by
	// name must also appear in AllOperators(), keeping the whitelist
	// (used by AddCondition/handler validation) from silently drifting
	// narrower than what evalCondition actually implements.
	for _, op := range []Operator{OpIn, OpNotIn, OpExists, OpEquals, OpNotEquals, OpContains, OpNotContains, OpStartsWith, OpEndsWith, OpGT, OpGTE, OpLT, OpLTE, OpMatches} {
		if !seen[string(op)] {
			t.Errorf("operator %q implemented by evalCondition but missing from AllOperators()", op)
		}
	}
}

// --- script conditions ---

func TestEvalScriptConditionUnknownWithoutResult(t *testing.T) {
	c := Condition{ID: 42, Kind: KindScript, ScriptSource: "exit 0", Operator: OpExists}
	got := evalCondition(c, map[string]string{})
	if got != "unknown" {
		t.Fatalf("want unknown without script_result fact, got %s", got)
	}
}

func TestEvalScriptConditionStdoutOperatorPass(t *testing.T) {
	c := Condition{ID: 42, Kind: KindScript, Operator: OpContains, Values: []string{"3"}}
	got := evalCondition(c, map[string]string{"script_result/42": "6.3.0-generic"})
	if got != "pass" {
		t.Fatalf("want pass, got %s", got)
	}
}

func TestEvalScriptConditionStdoutOperatorFail(t *testing.T) {
	c := Condition{ID: 42, Kind: KindScript, Operator: OpEquals, Values: []string{"expected"}}
	got := evalCondition(c, map[string]string{"script_result/42": "actual"})
	if got != "fail" {
		t.Fatalf("want fail, got %s", got)
	}
}

func TestEvalScriptConditionExitFailSentinel(t *testing.T) {
	c := Condition{ID: 42, Kind: KindScript, Operator: OpContains, Values: []string{"anything"}}
	got := evalCondition(c, map[string]string{"script_result/42": ExitFailSentinel})
	if got != "fail" {
		t.Fatalf("want fail on non-zero exit regardless of operator, got %s", got)
	}
}

func TestEvalCommandConditionSameContract(t *testing.T) {
	c := Condition{ID: 7, Kind: KindCommand, ScriptSource: "uname -r", Operator: OpContains, Values: []string{"3"}}
	if got := evalCondition(c, map[string]string{"script_result/7": "3.10.0"}); got != "pass" {
		t.Fatalf("command pass: got %s", got)
	}
	if got := evalCondition(c, map[string]string{}); got != "unknown" {
		t.Fatalf("command unreported: want unknown, got %s", got)
	}
}

func TestEvalScriptConditionKeyedByID(t *testing.T) {
	// A script_result fact for a different condition ID must not leak in.
	c := Condition{ID: 1, Kind: KindScript, Operator: OpExists}
	got := evalCondition(c, map[string]string{"script_result/2": "output"})
	if got != "unknown" {
		t.Fatalf("want unknown when only a different condition's script_result is present, got %s", got)
	}
}

// --- match_mode precedence matrix ---
//
// These use script-kind conditions as a convenient way to force a
// specific evalCondition verdict (pass/fail/unknown) per synthetic
// condition ID, independent of the param-path/operator machinery
// exercised above.

func TestMatchModeAllPrecedence(t *testing.T) {
	pass := Condition{ID: 1, Kind: KindScript, Operator: OpExists}
	fail := Condition{ID: 2, Kind: KindScript, Operator: OpExists}
	unknown := Condition{ID: 3, Kind: KindScript, Operator: OpExists}
	facts := map[string]string{"script_result/1": "output", "script_result/2": ExitFailSentinel}

	cases := []struct {
		name  string
		conds []Condition
		want  string
	}{
		{"all-pass", []Condition{pass, pass}, "pass"},
		{"fail-dominates-unknown", []Condition{fail, unknown}, "fail"},
		{"pass-and-unknown-is-unknown", []Condition{pass, unknown}, "unknown"},
		{"single-fail", []Condition{fail}, "fail"},
		{"vacuous-pass", []Condition{}, "pass"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := Group{MatchMode: MatchAll, Conditions: tc.conds}
			got := evalGroup(g, facts)
			if got != tc.want {
				t.Fatalf("evalGroup(all, %v) = %q, want %q", tc.conds, got, tc.want)
			}
		})
	}
}

func TestMatchModeAnyPrecedence(t *testing.T) {
	pass := Condition{ID: 1, Kind: KindScript, Operator: OpExists}
	fail := Condition{ID: 2, Kind: KindScript, Operator: OpExists}
	unknown := Condition{ID: 3, Kind: KindScript, Operator: OpExists}
	facts := map[string]string{"script_result/1": "output", "script_result/2": ExitFailSentinel}

	cases := []struct {
		name  string
		conds []Condition
		want  string
	}{
		{"any-pass-dominates", []Condition{fail, pass, unknown}, "pass"},
		{"unknown-dominates-fail", []Condition{fail, unknown}, "unknown"},
		{"all-fail", []Condition{fail, fail}, "fail"},
		{"single-pass", []Condition{pass}, "pass"},
		{"vacuous-fail", []Condition{}, "fail"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := Group{MatchMode: MatchAny, Conditions: tc.conds}
			got := evalGroup(g, facts)
			if got != tc.want {
				t.Fatalf("evalGroup(any, %v) = %q, want %q", tc.conds, got, tc.want)
			}
		})
	}
}

// TestMatchModeZeroValueBehavesAsAll guards the compatibility contract:
// a Group with MatchMode unset (as every pre-Task-2.1 in-memory Group
// literal is) must behave exactly like MatchAll.
func TestMatchModeZeroValueBehavesAsAll(t *testing.T) {
	fail := Condition{ID: 2, Kind: KindScript, Operator: OpExists}
	facts := map[string]string{"script_result/2": ExitFailSentinel}
	g := Group{Conditions: []Condition{fail}} // MatchMode zero value
	if got := evalGroup(g, facts); got != "fail" {
		t.Fatalf("zero-value MatchMode: want fail (same as explicit all), got %s", got)
	}
}
