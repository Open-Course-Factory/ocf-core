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

// Platform role names used as RoutePermission.Role values. They match the
// existing Casbin role strings — modules should use these constants instead of
// repeating the "member" / "administrator" literals.
const (
	RoleMember        = "member"
	RoleAdministrator = "administrator"
)

// RoutePermission declares the full authorization contract for a single route.
// Both Layer 1 (role-based gateway) and Layer 2 (business logic) are described here.
type RoutePermission struct {
	Path        string     `json:"path"`
	Method      string     `json:"method"`
	Category    string     `json:"category"`
	Role        string     `json:"role"` // "member" or "administrator"
	Access      AccessRule `json:"access"`
	Description string     `json:"description"`
	// CasbinPath overrides the Layer 1 policy path when it must differ from the
	// Gin route pattern used for the Layer 2 registry lookup (e.g. keyMatch2
	// wants /* where the Gin pattern is /*path).
	CasbinPath string `json:"casbin_path,omitempty"`
	// NoGateway declares the route in the registry but registers NO Casbin
	// policy — for routes mounted without AuthManagement().
	NoGateway bool `json:"no_gateway,omitempty"`
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
	// ArrayOwner marks OwnerField as an array column (e.g., BaseModel.OwnerIDs,
	// column "owner_ids") rather than a scalar owner column. Read scoping then
	// matches rows whose array CONTAINS the caller instead of equals the caller.
	ArrayOwner bool `json:"array_owner,omitempty"`
}

// VisibilityScopeConfig declares a boolean-flag read scope on a generic entity:
// non-admin callers (including unauthenticated ones) only see rows whose Field
// is true; admins bypass and see every row. Unlike OwnershipConfig this is not
// keyed on the caller's identity — a missing userId still sees the visible rows
// (e.g. the public pricing page listing catalog plans). The generic GET handlers
// enforce it: list filters to visible rows, get-by-id returns 404 for a hidden
// row so its existence is never disclosed.
type VisibilityScopeConfig struct {
	// Field is the entity's bool struct field, e.g. "IsCatalog". A row is
	// visible to non-admins when this field is true.
	Field string `json:"field"`
}
