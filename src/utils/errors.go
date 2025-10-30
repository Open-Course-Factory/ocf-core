package utils

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// EntityError represents a standardized error for entity operations
type EntityError struct {
	EntityType string // e.g., "group", "organization", "terminal"
	Operation  string // e.g., "create", "update", "delete", "fetch"
	Reason     string // Human-readable reason
	EntityID   string // Optional: specific entity ID that caused the error
	UserID     string // Optional: user ID involved
}

func (e EntityError) Error() string {
	if e.EntityID != "" {
		return fmt.Sprintf("failed to %s %s (ID: %s): %s", e.Operation, e.EntityType, e.EntityID, e.Reason)
	}
	return fmt.Sprintf("failed to %s %s: %s", e.Operation, e.EntityType, e.Reason)
}

// NewEntityError creates a new EntityError
func NewEntityError(entityType, operation, reason string) EntityError {
	return EntityError{
		EntityType: entityType,
		Operation:  operation,
		Reason:     reason,
	}
}

// WithEntityID adds entity ID to the error
func (e EntityError) WithEntityID(id uuid.UUID) EntityError {
	e.EntityID = id.String()
	return e
}

// WithUserID adds user ID to the error
func (e EntityError) WithUserID(userID string) EntityError {
	e.UserID = userID
	return e
}

// Common operation constants for consistency
const (
	OpCreate = "create"
	OpUpdate = "update"
	OpDelete = "delete"
	OpFetch  = "fetch"
	OpList   = "list"
)

// Common reason templates for consistency
const (
	ReasonNotFound         = "not found"
	ReasonAlreadyExists    = "already exists"
	ReasonPermissionDenied = "permission denied"
	ReasonInvalidInput     = "invalid input"
	ReasonLimitReached     = "limit reached"
	ReasonExpired          = "expired"
	ReasonInactive         = "inactive"
	ReasonOwnerRequired    = "owner access required"
	ReasonDatabaseError    = "database error"
	ReasonValidationFailed = "validation failed"
)

// Pre-defined error constructors for common scenarios

// ErrEntityNotFound creates a "not found" error
func ErrEntityNotFound(entityType string, entityID uuid.UUID) error {
	return NewEntityError(entityType, OpFetch, ReasonNotFound).WithEntityID(entityID)
}

// ErrEntityAlreadyExists creates an "already exists" error
func ErrEntityAlreadyExists(entityType, identifier string) error {
	return NewEntityError(entityType, OpCreate, fmt.Sprintf("you already have a %s with this %s", entityType, identifier))
}

// ErrPermissionDenied creates a "permission denied" error
func ErrPermissionDenied(entityType, operation string) error {
	return NewEntityError(entityType, operation, fmt.Sprintf("you don't have permission to %s this %s", operation, entityType))
}

// ErrLimitReached creates a "limit reached" error
func ErrLimitReached(entityType string, limit int) error {
	return NewEntityError(entityType, OpCreate, fmt.Sprintf("%s limit reached (%d)", entityType, limit))
}

// ErrCannotModifyOwner creates an error for attempting to modify owner
func ErrCannotModifyOwner(entityType string) error {
	return NewEntityError(entityType, OpUpdate, fmt.Sprintf("cannot modify the %s owner", entityType))
}

// ErrCannotRemoveOwner creates an error for attempting to remove owner
func ErrCannotRemoveOwner(entityType string) error {
	return NewEntityError(entityType, OpDelete, fmt.Sprintf("cannot remove the %s owner", entityType))
}

// ErrMemberNotFound creates an error for member not found
func ErrMemberNotFound(entityType string, userID string) error {
	err := NewEntityError(entityType, OpFetch, "member not found")
	err.UserID = userID
	return err
}

// ErrInvalidRole creates an error for invalid role
func ErrInvalidRole(entityType, role string) error {
	return NewEntityError(entityType, OpUpdate, fmt.Sprintf("invalid role: %s", role))
}

// ErrEntityExpired creates an error for expired entities
func ErrEntityExpired(entityType string, entityID uuid.UUID) error {
	return NewEntityError(entityType, OpFetch, ReasonExpired).WithEntityID(entityID)
}

// ErrEntityInactive creates an error for inactive entities
func ErrEntityInactive(entityType string, entityID uuid.UUID) error {
	return NewEntityError(entityType, OpFetch, ReasonInactive).WithEntityID(entityID)
}

// ValidationError represents a validation error with field-specific details
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

// NewValidationError creates a new validation error
func NewValidationError(field, message string) ValidationError {
	return ValidationError{
		Field:   field,
		Message: message,
	}
}

// MultiError represents multiple errors that occurred
type MultiError struct {
	Errors []error
}

