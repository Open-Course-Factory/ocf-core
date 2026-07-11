package entityManagementInterfaces

import (
	access "soli/formations/src/auth/access"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ActionScope selects the URL shape of a custom entity action: an item action
// targets a single instance and mounts under /:id, a collection action targets
// the whole collection and mounts directly under the entity base path.
type ActionScope string

const (
	ActionScopeItem       ActionScope = "item"
	ActionScopeCollection ActionScope = "collection"
)

// ActionHandlerFactory builds a gin handler (or middleware) bound to the request
// database handle at mount time. Actions use factories rather than plain
// gin.HandlerFunc so the route generator can inject its *gorm.DB the same way the
// generic CRUD controller receives it.
type ActionHandlerFactory func(db *gorm.DB) gin.HandlerFunc

// PlanRequirement declares, in a dependency-free form, which plan-gating
// middlewares a route needs. The payment layer turns it into a concrete gin
// chain via PlanChain. Keeping this struct free of any payment import lets an
// entity registration (and the swagger route generator) reference it without
// creating a swagger→payment import cycle.
type PlanRequirement struct {
	// OrgContext resolves organization_id from the request into the context so
	// plan resolution and the handler can scope to the org.
	OrgContext bool
	// RequirePlan resolves the caller's effective plan and rejects the request
	// (403) when no active plan resolves.
	RequirePlan bool
	// CheckHostRAM verifies the terminal backend has RAM headroom for the
	// requested session size before the handler runs.
	CheckHostRAM bool
}

// ActionConfig declares a custom REST action on an entity registration, in
// addition to the generated CRUD verbs. The route generator mounts each action
// at <basePath>[/:id]/<Name> and the registration service registers its Layer 1
// Casbin policy and Layer 2 RoutePermission from Role/Access.
type ActionConfig struct {
	Name        string
	Method      string
	Scope       ActionScope
	Handler     ActionHandlerFactory
	Middlewares []ActionHandlerFactory
	Role        string
	Access      access.AccessRule
	Description string
	Swagger     *SwaggerOperation
	RequestDTO  any
	ResponseDTO any
	// Plan, when non-nil, gates the action behind the plan-gating chain built
	// from this requirement. A nil Plan means the action carries no plan gate.
	Plan *PlanRequirement
}
