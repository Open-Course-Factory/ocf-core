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
}
