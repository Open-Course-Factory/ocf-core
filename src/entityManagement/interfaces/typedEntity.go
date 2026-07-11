package entityManagementInterfaces

import (
	"github.com/google/uuid"

	access "soli/formations/src/auth/access"
)

// EntityModel is a constraint satisfied by any model that embeds BaseModel.
type EntityModel interface {
	GetID() uuid.UUID
}

// TypedEntityConverters holds type-safe converter functions for an entity.
// M = model, C = create DTO, E = edit DTO, O = output DTO.
type TypedEntityConverters[M any, C any, E any, O any] struct {
	ModelToDto func(*M) (O, error)
	DtoToModel func(C) *M
	DtoToMap   func(E) map[string]any
}

// TypedEntityRegistration holds the full registration config for an entity.
type TypedEntityRegistration[M any, C any, E any, O any] struct {
	Converters          TypedEntityConverters[M, C, E, O]
	Roles               EntityRoles
	SubEntities         []any
	SwaggerConfig       *EntitySwaggerConfig
	RelationshipFilters []RelationshipFilter
	MembershipConfig    *MembershipConfig
	DefaultIncludes     []string
	// OwnershipConfig declares the owner field and the operations it guards.
	// RegisterOwnershipHooks(db) (called once at startup) reads these declarations
	// and wires the write-side hooks: "create" forces the owner to the caller,
	// "update"/"delete" verify ownership. The "read" op enables request-time read
	// scoping in the generic GET handlers instead. No hand-written hook needed.
	OwnershipConfig *access.OwnershipConfig `json:"-"`
	// Actions declares custom REST actions beyond the generated CRUD verbs. Each
	// is mounted by the route generator and gets its Layer 1 / Layer 2 policies
	// registered from Role/Access at registration time.
	Actions []ActionConfig `json:"-"`
}