func (m MultiError) Error() string {
	if len(m.Errors) == 0 {
		return "no errors"
	}
	if len(m.Errors) == 1 {
		return m.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors occurred: %s (and %d more)", len(m.Errors), m.Errors[0].Error(), len(m.Errors)-1)
}

// HasErrors returns true if there are any errors
func (m MultiError) HasErrors() bool {
	return len(m.Errors) > 0
}

// AddError adds an error to the collection
func (m *MultiError) AddError(err error) {
	if err != nil {
		m.Errors = append(m.Errors, err)
	}
}

// ToError returns the MultiError as an error if there are errors, nil otherwise
func (m MultiError) ToError() error {
	if m.HasErrors() {
		return m
	}
	return nil
}

// ==========================================
// Simple Error Helper Functions
// (Complement the structured EntityError for common service layer patterns)
// ==========================================

// EntityNotFoundError creates a simple "not found" error (lowercase)
//
// Example:
//
//	EntityNotFoundError("group") -> "group not found"
func EntityNotFoundError(entityType string) error {
	return fmt.Errorf("%s not found", entityType)
}

// PermissionDeniedError creates a simple permission error
//
// Example:
//
//	PermissionDeniedError("manage", "group") -> "you don't have permission to manage this group"
func PermissionDeniedError(action, entityType string) error {
	return fmt.Errorf("you don't have permission to %s this %s", action, entityType)
}

// CapacityExceededError creates a capacity limit error
//
// Example:
//
//	CapacityExceededError("group", 45, 50) -> "group is full (max 50 members)"
func CapacityExceededError(entityType string, current, max int) error {
	return fmt.Errorf("%s is full (max %d members)", entityType, max)
}

// CapacityWillExceedError creates an error for operations that would exceed capacity
//
// Example:
//
//	CapacityWillExceedError("group", 45, 10, 50) -> "adding 10 members would exceed group capacity (45+10 > 50)"
func CapacityWillExceedError(entityType string, current, adding, max int) error {
	return fmt.Errorf("adding %d members would exceed %s capacity (%d+%d > %d)",
		adding, entityType, current, adding, max)
}

// AlreadyExistsError creates an "already exists" error
//
// Example:
//
//	AlreadyExistsError("group", "my-group") -> "you already have a group named my-group"
//	AlreadyExistsError("organization", "acme") -> "you already have an organization named acme"
func AlreadyExistsError(entityType, identifier string) error {
	article := "a"
	// Use "an" for words starting with vowels
	if len(entityType) > 0 {
		firstChar := strings.ToLower(string(entityType[0]))
		if firstChar == "a" || firstChar == "e" || firstChar == "i" || firstChar == "o" || firstChar == "u" {
			article = "an"
		}
	}
	return fmt.Errorf("you already have %s %s named %s", article, entityType, identifier)
}

// AlreadyMemberError creates an "already a member" error
//
// Example:
//
//	AlreadyMemberError("user123", "group") -> "user user123 is already a member of this group"
func AlreadyMemberError(userID, entityType string) error {
	return fmt.Errorf("user %s is already a member of this %s", userID, entityType)
}

// WrapRepositoryError wraps a repository error with context
//
// Example:
//
//	WrapRepositoryError("create", "group", err) -> "failed to create group: <original error>"
func WrapRepositoryError(operation, entityType string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to %s %s: %w", operation, entityType, err)
}

// WrapDatabaseError wraps a database error with context
//
// Example:
//
//	WrapDatabaseError("saving group", err) -> "database error while saving group: <original error>"
func WrapDatabaseError(context string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("database error while %s: %w", context, err)
}

// OwnerOnlyError creates an error for operations restricted to owners
//
// Example:
//
//	OwnerOnlyError("group", "delete") -> "only the group owner can delete the group"
//	OwnerOnlyError("organization", "transfer ownership") -> "only the organization owner can transfer ownership the organization"
func OwnerOnlyError(entityType, action string) error {
	return fmt.Errorf("only the %s owner can %s the %s", entityType, action, entityType)
}

// InvalidUUIDError creates an error for invalid UUID format
//
// Example:
//
//	InvalidUUIDError("group_id", "invalid-uuid") -> "validation error on field 'group_id': invalid UUID format: invalid-uuid"
func InvalidUUIDError(field string, value string) error {
	return NewValidationError(field, fmt.Sprintf("invalid UUID format: %s", value))
}

// MetadataFieldMissingError creates an error for missing metadata fields (primarily for Stripe objects)
//
// Example:
//
//	MetadataFieldMissingError("subscription", "user_id") -> "validation error on field 'subscription.metadata.user_id': required field missing"
func MetadataFieldMissingError(entityType, field string) error {
	return NewValidationError(fmt.Sprintf("%s.metadata.%s", entityType, field), "required field missing")
}

// ==========================================
// Subscription/Payment Error Helpers
// ==========================================

// SubscriptionRequiredError creates a subscription requirement error
//
// Example:
//
//	SubscriptionRequiredError("create terminals") -> "active subscription required to create terminals"
func SubscriptionRequiredError(action string) error {
	return fmt.Errorf("active subscription required to %s", action)
}

// UsageLimitExceededError creates a usage limit error
//
// Example:
//
//	UsageLimitExceededError("terminals", 10, 10) -> "usage limit exceeded for terminals (10/10)"
func UsageLimitExceededError(resourceType string, current, limit int) error {
	return fmt.Errorf("usage limit exceeded for %s (%d/%d)", resourceType, current, limit)
}

// ==========================================
// External API Error Helpers
// ==========================================

// ExternalAPIError creates an external API error
//
// Example:
//
//	ExternalAPIError("Terminal Trainer", "create session", err) -> "Terminal Trainer API error (create session): <original error>"
func ExternalAPIError(serviceName, operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s API error (%s): %w", serviceName, operation, err)
}

// ExternalAPIStatusError creates an external API HTTP status error
//
// Example:
//
//	ExternalAPIStatusError("Stripe", "create customer", 400, "Invalid email") -> "Stripe API returned 400 (create customer): Invalid email"
func ExternalAPIStatusError(serviceName, operation string, statusCode int, body string) error {
	return fmt.Errorf("%s API returned %d (%s): %s", serviceName, statusCode, operation, body)
}
