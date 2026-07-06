package policies

// customPolicies — tenant-authored catalog entries (ADR-007). v1 stores
// them inline as a mock; once the backend slice lands these become rows
// in `db/schema/custom_policy.go` filtered by tenant. URN convention:
// `tenant.<tenant_id>.<slug>` so a custom policy can never collide with
// a bundled one.
//
// To add a real custom policy in v1: append a Policy to this slice with
// Custom=true, TenantID=<tenant>, and ID prefix matching the URN
// convention. The wizard (Phase 0 UI scaffolding, Phase 1 backend) will
// generate these rows from the form output.
var customPolicies = []Policy{
	{
		ID:          "tenant.acme.ssh.banner",
		Name:        "Corporate SSH login banner",
		WinGPName:   "", // no Windows GP equivalent — custom
		WinGPPath:   "",
		Category:    []string{"Custom (Acme)", "Remote Access", "SSH"},
		Scope:       ScopeComputer,
		Description: "Drops a corporate banner into /etc/issue.net so every SSH login displays the configured legal text. Linked to module tenant.acme.security-banner.",
		LinuxImpl:   "/etc/issue.net via tenant module",
		Custom:      true,
		TenantID:    "acme",
	},
}
