package terminalTrainer_tests

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	terminalController "soli/formations/src/terminalTrainer/routes"
)

// TestRouteRegistration_NoParamConflicts verifies that ALL terminal routes can
// be registered on a single Gin engine without causing a panic from conflicting
// parameter names. This is the most direct regression test: it mirrors what
// happens at real server startup.
//
// The generic entity management system registers routes like:
//   GET /api/v1/organizations/:id
//   GET /api/v1/class-groups/:id
//
// Custom terminal routes MUST also use ":id" for the same path segment.
// Using ":orgId" or ":groupId" causes a Gin router panic at startup.
func TestRouteRegistration_NoParamConflicts(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	apiGroup := router.Group("/api/v1")

	// Stub generic entity routes (simulating routeGenerator which always uses :id)
	apiGroup.GET("/organizations/:id", func(c *gin.Context) {})
	apiGroup.GET("/class-groups/:id", func(c *gin.Context) {})

	// This must NOT panic — if it does, there's a param name mismatch
	require.NotPanics(t, func() {
		terminalController.TerminalRoutes(apiGroup, nil, db)
	}, "TerminalRoutes registration panicked due to route parameter name conflict — "+
		"a custom route likely uses a non-standard param name (e.g. :orgId, :groupId) "+
		"instead of :id")

	// Verify the router is functional after registration
	req := httptest.NewRequest("GET", "/api/v1/terminals/user-sessions", nil)
	w := httptest.NewRecorder()

	require.NotPanics(t, func() {
		router.ServeHTTP(w, req)
	}, "Router should handle requests without panicking after route registration")
}
