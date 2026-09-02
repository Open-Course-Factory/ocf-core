package scenarioController

import (
	"log"

	access "soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

// RegisterScenarioPermissions registers all Casbin policies for scenario routes.
// This includes session routes, teacher dashboard, group/org scenario management,
// project file routes, and admin-only scenario management routes.
func RegisterScenarioPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up scenario permissions (from routes package) ===")

	access.RegisterEnforced(enforcer, "Scenario Sessions",
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/start", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Start a new scenario session for the authenticated user",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/my", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List the authenticated user's scenario sessions",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/available", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List scenarios available to the authenticated user",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/by-terminal/:terminalId", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Get session by terminal ID (controller verifies session.UserID == authenticated user)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/info", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get session info (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/flags", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get session flags (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/current-step", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get current step of a session (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/step/:stepOrder", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Get a specific step of a session (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/verify", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Verify step completion (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/submit-flag", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Submit a flag answer (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/submit-quiz", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Submit quiz answers for a scenario session step (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/steps/:stepOrder/hints/:level/reveal", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Reveal a hint for a step (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/abandon", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Abandon a session (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/:id/reprovision-step", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.EntityOwner, Entity: "ScenarioSession", Field: "UserID"},
			Description: "Re-run the current step's setup script (must own the session)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenario-sessions/launch", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Launch a scenario with auto-provisioned terminal",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/preview", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Preview a scenario without group assignment (creator/org manager/admin)",
		},
	)

	access.RegisterEnforced(enforcer, "Teacher Dashboard",
		access.RoutePermission{
			// Self-scoped rather than GroupRole: there is no :groupId to enforce on.
			// The handler derives the classes from the authenticated caller id, so
			// no client input can widen the scope.
			Path: "/api/v1/teacher/groups", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "List the classes the authenticated user owns or manages, with per-class aggregates",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/activity", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View group activity overview",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/live-progress", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View the merged per-learner live class view (presence + scenario position + results)",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/assignments-progress", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View per-scenario assignment progress for a group",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/results", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View scenario results for a group",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/analytics", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View scenario analytics for a group",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/sessions/:sessionId/detail", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View detailed session info for a student",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/sessions/details", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Get session details for a group in bulk (CSV export)",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/sessions/:sessionId/commands", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View terminal command history for a student's scenario session (proxies to tt-backend)",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/bulk-start", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Bulk-start scenario sessions for group members",
		},
		access.RoutePermission{
			Path: "/api/v1/teacher/groups/:groupId/scenarios/:scenarioId/reset-sessions", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Reset scenario sessions for group members",
		},
	)

	access.RegisterEnforced(enforcer, "Scenario Management",
		// Group scenario routes
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "List scenarios available to a group (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Create a blank scenario for a group (manager+, auto-assigns)",
		},
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/upload", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Upload a scenario to a group",
		},
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/import-json", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Import a scenario from JSON into a group",
		},
		access.RoutePermission{
			Path: "/api/v1/groups/:groupId/scenarios/:scenarioId/export", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "Export a scenario from a group",
		},
		// Organization scenario routes
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "List scenarios in an organization (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Create a blank scenario in an organization (manager+)",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/upload", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Upload a scenario to an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/import-json", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Import a scenario from JSON into an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/:scenarioId/export", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Export a scenario from an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/:scenarioId", Method: "DELETE",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Delete a scenario from an organization",
		},
		access.RoutePermission{
			Path: "/api/v1/organizations/:id/scenarios/:scenarioId/duplicate", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "manager"},
			Description: "Duplicate a scenario within an organization",
		},
		// Admin scenario routes
		access.RoutePermission{
			Path: "/api/v1/scenarios/import", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Import scenarios (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/seed", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Seed default scenarios (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/upload", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Upload a scenario at platform level (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/export", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Export a scenario at platform level (controller verifies CanManageScenario: creator, org manager, group manager, or admin)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/export", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Bulk export scenarios at platform level (controller verifies CanManageScenario for every requested ID)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/import-json", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Import scenarios from JSON (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/lexicon", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Read a scenario's world vocabulary (controller verifies CanManageScenario)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/lexicon", Method: "PUT",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Replace a scenario's world vocabulary (controller verifies CanManageScenario)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/translation-coverage", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Report translation coverage per locale (controller verifies CanManageScenario: creator, org manager, group manager, or admin)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/health", Method: "GET",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Report what every scenario claims but cannot deliver (platform operators)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/archive", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Archive a scenario (controller verifies CanManageScenario: creator, org manager, group manager, or admin)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/unarchive", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.SelfScoped},
			Description: "Unarchive a scenario (controller verifies CanManageScenario: creator, org manager, group manager, or admin)",
		},
		access.RoutePermission{
			Path: "/api/v1/scenarios/:id/duplicate", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Duplicate a scenario at platform level (admin only)",
		},
		// Project file routes
		// These two routes are AdminOnly at Layer 2 and the handler self-enforces
		// isProjectFileAdmin; declaring Layer 1 as administrator makes Casbin deny
		// non-admins at the gateway too (defense-in-depth) and keeps the declaration
		// self-consistent with its access rule.
		access.RoutePermission{
			Path: "/api/v1/project-files/by-scenario/:scenarioId", Method: "GET",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List project files for a scenario (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/project-files/:id/usage", Method: "GET",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Get project file usage info (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/project-files/:id/content", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Get project file content (scripts require admin, others public)",
		},
		access.RoutePermission{
			Path: "/api/v1/project-files/image/:scenarioId/*", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Get scenario image (public to all authenticated users)",
		},
	)

	log.Println("=== Scenario permissions setup completed ===")

	// A platform operator carries `administrator` and not `member` — that is what
	// the role is for, since every real user is a member instead. The gateway
	// matches on the role alone, so a member-only policy locks the operator out
	// of routes the product plainly expects them to use.
	//
	// That is not hypothetical: seeding a scenario is an administrator route,
	// writing the vocabulary those scenario's scripts then depend on was not, so
	// a seed succeeded, its vocabulary was refused, and the scenario was left
	// referencing names that nothing defined.
	//
	// Only the gateway is widened here. Layer 2 still runs the member rule these
	// routes declare above, and the controller still verifies CanManageScenario.
	for _, route := range []struct{ path, method string }{
		{"/api/v1/scenarios/:id/lexicon", "GET"},
		{"/api/v1/scenarios/:id/lexicon", "PUT"},
		{"/api/v1/scenarios/:id/translation-coverage", "GET"},
		{"/api/v1/scenarios/health", "GET"},
	} {
		access.ReconcilePolicy(enforcer, access.RoleAdministrator, route.path, route.method)
	}
}
