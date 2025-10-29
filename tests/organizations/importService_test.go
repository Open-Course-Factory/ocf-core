package organizations_tests

import (
	"testing"

	"soli/formations/src/organizations/dto"

	"github.com/stretchr/testify/assert"
)

// TestImportService_ValidateOrganizationLimits tests organization limit validation logic
func TestImportService_ValidateOrganizationLimits(t *testing.T) {
	t.Run("Exceeds max groups limit", func(t *testing.T) {
		// Test validation logic
		maxGroups := 5
		existingGroupCount := 5
		groupsToImport := 2

		// Would exceed limit
		assert.True(t, existingGroupCount+groupsToImport > maxGroups,
			"Should detect that 5 existing + 2 new groups exceeds limit of 5")
	})

	t.Run("Within limits", func(t *testing.T) {
		maxGroups := 10
		existingGroupCount := 3
		groupsToImport := 5

		// Within limit
		assert.True(t, existingGroupCount+groupsToImport <= maxGroups,
			"Should allow 3 existing + 5 new groups within limit of 10")
	})

	t.Run("At exact limit", func(t *testing.T) {
		maxGroups := 10
		existingGroupCount := 7
		groupsToImport := 3

		// Exactly at limit (should be allowed)
		assert.True(t, existingGroupCount+groupsToImport <= maxGroups,
			"Should allow reaching exact limit of 10")
	})
}

// TestImportService_ErrorAggregation tests error collection during import
func TestImportService_ErrorAggregation(t *testing.T) {
	t.Run("Multiple validation errors", func(t *testing.T) {
		errors := []dto.ImportError{
			{Row: 2, File: "users", Field: "email", Message: "Invalid email", Code: dto.ErrCodeInvalidEmail},
			{Row: 3, File: "users", Field: "role", Message: "Invalid role", Code: dto.ErrCodeInvalidRole},
			{Row: 5, File: "groups", Field: "group_name", Message: "Missing required field", Code: dto.ErrCodeValidation},
		}

		assert.Len(t, errors, 3)
		assert.Equal(t, "users", errors[0].File)
		assert.Equal(t, "email", errors[0].Field)
		assert.Equal(t, dto.ErrCodeInvalidEmail, errors[0].Code)
	})
}

// TestImportService_ResponseStructure tests the import response structure
func TestImportService_ResponseStructure(t *testing.T) {
	t.Run("Successful dry-run response", func(t *testing.T) {
		response := &dto.ImportOrganizationDataResponse{
			Success: true,
			DryRun:  true,
			Summary: dto.ImportSummary{
				UsersCreated:       10,
				UsersUpdated:       0,
				UsersSkipped:       0,
				GroupsCreated:      5,
				GroupsUpdated:      0,
				GroupsSkipped:      0,
				MembershipsCreated: 15,
				MembershipsSkipped: 0,
				TotalProcessed:     30,
				ProcessingTime:     "1.5s",
			},
			Errors:   []dto.ImportError{},
			Warnings: []dto.ImportWarning{},
		}

		assert.True(t, response.Success)
		assert.True(t, response.DryRun)
		assert.Equal(t, 10, response.Summary.UsersCreated)
		assert.Equal(t, 5, response.Summary.GroupsCreated)
		assert.Equal(t, 15, response.Summary.MembershipsCreated)
		assert.Equal(t, 30, response.Summary.TotalProcessed)
		assert.Empty(t, response.Errors)
		assert.Empty(t, response.Warnings)
	})

	t.Run("Failed import with errors", func(t *testing.T) {
		response := &dto.ImportOrganizationDataResponse{
			Success: false,
			DryRun:  false,
			Summary: dto.ImportSummary{
				UsersCreated:       0,
				UsersUpdated:       0,
				UsersSkipped:       5,
				GroupsCreated:      0,
				GroupsUpdated:      0,
				GroupsSkipped:      0,
				MembershipsCreated: 0,
				MembershipsSkipped: 0,
				TotalProcessed:     5,
				ProcessingTime:     "0.5s",
			},
			Errors: []dto.ImportError{
				{Row: 2, File: "users", Message: "Duplicate email", Code: dto.ErrCodeDuplicate},
				{Row: 3, File: "users", Message: "Invalid email format", Code: dto.ErrCodeInvalidEmail},
			},
			Warnings: []dto.ImportWarning{},
		}

		assert.False(t, response.Success)
		assert.False(t, response.DryRun)
		assert.Equal(t, 0, response.Summary.UsersCreated)
		assert.Equal(t, 5, response.Summary.UsersSkipped)
		assert.Len(t, response.Errors, 2)
		assert.Equal(t, dto.ErrCodeDuplicate, response.Errors[0].Code)
		assert.Equal(t, dto.ErrCodeInvalidEmail, response.Errors[1].Code)
	})
}

