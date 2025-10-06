package errors_test

import (
	"errors"
	"net/http"
	"testing"

	entityErrors "soli/formations/src/entityManagement/errors"

	"github.com/google/uuid"
)

// TestEntityError_Error tests the Error() method implementation
func TestEntityError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *entityErrors.EntityError
		expected string
	}{
		{
			name: "simple error without wrapped error",
			err: &entityErrors.EntityError{
				Code:    "ENT001",
				Message: "Entity not found",
			},
			expected: "ENT001: Entity not found",
		},
		{
			name: "error with wrapped error",
			err: &entityErrors.EntityError{
				Code:    "ENT005",
				Message: "Database operation failed",
				Err:     errors.New("connection refused"),
			},
			expected: "ENT005: Database operation failed (connection refused)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestEntityError_Unwrap tests error unwrapping
func TestEntityError_Unwrap(t *testing.T) {
	originalErr := errors.New("original error")
	entityErr := &entityErrors.EntityError{
		Code:    "ENT005",
		Message: "Database error",
		Err:     originalErr,
	}

	unwrapped := entityErr.Unwrap()
	if unwrapped != originalErr {
		t.Errorf("Unwrap() returned wrong error: got %v, want %v", unwrapped, originalErr)
	}

	// Test errors.Is
	if !errors.Is(entityErr, originalErr) {
		t.Error("errors.Is() should find the wrapped error")
	}
}

// TestEntityError_WithDetails tests the chainable WithDetails method
func TestEntityError_WithDetails(t *testing.T) {
	err := &entityErrors.EntityError{
		Code:    "ENT001",
		Message: "Entity not found",
	}

	// Chain multiple details
	err = err.WithDetails("entityName", "Course").
		WithDetails("id", "123")

	if err.Details["entityName"] != "Course" {
		t.Errorf("Details[entityName] = %v, want Course", err.Details["entityName"])
	}

	if err.Details["id"] != "123" {
		t.Errorf("Details[id] = %v, want 123", err.Details["id"])
	}
}

// TestNewEntityNotFound tests the constructor for EntityNotFound errors
func TestNewEntityNotFound(t *testing.T) {
	id := uuid.New()
	err := entityErrors.NewEntityNotFound("Course", id)

	if err.Code != "ENT001" {
		t.Errorf("Code = %v, want ENT001", err.Code)
	}

	if err.HTTPStatus != http.StatusNotFound {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, http.StatusNotFound)
	}

	if err.Details["entityName"] != "Course" {
		t.Errorf("Details[entityName] = %v, want Course", err.Details["entityName"])
	}

	if err.Details["id"] == "" {
		t.Error("Details[id] should not be empty")
	}
}

// TestNewEntityNotRegistered tests entity not registered errors
func TestNewEntityNotRegistered(t *testing.T) {
	err := entityErrors.NewEntityNotRegistered("NonExistentEntity")

	if err.Code != "ENT002" {
		t.Errorf("Code = %v, want ENT002", err.Code)
	}

	if err.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, http.StatusInternalServerError)
	}

	if err.Details["entityName"] != "NonExistentEntity" {
		t.Errorf("Details[entityName] = %v, want NonExistentEntity", err.Details["entityName"])
	}
}

// TestNewConversionError tests conversion error constructor
func TestNewConversionError(t *testing.T) {
	err := entityErrors.NewConversionError("Course", "invalid DTO format")

	if err.Code != "ENT003" {
		t.Errorf("Code = %v, want ENT003", err.Code)
	}

	if err.Details["entityName"] != "Course" {
		t.Errorf("Details[entityName] = %v, want Course", err.Details["entityName"])
	}

	if err.Details["reason"] != "invalid DTO format" {
		t.Errorf("Details[reason] = %v, want 'invalid DTO format'", err.Details["reason"])
	}
}

// TestNewValidationError tests validation error constructor
func TestNewValidationError(t *testing.T) {
	err := entityErrors.NewValidationError("email", "must be a valid email address")

	if err.Code != "ENT004" {
		t.Errorf("Code = %v, want ENT004", err.Code)
	}

	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, http.StatusBadRequest)
	}

	if err.Details["field"] != "email" {
		t.Errorf("Details[field] = %v, want email", err.Details["field"])
	}

	if err.Details["reason"] != "must be a valid email address" {
		t.Errorf("Details[reason] = %v, want 'must be a valid email address'", err.Details["reason"])
	}
}

// TestWrapDatabaseError tests database error wrapping
func TestWrapDatabaseError(t *testing.T) {
	originalErr := errors.New("connection timeout")
	err := entityErrors.WrapDatabaseError(originalErr, "create entity")

	if err.Code != "ENT005" {
		t.Errorf("Code = %v, want ENT005", err.Code)
	}

	if err.Err != originalErr {
		t.Error("Wrapped error should be preserved")
	}

	if err.Details["operation"] != "create entity" {
		t.Errorf("Details[operation] = %v, want 'create entity'", err.Details["operation"])
	}

	if err.Details["original"] != "connection timeout" {
		t.Errorf("Details[original] = %v, want 'connection timeout'", err.Details["original"])
	}
}

