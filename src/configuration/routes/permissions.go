package routes

import (
	"soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

func RegisterBrandingPermissions(enforcer interfaces.EnforcerInterface) {
	access.RegisterEnforced(enforcer, "Branding", access.RoutePermission{
		Path: "/api/v1/branding", Method: "PUT", Role: access.RoleAdministrator,
		Access:      access.AccessRule{Type: access.AdminOnly},
		Description: "Update the instance logo, name and colors (administrator only)",
	})
}
