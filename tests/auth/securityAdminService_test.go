package auth_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	authDto "soli/formations/src/auth/dto"
	"soli/formations/src/auth/interfaces"
	"soli/formations/src/auth/mocks"
	securityAdminRoutes "soli/formations/src/auth/routes/securityAdminRoutes"
	services "soli/formations/src/auth/services"
	ems "soli/formations/src/entityManagement/entityManagementService"
	entityManagementInterfaces "soli/formations/src/entityManagement/interfaces"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// ============================================================================
// Test Entity for Role Matrix / Health Check Tests
// ============================================================================

// secAdminTestEntity is a minimal entity model used to register entity roles
// in the global registration service for testing GetEntityRoleMatrix and health checks.
type secAdminTestEntity struct {
	entityManagementModels.BaseModel
	Name string `json:"name"`
}

type secAdminTestInput struct {
	Name string `json:"name"`
}

type secAdminTestOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// registerTestEntity registers a test entity with given roles in the global
// entity registration service. Returns a cleanup function to unregister it.
func registerTestEntity(t *testing.T, entityName string, roles map[string]string, mockEnforcer *mocks.MockEnforcer) {
	// Save and replace the global enforcer so RegisterTypedEntity uses our mock
	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer

	ems.RegisterTypedEntity[secAdminTestEntity, secAdminTestInput, map[string]any, secAdminTestOutput](
		ems.GlobalEntityRegistrationService,
		entityName,
		entityManagementInterfaces.TypedEntityRegistration[secAdminTestEntity, secAdminTestInput, map[string]any, secAdminTestOutput]{
			Converters: entityManagementInterfaces.TypedEntityConverters[secAdminTestEntity, secAdminTestInput, map[string]any, secAdminTestOutput]{
				ModelToDto: func(entity *secAdminTestEntity) (secAdminTestOutput, error) {
					return secAdminTestOutput{ID: entity.ID.String(), Name: entity.Name}, nil
				},
				DtoToModel: func(dto secAdminTestInput) *secAdminTestEntity {
					return &secAdminTestEntity{Name: dto.Name}
				},
				DtoToMap: func(dto map[string]any) map[string]any { return dto },
			},
			Roles: entityManagementInterfaces.EntityRoles{Roles: roles},
		},
	)

	// Restore the original enforcer
	casdoor.Enforcer = originalEnforcer

	t.Cleanup(func() {
		ems.GlobalEntityRegistrationService.UnregisterEntity(entityName)
	})
}

// ============================================================================
// Test Helpers
// ============================================================================

// createTestSecurityAdminService creates a SecurityAdminService with a mock enforcer
// and an in-memory SQLite DB for testing.
func createTestSecurityAdminService(t *testing.T, mockEnforcer *mocks.MockEnforcer) *services.SecurityAdminService {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	return services.NewSecurityAdminService(mockEnforcer, db)
}

// setupControllerTest creates a gin test router with the SecurityAdminController.
// The middleware injects userRoles into the context, simulating what AuthManagement does.
func setupControllerTest(t *testing.T, mockEnforcer *mocks.MockEnforcer, userRoles []string) (*gin.Engine, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	controller := securityAdminRoutes.NewSecurityAdminController(mockEnforcer, db)

	// Simulate the auth middleware setting userRoles
	adminGroup := router.Group("/api/v1/admin/security")
	adminGroup.Use(func(ctx *gin.Context) {
		ctx.Set("userRoles", userRoles)
		ctx.Set("userId", "test-user-id")
		ctx.Next()
	})

	adminGroup.GET("/policies", controller.GetPolicyOverview)
	adminGroup.GET("/health-checks", controller.GetPolicyHealthChecks)
	adminGroup.GET("/entity-roles", controller.GetEntityRoleMatrix)

	return router, db
}

// ============================================================================
// GetPolicyOverview Tests
// ============================================================================

func TestGetPolicyOverview_ClassifiesUUIDAsUserPolicy(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	userUUID := "550e8400-e29b-41d4-a716-446655440000"

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{userUUID, "/api/v1/courses", "(GET|POST)"},
			{"member", "/api/v1/courses", "(GET)"},
		}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyOverview()

	require.NoError(t, err)
	assert.Len(t, result.UserPolicies, 1, "UUID subject should be classified as user policy")
	assert.Len(t, result.RolePolicies, 1, "Non-UUID subject should be classified as role policy")

	// Verify the UUID subject ended up in user policies
	assert.Equal(t, userUUID, result.UserPolicies[0].Subject)
	assert.Equal(t, "member", result.RolePolicies[0].Subject)
}

