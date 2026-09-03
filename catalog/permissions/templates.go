package permissions

// Builtin template matrices per
// docs/history/specs/2026-07-08-pluris-policy-authz-design.md
// section "Builtin template matrices". Values: scoped actions use
// "none"|"own"|"all"; unscoped actions use "no"|"yes".
var builtinTemplates = map[string]map[string]string{
	"super_admin": {
		"identity.view":          "all",
		"identity.update":        "all",
		"identity.create":        "yes",
		"identity.delete":        "yes",
		"identity.assign_roles":  "yes",
		"identity.assign_groups": "yes",

		"asset.view":          "all",
		"asset.update":        "all",
		"asset.create":        "yes",
		"asset.delete":        "yes",
		"asset.set_owner":     "yes",
		"asset.manage_groups": "yes",

		"group.view":           "all",
		"group.create":         "yes",
		"group.update":         "yes",
		"group.delete":         "yes",
		"group.manage_members": "yes",

		"endpoint_policy.view":                     "yes",
		"endpoint_policy.manage_catalog":           "yes",
		"endpoint_policy.manage_config_groups":     "yes",
		"endpoint_policy.manage_dependency_groups": "yes",
		"endpoint_policy.manage_modules":           "yes",
		"endpoint_policy.assign_policies":          "yes",

		"console_access.view_roles":              "yes",
		"console_access.manage_role_assignments": "yes",
		"console_access.manage_permissions":      "yes",

		"server_admin.access":        "yes",
		"server_admin.manage_data":   "yes",
		"server_admin.tenant_switch": "yes",
	},
	"admin": {
		"identity.view":          "all",
		"identity.update":        "all",
		"identity.create":        "yes",
		"identity.delete":        "yes",
		"identity.assign_roles":  "yes",
		"identity.assign_groups": "yes",

		"asset.view":          "all",
		"asset.update":        "all",
		"asset.create":        "yes",
		"asset.delete":        "yes",
		"asset.set_owner":     "yes",
		"asset.manage_groups": "yes",

		"group.view":           "all",
		"group.create":         "yes",
		"group.update":         "yes",
		"group.delete":         "yes",
		"group.manage_members": "yes",

		"endpoint_policy.view":                     "yes",
		"endpoint_policy.manage_catalog":           "yes",
		"endpoint_policy.manage_config_groups":     "yes",
		"endpoint_policy.manage_dependency_groups": "yes",
		"endpoint_policy.manage_modules":           "yes",
		"endpoint_policy.assign_policies":          "yes",

		"console_access.view_roles":              "yes",
		"console_access.manage_role_assignments": "yes",
		"console_access.manage_permissions":      "yes",

		"server_admin.access":        "yes",
		"server_admin.manage_data":   "yes",
		"server_admin.tenant_switch": "no",
	},
	"technician": {
		"identity.view":          "all",
		"identity.update":        "all",
		"identity.create":        "yes",
		"identity.delete":        "no",
		"identity.assign_roles":  "no",
		"identity.assign_groups": "yes",

		"asset.view":          "all",
		"asset.update":        "all",
		"asset.create":        "yes",
		"asset.delete":        "yes",
		"asset.set_owner":     "yes",
		"asset.manage_groups": "yes",

		"group.view":           "all",
		"group.create":         "no",
		"group.update":         "no",
		"group.delete":         "no",
		"group.manage_members": "yes",

		"endpoint_policy.view":                     "yes",
		"endpoint_policy.manage_catalog":           "yes",
		"endpoint_policy.manage_config_groups":     "yes",
		"endpoint_policy.manage_dependency_groups": "yes",
		"endpoint_policy.manage_modules":           "yes",
		"endpoint_policy.assign_policies":          "yes",

		"console_access.view_roles":              "yes",
		"console_access.manage_role_assignments": "no",
		"console_access.manage_permissions":      "no",

		"server_admin.access":        "no",
		"server_admin.manage_data":   "no",
		"server_admin.tenant_switch": "no",
	},
	"user": {
		"identity.view":          "own",
		"identity.update":        "own",
		"identity.create":        "no",
		"identity.delete":        "no",
		"identity.assign_roles":  "no",
		"identity.assign_groups": "no",

		"asset.view":          "own",
		"asset.update":        "none",
		"asset.create":        "no",
		"asset.delete":        "no",
		"asset.set_owner":     "no",
		"asset.manage_groups": "no",

		"group.view":           "none",
		"group.create":         "no",
		"group.update":         "no",
		"group.delete":         "no",
		"group.manage_members": "no",

		"endpoint_policy.view":                     "yes",
		"endpoint_policy.manage_catalog":           "no",
		"endpoint_policy.manage_config_groups":     "no",
		"endpoint_policy.manage_dependency_groups": "no",
		"endpoint_policy.manage_modules":           "no",
		"endpoint_policy.assign_policies":          "no",

		"console_access.view_roles":              "no",
		"console_access.manage_role_assignments": "no",
		"console_access.manage_permissions":      "no",

		"server_admin.access":        "no",
		"server_admin.manage_data":   "no",
		"server_admin.tenant_switch": "no",
	},
}

// TemplateGrants returns a fresh copy of the builtin role template matrix
// for slug ("super_admin", "admin", "technician", "user"), or nil if slug
// is not a builtin template. Callers may freely mutate the returned map.
func TemplateGrants(slug string) map[string]string {
	src, ok := builtinTemplates[slug]
	if !ok {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
