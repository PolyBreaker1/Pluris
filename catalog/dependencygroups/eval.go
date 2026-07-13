package dependencygroups

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// paramKey returns the trailing segment of a canonical path, which is the
// entity agnostic fact key used to look up device facts.
func paramKey(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func contains(vs []string, v string) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}

// evalCondition returns "pass", "fail", or "unknown". A fact absent from
// facts is always "unknown" for param-kind conditions (the device has
// not reported it), never a false pass or fail. Script-kind conditions
// are handled separately by evalScriptCondition — see Condition.Kind's
// doc comment for the agent contract.
//
// For single-operand string/numeric/regex operators, Condition.Values[0]
// is the operand; an empty Values slice is "unknown" (nothing to compare
// against), same as a missing fact.
func evalCondition(c Condition, facts map[string]string) string {
	if c.Kind == KindScript {
		return evalScriptCondition(c, facts)
	}

	v, ok := facts[paramKey(c.ParamPath)]
	switch c.Operator {
	case OpExists:
		if !ok {
			return "unknown"
		}
		if v != "" {
			return "pass"
		}
		return "fail"
	case OpIn:
		if !ok {
			return "unknown"
		}
		if contains(c.Values, v) {
			return "pass"
		}
		return "fail"
	case OpNotIn:
		if !ok {
			return "unknown"
		}
		if !contains(c.Values, v) {
			return "pass"
		}
		return "fail"
	case OpEquals, OpNotEquals, OpContains, OpNotContains, OpStartsWith, OpEndsWith:
		if !ok || len(c.Values) == 0 {
			return "unknown"
		}
		operand := c.Values[0]
		var match bool
		switch c.Operator {
		case OpEquals:
			match = v == operand
		case OpNotEquals:
			match = v != operand
		case OpContains:
			match = strings.Contains(v, operand)
		case OpNotContains:
			match = !strings.Contains(v, operand)
		case OpStartsWith:
			match = strings.HasPrefix(v, operand)
		case OpEndsWith:
			match = strings.HasSuffix(v, operand)
		}
		if match {
			return "pass"
		}
		return "fail"
	case OpGT, OpGTE, OpLT, OpLTE:
		if !ok || len(c.Values) == 0 {
			return "unknown"
		}
		fv, err1 := strconv.ParseFloat(v, 64)
		ov, err2 := strconv.ParseFloat(c.Values[0], 64)
		if err1 != nil || err2 != nil {
			return "unknown"
		}
		var match bool
		switch c.Operator {
		case OpGT:
			match = fv > ov
		case OpGTE:
			match = fv >= ov
		case OpLT:
			match = fv < ov
		case OpLTE:
			match = fv <= ov
		}
		if match {
			return "pass"
		}
		return "fail"
	case OpMatches:
		// An empty pattern is treated like a compile error: it would
		// otherwise compile fine and match everything, which is never
		// what a condition author meant — so it's "unknown", not a
		// silent always-pass.
		if !ok || len(c.Values) == 0 || c.Values[0] == "" {
			return "unknown"
		}
		re, err := regexp.Compile(c.Values[0])
		if err != nil {
			return "unknown"
		}
		if re.MatchString(v) {
			return "pass"
		}
		return "fail"
	}
	return "unknown"
}

// evalScriptCondition looks up the agent-reported result of a script
// condition. The agent contract: a fact keyed "script_result/<ID>" (ID is
// the condition's database row id) whose value is "pass" or "fail". Any
// other value, or the key being absent entirely (the agent hasn't run it
// yet, or never will for a device it doesn't apply to), is "unknown" —
// never a false pass or fail. Running the script itself is out of scope
// here; this package only interprets the reported result.
func evalScriptCondition(c Condition, facts map[string]string) string {
	v, ok := facts[fmt.Sprintf("script_result/%d", c.ID)]
	if !ok {
		return "unknown"
	}
	switch v {
	case "pass":
		return "pass"
	case "fail":
		return "fail"
	}
	return "unknown"
}

