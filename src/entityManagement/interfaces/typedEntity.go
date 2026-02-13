package entityManagementInterfaces

import "github.com/google/uuid"

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

// TypedEntityRegistration is the type-safe replacement for EntityRegistrationInput.
type TypedEntityRegistration[M any, C any, E any, O any] struct {
	Converters          TypedEntityConverters[M, C, E, O]
	Roles               EntityRoles
	SubEntities         []any
	SwaggerConfig       *EntitySwaggerConfig
	RelationshipFilters []RelationshipFilter
	MembershipConfig    *MembershipConfig
	DefaultIncludes     []string
}
