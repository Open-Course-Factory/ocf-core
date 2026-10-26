package brandingRoutes

import (
	"soli/formations/src/auth/access"
	"soli/formations/src/auth/interfaces"
)

func RegisterBrandingPermissions(enforcer interfaces.EnforcerInterface) {
	access.RegisterEnforced(enforcer, "Branding",
		access.RoutePermission{
			Path: "/api/v1/branding", Method: "GET", NoGateway: true,
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "View the instance name, colors and logo version (no sign-in needed)",
		},
		access.RoutePermission{
			Path: "/api/v1/branding/logo", Method: "GET", NoGateway: true,
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Download the instance logo (no sign-in needed)",
		},
		access.RoutePermission{
			Path: "/api/v1/branding", Method: "PUT", Role: access.RoleAdministrator,
			Access:      access.AccessRule{Type: access.AdminOnly},
			Description: "Update the instance logo, name and colors (administrator only)",
		},
	)
}
