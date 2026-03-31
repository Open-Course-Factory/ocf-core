package scenarioController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	access "soli/formations/src/auth/access"
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
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Teacher dashboard routes - available to all authenticated members
	// (Layer 2 enforces GroupRole:manager with admin bypass)
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
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Group-level scenario routes - available to all authenticated members
	// (Layer 2 enforces GroupRole with admin bypass)
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
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Organization-level scenario routes - available to all authenticated members
	// (Layer 2 enforces OrgRole with admin bypass)
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
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
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
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Admin-only scenario management routes
	// (Layer 1 restricts to administrator, Layer 2 enforces AdminOnly)
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
		access.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// --- Route Registry: declarative permission metadata ---

	access.RouteRegistry.Register("Scenario Sessions",
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/start", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Start a new scenario session for the authenticated user",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/my", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List the authenticated user's scenario sessions",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/available", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List scenarios available to the authenticated user",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/by-terminal/:terminalId", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get session by terminal ID (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/info", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get session info (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/flags", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get session flags (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/current-step", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get current step of a session (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/step/:stepOrder", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get a specific step of a session (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/verify", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Verify step completion (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/submit-flag", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Submit a flag answer (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/steps/:stepOrder/hints/:level/reveal", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Reveal a hint for a step (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/abandon", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Abandon a session (must own the session)",
		},
	)

	access.RouteRegistry.Register("Teacher Dashboard",
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/activity", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View group activity overview",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/results", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View scenario results for a group",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/analytics", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View scenario analytics for a group",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/sessions/:sessionId/detail", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View detailed session info for a student",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/bulk-start", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Bulk-start scenario sessions for group members",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/reset-sessions", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Reset scenario sessions for group members",
		},
	)

	access.RouteRegistry.Register("Scenario Management",
		// Group scenario routes
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "List scenarios available to a group (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/upload", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Upload a scenario to a group",
		},
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/import-json", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Import a scenario from JSON into a group",
		},
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/:scenarioId/export", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Export a scenario from a group",
		},
		// Organization scenario routes
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "List scenarios in an organization (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/upload", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Upload a scenario to an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/import-json", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Import a scenario from JSON into an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/:scenarioId/export", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Export a scenario from an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/:scenarioId", Method: "DELETE",
			Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Delete a scenario from an organization",
		},
		// Admin scenario routes
		access.RoutePermission{
			Path: "/api/v1/scenarios/import", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Import scenarios (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/seed", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Seed default scenarios (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/upload", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Upload a scenario at platform level (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/export", Method: "GET",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Export a scenario at platform level (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/export", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Bulk export scenarios (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/import-json", Method: "POST",
			Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Import scenarios from JSON (admin only)",
		},
		// Project file routes
		access.RoutePermission{
			Path: "/api/v1/project-files/by-scenario/:scenarioId", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List project files for a scenario (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/project-files/:id/usage", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Get project file usage info (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/project-files/:id/content", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Get project file content (scripts require admin, others public)",
		},
		access.RoutePermission{
			Path: "/api/v1/project-files/image/:scenarioId/*", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Get scenario image (public to all authenticated users)",
		},
	)

	log.Println("=== Scenario permissions setup completed ===")
}
