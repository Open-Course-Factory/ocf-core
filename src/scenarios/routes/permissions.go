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
			Path: "/api/v1/teacher/groups/:groupId/activity", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.GroupRole, Param: "groupId", MinRole: "manager"},
			Description: "View group activity overview",
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
			Path: "/api/v1/scenarios/:id/duplicate", Method: "POST",
			Role: access.RoleAdministrator, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "Duplicate a scenario at platform level (admin only)",
		},
		// Project file routes
		access.RoutePermission{
			Path: "/api/v1/project-files/by-scenario/:scenarioId", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.AdminOnly},
			Description: "List project files for a scenario (admin only)",
		},
		access.RoutePermission{
			Path: "/api/v1/project-files/:id/usage", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.AdminOnly},
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
}
