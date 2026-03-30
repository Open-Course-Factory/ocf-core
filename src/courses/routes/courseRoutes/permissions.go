package courseController

import (
	"log"

	"soli/formations/src/auth/interfaces"
	casbinUtils "soli/formations/src/auth/casbin"
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
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
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
		casbinUtils.ReconcilePolicy(enforcer, "member", route.path, route.method)
	}

	// --- Route Registry: declarative permission metadata ---

	casbinUtils.RouteRegistry.Register("Courses",
		casbinUtils.RoutePermission{
			Path: "/api/v1/courses/git", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Create a course from a Git repository",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/courses/source", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Create a course from uploaded source",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/courses/generate", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Trigger course generation",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/courses/versions", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "List available course versions",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/courses/by-version", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Get a course by version",
		},
	)

	casbinUtils.RouteRegistry.Register("Generations",
		casbinUtils.RoutePermission{
			Path: "/api/v1/generations", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "List generation jobs",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/generations", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Create a new generation job",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/generations/:id", Method: "DELETE",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Delete a generation job",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/generations/:id/status", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Get generation job status",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/generations/:id/download", Method: "GET",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Download generation output",
		},
		casbinUtils.RoutePermission{
			Path: "/api/v1/generations/:id/retry", Method: "POST",
			CasbinRole: "member", Access: casbinUtils.AccessRule{Type: casbinUtils.Public},
			Description: "Retry a failed generation job",
		},
	)

	log.Println("=== Course and generation permissions setup completed ===")
}