func TestGetPolicyOverview_RoleSubjectGoesToRolePolicies(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"administrator", "/api/v1/courses", "(GET|POST|PATCH|DELETE)"},
			{"member", "/api/v1/courses", "(GET)"},
			{"trainer", "/api/v1/sessions", "(GET|POST)"},
		}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyOverview()

	require.NoError(t, err)
	assert.Len(t, result.RolePolicies, 3, "All role-name subjects should be in role_policies")
	assert.Len(t, result.UserPolicies, 0, "No UUID subjects means no user_policies")
	assert.Equal(t, 3, result.TotalPolicies)
}

func TestGetPolicyOverview_SubjectNamePopulatedForUUID(t *testing.T) {
	// This test verifies that when a policy subject is a UUID, the service
	// resolves the user's display name and populates SubjectName.
	// RED PHASE: PolicySubject does not yet have a SubjectName field.
	// This test will fail to compile until the backend-dev adds the field.
	mockEnforcer := mocks.NewMockEnforcer()

	userUUID := "550e8400-e29b-41d4-a716-446655440000"
	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{userUUID, "/api/v1/courses", "(GET)"},
		}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyOverview()

	require.NoError(t, err)
	require.Len(t, result.UserPolicies, 1)

	// RED: SubjectName field does not exist on PolicySubject yet.
	// The backend-dev will add this field and the name resolution logic.
	// COMPILE FAILURE EXPECTED: uncomment the lines below once SubjectName is added to PolicySubject.
	assert.Equal(t, userUUID, result.UserPolicies[0].Subject)
	// assert.NotEmpty(t, result.UserPolicies[0].SubjectName,
	//     "SubjectName should be populated with the user's display name for UUID subjects")
	t.Skip("BLOCKED: PolicySubject.SubjectName field does not exist yet — will be added by backend-dev")
}

func TestGetPolicyOverview_EmptyPolicies_ReturnsEmptyArraysNotNil(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyOverview()

	require.NoError(t, err)
	assert.NotNil(t, result.RolePolicies, "RolePolicies should be empty array, not nil")
	assert.NotNil(t, result.UserPolicies, "UserPolicies should be empty array, not nil")
	assert.Len(t, result.RolePolicies, 0)
	assert.Len(t, result.UserPolicies, 0)
	assert.Equal(t, 0, result.TotalPolicies)
}

func TestGetPolicyOverview_MethodStringParsedIntoArray(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"member", "/api/v1/courses", "(GET|POST)"},
		}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyOverview()

	require.NoError(t, err)
	require.Len(t, result.RolePolicies, 1)
	require.Len(t, result.RolePolicies[0].Policies, 1)

	methods := result.RolePolicies[0].Policies[0].Methods
	assert.Contains(t, methods, "GET", "Methods should contain GET")
	assert.Contains(t, methods, "POST", "Methods should contain POST")
	assert.Len(t, methods, 2, "Should have exactly 2 methods parsed from (GET|POST)")
}

func TestGetPolicyOverview_WildcardMethodParsedCorrectly(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"administrator", "/api/v1/admin/*", "*"},
		}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyOverview()

	require.NoError(t, err)
	require.Len(t, result.RolePolicies, 1)
	require.Len(t, result.RolePolicies[0].Policies, 1)

	methods := result.RolePolicies[0].Policies[0].Methods
	assert.Contains(t, methods, "*", "Wildcard method should be preserved")
}

func TestGetPolicyOverview_MultiplePoliciesGroupedBySubject(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"member", "/api/v1/courses", "(GET)"},
			{"member", "/api/v1/sessions", "(GET|POST)"},
			{"administrator", "/api/v1/courses", "(GET|POST|PATCH|DELETE)"},
		}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyOverview()

	require.NoError(t, err)
	assert.Equal(t, 3, result.TotalPolicies)

	// Find the member policy
	var memberPolicy *authDto.PolicySubject
	for i := range result.RolePolicies {
		if result.RolePolicies[i].Subject == "member" {
			memberPolicy = &result.RolePolicies[i]
			break
		}
	}
	require.NotNil(t, memberPolicy, "Should find member policy subject")
	assert.Len(t, memberPolicy.Policies, 2, "Member should have 2 policy rules grouped together")
}

