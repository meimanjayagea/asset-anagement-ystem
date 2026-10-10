package app

import "testing"

func TestRoleCapabilityBoundaries(t *testing.T) {
	for _, role := range []string{"admin", "branch_admin", "manager", "operator", "staff", "employee", "finance", "it_support", "it_developer", "auditor"} {
		if !validRole(role) {
			t.Fatalf("role %q missing from permission matrix", role)
		}
	}
	checks := []struct {
		role       string
		capability string
		want       bool
	}{
		{"admin", "branches.manage", true},
		{"admin", "audit.read", true},
		{"branch_admin", "assets.archive", true},
		{"branch_admin", "assets.finance", true},
		{"branch_admin", "users.manage", true},
		{"branch_admin", "categories.manage", true},
		{"branch_admin", "categories.archive", true},
		{"branch_admin", "audit.read", true},
		{"branch_admin", "branches.manage", false},
		{"branch_admin", "finance.read", true},
		{"branch_admin", "valuation.decide", true},
		{"manager", "requests.decide", true},
		{"manager", "audit.read", false},
		{"manager", "contracts.manage", true},
		{"manager", "finance.read", false},
		{"finance", "assets.finance", true},
		{"finance", "finance.manage", true},
		{"finance", "assets.export", true},
		{"finance", "assets.write", false},
		{"it_support", "maintenance.manage", true},
		{"it_support", "assets.finance", false},
		{"it_support", "contracts.read", false},
		{"it_support", "reports.read", false},
		{"it_developer", "assets.read", true},
		{"it_developer", "assets.write", false},
		{"it_developer", "audit.read", false},
		{"employee", "requests.create", true},
		{"employee", "requests.decide", false},
		{"auditor", "assets.read", true},
		{"auditor", "audit.read", true},
	}
	for _, check := range checks {
		if got := hasCapability(check.role, check.capability); got != check.want {
			t.Errorf("hasCapability(%q, %q) = %v, want %v", check.role, check.capability, got, check.want)
		}
	}
}

func TestRoleAssignmentBoundaries(t *testing.T) {
	if !canAssignRole("admin", "branch_admin") || !canAssignRole("admin", "admin") {
		t.Fatal("central admin must be able to manage administrator roles")
	}
	for _, role := range []string{"admin", "branch_admin"} {
		if canAssignRole("branch_admin", role) {
			t.Fatalf("branch admin must not assign elevated role %q", role)
		}
	}
	for _, role := range []string{"manager", "operator", "staff", "employee", "finance", "it_support", "it_developer", "auditor"} {
		if !canAssignRole("branch_admin", role) {
			t.Fatalf("branch admin should be able to manage branch role %q", role)
		}
	}
}
