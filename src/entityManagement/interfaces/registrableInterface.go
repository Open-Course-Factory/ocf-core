package entityManagementInterfaces

import (
	"net/http"
	"soli/formations/src/auth/models"

	"github.com/mitchellh/mapstructure"
)

type EntityRegistrationInput struct {
	EntityInterface     any
	EntityConverters    EntityConverters
	EntityDtos          EntityDtos
	EntityRoles         EntityRoles
	EntitySubEntities   []any
	SwaggerConfig       *EntitySwaggerConfig `json:"swagger_config,omitempty"`
	RelationshipFilters []RelationshipFilter
	MembershipConfig    *MembershipConfig `json:"membership_config,omitempty"` // NEW: Generic membership filtering
	DefaultIncludes     []string          `json:"default_includes,omitempty"`  // Relations to always preload (e.g., ["Members"])
}

type EntityConverters struct {
	ModelToDto any
	DtoToModel any
	DtoToMap   any
}

type EntityDtos struct {
	InputCreateDto any
	InputEditDto   any
	OutputDto      any
}

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
	FeatureProvider  FeatureProvider // Optional: provider for fetching entity-specific features (e.g., subscription features)
}

type RegistrableInterface interface {
	GetEntityRegistrationInput() EntityRegistrationInput
	EntityModelToEntityOutput(input any) (any, error)
	EntityInputDtoToEntityModel(input any) any
	GetEntityRoles() EntityRoles
}

type AbstractRegistrableInterface struct{ RegistrableInterface }

func (a AbstractRegistrableInterface) GetEntityRoles() EntityRoles {
	roleMap := make(map[string]string)
	roleMap[string(models.Member)] = "(" + http.MethodGet + "|" + http.MethodPost + ")"
	res := EntityRoles{
		Roles: roleMap,
	}
	return res
}

func (a AbstractRegistrableInterface) EntityDtoToMap(input any) map[string]any {
	resMap := make(map[string]any)

	config := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           &resMap,
	}

	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		panic(err)
	}

	decoder.Decode(input)

	return resMap
}