// evalGroup combines a group's conditions per its MatchMode. The zero
// value ("") behaves like MatchAll, so existing in-memory Groups built
// without setting MatchMode (e.g. pre-Task-2.1 callers/tests) keep the
// original AND semantics byte-for-byte.
func evalGroup(g Group, facts map[string]string) string {
	if g.MatchMode == MatchAny {
		return evalGroupAny(g, facts)
	}
	return evalGroupAll(g, facts)
}

// EvalGroup is the exported entry point for evaluating a single Group's
// conditions against a fact map, returning "pass" | "fail" | "unknown"
// (the same verdict values Eligible's GroupResult.Pass uses). It is the
// same engine Eligible uses internally (evalGroup) — exported so other
// callers that need a bare group verdict without the module-link/role
// framing Eligible imposes (e.g. dynamic group membership rules, which
// reuse this exact condition machinery per pkg/services/groups.go's
// EvaluateDynamicMembership) don't have to reimplement match-mode
// combination logic.
func EvalGroup(g Group, facts map[string]string) string {
	return evalGroup(g, facts)
}

// evalGroupAll ANDs a group's conditions. A definitive fail dominates an
// unknown; all-pass (including the vacuous zero-conditions case) is
// pass; otherwise unknown.
func evalGroupAll(g Group, facts map[string]string) string {
	verdict := "pass"
	for _, c := range g.Conditions {
		switch evalCondition(c, facts) {
		case "fail":
			return "fail"
		case "unknown":
			verdict = "unknown"
		}
	}
	return verdict
}

// evalGroupAny ORs a group's conditions. A definitive pass dominates an
// unknown; otherwise any unknown yields unknown; all-fail (including the
// vacuous zero-conditions case) is fail.
func evalGroupAny(g Group, facts map[string]string) string {
	sawUnknown := false
	for _, c := range g.Conditions {
		switch evalCondition(c, facts) {
		case "pass":
			return "pass"
		case "unknown":
			sawUnknown = true
		}
	}
	if sawUnknown {
		return "unknown"
	}
	return "fail"
}

// Eligible evaluates a module's dependency links against device facts.
// Platform links: pass if ANY passes (none linked = agnostic pass).
// Requirement links: pass only if ALL pass. Overall: ineligible if either
// aggregate fails, eligible if both pass, otherwise unknown.
func Eligible(links []ModuleLink, groups map[int64]Group, facts map[string]string) Result {
	var res Result
	platAny, platUnknown, platCount := false, false, 0
	reqFail, reqUnknown, reqCount := false, false, 0

	for _, l := range links {
		g, ok := groups[l.GroupID]
		if !ok {
			continue
		}
		v := evalGroup(g, facts)
		gr := GroupResult{GroupID: g.ID, Slug: g.Slug, Name: g.Name, Role: l.Role, Pass: v, Reason: reasonFor(g, v)}
		switch l.Role {
		case RolePlatform:
			platCount++
			res.Platforms = append(res.Platforms, gr)
			if v == "pass" {
				platAny = true
			} else if v == "unknown" {
				platUnknown = true
			}
		case RoleRequirement:
			reqCount++
			res.Requirements = append(res.Requirements, gr)
			if v == "fail" {
				reqFail = true
			} else if v == "unknown" {
				reqUnknown = true
			}
		}
	}

	platOK := "pass"
	if platCount > 0 {
		switch {
		case platAny:
			platOK = "pass"
		case platUnknown:
			platOK = "unknown"
		default:
			platOK = "fail"
		}
	}
	reqOK := "pass"
	if reqCount > 0 {
		switch {
		case reqFail:
			reqOK = "fail"
		case reqUnknown:
			reqOK = "unknown"
		default:
			reqOK = "pass"
		}
	}

	switch {
	case platOK == "fail" || reqOK == "fail":
		res.Status = StatusIneligible
	case platOK == "pass" && reqOK == "pass":
		res.Status = StatusEligible
	default:
		res.Status = StatusUnknown
	}
	return res
}

func reasonFor(g Group, v string) string {
	switch v {
	case "pass":
		return g.Name + " matched"
	case "fail":
		return g.Name + " did not match"
	default:
		return g.Name + " needs agent inventory"
	}
}
