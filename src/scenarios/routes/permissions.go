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

	log.Println("=== Scenario permissions setup completed ===")
}
