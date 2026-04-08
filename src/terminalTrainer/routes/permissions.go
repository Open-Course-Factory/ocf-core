package terminalController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	access "soli/formations/src/auth/access"
)

// RegisterTerminalPermissions registers all Casbin policies for terminal routes.
func RegisterTerminalPermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Registering terminal module permissions ===")

	// User Terminal Key routes
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-terminal-keys/regenerate", "POST")
	access.ReconcilePolicy(enforcer, "member", "/api/v1/user-terminal-keys/my-key", "GET")

	// Terminal member routes
	terminalRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/terminals/user-sessions", "GET"},
		{"/api/v1/terminals/shared-with-me", "GET"},
		{"/api/v1/terminals/sync-all", "POST"},
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
		{"/api/v1/terminals/distributions", "GET"},
		{"/api/v1/terminals/catalog-sizes", "GET"},
		{"/api/v1/terminals/catalog-features", "GET"},
		{"/api/v1/terminals/session-options", "GET"},
		{"/api/v1/terminals/start-composed-session", "POST"},
	}

	for _, route := range terminalRoutes {
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
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
		access.ReconcilePolicy(enforcer, "administrator", route.path, route.method)
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
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Organization terminal sessions (fine-grained org checks in controller)
	access.ReconcilePolicy(enforcer, "member", "/api/v1/organizations/:id/terminal-sessions", "GET")

	// Incus UI proxy (fine-grained backend access checks in controller)
	access.ReconcilePolicy(enforcer, "member", "/api/v1/incus-ui/:backendId/*", "(GET|POST|PUT|PATCH|DELETE)")

	// Declarative route permission registry
	access.RouteRegistry.Register("Terminals",
		// Session management
		access.RoutePermission{Path: "/api/v1/terminals/user-sessions", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "List current user's active terminal sessions"},
		access.RoutePermission{Path: "/api/v1/terminals/shared-with-me", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "List terminals shared with current user"},
		access.RoutePermission{Path: "/api/v1/terminals/my-history", Method: "DELETE", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "Delete all command history for current user"},
		access.RoutePermission{Path: "/api/v1/terminals/sync-all", Method: "POST", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "Sync all terminal sessions for current user"},

		// Per-terminal operations (owner-scoped)
		access.RoutePermission{Path: "/api/v1/terminals/:id/console", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Connect to terminal console via WebSocket"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/stop", Method: "POST", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Stop a running terminal session"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/share", Method: "POST", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Share terminal with another user"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/share/:user_id", Method: "DELETE", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Revoke terminal access from a user"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/shares", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "List users a terminal is shared with"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/info", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Get terminal session details"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/hide", Method: "POST", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Hide a terminal from the session list"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/hide", Method: "DELETE", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Unhide a terminal in the session list"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/sync", Method: "POST", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Sync terminal session state with backend"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/status", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Get terminal session status"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/history", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Get command history for a terminal session"},
		access.RoutePermission{Path: "/api/v1/terminals/:id/history", Method: "DELETE", Role: "member", Access: access.AccessRule{Type: access.EntityOwner, Entity: "Terminal", Field: "UserID"}, Description: "Delete command history for a terminal session"},

		// Access status (self-scoped - checks own access level)
		access.RoutePermission{Path: "/api/v1/terminals/:id/access-status", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "Check current user's access level for a terminal"},

		// Public configuration routes
		access.RoutePermission{Path: "/api/v1/terminals/consent-status", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.Public}, Description: "Get consent policy status for command recording"},
		access.RoutePermission{Path: "/api/v1/terminals/metrics", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.Public}, Description: "Get terminal server metrics"},
		access.RoutePermission{Path: "/api/v1/terminals/backends", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.Public}, Description: "List available terminal backends"},
		access.RoutePermission{Path: "/api/v1/terminals/distributions", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.Public}, Description: "List available distributions"},
		access.RoutePermission{Path: "/api/v1/terminals/catalog-sizes", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.Public}, Description: "List available resource sizes for session composition"},
		access.RoutePermission{Path: "/api/v1/terminals/catalog-features", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.Public}, Description: "List available features for session composition"},
		access.RoutePermission{Path: "/api/v1/terminals/session-options", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "Get session composition options for a distribution"},
		access.RoutePermission{Path: "/api/v1/terminals/start-composed-session", Method: "POST", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "Start a composed terminal session"},

		// Admin routes
		access.RoutePermission{Path: "/api/v1/terminals/backends/:backendId/set-default", Method: "PATCH", Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly}, Description: "Set the default terminal backend"},
		access.RoutePermission{Path: "/api/v1/terminals/enums/status", Method: "GET", Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly}, Description: "Get enum cache status for diagnostics"},
		access.RoutePermission{Path: "/api/v1/terminals/enums/refresh", Method: "POST", Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly}, Description: "Refresh enum caches from backend"},
		access.RoutePermission{Path: "/api/v1/terminals/fix-hide-permissions", Method: "POST", Role: "administrator", Access: access.AccessRule{Type: access.AdminOnly}, Description: "Fix terminal hide permissions for all users"},

		// Group terminal routes
		access.RoutePermission{Path: "/api/v1/class-groups/:id/bulk-create-terminals", Method: "POST", Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "id", MinRole: "manager"}, Description: "Bulk create terminals for all group members"},
		access.RoutePermission{Path: "/api/v1/class-groups/:id/command-history", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "id", MinRole: "manager"}, Description: "Get command history for all group members"},
		access.RoutePermission{Path: "/api/v1/class-groups/:id/command-history-stats", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.GroupRole, Param: "id", MinRole: "manager"}, Description: "Get command history statistics for a group"},

		// Organization terminal routes
		access.RoutePermission{Path: "/api/v1/organizations/:id/terminal-sessions", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "id", MinRole: "member"}, Description: "List terminal sessions for an organization"},

		// Incus UI proxy
		access.RoutePermission{Path: "/api/v1/incus-ui/:backendId/*", Method: "(GET|POST|PUT|PATCH|DELETE)", Role: "member", Access: access.AccessRule{Type: access.OrgRole, Param: "backendId", MinRole: "member"}, Description: "Proxy requests to Incus UI for a backend"},

		// User terminal keys
		access.RoutePermission{Path: "/api/v1/user-terminal-keys/regenerate", Method: "POST", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "Regenerate terminal authentication key"},
		access.RoutePermission{Path: "/api/v1/user-terminal-keys/my-key", Method: "GET", Role: "member", Access: access.AccessRule{Type: access.SelfScoped}, Description: "Get current user's terminal authentication key"},
	)

	log.Println("=== Terminal module permissions registered ===")
}
