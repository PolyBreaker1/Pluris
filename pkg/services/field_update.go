package services

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/pluris/pluris/catalog/params"
)

// ErrFieldNotFound is returned by IdentityService.UpdateFields /
// AssetService.UpdateFields when the target entity does not exist (or
// does not belong to the caller's tenant) -- handlers map this to a 404
// so a cross-tenant probe reads identically to a missing id.
var ErrFieldNotFound = errors.New("entity not found")

// ErrFieldValidation is returned when the requested section, a field
// key, or a field's value fails validation (unknown section, key not
// mounted in that section, non-editable key, or a type-coercion
// failure). The wrapped message names the offending key. Handlers map
// this to a 400 with the message as the error body.
var ErrFieldValidation = errors.New("field validation failed")

// sectionByKey finds a mounted section within schema by its key, or nil
// when schema is nil or has no such section.
func sectionByKey(schema *params.SubtypeSchema, key string) *params.SchemaSection {
	if schema == nil {
		return nil
	}
	for i := range schema.Sections {
		if schema.Sections[i].Key == key {
			return &schema.Sections[i]
		}
	}
	return nil
}

// sectionHasParam reports whether sec mounts the param key.
func sectionHasParam(sec *params.SchemaSection, key string) bool {
	for _, k := range sec.Params {
		if k == key {
			return true
		}
	}
	return false
}

// coerceParamValue converts the raw string field value submitted by the
// field-update API into a Go value matching def.Type:
//   - TypeBool:  strconv.ParseBool
//   - TypeInt:   strconv.ParseInt
//   - TypeList:  comma-split, trimmed, empty entries dropped -> []string
//   - TypeEnum:  validated against def.EnumValues, kept as string
//   - everything else (string/date/time): passed through unchanged
//
// The returned error is unwrapped -- callers wrap it with the field key
// via %w-chaining onto ErrFieldValidation.
func coerceParamValue(def *params.ParamDef, raw string) (interface{}, error) {
	switch def.Type {
	case params.TypeBool:
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("expected a boolean, got %q", raw)
		}
		return v, nil
	case params.TypeInt:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected an integer, got %q", raw)
		}
		return v, nil
	case params.TypeList:
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	case params.TypeEnum:
		for _, v := range def.EnumValues {
			if v == raw {
				return raw, nil
			}
		}
		return nil, fmt.Errorf("invalid value %q (want one of %s)", raw, strings.Join(def.EnumValues, ", "))
	default:
		return raw, nil
	}
}

// fieldErr wraps ErrFieldValidation with a message naming key, so
// handlers can both errors.Is(err, ErrFieldValidation) for status-code
// mapping and print err.Error() as the response body.
func fieldErr(key, format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s: %s", ErrFieldValidation, key, fmt.Sprintf(format, args...))
}
