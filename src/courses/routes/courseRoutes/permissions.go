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

	log.Println("=== Course and generation permissions setup completed ===")
}
