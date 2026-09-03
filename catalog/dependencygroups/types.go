// Package dependencygroups holds the pure applicability model for policy
// modules: a dependency group is a set of conditions over device fact
// keys (or agent-executed scripts), combined with an all/any match mode,
// and a module links to groups in a platform (match any) or requirement
// (match all) role. Persistence lives in pkg/services; this package is
// deliberately dependency free and unit tested in isolation. The same
// eval engine (eval.go) is intended to later power dynamic group
// membership too, so it stays pure — no DB imports, no side effects.
package dependencygroups

type Operator string

const (
	OpIn     Operator = "in"
	OpNotIn  Operator = "not_in"
	OpExists Operator = "exists"

	OpEquals      Operator = "equals"
	OpNotEquals   Operator = "not_equals"
	OpContains    Operator = "contains"
	OpNotContains Operator = "not_contains"
	OpStartsWith  Operator = "starts_with"
	OpEndsWith    Operator = "ends_with"
	OpGT          Operator = "gt"
	OpGTE         Operator = "gte"
	OpLT          Operator = "lt"
	OpLTE         Operator = "lte"
	OpMatches     Operator = "matches"
)

// Label returns the human-readable dropdown text for an operator, or the
// raw key for anything outside the supported enum (defensive default —
// AddCondition validation rejects those before they'd ever reach here).
func (o Operator) Label() string {
	switch o {
	case OpIn:
		return "is any of"
	case OpNotIn:
		return "is none of"
	case OpExists:
		return "is set"
	case OpEquals:
		return "is exactly"
	case OpNotEquals:
		return "is not"
	case OpContains:
		return "contains"
	case OpNotContains:
		return "does not contain"
	case OpStartsWith:
		return "starts with"
	case OpEndsWith:
		return "ends with"
	case OpGT:
		return "greater than"
	case OpGTE:
		return "greater than or equal"
	case OpLT:
		return "less than"
	case OpLTE:
		return "less than or equal"
	case OpMatches:
		return "matches regex"
	}
	return string(o)
}

// AllOperators lists every operator the dependency-group eval engine
// (Eligible, in eval.go) actually knows how to execute. Task 2.1 widened
// this from the original in/not_in/exists to the full string/numeric/
// regex set below; the condition-add UI sources its operator options
// from here (not from catalog/params) so this list stays the single
// source of truth for what a param-kind condition may use.
//
// Key alignment: equals, not_equals, contains, not_contains, starts_with,
// ends_with, gt, gte, lt, lte are spelled exactly like catalog/params/
// operators.go's opsString/opsNumeric keys for the same concepts — that
// package's keys won by construction here. "matches" (regex) has no
// equivalent in catalog/params today; it's new with this task.
func AllOperators() []Operator {
	return []Operator{
		OpIn, OpNotIn, OpExists,
		OpEquals, OpNotEquals,
		OpContains, OpNotContains,
		OpStartsWith, OpEndsWith,
		OpGT, OpGTE, OpLT, OpLTE,
		OpMatches,
	}
}

type Role string

const (
	RolePlatform    Role = "platform"
	RoleRequirement Role = "requirement"
)

type Status string

const (
	StatusEligible   Status = "eligible"
	StatusIneligible Status = "ineligible"
	StatusUnknown    Status = "unknown"
)

// ConditionKind discriminates a condition's evaluation strategy.
type ConditionKind string

const (
	// KindParam evaluates Operator/Values against a device fact keyed by
	// ParamPath's trailing segment (the original, and default, kind).
	KindParam ConditionKind = "param"
	// KindScript evaluates Operator/Values against the stdout an agent
	// reports for the referenced (ScriptRef) or inline (ScriptSource)
	// script. See Condition.Kind's doc comment for the agent contract.
	KindScript ConditionKind = "script"
	// KindCommand evaluates Operator/Values against the stdout an agent
	// reports for the one-line shell command in ScriptSource. Same fact
	// contract as KindScript.
	KindCommand ConditionKind = "command"
)

// Condition is one predicate. ParamPath is a full canonical path (for
// display and interconnection); matching uses only its trailing key.
//
// Kind selects the evaluation strategy:
//
//   - "param" (default, KindParam): Operator/Values are evaluated against
//     the device fact keyed by ParamPath's trailing segment, as before.
//   - "script" (KindScript) and "command" (KindCommand): ScriptSource
//     (inline source or a one-line command) or ScriptRef (a library
//     script id, script kind only) describe something an agent runs out
//     of band. This package does not run it — evalCondition looks up a
//     fact named "script_result/<ID>" (ID is this condition's database
//     id) holding the run's REPORTED STDOUT (trimmed of one trailing
//     newline), and applies Operator/Values to that stdout exactly like
//     a param-kind condition applies them to a device fact. The agent
//     reports a failed run (non-zero exit) as the sentinel fact value
//     "\x00exit_fail", which evaluates to "fail" regardless of operator.
//     An absent fact is "unknown" — never a false pass or fail.
//     ScriptExpect is the pre-011 expectation format: dead, retained
//     only for column parity; nothing writes or evaluates it.
type Condition struct {
	// ID is the condition's database row id. It is zero for conditions
	// built in memory (e.g. tests) that were never persisted; only
	// script/command-kind conditions need it, to key their
	// "script_result/<ID>" fact lookup.
	ID           int64
	ParamPath    string
	Operator     Operator
	Values       []string
	Kind         ConditionKind
	ScriptSource string
	ScriptRef    string
	ScriptExpect string
}

// ExitFailSentinel is the fact value an agent reports for a
// script/command condition whose run exited non-zero: the run happened,
// so the verdict is a definitive "fail", but there is no stdout worth
// comparing. NUL-prefixed so no real stdout can collide with it.
const ExitFailSentinel = "\x00exit_fail"

// MatchMode controls how a group's conditions combine.
type MatchMode string

const (
	// MatchAll is the original AND semantics: a single fail dominates;
	// otherwise any unknown yields unknown; all-pass yields pass.
	MatchAll MatchMode = "all"
	// MatchAny is OR semantics: any single pass yields pass; otherwise
	// any unknown yields unknown; all-fail yields fail.
	MatchAny MatchMode = "any"
)

type Group struct {
	ID          int64
	Slug        string
	Name        string
	Description string
	Builtin     bool
	MatchMode   MatchMode
	Conditions  []Condition
}

type ModuleLink struct {
	GroupID int64
	Role    Role
}

// GroupResult is one group's verdict; Pass is "pass" | "fail" | "unknown".
type GroupResult struct {
	GroupID int64
	Slug    string
	Name    string
	Role    Role
	Pass    string
	Reason  string
}

type Result struct {
	Status       Status
	Platforms    []GroupResult
	Requirements []GroupResult
}