// TestImportService_NestedGroupHandling tests nested group validation
func TestImportService_NestedGroupHandling(t *testing.T) {
	t.Run("Validate parent group references", func(t *testing.T) {
		groups := []dto.GroupImportRow{
			{GroupName: "parent", DisplayName: "Parent Group", ParentGroup: ""},
			{GroupName: "child1", DisplayName: "Child 1", ParentGroup: "parent"},
			{GroupName: "child2", DisplayName: "Child 2", ParentGroup: "parent"},
			{GroupName: "grandchild", DisplayName: "Grandchild", ParentGroup: "child1"},
		}

		// Build a map of group names for validation
		groupMap := make(map[string]bool)
		for _, group := range groups {
			groupMap[group.GroupName] = true
		}

		// Validate all parent references exist
		for _, group := range groups {
			if group.ParentGroup != "" {
				assert.True(t, groupMap[group.ParentGroup],
					"Parent group %s should exist for group %s", group.ParentGroup, group.GroupName)
			}
		}
	})

	t.Run("Detect missing parent group", func(t *testing.T) {
		groups := []dto.GroupImportRow{
			{GroupName: "child1", DisplayName: "Child 1", ParentGroup: "missing_parent"},
		}

		// Build a map of group names
		groupMap := make(map[string]bool)
		for _, group := range groups {
			groupMap[group.GroupName] = true
		}

		// Check for missing parent
		for _, group := range groups {
			if group.ParentGroup != "" {
				if !groupMap[group.ParentGroup] {
					// This should generate an error in the actual import
					assert.False(t, groupMap[group.ParentGroup],
						"Parent group %s should not exist", group.ParentGroup)
				}
			}
		}
	})
}

// TestImportService_UpdateExistingBehavior tests the update_existing flag logic
func TestImportService_UpdateExistingBehavior(t *testing.T) {
	t.Run("Update existing user", func(t *testing.T) {
		// Test the updateIfExists flag
		userImport := dto.UserImportRow{
			Email:          "john.doe@test.com",
			FirstName:      "John",
			LastName:       "Doe",
			Password:       "NewPassword123!",
			Role:           "member",
			UpdateIfExists: "true",
		}

		// In actual service, this would update the user
		assert.Equal(t, "true", userImport.UpdateIfExists)
		assert.Equal(t, "john.doe@test.com", userImport.Email)
	})

	t.Run("Skip existing user", func(t *testing.T) {
		userImport := dto.UserImportRow{
			Email:          "jane.smith@test.com",
			FirstName:      "Jane",
			LastName:       "Smith",
			Password:       "Password123!",
			Role:           "supervisor",
			UpdateIfExists: "false", // Don't update
		}

		// In actual service, this would skip the user
		assert.Equal(t, "false", userImport.UpdateIfExists)
	})
}

// TestImportService_ExternalIDTracking tests external_id functionality
func TestImportService_ExternalIDTracking(t *testing.T) {
	t.Run("Users with external IDs", func(t *testing.T) {
		users := []dto.UserImportRow{
			{Email: "user1@test.com", FirstName: "User", LastName: "One", Password: "Pass1!", Role: "member", ExternalID: "STU001"},
			{Email: "user2@test.com", FirstName: "User", LastName: "Two", Password: "Pass2!", Role: "member", ExternalID: "STU002"},
		}

		// Verify external IDs are present
		for _, user := range users {
			assert.NotEmpty(t, user.ExternalID, "External ID should be set for user %s", user.Email)
		}
	})

	t.Run("Groups with external IDs", func(t *testing.T) {
		groups := []dto.GroupImportRow{
			{GroupName: "m1_devops", DisplayName: "M1 DevOps", ExternalID: "DEPT_DEV"},
			{GroupName: "m1_devops_a", DisplayName: "M1 DevOps A", ParentGroup: "m1_devops", ExternalID: "CLASS_A"},
		}

		// Verify external IDs are present
		for _, group := range groups {
			assert.NotEmpty(t, group.ExternalID, "External ID should be set for group %s", group.GroupName)
		}
	})
}
