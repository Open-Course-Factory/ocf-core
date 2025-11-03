package services

import (
	"testing"

	"soli/formations/src/auth/casdoor"
	authDto "soli/formations/src/auth/dto"
	"soli/formations/src/auth/mocks"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create tables
	db.Exec(`
		CREATE TABLE organizations (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL
		)
	`)

	db.Exec(`
		CREATE TABLE organization_members (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			is_active BOOLEAN NOT NULL DEFAULT 1
		)
	`)

	db.Exec(`
		CREATE TABLE class_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL
		)
	`)

	db.Exec(`
		CREATE TABLE group_members (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'member',
			is_active BOOLEAN NOT NULL DEFAULT 1
		)
	`)

	db.Exec(`
		CREATE TABLE subscription_plans (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			features TEXT, -- JSON array
			priority INTEGER DEFAULT 0,
			is_active BOOLEAN DEFAULT 1,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`)

	db.Exec(`
		CREATE TABLE organization_subscriptions (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			subscription_plan_id TEXT NOT NULL,
			status TEXT DEFAULT 'active',
			current_period_start DATETIME,
			current_period_end DATETIME,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME,
			FOREIGN KEY (organization_id) REFERENCES organizations(id),
			FOREIGN KEY (subscription_plan_id) REFERENCES subscription_plans(id)
		)
	`)

	return db
}

func TestUserPermissionsService_GetUserPermissions_BasicPermissions(t *testing.T) {
	// Setup test database
	db := setupTestDB(t)

	// Setup mock enforcer
	mockEnforcer := mocks.NewMockEnforcer()

	// Mock Casbin responses
	mockEnforcer.GetImplicitPermissionsForUserFunc = func(name string) ([][]string, error) {
		return [][]string{
			{"user123", "/api/v1/groups/abc", "GET"},
			{"user123", "/api/v1/organizations/def", "(GET|POST)"},
		}, nil
	}

	mockEnforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		return []string{"student", "member"}, nil
	}

	// Replace global enforcer with mock
	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = originalEnforcer }()

	// Create service
	service := NewUserPermissionsService(db)

	// Test
	result, err := service.GetUserPermissions("user123")

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "user123", result.UserID)
	assert.Len(t, result.Permissions, 2)
	assert.Len(t, result.Roles, 2)
	assert.False(t, result.IsSystemAdmin)
	assert.True(t, result.CanCreateOrganization) // All users can create orgs
}

func TestUserPermissionsService_GetUserPermissions_WithOrganizations(t *testing.T) {
	// Setup test database
	db := setupTestDB(t)

	// Insert test data
	orgID := uuid.New().String()
	memberID := uuid.New().String()

	db.Exec("INSERT INTO organizations (id, name, display_name) VALUES (?, ?, ?)",
		orgID, "test-org", "Test Organization")

	db.Exec("INSERT INTO organization_members (id, organization_id, user_id, role, is_active) VALUES (?, ?, ?, ?, ?)",
		memberID, orgID, "user123", "owner", true)

	// Setup mock enforcer
	mockEnforcer := mocks.NewMockEnforcer()
	mockEnforcer.GetImplicitPermissionsForUserFunc = func(name string) ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		return []string{"member"}, nil
	}

	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = originalEnforcer }()

	// Create service
	service := NewUserPermissionsService(db)

	// Test
	result, err := service.GetUserPermissions("user123")

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.OrganizationMemberships, 1)
	assert.Equal(t, "Test Organization", result.OrganizationMemberships[0].OrganizationName)
	assert.Equal(t, "owner", result.OrganizationMemberships[0].Role)
	assert.True(t, result.OrganizationMemberships[0].IsOwner)
	assert.True(t, result.CanCreateGroup) // Can create groups because member of org
}

func TestUserPermissionsService_GetUserPermissions_WithGroups(t *testing.T) {
	// Setup test database
	db := setupTestDB(t)

	// Insert test data
	groupID := uuid.New().String()
	memberID := uuid.New().String()

	db.Exec("INSERT INTO class_groups (id, name, display_name) VALUES (?, ?, ?)",
		groupID, "test-group", "Test Group")

	db.Exec("INSERT INTO group_members (id, group_id, user_id, role, is_active) VALUES (?, ?, ?, ?, ?)",
		memberID, groupID, "user123", "member", true)

	// Setup mock enforcer
	mockEnforcer := mocks.NewMockEnforcer()
	mockEnforcer.GetImplicitPermissionsForUserFunc = func(name string) ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		return []string{"member"}, nil
	}

	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = originalEnforcer }()

	// Create service
	service := NewUserPermissionsService(db)

	// Test
	result, err := service.GetUserPermissions("user123")

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.GroupMemberships, 1)
	assert.Equal(t, "Test Group", result.GroupMemberships[0].GroupName)
	assert.Equal(t, "member", result.GroupMemberships[0].Role)
	assert.False(t, result.GroupMemberships[0].IsOwner)
}

