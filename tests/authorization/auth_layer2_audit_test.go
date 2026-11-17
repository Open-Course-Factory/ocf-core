package authorization_tests

// Auth module Layer 2 route catalog (#271).
//
// Lists the AdminOnly routes declared in
//   - src/auth/routes/usersRoutes/permissions.go
//   - src/auth/routes/securityAdminRoutes/permissions.go
// Their enforcement scenarios (Outsider denied / Admin allowed) run in the
// generic parameterized suite in layer2_audit_parameterized_test.go via
// adaptAuthAdminRoutes().
//
// SelfScoped and Public routes are out of scope: Layer 2 does not enforce
// them, the controller does.

import (
	access "soli/formations/src/auth/access"
)

// Route catalog — mirrors the two permission files named above.
type authAuditRoute struct {
	module         string
	method         string
	registeredPath string
	requestPath    string
	ruleType       access.AccessRuleType
}

// authAuditAdminRoutes — 11 AdminOnly routes split across two files.
// These ARE Layer 2-enforced. Enforcement scenarios are covered by the
// parameterized suite in layer2_audit_parameterized_test.go.
var authAuditAdminRoutes = []authAuditRoute{
	// usersRoutes — 7 routes
	{module: "User Management", method: "DELETE", registeredPath: "/api/v1/users/:id", requestPath: "/api/v1/users/audit-uid", ruleType: access.AdminOnly},
	{module: "Access Control", method: "POST", registeredPath: "/api/v1/accesses", requestPath: "/api/v1/accesses", ruleType: access.AdminOnly},
	{module: "Access Control", method: "DELETE", registeredPath: "/api/v1/accesses", requestPath: "/api/v1/accesses", ruleType: access.AdminOnly},
	{module: "Access Control", method: "GET", registeredPath: "/api/v1/hooks", requestPath: "/api/v1/hooks", ruleType: access.AdminOnly},
	{module: "Access Control", method: "POST", registeredPath: "/api/v1/hooks/:hook_name/enable", requestPath: "/api/v1/hooks/audit-hook/enable", ruleType: access.AdminOnly},
	{module: "Access Control", method: "POST", registeredPath: "/api/v1/hooks/:hook_name/disable", requestPath: "/api/v1/hooks/audit-hook/disable", ruleType: access.AdminOnly},
	{module: "Access Control", method: "POST", registeredPath: "/api/v1/email-templates/:id/test", requestPath: "/api/v1/email-templates/audit-tpl/test", ruleType: access.AdminOnly},
	// securityAdminRoutes — 4 routes
	{module: "Security Administration", method: "GET", registeredPath: "/api/v1/admin/security/policies", requestPath: "/api/v1/admin/security/policies", ruleType: access.AdminOnly},
	{module: "Security Administration", method: "GET", registeredPath: "/api/v1/admin/security/user-permissions", requestPath: "/api/v1/admin/security/user-permissions", ruleType: access.AdminOnly},
	{module: "Security Administration", method: "GET", registeredPath: "/api/v1/admin/security/entity-roles", requestPath: "/api/v1/admin/security/entity-roles", ruleType: access.AdminOnly},
	{module: "Security Administration", method: "GET", registeredPath: "/api/v1/admin/security/health-checks", requestPath: "/api/v1/admin/security/health-checks", ruleType: access.AdminOnly},
}
