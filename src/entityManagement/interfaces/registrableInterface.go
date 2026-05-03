package entityManagementInterfaces

type EntityRoles struct {
	Roles map[string]string
}

// RelationshipFilter defines how to filter an entity through nested relationships
type RelationshipFilter struct {
	FilterName   string             // e.g., "courseId" - the query parameter name
	Path         []RelationshipStep // The path of joins to reach the target
	TargetColumn string             // e.g., "id" - the column on the final table to compare
}

// RelationshipStep defines one step in a relationship path
type RelationshipStep struct {
	JoinTable    string // e.g., "chapter_sections"
	SourceColumn string // e.g., "section_id" - column that references current entity
	TargetColumn string // e.g., "chapter_id" - column that references next entity
	NextTable    string // e.g., "chapters" - the next table in the chain
}

// FeatureProvider defines an interface for fetching features associated with an entity
// This enables generic feature retrieval for any entity type (organizations, subscriptions, etc.)
type FeatureProvider interface {
	GetFeatures(entityID string) ([]string, bool, error) // Returns (features, hasSubscription, error)
}

// OrgIDViaParent describes how to resolve organization_id when the entity
// table does not have its own organization_id column and instead inherits
// org scoping from a parent table linked via EntityIDColumn.
//
// Example: group_members has no organization_id column; the org id lives on
// the parent class_groups row. The membership filter joins through the parent
// table to resolve org access.
type OrgIDViaParent struct {
	ParentTable       string // e.g., "class_groups" - the parent table holding organization_id
	ParentJoinColumn  string // e.g., "id" - column on the parent table joined to EntityIDColumn (typically the parent's primary key)
	ParentOrgIDColumn string // e.g., "organization_id" - column on the parent table holding the organization id
}

// MembershipConfig defines how to filter entities based on user membership
// This enables automatic access control for entities with membership relationships
type MembershipConfig struct {
	MemberTable      string          // e.g., "organization_members" or "group_members"
	EntityIDColumn   string          // e.g., "organization_id" or "group_id" - column linking to entity
	UserIDColumn     string          // e.g., "user_id" - column containing user ID
	RoleColumn       string          // e.g., "role" - column containing user role (optional)
	ManagerRoles     []string        // e.g., ["owner", "manager"] - roles that grant access to all child resources (optional)
	IsActiveColumn   string          // e.g., "is_active" - column for active status check (optional, defaults to "is_active")
	OrgAccessEnabled bool            // If true, also grant access via organization membership (for nested entities like groups)
	OrgIDViaParent   *OrgIDViaParent // Optional: resolve organization_id via a parent table when the entity table itself has no organization_id column
	FeatureProvider  FeatureProvider // Optional: provider for fetching entity-specific features (e.g., subscription features)
}
