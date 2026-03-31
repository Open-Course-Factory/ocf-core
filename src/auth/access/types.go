package access

// AccessRuleType defines how Layer 2 authorization is enforced on a route.
// This is a string type — plugins can define new values without modifying this file.
// Register a handler for custom types via RegisterAccessEnforcer().
//
// Example for a plugin:
//
//	const TenantScoped access.AccessRuleType = "tenant_scoped"
//	access.RegisterAccessEnforcer(TenantScoped, func(ctx *gin.Context, rule access.AccessRule, ...) bool { ... })
type AccessRuleType string

// Built-in access rule types. These are the defaults provided by the framework.
// Projects can define additional types as plain string constants and register
// their enforcement handlers via RegisterAccessEnforcer().
const (
	// Public means the RBAC role check is sufficient — no additional Layer 2 check.
	Public AccessRuleType = "public"
	// AdminOnly means Layer 2 enforces that only administrators can access the route.
	AdminOnly AccessRuleType = "admin_only"
	// SelfScoped means the handler operates on the authenticated user's own data (userId from JWT).
	// WARNING: The enforcement middleware does NOT enforce self-scoping — it is a documentation
	// marker only. Each handler must verify userId scoping independently.
	SelfScoped AccessRuleType = "self_scoped"
	// EntityOwner means the handler verifies the user owns the specific entity instance.
	EntityOwner AccessRuleType = "entity_owner"
	// GroupRole means the handler checks the user's role within a class group.
	GroupRole AccessRuleType = "group_role"
	// OrgRole means the handler checks the user's role within an organization.
	OrgRole AccessRuleType = "org_role"
)

// AccessRule describes the Layer 2 authorization check applied to a route.
type AccessRule struct {
	Type    AccessRuleType `json:"type"`
	Entity  string         `json:"entity,omitempty"`   // For EntityOwner: which entity (e.g., "Terminal")
	Field   string         `json:"field,omitempty"`    // For EntityOwner: ownership field (e.g., "UserID")
	MinRole string         `json:"min_role,omitempty"` // For GroupRole/OrgRole: minimum required role
	Param   string         `json:"param,omitempty"`    // For GroupRole/OrgRole: URL param containing the ID
}

// RoutePermission declares the full authorization contract for a single route.
// Both Layer 1 (role-based gateway) and Layer 2 (business logic) are described here.
type RoutePermission struct {
	Path        string     `json:"path"`
	Method      string     `json:"method"`
	Category    string     `json:"category"`
	Role        string     `json:"role"` // "member" or "administrator"
	Access      AccessRule `json:"access"`
	Description string     `json:"description"`
}

// EntityCRUDPermissions declares the Layer 2 rules for a generic entity's CRUD operations.
type EntityCRUDPermissions struct {
	Entity string     `json:"entity"`
	Create AccessRule `json:"create"`
	Read   AccessRule `json:"read"`
	Update AccessRule `json:"update"`
	Delete AccessRule `json:"delete"`
}

// PermissionCategory groups routes for the reference page display.
type PermissionCategory struct {
	Name   string            `json:"name"`
	Routes []RoutePermission `json:"routes"`
}

// PermissionReference is the full response for the permissions reference endpoint.
type PermissionReference struct {
	Categories []PermissionCategory `json:"categories"`
	Entities   []EntityCRUDPermissions `json:"entities"`
}

// OwnershipConfig declares how ownership is enforced on a generic entity.
type OwnershipConfig struct {
	OwnerField  string   `json:"owner_field"`  // e.g., "UserID"
	Operations  []string `json:"operations"`   // e.g., ["create", "update", "delete"]
	AdminBypass bool     `json:"admin_bypass"` // whether admins skip ownership checks
}
