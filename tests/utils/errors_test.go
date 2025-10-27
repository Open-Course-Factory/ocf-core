package utils_test

import (
	"errors"
	"testing"

	"soli/formations/src/utils"

	"github.com/stretchr/testify/assert"
)

// ==========================================
// Simple Error Helper Tests
// ==========================================

func TestEntityNotFoundError(t *testing.T) {
	tests := []struct {
		name         string
		entityType   string
		expectedMsg  string
	}{
		{"Group not found", "group", "group not found"},
		{"Organization not found", "organization", "organization not found"},
		{"Terminal not found", "terminal", "terminal not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.EntityNotFoundError(tt.entityType)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedMsg, err.Error())
		})
	}
}

func TestPermissionDeniedError(t *testing.T) {
	tests := []struct {
		name         string
		action       string
		entityType   string
		expectedMsg  string
	}{
		{
			"Manage group",
			"manage",
			"group",
			"you don't have permission to manage this group",
		},
		{
			"Delete organization",
			"delete",
			"organization",
			"you don't have permission to delete this organization",
		},
		{
			"Update terminal",
			"update",
			"terminal",
			"you don't have permission to update this terminal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.PermissionDeniedError(tt.action, tt.entityType)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedMsg, err.Error())
		})
	}
}

func TestCapacityExceededError(t *testing.T) {
	tests := []struct {
		name        string
		entityType  string
		current     int
		max         int
		expectedMsg string
	}{
		{
			"Group full",
			"group",
			50,
			50,
			"group is full (max 50 members)",
		},
		{
			"Organization full",
			"organization",
			100,
			100,
			"organization is full (max 100 members)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.CapacityExceededError(tt.entityType, tt.current, tt.max)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedMsg, err.Error())
		})
	}
}

func TestCapacityWillExceedError(t *testing.T) {
	err := utils.CapacityWillExceedError("group", 45, 10, 50)
	assert.Error(t, err)
	assert.Equal(t, "adding 10 members would exceed group capacity (45+10 > 50)", err.Error())
}

func TestAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name        string
		entityType  string
		identifier  string
		expectedMsg string
	}{
		{
			"Group exists",
			"group",
			"my-group",
			"you already have a group named my-group",
		},
		{
			"Organization exists",
			"organization",
			"acme-corp",
			"you already have an organization named acme-corp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.AlreadyExistsError(tt.entityType, tt.identifier)
			assert.Error(t, err)
			assert.Equal(t, tt.expectedMsg, err.Error())
		})
	}
}

func TestAlreadyMemberError(t *testing.T) {
	err := utils.AlreadyMemberError("user123", "group")
	assert.Error(t, err)
	assert.Equal(t, "user user123 is already a member of this group", err.Error())
}

// ==========================================
// Error Wrapper Tests
// ==========================================

func TestWrapRepositoryError(t *testing.T) {
	t.Run("Wraps error with context", func(t *testing.T) {
		originalErr := errors.New("connection timeout")
		err := utils.WrapRepositoryError("create", "group", originalErr)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create group")
		assert.Contains(t, err.Error(), "connection timeout")
		assert.True(t, errors.Is(err, originalErr), "Should wrap the original error")
	})

	t.Run("Returns nil for nil error", func(t *testing.T) {
		err := utils.WrapRepositoryError("create", "group", nil)
		assert.NoError(t, err)
	})

	t.Run("Different operations", func(t *testing.T) {
		originalErr := errors.New("validation failed")

		createErr := utils.WrapRepositoryError("create", "group", originalErr)
		assert.Contains(t, createErr.Error(), "failed to create group")

		updateErr := utils.WrapRepositoryError("update", "organization", originalErr)
		assert.Contains(t, updateErr.Error(), "failed to update organization")

		deleteErr := utils.WrapRepositoryError("delete", "terminal", originalErr)
		assert.Contains(t, deleteErr.Error(), "failed to delete terminal")
	})
}

func TestWrapDatabaseError(t *testing.T) {
	t.Run("Wraps error with context", func(t *testing.T) {
		originalErr := errors.New("deadlock detected")
		err := utils.WrapDatabaseError("saving group", originalErr)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database error while saving group")
		assert.Contains(t, err.Error(), "deadlock detected")
		assert.True(t, errors.Is(err, originalErr))
	})

	t.Run("Returns nil for nil error", func(t *testing.T) {
		err := utils.WrapDatabaseError("saving group", nil)
		assert.NoError(t, err)
	})
}

// ==========================================
// Subscription/Payment Error Tests
// ==========================================

func TestSubscriptionRequiredError(t *testing.T) {
	err := utils.SubscriptionRequiredError("create terminals")
	assert.Error(t, err)
	assert.Equal(t, "active subscription required to create terminals", err.Error())
}

