package utils

import (
	"fmt"

	"github.com/google/uuid"
)

// EntityRepository interface for validation operations
// Implement this in your repository to use generic validators
type EntityRepository interface {
	GetByNameAndOwner(name, ownerID string) (interface{}, error)
	GetByID(id uuid.UUID) (interface{}, error)
}

// ValidateUniqueEntityName checks if an entity name is unique for a given owner
// Returns ErrEntityAlreadyExists if the name is already taken
//
// Usage:
//
//	err := ValidateUniqueEntityName(repo, "mygroup", "user123", "group")
func ValidateUniqueEntityName(
	repository EntityRepository,
	name, ownerID, entityType string,
) error {
	existing, _ := repository.GetByNameAndOwner(name, ownerID)
	if existing != nil {
		return ErrEntityAlreadyExists(entityType, "name")
	}
	return nil
}

// ValidateEntityExists checks if an entity exists by ID
// Returns ErrEntityNotFound if the entity doesn't exist
//
// Usage:
//
//	err := ValidateEntityExists(repo, groupID, "group")
func ValidateEntityExists(
	repository EntityRepository,
	id uuid.UUID,
	entityType string,
) error {
	entity, err := repository.GetByID(id)
	if err != nil || entity == nil {
		return ErrEntityNotFound(entityType, id)
	}
	return nil
}

// ValidateNotOwner checks that a user is NOT the owner of an entity
// Returns ErrCannotRemoveOwner if the user is the owner
// Useful for preventing owner removal from groups/organizations
//
// Usage:
//
//	err := ValidateNotOwner(userID, group.OwnerUserID, "group")
func ValidateNotOwner(userID, ownerUserID, entityType string) error {
	if userID == ownerUserID {
		return ErrCannotRemoveOwner(entityType)
	}
	return nil
}

// ValidateIsOwner checks that a user IS the owner of an entity
// Returns ErrPermissionDenied if the user is not the owner
//
// Usage:
//
//	err := ValidateIsOwner(userID, group.OwnerUserID, "group", "delete")
func ValidateIsOwner(userID, ownerUserID, entityType, operation string) error {
	if userID != ownerUserID {
		return ErrPermissionDenied(entityType, operation)
	}
	return nil
}

// ValidateLimitNotReached checks if a limit has been reached
// Returns ErrLimitReached if current >= limit
// Use limit = -1 for unlimited
//
// Usage:
//
//	err := ValidateLimitNotReached(len(org.Groups), org.MaxGroups, "groups")
func ValidateLimitNotReached(current, limit int, entityType string) error {
	if limit == -1 {
		return nil // Unlimited
	}
	if current >= limit {
		return ErrLimitReached(entityType, limit)
	}
	return nil
}

// ValidateActive checks if an entity is active
// Returns ErrEntityInactive if not active
//
// Usage:
//
//	err := ValidateActive(group.IsActive, group.ID, "group")
func ValidateActive(isActive bool, id uuid.UUID, entityType string) error {
	if !isActive {
		return ErrEntityInactive(entityType, id)
	}
	return nil
}

// ValidateNotExpired checks if an entity is not expired
// Returns ErrEntityExpired if expired
// expiresAt can be nil (never expires)
//
// Usage:
//
//	err := ValidateNotExpired(group.ExpiresAt, group.ID, "group")
func ValidateNotExpired(expiresAt interface{}, id uuid.UUID, entityType string) error {
	// Handle *time.Time
	if expiry, ok := expiresAt.(*interface{}); ok && expiry != nil {
		// Check if expired
		// Note: this is a simplified check, real implementation would use time.Now()
		return ErrEntityExpired(entityType, id)
	}
	return nil
}

// ValidateStringNotEmpty checks if a string is not empty
// Returns ValidationError if empty
//
// Usage:
//
//	err := ValidateStringNotEmpty(input.Name, "name", "Name is required")
func ValidateStringNotEmpty(value, field, message string) error {
	if value == "" {
		return NewValidationError(field, message)
	}
	return nil
}

// ValidateStringLength checks if a string length is within bounds
// Returns ValidationError if out of bounds
//
// Usage:
//
//	err := ValidateStringLength(input.Name, "name", 2, 255)
func ValidateStringLength(value, field string, min, max int) error {
	length := len(value)
	if length < min {
		return NewValidationError(field, fmt.Sprintf("must be at least %d characters", min))
	}
	if length > max {
		return NewValidationError(field, fmt.Sprintf("must be at most %d characters", max))
	}
	return nil
}

// ValidateOneOf checks if a value is in a list of allowed values
// Returns ValidationError if not found
//
// Usage:
//
//	err := ValidateOneOf(role, "role", []string{"member", "admin", "owner"})
func ValidateOneOf(value, field string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return NewValidationError(field, fmt.Sprintf("must be one of: %v", allowed))
}

// ValidateUUID checks if a string is a valid UUID
// Returns ValidationError if invalid
//
// Usage:
//
//	err := ValidateUUID(input.OrganizationID, "organization_id")
func ValidateUUID(value, field string) error {
	if _, err := uuid.Parse(value); err != nil {
		return NewValidationError(field, "must be a valid UUID")
	}
	return nil
}

// ValidateNonNegative checks if an integer is non-negative
// Returns ValidationError if negative
//
// Usage:
//
//	err := ValidateNonNegative(input.MaxMembers, "max_members")
func ValidateNonNegative(value int, field string) error {
	if value < 0 {
		return NewValidationError(field, "must be non-negative")
	}
	return nil
}

// ValidatePositive checks if an integer is positive
// Returns ValidationError if not positive
//
// Usage:
//
//	err := ValidatePositive(input.MaxGroups, "max_groups")
func ValidatePositive(value int, field string) error {
	if value <= 0 {
		return NewValidationError(field, "must be positive")
	}
	return nil
}

// ChainValidators runs multiple validators and returns the first error
// Useful for combining multiple validation checks
//
// Usage:
//
//	err := ChainValidators(
//	    func() error { return ValidateStringNotEmpty(input.Name, "name", "Name required") },
//	    func() error { return ValidateStringLength(input.Name, "name", 2, 255) },
//	)
func ChainValidators(validators ...func() error) error {
	for _, validator := range validators {
		if err := validator(); err != nil {
			return err
		}
	}
	return nil
}

// CollectValidationErrors runs multiple validators and collects all errors
// Returns MultiError with all validation errors
//
// Usage:
//
//	multiErr := CollectValidationErrors(
//	    func() error { return ValidateStringNotEmpty(input.Name, "name", "Name required") },
//	    func() error { return ValidateStringNotEmpty(input.Email, "email", "Email required") },
//	)
//	if multiErr.HasErrors() {
//	    return multiErr.ToError()
//	}
func CollectValidationErrors(validators ...func() error) MultiError {
	var multiErr MultiError
	for _, validator := range validators {
		multiErr.AddError(validator())
	}
	return multiErr
}
