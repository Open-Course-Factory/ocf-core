// Package errors provides standardized error types for the entity management system.
//
// All errors follow a consistent structure with error codes, HTTP status codes,
// and optional details for debugging.
//
// # Error Codes
//
// The following error codes are defined:
//   - ENT001: Entity not found (404)
//   - ENT002: Entity not registered in system (500)
//   - ENT003: Entity conversion failed (500)
//   - ENT004: Validation failed (400)
//   - ENT005: Database operation failed (500)
//   - ENT006: Unauthorized access (403)
//   - ENT007: Hook execution failed (500)
//   - ENT008: Invalid input data (400)
//   - ENT009: Invalid pagination parameters (400)
//   - ENT010: Invalid cursor (400)
//   - ENT011: Constraint violation / FK conflict (409)
//
// # Usage
//
// Create errors using the helper constructors:
//
//	err := errors.NewEntityNotFound("Course", courseID)
//	return nil, err
//
// Wrap existing errors:
//
//	if dbErr != nil {
//	    return errors.WrapDatabaseError(dbErr, "create entity")
//	}
//
// Check error types using errors.As:
//
//	var entityErr *errors.EntityError
//	if errors.As(err, &entityErr) {
//	    log.Printf("Error code: %s", entityErr.Code)
//	}
//
// # HTTP Response Format
//
// Errors are automatically converted to JSON by the HandleEntityError middleware:
//
//	{
//	  "error": {
//	    "code": "ENT001",
//	    "message": "Entity not found",
//	    "details": {
//	      "entityName": "Course",
//	      "id": "550e8400-e29b-41d4-a716-446655440000"
//	    }
//	  }
//	}
package errors

import (
	"fmt"
	"net/http"
)

// EntityError represents a standardized error in the entity management system.
// It includes a code for programmatic handling, a message for humans, and optional details.
type EntityError struct {
	Code       string                 `json:"code"`              // Error code (e.g., "ENT001")
	Message    string                 `json:"message"`           // Human-readable message
	HTTPStatus int                    `json:"-"`                 // HTTP status code to return
	Details    map[string]any `json:"details,omitempty"` // Additional context
	Err        error                  `json:"-"`                 // Wrapped error (if any)
}