// TestNewUnauthorizedError tests unauthorized error constructor
func TestNewUnauthorizedError(t *testing.T) {
	err := entityErrors.NewUnauthorizedError("user123", "courses", "DELETE")

	if err.Code != "ENT006" {
		t.Errorf("Code = %v, want ENT006", err.Code)
	}

	if err.HTTPStatus != http.StatusForbidden {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, http.StatusForbidden)
	}

	if err.Details["userId"] != "user123" {
		t.Errorf("Details[userId] = %v, want user123", err.Details["userId"])
	}

	if err.Details["resource"] != "courses" {
		t.Errorf("Details[resource] = %v, want courses", err.Details["resource"])
	}

	if err.Details["action"] != "DELETE" {
		t.Errorf("Details[action] = %v, want DELETE", err.Details["action"])
	}
}

// TestWrapHookError tests hook error wrapping
func TestWrapHookError(t *testing.T) {
	originalErr := errors.New("validation failed in hook")
	err := entityErrors.WrapHookError("BeforeCreate", "Course", originalErr)

	if err.Code != "ENT007" {
		t.Errorf("Code = %v, want ENT007", err.Code)
	}

	if err.Err != originalErr {
		t.Error("Wrapped error should be preserved")
	}

	if err.Details["hookName"] != "BeforeCreate" {
		t.Errorf("Details[hookName] = %v, want BeforeCreate", err.Details["hookName"])
	}

	if err.Details["entityName"] != "Course" {
		t.Errorf("Details[entityName] = %v, want Course", err.Details["entityName"])
	}
}

// TestNewInvalidInputError tests invalid input error constructor
func TestNewInvalidInputError(t *testing.T) {
	err := entityErrors.NewInvalidInputError("title", "", "title cannot be empty")

	if err.Code != "ENT008" {
		t.Errorf("Code = %v, want ENT008", err.Code)
	}

	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, http.StatusBadRequest)
	}

	if err.Details["field"] != "title" {
		t.Errorf("Details[field] = %v, want title", err.Details["field"])
	}

	if err.Details["reason"] != "title cannot be empty" {
		t.Errorf("Details[reason] = %v, want 'title cannot be empty'", err.Details["reason"])
	}
}

// TestNewInvalidPaginationError tests pagination error constructor
func TestNewInvalidPaginationError(t *testing.T) {
	err := entityErrors.NewInvalidPaginationError("page", -1, "page must be positive")

	if err.Code != "ENT009" {
		t.Errorf("Code = %v, want ENT009", err.Code)
	}

	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, http.StatusBadRequest)
	}

	if err.Details["parameter"] != "page" {
		t.Errorf("Details[parameter] = %v, want page", err.Details["parameter"])
	}

	if err.Details["value"] != "-1" {
		t.Errorf("Details[value] = %v, want -1", err.Details["value"])
	}
}

// TestNewInvalidCursorError tests cursor error constructor
func TestNewInvalidCursorError(t *testing.T) {
	err := entityErrors.NewInvalidCursorError("invalid-cursor", "failed to decode base64")

	if err.Code != "ENT010" {
		t.Errorf("Code = %v, want ENT010", err.Code)
	}

	if err.HTTPStatus != http.StatusBadRequest {
		t.Errorf("HTTPStatus = %v, want %v", err.HTTPStatus, http.StatusBadRequest)
	}

	if err.Details["cursor"] != "invalid-cursor" {
		t.Errorf("Details[cursor] = %v, want invalid-cursor", err.Details["cursor"])
	}

	if err.Details["reason"] != "failed to decode base64" {
		t.Errorf("Details[reason] = %v, want 'failed to decode base64'", err.Details["reason"])
	}
}

// TestErrorsAs tests that errors.As works with EntityError
func TestErrorsAs(t *testing.T) {
	originalErr := entityErrors.NewEntityNotFound("Course", uuid.New())

	var entityErr *entityErrors.EntityError
	if !errors.As(originalErr, &entityErr) {
		t.Error("errors.As should work with EntityError")
	}

	if entityErr.Code != "ENT001" {
		t.Errorf("errors.As extracted error with Code = %v, want ENT001", entityErr.Code)
	}
}

// TestPredefinedErrorsAreImmutable tests that predefined errors are copied, not modified
func TestPredefinedErrorsAreImmutable(t *testing.T) {
	// Get the predefined error
	baseErr := entityErrors.ErrEntityNotFound

	// Create a new error with details
	err1 := entityErrors.NewEntityNotFound("Course", uuid.New())
	err2 := entityErrors.NewEntityNotFound("Chapter", uuid.New())

	// Base error should not have details
	if baseErr.Details != nil {
		t.Error("Predefined error should not have details")
	}

	// Created errors should have different details
	if err1.Details["entityName"] == err2.Details["entityName"] {
		t.Error("Each created error should have independent details")
	}
}
