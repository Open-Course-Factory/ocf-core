package scenarioController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	casbinUtils "soli/formations/src/auth/casbin"
)

// RegisterScenarioPermissions registers all Casbin policies for scenario routes.
// This includes session routes, teacher dashboard, group/org scenario management,
// project file routes, and admin-only scenario management routes.
func RegisterScenarioPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up scenario permissions (from routes package) ===")

	// Scenario session routes - available to all authenticated members
	// (fine-grained access checks happen in the controller)
	sessionRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/scenario-sessions/available", "GET"},
		{"/api/v1/scenario-sessions/my", "GET"},
		{"/api/v1/scenario-sessions/start", "POST"},
		{"/api/v1/scenario-sessions/by-terminal/:terminalId", "GET"},
		{"/api/v1/scenario-sessions/:id/info", "GET"},
		{"/api/v1/scenario-sessions/:id/flags", "GET"},
		{"/api/v1/scenario-sessions/:id/current-step", "GET"},
		{"/api/v1/scenario-sessions/:id/step/:stepOrder", "GET"},
		{"/api/v1/scenario-sessions/:id/verify", "POST"},
		{"/api/v1/scenario-sessions/:id/submit-flag", "POST"},
		{"/api/v1/scenario-sessions/:id/steps/:stepOrder/hints/:level/reveal", "POST"},
		{"/api/v1/scenario-sessions/:id/abandon", "POST"},
	}

	for _, route := range sessionRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Teacher dashboard routes - available to all authenticated members
	// (fine-grained group ownership checks happen in the controller)
	teacherRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/teacher/groups/:groupId/activity", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/results", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/analytics", "GET"},
		{"/api/v1/teacher/groups/:groupId/sessions/:sessionId/detail", "GET"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/bulk-start", "POST"},
		{"/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/reset-sessions", "POST"},
	}

	for _, route := range teacherRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Group-level scenario routes - available to all authenticated members
	// (fine-grained group ownership checks happen in the controller via validateTeacherAccess)
	groupScenarioRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/groups/:groupId/scenarios", "GET"},
		{"/api/v1/groups/:groupId/scenarios/upload", "POST"},
		{"/api/v1/groups/:groupId/scenarios/import-json", "POST"},
		{"/api/v1/groups/:groupId/scenarios/:scenarioId/export", "GET"},
	}

	for _, route := range groupScenarioRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Organization-level scenario routes - available to all authenticated members
	// (fine-grained org ownership checks happen in the controller via validateOrgManagerAccess)
	orgScenarioRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/organizations/:id/scenarios", "GET"},
		{"/api/v1/organizations/:id/scenarios/upload", "POST"},
		{"/api/v1/organizations/:id/scenarios/import-json", "POST"},
		{"/api/v1/organizations/:id/scenarios/:scenarioId/export", "GET"},
		{"/api/v1/organizations/:id/scenarios/:scenarioId", "DELETE"},
	}

	for _, route := range orgScenarioRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Project file routes - available to all authenticated members
	projectFileRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/project-files/by-scenario/:scenarioId", "GET"},
		{"/api/v1/project-files/image/:scenarioId/*", "GET"},
		{"/api/v1/project-files/:id/content", "GET"},
		{"/api/v1/project-files/:id/usage", "GET"},
	}

	for _, route := range projectFileRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin-only scenario management routes
	// (handlers have isAdmin() checks for additional protection)
	adminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/scenarios/import", "POST"},
		{"/api/v1/scenarios/seed", "POST"},
		{"/api/v1/scenarios/upload", "POST"},
		{"/api/v1/scenarios/:id/export", "GET"},
		{"/api/v1/scenarios/export", "POST"},
		{"/api/v1/scenarios/import-json", "POST"},
	}

	for _, route := range adminRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// --- Route Registry: declarative permission metadata ---

	casbinUtils.RouteRegistry.Register("Scenario Sessions",
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/start", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "Start a new scenario session for the authenticated user",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/my", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "List the authenticated user's scenario sessions",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/available", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.SelfScoped},
			Description: "List scenarios available to the authenticated user",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/by-terminal/:terminalId", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get session by terminal ID (must own the session)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/info", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get session info (must own the session)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/flags", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get session flags (must own the session)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/current-step", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get current step of a session (must own the session)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/step/:stepOrder", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get a specific step of a session (must own the session)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/verify", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Verify step completion (must own the session)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/submit-flag", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Submit a flag answer (must own the session)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/steps/:stepOrder/hints/:level/reveal", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Reveal a hint for a step (must own the session)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/abandon", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Abandon a session (must own the session)",
		},
	)

	casbinUtils.RouteRegistry.Register("Teacher Dashboard",
		casbinUtils.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/activity", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View group activity overview",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/results", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View scenario results for a group",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/analytics", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View scenario analytics for a group",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/sessions/:sessionId/detail", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View detailed session info for a student",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/bulk-start", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Bulk-start scenario sessions for group members",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/reset-sessions", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Reset scenario sessions for group members",
		},
	)

	casbinUtils.RouteRegistry.Register("Scenario Management",
		// Group scenario routes
		casbinUtils.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "member"},
			Description: "List scenarios assigned to a group",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/upload", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Upload a scenario to a group",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/import-json", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Import a scenario from JSON into a group",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/:scenarioId/export", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Export a scenario from a group",
		},
		// Organization scenario routes
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "member"},
			Description: "List scenarios in an organization",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/upload", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Upload a scenario to an organization",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/import-json", Method: "POST",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Import a scenario from JSON into an organization",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/:scenarioId/export", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Export a scenario from an organization",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/:scenarioId", Method: "DELETE",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Delete a scenario from an organization",
		},
		// Admin scenario routes
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenarios/import", Method: "POST",
			Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Import scenarios (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenarios/seed", Method: "POST",
			Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Seed default scenarios (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenarios/upload", Method: "POST",
			Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Upload a scenario at platform level (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenarios/:id/export", Method: "GET",
			Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Export a scenario at platform level (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenarios/export", Method: "POST",
			Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Bulk export scenarios (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/scenarios/import-json", Method: "POST",
			Role: "administrator", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Import scenarios from JSON (admin only)",
		},
		// Project file routes
		casbinUtils.RoutePermission{
			Path: "/api/v1/project-files/by-scenario/:scenarioId", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "List project files for a scenario (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/project-files/:id/usage", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.AdminOnly},
			Description: "Get project file usage info (admin only)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/project-files/:id/content", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Get project file content (scripts require admin, others public)",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/project-files/image/:scenarioId/*", Method: "GET",
			Role: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Get scenario image (public to all authenticated users)",
		},
	)

	log.Println("=== Scenario permissions setup completed ===")
}