func TestGetPolicyOverview_ShortPolicySkipped(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"member", "/api/v1/courses"}, // Only 2 elements, should be skipped
			{"member", "/api/v1/sessions", "(GET)"},
		}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyOverview()

	require.NoError(t, err)
	assert.Len(t, result.RolePolicies, 1, "Short policy should be skipped")
}

// ============================================================================
// GetPolicyHealthChecks Tests
// ============================================================================

func TestGetPolicyHealthChecks_OverlyPermissiveWildcardMethod(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"trainer", "/api/v1/courses", "*"},
		}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		return []string{}, nil
	}

	// Register a test entity so Check 4 (missing admin DELETE) has something to examine
	registerTestEntity(t, "HealthWildcardTestEntity", map[string]string{
		"administrator": "(GET|POST|PATCH|DELETE)",
	}, mockEnforcer)

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	// Find the overly_permissive finding
	var found *authDto.HealthFinding
	for i := range result.Findings {
		if result.Findings[i].Category == "overly_permissive" {
			found = &result.Findings[i]
			break
		}
	}

	require.NotNil(t, found, "Should detect overly permissive wildcard policy")
	assert.Equal(t, "medium", found.Severity)
	assert.Contains(t, found.Description, "trainer")
}

func TestGetPolicyHealthChecks_OverlyPermissiveFullMethods(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"member", "/api/v1/users", "(GET|POST|PATCH|DELETE)"},
		}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		return []string{}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	var found *authDto.HealthFinding
	for i := range result.Findings {
		if result.Findings[i].Category == "overly_permissive" {
			found = &result.Findings[i]
			break
		}
	}

	require.NotNil(t, found, "Should detect (GET|POST|PATCH|DELETE) as overly permissive")
	assert.Equal(t, "medium", found.Severity)
	assert.Contains(t, found.Description, "member")
}

func TestGetPolicyHealthChecks_AdminSubjectSkippedForOverlyPermissive(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"administrator", "/api/v1/courses", "*"},
		}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		return []string{}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	for _, f := range result.Findings {
		assert.NotEqual(t, "overly_permissive", f.Category,
			"Administrator with wildcard should NOT be flagged as overly permissive")
	}
}

func TestGetPolicyHealthChecks_AdminUserCount_InfoSeverity(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	adminUUIDs := []string{
		"550e8400-e29b-41d4-a716-446655440001",
		"550e8400-e29b-41d4-a716-446655440002",
	}

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		if name == "administrator" {
			return adminUUIDs, nil
		}
		return []string{}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	var found *authDto.HealthFinding
	for i := range result.Findings {
		if result.Findings[i].Category == "admin_users" {
			found = &result.Findings[i]
			break
		}
	}

	require.NotNil(t, found, "Should have admin_users finding")
	assert.Equal(t, "info", found.Severity, "2 admins should be info severity (<=5)")
	assert.Contains(t, found.Description, "2")
}

func TestGetPolicyHealthChecks_AdminUserCount_MediumSeverity(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	// 6 admin users -> should trigger medium severity
	adminUUIDs := make([]string, 6)
	for i := 0; i < 6; i++ {
		adminUUIDs[i] = fmt.Sprintf("550e8400-e29b-41d4-a716-44665544%04d", i)
	}

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		if name == "administrator" {
			return adminUUIDs, nil
		}
		return []string{}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	var found *authDto.HealthFinding
	for i := range result.Findings {
		if result.Findings[i].Category == "admin_users" {
			found = &result.Findings[i]
			break
		}
	}

	require.NotNil(t, found, "Should have admin_users finding")
	assert.Equal(t, "medium", found.Severity, "6 admins (>5) should be medium severity")
	assert.Contains(t, found.Description, "6")
}