func TestUserPermissionsService_GetUserPermissions_SystemAdmin(t *testing.T) {
	// Setup test database
	db := setupTestDB(t)

	// Setup mock enforcer
	mockEnforcer := mocks.NewMockEnforcer()
	mockEnforcer.GetImplicitPermissionsForUserFunc = func(name string) ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		return []string{"administrator"}, nil
	}

	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = originalEnforcer }()

	// Create service
	service := NewUserPermissionsService(db)

	// Test
	result, err := service.GetUserPermissions("admin123")

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsSystemAdmin)
}

func TestUserPermissionsService_GetUserPermissions_WithSubscriptionFeatures(t *testing.T) {
	// Setup test database
	db := setupTestDB(t)

	// Insert test data
	orgID := uuid.New().String()
	memberID := uuid.New().String()
	planID := uuid.New().String()
	subID := uuid.New().String()

	// Create organization
	db.Exec("INSERT INTO organizations (id, name, display_name) VALUES (?, ?, ?)",
		orgID, "premium-org", "Premium Organization")

	// Create subscription plan with features
	db.Exec("INSERT INTO subscription_plans (id, name, features, priority) VALUES (?, ?, ?, ?)",
		planID, "Premium Plan", `["advanced_labs", "custom_themes", "priority_support"]`, 20)

	// Create organization subscription
	db.Exec(`INSERT INTO organization_subscriptions
		(id, organization_id, subscription_plan_id, status, current_period_start, current_period_end)
		VALUES (?, ?, ?, ?, datetime('now'), datetime('now', '+1 month'))`,
		subID, orgID, planID, "active")

	// Create organization membership
	db.Exec("INSERT INTO organization_members (id, organization_id, user_id, role, is_active) VALUES (?, ?, ?, ?, ?)",
		memberID, orgID, "user123", "member", true)

	// Setup mock enforcer
	mockEnforcer := mocks.NewMockEnforcer()
	mockEnforcer.GetImplicitPermissionsForUserFunc = func(name string) ([][]string, error) {
		return [][]string{}, nil
	}
	mockEnforcer.GetRolesForUserFunc = func(name string) ([]string, error) {
		return []string{"member"}, nil
	}

	originalEnforcer := casdoor.Enforcer
	casdoor.Enforcer = mockEnforcer
	defer func() { casdoor.Enforcer = originalEnforcer }()

	// Create service
	service := NewUserPermissionsService(db)

	// Test
	result, err := service.GetUserPermissions("user123")

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.OrganizationMemberships, 1)

	// Check organization details
	org := result.OrganizationMemberships[0]
	assert.Equal(t, "Premium Organization", org.OrganizationName)
	assert.Equal(t, "member", org.Role)
	assert.False(t, org.IsOwner)
	assert.True(t, org.HasSubscription)

	// Check features are loaded
	assert.Len(t, org.Features, 3)
	assert.Contains(t, org.Features, "advanced_labs")
	assert.Contains(t, org.Features, "custom_themes")
	assert.Contains(t, org.Features, "priority_support")

	// Check aggregated features
	assert.Len(t, result.AggregatedFeatures, 3)
	assert.Contains(t, result.AggregatedFeatures, "advanced_labs")
	assert.True(t, result.HasAnySubscription)
}

func TestCasbinPermissionToRule(t *testing.T) {
	tests := []struct {
		name       string
		permission []string
		expected   *authDto.PermissionRule
	}{
		{
			name:       "Single method",
			permission: []string{"user123", "/api/v1/groups/:id", "GET"},
			expected: &authDto.PermissionRule{
				Resource: "/api/v1/groups/:id",
				Methods:  []string{"GET"},
			},
		},
		{
			name:       "Multiple methods",
			permission: []string{"user123", "/api/v1/organizations/:id", "(GET|POST|DELETE)"},
			expected: &authDto.PermissionRule{
				Resource: "/api/v1/organizations/:id",
				Methods:  []string{"GET", "POST", "DELETE"},
			},
		},
		{
			name:       "Invalid permission - too few fields",
			permission: []string{"user123", "/api/v1/groups/:id"},
			expected:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := authDto.CasbinPermissionToRule(tt.permission)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.Resource, result.Resource)
				assert.Equal(t, tt.expected.Methods, result.Methods)
			}
		})
	}
}
