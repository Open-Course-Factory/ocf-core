package routes

import (
	"log"

	"soli/formations/src/auth/interfaces"
)

// RegisterGroupPermissions registers Casbin policies for custom group routes.
// CRUD policies for the ClassGroup entity are registered automatically via
// entity registration. There are currently no custom group routes — this
// function is kept as a no-op to preserve its call site in main.go for
// future extensions.
func RegisterGroupPermissions(_ interfaces.EnforcerInterface) {
	log.Println("=== Registering group custom route permissions (none) ===")
}