func TestUsageLimitExceededError(t *testing.T) {
	err := utils.UsageLimitExceededError("terminals", 10, 10)
	assert.Error(t, err)
	assert.Equal(t, "usage limit exceeded for terminals (10/10)", err.Error())
}

// ==========================================
// External API Error Tests
// ==========================================

func TestExternalAPIError(t *testing.T) {
	t.Run("Wraps API error", func(t *testing.T) {
		originalErr := errors.New("timeout")
		err := utils.ExternalAPIError("Terminal Trainer", "create session", originalErr)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Terminal Trainer API error")
		assert.Contains(t, err.Error(), "create session")
		assert.Contains(t, err.Error(), "timeout")
		assert.True(t, errors.Is(err, originalErr))
	})

	t.Run("Returns nil for nil error", func(t *testing.T) {
		err := utils.ExternalAPIError("Stripe", "create customer", nil)
		assert.NoError(t, err)
	})
}

func TestExternalAPIStatusError(t *testing.T) {
	err := utils.ExternalAPIStatusError("Stripe", "create customer", 400, "Invalid email")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Stripe API returned 400")
	assert.Contains(t, err.Error(), "create customer")
	assert.Contains(t, err.Error(), "Invalid email")
}

// ==========================================
// Error Message Format Tests
// ==========================================

func TestErrorMessageFormats(t *testing.T) {
	t.Run("All error messages use entity types consistently", func(t *testing.T) {
		tests := []struct {
			name  string
			error error
		}{
			{"EntityNotFoundError", utils.EntityNotFoundError("group")},
			{"PermissionDeniedError", utils.PermissionDeniedError("manage", "group")},
			{"AlreadyExistsError", utils.AlreadyExistsError("group", "test")},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// All error messages should contain the entity type
				errMsg := tt.error.Error()
				assert.Contains(t, errMsg, "group", "Error message should contain entity type: %s", errMsg)
			})
		}
	})
}

// ==========================================
// Error Composition Tests
// ==========================================

func TestErrorComposition(t *testing.T) {
	t.Run("Repository error wrapping preserves original error", func(t *testing.T) {
		// Simulate a service layer error
		dbErr := errors.New("unique constraint violation")
		repoErr := utils.WrapDatabaseError("inserting group", dbErr)
		serviceErr := utils.WrapRepositoryError("create", "group", repoErr)

		// Should be able to unwrap to original error
		assert.True(t, errors.Is(serviceErr, dbErr),
			"Should be able to unwrap to original database error")

		// Error message should include all context
		errMsg := serviceErr.Error()
		assert.Contains(t, errMsg, "failed to create group")
		assert.Contains(t, errMsg, "database error while inserting group")
		assert.Contains(t, errMsg, "unique constraint violation")
	})
}

// ==========================================
// Integration Tests
// ==========================================

func TestErrorHelpers_Integration(t *testing.T) {
	t.Run("Complete error handling workflow", func(t *testing.T) {
		// Scenario: User tries to create a group, but database fails

		// 1. Database error
		dbErr := errors.New("connection lost")

		// 2. Repository wraps it
		repoErr := utils.WrapDatabaseError("saving new group", dbErr)
		assert.Error(t, repoErr)
		assert.True(t, errors.Is(repoErr, dbErr))

		// 3. Service wraps it further
		serviceErr := utils.WrapRepositoryError("create", "group", repoErr)
		assert.Error(t, serviceErr)
		assert.True(t, errors.Is(serviceErr, dbErr))

		// 4. Error chain is preserved
		errMsg := serviceErr.Error()
		assert.Contains(t, errMsg, "failed to create group")
		assert.Contains(t, errMsg, "database error")
		assert.Contains(t, errMsg, "connection lost")
	})

	t.Run("Permission and capacity error combination", func(t *testing.T) {
		// User tries to add members but lacks permission
		permErr := utils.PermissionDeniedError("add members to", "group")
		assert.Contains(t, permErr.Error(), "permission")

		// Even if they had permission, group is full
		capErr := utils.CapacityExceededError("group", 50, 50)
		assert.Contains(t, capErr.Error(), "full")

		// Different error types for different scenarios
		assert.NotEqual(t, permErr.Error(), capErr.Error())
	})
}

// ==========================================
// Nil Safety Tests
// ==========================================

func TestErrorHelpers_NilSafety(t *testing.T) {
	tests := []struct {
		name  string
		error error
	}{
		{"WrapRepositoryError with nil", utils.WrapRepositoryError("create", "group", nil)},
		{"WrapDatabaseError with nil", utils.WrapDatabaseError("saving", nil)},
		{"ExternalAPIError with nil", utils.ExternalAPIError("API", "operation", nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, tt.error, "Should return nil for nil input errors")
		})
	}
}
