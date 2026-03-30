package terminalController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	casbinUtils "soli/formations/src/auth/casbin"
)

// RegisterTerminalPermissions registers all Casbin policies for terminal routes.
func RegisterTerminalPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering terminal module permissions ===")

	// User Terminal Key routes
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-terminal-keys/regenerate", "POST")
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/user-terminal-keys/my-key", "GET")

	// Terminal member routes
	terminalRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/terminals/start-session", "POST"},
		{"/api/v1/terminals/user-sessions", "GET"},
		{"/api/v1/terminals/shared-with-me", "GET"},
		{"/api/v1/terminals/sync-all", "POST"},
		{"/api/v1/terminals/instance-types", "GET"},
		{"/api/v1/terminals/metrics", "GET"},
		{"/api/v1/terminals/:id/console", "GET"},
		{"/api/v1/terminals/:id/stop", "POST"},
		{"/api/v1/terminals/:id/share", "POST"},
		{"/api/v1/terminals/:id/share/:user_id", "DELETE"},
		{"/api/v1/terminals/:id/shares", "GET"},
		{"/api/v1/terminals/:id/info", "GET"},
		{"/api/v1/terminals/:id/hide", "POST"},
		{"/api/v1/terminals/:id/hide", "DELETE"},
		{"/api/v1/terminals/:id/sync", "POST"},
		{"/api/v1/terminals/:id/status", "GET"},
		{"/api/v1/terminals/:id/history", "GET"},
		{"/api/v1/terminals/:id/history", "DELETE"},
		{"/api/v1/terminals/my-history", "DELETE"},
		{"/api/v1/terminals/:id/access-status", "GET"},
		{"/api/v1/terminals/consent-status", "GET"},
		{"/api/v1/terminals/backends", "GET"},
	}

	for _, route := range terminalRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Terminal admin routes
	terminalAdminRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/terminals/backends/:backendId/set-default", "PATCH"},
		{"/api/v1/terminals/enums/status", "GET"},
		{"/api/v1/terminals/enums/refresh", "POST"},
		{"/api/v1/terminals/fix-hide-permissions", "POST"},
	}

	for _, route := range terminalAdminRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
	}

	// Group terminal routes (fine-grained group checks in controller)
	groupTerminalRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/class-groups/:id/bulk-create-terminals", "POST"},
		{"/api/v1/class-groups/:id/command-history", "GET"},
		{"/api/v1/class-groups/:id/command-history-stats", "GET"},
	}

	for _, route := range groupTerminalRoutes {
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Organization terminal sessions (fine-grained org checks in controller)
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/terminal-sessions", "GET")

	// Incus UI proxy (fine-grained backend access checks in controller)
	casbinUtils.ReconcilePolicy(enforcer, "member", "/api/v1/incus-ui/:backendId/*", "(GET|POST|PUT|PATCH|DELETE)")

	log.Println("=== Terminal module permissions registered ===")
}
