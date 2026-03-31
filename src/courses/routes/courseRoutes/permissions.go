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

	// Course custom routes - available to all authenticated members
	courseMemberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/courses/git", "POST"},
		{"/api/v1/courses/source", "POST"},
		{"/api/v1/courses/generate", "POST"},
		{"/api/v1/courses/versions", "GET"},
		{"/api/v1/courses/by-version", "GET"},
	}

	for _, route := range courseMemberRoutes {
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// Generation custom routes - available to all authenticated members
	generationMemberRoutes := []struct {
		path   string
		method string
	}{
		{"/api/v1/generations", "GET"},
		{"/api/v1/generations", "POST"},
		{"/api/v1/generations/:id", "DELETE"},
		{"/api/v1/generations/:id/status", "GET"},
		{"/api/v1/generations/:id/download", "GET"},
		{"/api/v1/generations/:id/retry", "POST"},
	}

	for _, route := range generationMemberRoutes {
		access.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// --- Route Registry: declarative permission metadata ---

	access.RouteRegistry.Register("Courses",
		access.RoutePermission{
			Path: "/api/v1/courses/git", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Create a course from a Git repository",
		},
		access.RoutePermission{
			Path: "/api/v1/courses/source", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Create a course from uploaded source",
		},
		access.RoutePermission{
			Path: "/api/v1/courses/generate", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Trigger course generation",
		},
		access.RoutePermission{
			Path: "/api/v1/courses/versions", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "List available course versions",
		},
		access.RoutePermission{
			Path: "/api/v1/courses/by-version", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Get a course by version",
		},
	)

	access.RouteRegistry.Register("Generations",
		access.RoutePermission{
			Path: "/api/v1/generations", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "List generation jobs",
		},
		access.RoutePermission{
			Path: "/api/v1/generations", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Create a new generation job",
		},
		access.RoutePermission{
			Path: "/api/v1/generations/:id", Method: "DELETE",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Delete a generation job",
		},
		access.RoutePermission{
			Path: "/api/v1/generations/:id/status", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Get generation job status",
		},
		access.RoutePermission{
			Path: "/api/v1/generations/:id/download", Method: "GET",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Download generation output",
		},
		access.RoutePermission{
			Path: "/api/v1/generations/:id/retry", Method: "POST",
			Role: "member", Access: access.AccessRule{Type: access.Public},
			Description: "Retry a failed generation job",
		},
	)

	log.Println("=== Course and generation permissions setup completed ===")
}