func TestGetPolicyHealthChecks_AdminUsersDetails_ShowsNamesNotUUIDs(t *testing.T) {
	// RED PHASE: Currently the Details field shows raw UUIDs joined by commas.
	// After implementation, it should show resolved display names instead.
	mockEnforcer := mocks.NewMockEnforcer()

	adminUUIDs := []string{
		"550e8400-e29b-41d4-a716-446655440001",
		"550e8400-e29b-41d4-a716-446655440002",
	}

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		if name == "administrator" {
			return adminUUIDs, nil
		}
		return []string{}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	var found *authDto.HealthFinding
	for i := range result.Findings {
		if result.Findings[i].Category == "admin_users" {
			found = &result.Findings[i]
			break
		}
	}

	require.NotNil(t, found, "Should have admin_users finding")

	// RED: Currently Details contains raw UUIDs. After implementation it should
	// contain resolved names like "Alice, Bob" instead of UUIDs.
	assert.NotContains(t, found.Details, "550e8400-e29b-41d4-a716-446655440001",
		"Details should show resolved display names, not raw UUIDs")
	assert.NotContains(t, found.Details, "550e8400-e29b-41d4-a716-446655440002",
		"Details should show resolved display names, not raw UUIDs")
}

func TestGetPolicyHealthChecks_UserSpecificPolicies_InfoFinding(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	userUUID := "550e8400-e29b-41d4-a716-446655440000"

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{userUUID, "/api/v1/courses/123", "(GET|PATCH|DELETE)"},
			{userUUID, "/api/v1/sessions/456", "(GET)"},
			{"member", "/api/v1/courses", "(GET)"},
		}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		return []string{}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	var found *authDto.HealthFinding
	for i := range result.Findings {
		if result.Findings[i].Category == "user_specific_policies" {
			found = &result.Findings[i]
			break
		}
	}

	require.NotNil(t, found, "Should detect user-specific policies")
	assert.Equal(t, "info", found.Severity)
	assert.Contains(t, found.Description, "2", "Should count 2 user-specific policies")
}

func TestGetPolicyHealthChecks_EntityWithoutAdminDelete_LowSeverity(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		return []string{}, nil
	}

	// Register an entity that has admin GET but NOT admin DELETE
	entityName := "NoDeleteTestEntity"
	registerTestEntity(t, entityName, map[string]string{
		"administrator": "(GET|POST|PATCH)",
		"member":        "(GET)",
	}, mockEnforcer)

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	// Find the missing_admin_delete finding for our entity
	var found *authDto.HealthFinding
	for i := range result.Findings {
		if result.Findings[i].Category == "missing_admin_delete" &&
			strings.Contains(result.Findings[i].Description, entityName) {
			found = &result.Findings[i]
			break
		}
	}

	require.NotNil(t, found, "Should detect entity without admin DELETE")
	assert.Equal(t, "low", found.Severity)
	assert.Contains(t, found.Description, entityName)
}

func TestGetPolicyHealthChecks_EntityWithAdminDelete_NoFinding(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		return []string{}, nil
	}

	entityName := "HasDeleteTestEntity"
	registerTestEntity(t, entityName, map[string]string{
		"administrator": "(GET|POST|PATCH|DELETE)",
		"member":        "(GET)",
	}, mockEnforcer)

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	for _, f := range result.Findings {
		if f.Category == "missing_admin_delete" &&
			strings.Contains(f.Description, entityName) {
			t.Errorf("Entity '%s' has admin DELETE, should not appear in missing_admin_delete findings", entityName)
		}
	}
}

func TestGetPolicyHealthChecks_SummaryCountsMatchFindings(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	userUUID := "550e8400-e29b-41d4-a716-446655440000"

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"trainer", "/api/v1/courses", "*"},      // overly_permissive -> medium
			{userUUID, "/api/v1/courses/1", "(GET)"},  // user_specific -> info
		}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		if name == "administrator" {
			return []string{"admin-uuid-1", "admin-uuid-2"}, nil
		}
		return []string{}, nil
	}

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetPolicyHealthChecks()

	require.NoError(t, err)

	// Count findings by severity manually
	expectedHigh := 0
	expectedMedium := 0
	expectedLow := 0
	expectedInfo := 0
	for _, f := range result.Findings {
		switch f.Severity {
		case "high":
			expectedHigh++
		case "medium":
			expectedMedium++
		case "low":
			expectedLow++
		case "info":
			expectedInfo++
		}
	}

	assert.Equal(t, expectedHigh, result.Summary.HighCount, "Summary high count should match findings")
	assert.Equal(t, expectedMedium, result.Summary.MediumCount, "Summary medium count should match findings")
	assert.Equal(t, expectedLow, result.Summary.LowCount, "Summary low count should match findings")
	assert.Equal(t, expectedInfo, result.Summary.InfoCount, "Summary info count should match findings")
}

// ============================================================================
// GetEntityRoleMatrix Tests
// ============================================================================

