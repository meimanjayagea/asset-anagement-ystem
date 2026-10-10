package app

type roleOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

var roleLabels = map[string]string{
	"admin":        "Administrator Pusat",
	"branch_admin": "Administrator Cabang",
	"manager":      "Manager",
	"operator":     "Operator",
	"staff":        "Staff",
	"employee":     "Karyawan",
	"finance":      "Finance",
	"it_support":   "IT Support",
	"it_developer": "IT Developer",
	"auditor":      "Auditor",
}

var allCapabilities = []string{
	"dashboard.read", "assets.read", "assets.write", "assets.operate", "assets.archive",
	"assets.finance", "assets.export", "requests.read", "requests.create", "requests.decide",
	"maintenance.read", "maintenance.manage", "stocktakes.read", "stocktakes.manage",
	"stocktakes.observe", "stocktakes.close", "branches.read", "branches.manage",
	"locations.read", "locations.manage", "locations.archive", "categories.read",
	"categories.manage", "categories.archive", "users.read", "users.manage",
	"users.manage_admin", "users.archive", "audit.read", "activity.read",
	"assets.history", "contracts.read", "contracts.manage", "finance.read",
	"finance.manage", "valuation.propose", "valuation.decide", "reports.read",
	"reports.export",
	"photos.write", "loans.read", "loans.request", "loans.manage", "maintenance.request",
}

var roleCapabilities = map[string][]string{
	"branch_admin": {
		"dashboard.read", "assets.read", "assets.write", "assets.operate", "assets.archive",
		"assets.finance", "assets.export", "requests.read", "requests.create", "requests.decide",
		"maintenance.read", "maintenance.manage", "stocktakes.read", "stocktakes.manage",
		"stocktakes.observe", "stocktakes.close", "branches.read", "locations.read",
		"locations.manage", "locations.archive", "categories.read", "users.read", "users.manage",
		"categories.manage", "categories.archive", "users.archive", "audit.read", "activity.read",
		"assets.history", "contracts.read", "contracts.manage", "finance.read", "finance.manage",
		"valuation.propose", "valuation.decide", "reports.read", "reports.export",
	},
	"manager": {
		"dashboard.read", "assets.read", "assets.write", "assets.operate", "requests.read",
		"requests.create", "requests.decide", "maintenance.read", "maintenance.manage",
		"stocktakes.read", "stocktakes.manage", "stocktakes.observe", "stocktakes.close",
		"branches.read", "locations.read", "locations.manage", "categories.read",
		"assets.history", "contracts.read", "contracts.manage", "reports.read", "valuation.propose",
	},
	"operator": {
		"dashboard.read", "assets.read", "assets.write", "assets.operate", "requests.read",
		"requests.create", "maintenance.read", "maintenance.manage", "stocktakes.read",
		"stocktakes.manage", "stocktakes.observe", "branches.read", "locations.read", "categories.read",
		"assets.history",
	},
	"staff": {
		"dashboard.read", "assets.read", "requests.read", "requests.create", "maintenance.read",
		"stocktakes.read", "stocktakes.observe", "branches.read", "locations.read", "categories.read",
	},
	"employee": {
		"dashboard.read", "assets.read", "requests.read", "requests.create", "branches.read",
		"locations.read", "categories.read", "maintenance.read",
	},
	"finance": {
		"dashboard.read", "assets.read", "assets.finance", "assets.export", "requests.read",
		"maintenance.read", "stocktakes.read", "branches.read", "locations.read", "categories.read",
		"assets.history", "contracts.read", "contracts.manage", "finance.read", "finance.manage",
		"valuation.propose", "valuation.decide", "reports.read", "reports.export",
	},
	"it_support": {
		"dashboard.read", "assets.read", "assets.write", "assets.operate", "requests.read",
		"requests.create", "maintenance.read", "maintenance.manage", "stocktakes.read",
		"stocktakes.manage", "stocktakes.observe", "stocktakes.close", "branches.read",
		"locations.read", "categories.read",
		"assets.history",
	},
	"it_developer": {
		"dashboard.read", "assets.read", "maintenance.read", "branches.read", "locations.read",
		"categories.read",
	},
	"auditor": {
		"dashboard.read", "assets.read", "requests.read", "maintenance.read", "stocktakes.read",
		"branches.read", "locations.read", "categories.read",
		"assets.history", "reports.read", "audit.read", "activity.read", "stocktakes.observe",
	},
}

func hasCapability(role, capability string) bool {
	if role == "admin" {
		for _, candidate := range allCapabilities {
			if candidate == capability {
				return true
			}
		}
		return false
	}
	switch capability {
	case "photos.write":
		return hasCapability(role, "assets.write") || hasCapability(role, "requests.create") || hasCapability(role, "stocktakes.observe")
	case "loans.read":
		return hasCapability(role, "assets.read")
	case "loans.request", "maintenance.request":
		return hasCapability(role, "requests.create")
	case "loans.manage":
		return hasCapability(role, "assets.operate")
	}
	for _, candidate := range roleCapabilities[role] {
		if candidate == capability {
			return true
		}
	}
	return false
}

func capabilitiesForRole(role string) []string {
	out := []string{}
	for _, capability := range allCapabilities {
		if hasCapability(role, capability) {
			out = append(out, capability)
		}
	}
	return out
}

func validRole(role string) bool {
	if role == "admin" {
		return true
	}
	_, ok := roleCapabilities[role]
	return ok
}

func canAssignRole(actorRole, targetRole string) bool {
	if !hasCapability(actorRole, "users.manage") || !validRole(targetRole) {
		return false
	}
	if actorRole == "branch_admin" {
		return targetRole != "admin" && targetRole != "branch_admin"
	}
	return actorRole == "admin"
}

func assignableRoles(actorRole string) []roleOption {
	roles := []string{"admin", "branch_admin", "manager", "operator", "staff", "employee", "finance", "it_support", "it_developer", "auditor"}
	out := make([]roleOption, 0, len(roles))
	for _, role := range roles {
		if canAssignRole(actorRole, role) {
			out = append(out, roleOption{ID: role, Label: roleLabels[role]})
		}
	}
	return out
}
