package courseController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	access "soli/formations/src/auth/access"
)

// RegisterCoursePermissions registers Casbin permissions for course and generation routes.
// Course routes are member-accessible (fine-grained checks happen in controllers).
func RegisterCoursePermissions(enforcer interfaces.EnforcerInterface) {
	log.Println("=== Setting up course and generation permissions ===")

	access.RegisterEnforced(enforcer, "Courses",
		access.RoutePermission{
			Path: "/api/v1/courses/git", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Create a course from a Git repository",
		},
		access.RoutePermission{
			Path: "/api/v1/courses/source", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Create a course from uploaded source",
		},
		access.RoutePermission{
			Path: "/api/v1/courses/generate", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Trigger course generation",
		},
		access.RoutePermission{
			Path: "/api/v1/courses/versions", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "List available course versions",
		},
		access.RoutePermission{
			Path: "/api/v1/courses/by-version", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Get a course by version",
		},
	)

	access.RegisterEnforced(enforcer, "Generations",
		access.RoutePermission{
			Path: "/api/v1/generations", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "List generation jobs",
		},
		access.RoutePermission{
			Path: "/api/v1/generations", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Create a new generation job",
		},
		access.RoutePermission{
			Path: "/api/v1/generations/:id", Method: "DELETE",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Delete a generation job",
		},
		access.RoutePermission{
			Path: "/api/v1/generations/:id/status", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Get generation job status",
		},
		access.RoutePermission{
			Path: "/api/v1/generations/:id/download", Method: "GET",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Download generation output",
		},
		access.RoutePermission{
			Path: "/api/v1/generations/:id/retry", Method: "POST",
			Role: access.RoleMember, Access: access.AccessRule{Type: access.Public},
			Description: "Retry a failed generation job",
		},
	)

	log.Println("=== Course and generation permissions setup completed ===")
}