func TestGetEntityRoleMatrix_ReturnsRegisteredEntities(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	entityName := "MatrixTestEntity"
	registerTestEntity(t, entityName, map[string]string{
		"administrator": "(GET|POST|PATCH|DELETE)",
		"member":        "(GET)",
	}, mockEnforcer)

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetEntityRoleMatrix()

	require.NoError(t, err)
	require.NotNil(t, result)

	// Find our test entity
	var found *authDto.EntityRoleEntry
	for i := range result.Entities {
		if result.Entities[i].EntityName == entityName {
			found = &result.Entities[i]
			break
		}
	}

	require.NotNil(t, found, "Should find the registered test entity")
	assert.Equal(t, entityName, found.EntityName)
}

func TestGetEntityRoleMatrix_MethodsParsedIntoArrays(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	entityName := "MatrixMethodsTestEntity"
	registerTestEntity(t, entityName, map[string]string{
		"administrator": "(GET|POST|PATCH|DELETE)",
		"member":        "(GET|POST)",
	}, mockEnforcer)

	svc := createTestSecurityAdminService(t, mockEnforcer)
	result, err := svc.GetEntityRoleMatrix()

	require.NoError(t, err)

	var found *authDto.EntityRoleEntry
	for i := range result.Entities {
		if result.Entities[i].EntityName == entityName {
			found = &result.Entities[i]
			break
		}
	}

	require.NotNil(t, found)

	// Check administrator methods
	adminMethods, ok := found.RoleMethods["administrator"]
	require.True(t, ok, "Should have administrator role")
	assert.Contains(t, adminMethods, "GET")
	assert.Contains(t, adminMethods, "POST")
	assert.Contains(t, adminMethods, "PATCH")
	assert.Contains(t, adminMethods, "DELETE")
	assert.Len(t, adminMethods, 4)

	// Check member methods
	memberMethods, ok := found.RoleMethods["member"]
	require.True(t, ok, "Should have member role")
	assert.Contains(t, memberMethods, "GET")
	assert.Contains(t, memberMethods, "POST")
	assert.Len(t, memberMethods, 2)
}

// ============================================================================
// Controller Admin Role Check Tests
// ============================================================================

func TestController_NonAdminUser_Gets403(t *testing.T) {
	// RED PHASE: The controller currently does NOT check userRoles for "administrator".
	// It relies only on the Casbin middleware. This test verifies that the controller
	// itself should explicitly reject non-admin users even if the middleware is bypassed.
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{
			{"member", "/api/v1/courses", "(GET)"},
		}, nil
	}

	// Set up router with non-admin roles
	router, _ := setupControllerTest(t, mockEnforcer, []string{"member"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/security/policies", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"Non-admin user should get 403 even if Casbin middleware is bypassed")
}

func TestController_AdminUser_Gets200(t *testing.T) {
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}

	// Set up router with admin role
	router, _ := setupControllerTest(t, mockEnforcer, []string{"administrator"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/security/policies", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"Administrator should have access to security admin endpoints")

	// Verify the response is valid JSON
	var result authDto.PolicyOverviewOutput
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err, "Response should be valid PolicyOverviewOutput JSON")
}

func TestController_NoRolesInContext_Gets403(t *testing.T) {
	// RED PHASE: Tests that when no userRoles are set, the controller rejects
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	controller := securityAdminRoutes.NewSecurityAdminController(mockEnforcer, db)

	// NO middleware injecting userRoles
	adminGroup := router.Group("/api/v1/admin/security")
	adminGroup.GET("/policies", controller.GetPolicyOverview)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/security/policies", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"Request without userRoles should get 403")
}

func TestController_HealthChecks_NonAdminGets403(t *testing.T) {
	// RED PHASE: Same admin check for health-checks endpoint
	mockEnforcer := mocks.NewMockEnforcer()

	mockEnforcer.GetPolicyFunc = func() ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetUsersForRoleFunc = func(name string) ([]string, error) {
		return []string{}, nil
	}

	router, _ := setupControllerTest(t, mockEnforcer, []string{"member", "trainer"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/security/health-checks", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"Non-admin user should get 403 on health-checks endpoint")
}

// ============================================================================
// Enforcer Interface Compliance
// ============================================================================

func TestMockEnforcer_ImplementsInterface(t *testing.T) {
	// Ensures the mock enforcer is always up to date with the interface
	var _ interfaces.EnforcerInterface = (*mocks.MockEnforcer)(nil)
}