// Error implements the error interface.
func (e *EntityError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s (%v)", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error for errors.Is/As support.
func (e *EntityError) Unwrap() error {
	return e.Err
}

// WithDetails adds details to the error (chainable).
func (e *EntityError) WithDetails(key string, value any) *EntityError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

// Predefined error types for common scenarios

var (
	// ErrEntityNotFound indicates an entity was not found in the database
	ErrEntityNotFound = &EntityError{
		Code:       "ENT001",
		Message:    "Entity not found",
		HTTPStatus: http.StatusNotFound,
	}

	// ErrEntityNotRegistered indicates an entity type is not registered in the system
	ErrEntityNotRegistered = &EntityError{
		Code:       "ENT002",
		Message:    "Entity not registered in the system",
		HTTPStatus: http.StatusInternalServerError,
	}

	// ErrConversionFailed indicates DTO to entity conversion failed
	ErrConversionFailed = &EntityError{
		Code:       "ENT003",
		Message:    "Entity conversion failed",
		HTTPStatus: http.StatusInternalServerError,
	}

	// ErrValidationFailed indicates validation failed on input data
	ErrValidationFailed = &EntityError{
		Code:       "ENT004",
		Message:    "Validation failed",
		HTTPStatus: http.StatusBadRequest,
	}

	// ErrDatabaseError indicates a database operation failed
	ErrDatabaseError = &EntityError{
		Code:       "ENT005",
		Message:    "Database operation failed",
		HTTPStatus: http.StatusInternalServerError,
	}

	// ErrUnauthorized indicates the user is not authorized for the operation
	ErrUnauthorized = &EntityError{
		Code:       "ENT006",
		Message:    "Unauthorized access",
		HTTPStatus: http.StatusForbidden,
	}

	// ErrHookExecutionFailed indicates a hook failed to execute
	ErrHookExecutionFailed = &EntityError{
		Code:       "ENT007",
		Message:    "Hook execution failed",
		HTTPStatus: http.StatusInternalServerError,
	}

	// ErrInvalidInput indicates input data is invalid
	ErrInvalidInput = &EntityError{
		Code:       "ENT008",
		Message:    "Invalid input data",
		HTTPStatus: http.StatusBadRequest,
	}

	// ErrInvalidPagination indicates pagination parameters are invalid
	ErrInvalidPagination = &EntityError{
		Code:       "ENT009",
		Message:    "Invalid pagination parameters",
		HTTPStatus: http.StatusBadRequest,
	}

	// ErrInvalidCursor indicates the cursor is invalid or malformed
	ErrInvalidCursor = &EntityError{
		Code:       "ENT010",
		Message:    "Invalid cursor",
		HTTPStatus: http.StatusBadRequest,
	}

	// ErrConstraintViolation indicates a delete was blocked by FK constraints
	ErrConstraintViolation = &EntityError{
		Code:       "ENT011",
		Message:    "Cannot delete: foreign key constraint violation, entity is referenced by other records",
		HTTPStatus: http.StatusConflict,
	}
)

// Helper constructors for common error scenarios

// NewEntityNotFound creates an EntityNotFound error with entity details.
//
// Example:
//
//	return nil, errors.NewEntityNotFound("Course", courseID)
func NewEntityNotFound(entityName string, id any) *EntityError {
	err := *ErrEntityNotFound // Copy the base error
	err.Details = map[string]any{
		"entityName": entityName,
		"id":         fmt.Sprintf("%v", id),
	}
	return &err
}

// NewEntityNotRegistered creates an EntityNotRegistered error with entity name.
func NewEntityNotRegistered(entityName string) *EntityError {
	err := *ErrEntityNotRegistered
	err.Details = map[string]any{
		"entityName": entityName,
	}
	return &err
}

// NewConversionError creates a ConversionFailed error with context.
func NewConversionError(entityName string, reason string) *EntityError {
	err := *ErrConversionFailed
	err.Details = map[string]any{
		"entityName": entityName,
		"reason":     reason,
	}
	return &err
}

// NewValidationError creates a ValidationFailed error with field details.
func NewValidationError(field string, reason string) *EntityError {
	err := *ErrValidationFailed
	err.Details = map[string]any{
		"field":  field,
		"reason": reason,
	}
	return &err
}

// WrapDatabaseError wraps a database error with context.
//
// This preserves the original error for errors.Is/As checks while adding
// structured details about the operation that failed.
//
// Example:
//
//	result := db.Create(&entity)
//	if result.Error != nil {
//	    return errors.WrapDatabaseError(result.Error, "create entity")
//	}
func WrapDatabaseError(dbErr error, operation string) *EntityError {
	err := *ErrDatabaseError
	err.Err = dbErr
	err.Details = map[string]any{
		"operation": operation,
		"original":  dbErr.Error(),
	}
	return &err
}

// NewUnauthorizedError creates an Unauthorized error with user and resource context.
func NewUnauthorizedError(userId string, resource string, action string) *EntityError {
	err := *ErrUnauthorized
	err.Details = map[string]any{
		"userId":   userId,
		"resource": resource,
		"action":   action,
	}
	return &err
}

// WrapHookError wraps a hook execution error with context.
func WrapHookError(hookName string, entityName string, hookErr error) *EntityError {
	err := *ErrHookExecutionFailed
	err.Err = hookErr
	err.Details = map[string]any{
		"hookName":   hookName,
		"entityName": entityName,
		"original":   hookErr.Error(),
	}
	return &err
}

// NewInvalidInputError creates an InvalidInput error with field context.
func NewInvalidInputError(field string, value any, reason string) *EntityError {
	err := *ErrInvalidInput
	err.Details = map[string]any{
		"field":  field,
		"value":  fmt.Sprintf("%v", value),
		"reason": reason,
	}
	return &err
}

// NewInvalidPaginationError creates an InvalidPagination error with parameter details.
func NewInvalidPaginationError(param string, value any, reason string) *EntityError {
	err := *ErrInvalidPagination
	err.Details = map[string]any{
		"parameter": param,
		"value":     fmt.Sprintf("%v", value),
		"reason":    reason,
	}
	return &err
}

// NewInvalidCursorError creates an InvalidCursor error with cursor details.
func NewInvalidCursorError(cursor string, reason string) *EntityError {
	err := *ErrInvalidCursor
	err.Details = map[string]any{
		"cursor": cursor,
		"reason": reason,
	}
	return &err
}

// NewConstraintViolationError creates a constraint violation error with details.
func NewConstraintViolationError(operation string, dbErr error) *EntityError {
	err := *ErrConstraintViolation
	err.Err = dbErr
	err.Details = map[string]any{
		"operation": operation,
		"original":  dbErr.Error(),
		"fix":       "Delete or reassign the referencing records before retrying",
	}
	return &err
}
