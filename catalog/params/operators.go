package params

// This file defines the operators available for each parameter type.
// The filter builder UI reads these to populate the operator dropdown
// dynamically based on which parameter the user selects.
//
// Design mirrors ITSM tools (GLPI, Lansweeper, ManageEngine):
//   - Operators adapt to field type (no "greater than" on text fields)
//   - Each operator has a human label and a machine key
//   - The system knows which operators are valid for which types

// Operator is a single filter comparison operation.
type Operator struct {
	// Key is the stable machine identifier (used in URLs, localStorage, APIs).
	Key string

	// Label is the human display text shown in the operator dropdown.
	Label string

	// NeedsValue reports whether this operator requires a user-supplied value.
	// Operators like "is empty" or "is not empty" don't.
	NeedsValue bool
}

// OperatorsForType returns the valid operators for a given ParamType.
// Order matters — it's the dropdown display order (most common first).
func OperatorsForType(t ParamType) []Operator {
	switch t {
	case TypeString:
		return opsString
	case TypeInt, TypeFloat:
		return opsNumeric
	case TypeEnum:
		return opsEnum
	case TypeBool:
		return opsBool
	case TypeTime, TypeDate:
		return opsDate
	case TypeList:
		return opsList
	case TypeMap:
		return opsMap
	case TypeLink:
		return opsLink
	case TypeCompound:
		return opsCompound
	}
	return opsString // fallback
}

// OperatorsForParam returns the valid operators for a specific parameter definition.
// This is the preferred entry point — it uses the param's Type.
func OperatorsForParam(def ParamDef) []Operator {
	return OperatorsForType(def.Type)
}

// ---------------------------------------------------------------------------
// Operator sets per type
// ---------------------------------------------------------------------------

var opsString = []Operator{
	{Key: "contains", Label: "contains", NeedsValue: true},
	{Key: "not_contains", Label: "does not contain", NeedsValue: true},
	{Key: "equals", Label: "is exactly", NeedsValue: true},
	{Key: "not_equals", Label: "is not", NeedsValue: true},
	{Key: "starts_with", Label: "starts with", NeedsValue: true},
	{Key: "ends_with", Label: "ends with", NeedsValue: true},
	{Key: "is_empty", Label: "is empty", NeedsValue: false},
	{Key: "is_not_empty", Label: "is not empty", NeedsValue: false},
}

var opsNumeric = []Operator{
	{Key: "equals", Label: "equals", NeedsValue: true},
	{Key: "not_equals", Label: "does not equal", NeedsValue: true},
	{Key: "gt", Label: "greater than", NeedsValue: true},
	{Key: "gte", Label: "greater than or equal", NeedsValue: true},
	{Key: "lt", Label: "less than", NeedsValue: true},
	{Key: "lte", Label: "less than or equal", NeedsValue: true},
	{Key: "between", Label: "between", NeedsValue: true},
	{Key: "is_empty", Label: "is empty", NeedsValue: false},
	{Key: "is_not_empty", Label: "is not empty", NeedsValue: false},
}

var opsEnum = []Operator{
	{Key: "equals", Label: "is", NeedsValue: true},
	{Key: "not_equals", Label: "is not", NeedsValue: true},
	{Key: "in", Label: "is any of", NeedsValue: true},
	{Key: "not_in", Label: "is none of", NeedsValue: true},
	{Key: "is_empty", Label: "is empty", NeedsValue: false},
	{Key: "is_not_empty", Label: "is not empty", NeedsValue: false},
}

var opsBool = []Operator{
	{Key: "equals", Label: "is", NeedsValue: true},
}

var opsDate = []Operator{
	{Key: "equals", Label: "is on", NeedsValue: true},
	{Key: "before", Label: "is before", NeedsValue: true},
	{Key: "after", Label: "is after", NeedsValue: true},
	{Key: "between", Label: "is between", NeedsValue: true},
	{Key: "last_n_days", Label: "in the last N days", NeedsValue: true},
	{Key: "more_than_n_days", Label: "more than N days ago", NeedsValue: true},
	{Key: "is_empty", Label: "is empty", NeedsValue: false},
	{Key: "is_not_empty", Label: "is not empty", NeedsValue: false},
}

var opsList = []Operator{
	{Key: "contains", Label: "contains", NeedsValue: true},
	{Key: "not_contains", Label: "does not contain", NeedsValue: true},
	{Key: "is_empty", Label: "is empty", NeedsValue: false},
	{Key: "is_not_empty", Label: "is not empty", NeedsValue: false},
	{Key: "count_gt", Label: "count greater than", NeedsValue: true},
	{Key: "count_lt", Label: "count less than", NeedsValue: true},
}

var opsMap = []Operator{
	{Key: "has_key", Label: "has key", NeedsValue: true},
	{Key: "not_has_key", Label: "does not have key", NeedsValue: true},
	{Key: "key_equals", Label: "key equals value", NeedsValue: true},
	{Key: "is_empty", Label: "is empty", NeedsValue: false},
	{Key: "is_not_empty", Label: "is not empty", NeedsValue: false},
}

var opsLink = []Operator{
	{Key: "equals", Label: "is", NeedsValue: true},
	{Key: "not_equals", Label: "is not", NeedsValue: true},
	{Key: "contains", Label: "contains", NeedsValue: true},
	{Key: "is_empty", Label: "is not set", NeedsValue: false},
	{Key: "is_not_empty", Label: "is set", NeedsValue: false},
}

var opsCompound = []Operator{
	{Key: "contains", Label: "contains", NeedsValue: true},
	{Key: "is_empty", Label: "is empty", NeedsValue: false},
	{Key: "is_not_empty", Label: "is not empty", NeedsValue: false},
}
