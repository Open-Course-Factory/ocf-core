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
			"ENT006: you don't have permission to manage this group",
		},
		{
			"Delete organization",
			"delete",
			"organization",
			"ENT006: you don't have permission to delete this organization",
		},
		{
			"Update terminal",
			"update",
			"terminal",
			"ENT006: you don't have permission to update this terminal",
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

// ==========================================
// Error Wrapper Tests
// ==========================================

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
// Integration Tests
// ==========================================

func TestErrorHelpers_Integration(t *testing.T) {
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
		{"WrapDatabaseError with nil", utils.WrapDatabaseError("saving", nil)},
		{"ExternalAPIError with nil", utils.ExternalAPIError("API", "operation", nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, tt.error, "Should return nil for nil input errors")
		})
	}
}
