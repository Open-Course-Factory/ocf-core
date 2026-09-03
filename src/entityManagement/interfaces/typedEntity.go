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
	// VisibilityScope declares a boolean-flag read scope: non-admin callers only
	// see rows whose named bool field is true, while admins see all. The generic
	// GET handlers enforce it (list filter + get-by-id 404). Unlike
	// OwnershipConfig it is not keyed on the caller's identity, so an
	// unauthenticated caller still sees the visible rows.
	VisibilityScope *access.VisibilityScopeConfig `json:"-"`
	// Archivable opts the entity into the framework's archiving capability. The
	// model must embed models.Archivable (checked at boot, panics otherwise).
	// The framework then synthesizes POST /:id/archive and /:id/unarchive item
	// actions whose access rule is derived from the entity's PATCH permission,
	// fires the Before/After Archive/Unarchive hooks around the write, and hides
	// archived rows from the generic list unless ?include_archived=true.
	// Get-by-id is unaffected. Who may archive, and what an archived row still
	// permits, is the entity's business: guard it with a BeforeArchive hook.
	Archivable bool `json:"-"`
	// Actions declares custom REST actions beyond the generated CRUD verbs. Each
	// is mounted by the route generator and gets its Layer 1 / Layer 2 policies
	// registered from Role/Access at registration time.
	Actions []ActionConfig `json:"-"`
}
