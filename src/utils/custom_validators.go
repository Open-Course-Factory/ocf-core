package utils

import (
	"regexp"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

// RegisterCustomValidators registers all custom validators with the validator instance
func RegisterCustomValidators(v *validator.Validate) error {
	validators := map[string]validator.Func{
		"username":         validateUsername,
		"plan_name":        validatePlanName,
		"org_name":         validateOrgName,
		"group_name":       validateGroupName,
		"future_date":      validateFutureDate,
		"past_date":        validatePastDate,
		"uuid_or_empty":    validateUUIDOrEmpty,
		"slug":             validateSlug,
		"billing_interval": validateBillingInterval,
		"stripe_id":        validateStripeID,
		"casdoor_owner":    validateCasdoorOwner,
		"snake_case_key":   validateSnakeCaseKey,
	}

	for tag, fn := range validators {
		if err := v.RegisterValidation(tag, fn); err != nil {
			return err
		}
	}

	return nil
}

// ==========================================
// String Format Validators
// ==========================================

// validateUsername validates usernames (alphanumeric, underscores, hyphens, 3-50 chars)
func validateUsername(fl validator.FieldLevel) bool {
	username := fl.Field().String()
	if len(username) < 3 || len(username) > 50 {
		return false
	}

	// Must start with alphanumeric, can contain _, -, alphanumeric
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`, username)
	return matched
}

// validatePlanName validates subscription plan names
func validatePlanName(fl validator.FieldLevel) bool {
	name := fl.Field().String()
	if len(name) < 2 || len(name) > 100 {
		return false
	}

	// Alphanumeric, spaces, hyphens
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9 -]+$`, name)
	return matched
}

// validateOrgName validates organization names
func validateOrgName(fl validator.FieldLevel) bool {
	name := fl.Field().String()
	if len(name) < 2 || len(name) > 255 {
		return false
	}

	// Must not be only whitespace
	return strings.TrimSpace(name) != ""
}

// validateGroupName validates class group names
func validateGroupName(fl validator.FieldLevel) bool {
	name := fl.Field().String()
	if len(name) < 2 || len(name) > 255 {
		return false
	}

	// Must not be only whitespace
	return strings.TrimSpace(name) != ""
}

// validateSnakeCaseKey validates snake_case keys (lowercase letter followed by lowercase alphanumeric + underscores)
func validateSnakeCaseKey(fl validator.FieldLevel) bool {
	key := fl.Field().String()
	matched, _ := regexp.MatchString(`^[a-z][a-z0-9_]*$`, key)
	return matched
}

// validateSlug validates URL-safe slugs (lowercase alphanumeric + hyphens)
func validateSlug(fl validator.FieldLevel) bool {
	slug := fl.Field().String()
	if len(slug) < 2 || len(slug) > 100 {
		return false
	}

	matched, _ := regexp.MatchString(`^[a-z0-9-]+$`, slug)
	return matched
}

// ==========================================
// Date/Time Validators
// ==========================================

// validateFutureDate ensures a date is in the future
func validateFutureDate(fl validator.FieldLevel) bool {
	date, ok := fl.Field().Interface().(time.Time)
	if !ok {
		// Try pointer
		datePtr, ok := fl.Field().Interface().(*time.Time)
		if !ok || datePtr == nil {
			return false
		}
		date = *datePtr
	}

	return date.After(time.Now())
}

// validatePastDate ensures a date is in the past
func validatePastDate(fl validator.FieldLevel) bool {
	date, ok := fl.Field().Interface().(time.Time)
	if !ok {
		// Try pointer
		datePtr, ok := fl.Field().Interface().(*time.Time)
		if !ok || datePtr == nil {
			return false
		}
		date = *datePtr
	}

	return date.Before(time.Now())
}

// ==========================================
// UUID Validators
// ==========================================

// validateUUIDOrEmpty validates UUID or allows empty string
func validateUUIDOrEmpty(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	if value == "" {
		return true
	}

	_, err := uuid.Parse(value)
	return err == nil
}

// ==========================================
// Business Logic Validators
// ==========================================

// validateBillingInterval validates billing interval values
func validateBillingInterval(fl validator.FieldLevel) bool {
	interval := fl.Field().String()
	validIntervals := []string{"month", "year", "week", "day"}

	for _, valid := range validIntervals {
		if interval == valid {
			return true
		}
	}

	return false
}

// validateStripeID validates Stripe ID format (starts with specific prefix)
func validateStripeID(fl validator.FieldLevel) bool {
	id := fl.Field().String()
	if id == "" {
		return false
	}

	// Stripe IDs start with prefixes like: cus_, sub_, pi_, pm_, etc.
	validPrefixes := []string{"cus_", "sub_", "pi_", "pm_", "prod_", "price_", "si_", "in_"}

	for _, prefix := range validPrefixes {
		if strings.HasPrefix(id, prefix) {
			return len(id) > len(prefix)+10 // Minimum length after prefix
		}
	}

	return false
}

// validateCasdoorOwner validates Casdoor owner format
func validateCasdoorOwner(fl validator.FieldLevel) bool {
	owner := fl.Field().String()
	if len(owner) < 2 || len(owner) > 50 {
		return false
	}

	// Casdoor owners are alphanumeric
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, owner)
	return matched
}

// ==========================================
// Validation Helper Functions
// ==========================================

// ValidateStruct validates a struct and returns formatted error messages
func ValidateStruct(v *validator.Validate, s any) map[string]string {
	errors := make(map[string]string)

	err := v.Struct(s)
	if err == nil {
		return nil
	}

	for _, err := range err.(validator.ValidationErrors) {
		field := err.Field()
		tag := err.Tag()

		errors[field] = FormatValidationError(field, tag, err.Param())
	}

	return errors
}

// FormatValidationError creates user-friendly error messages
func FormatValidationError(field, tag, param string) string {
	switch tag {
	case "required":
		return field + " is required"
	case "email":
		return field + " must be a valid email address"
	case "uuid", "uuid4":
		return field + " must be a valid UUID"
	case "url":
		return field + " must be a valid URL"
	case "min":
		return field + " must be at least " + param + " characters"
	case "max":
		return field + " must be at most " + param + " characters"
	case "gt":
		return field + " must be greater than " + param
	case "gte":
		return field + " must be greater than or equal to " + param
	case "lt":
		return field + " must be less than " + param
	case "lte":
		return field + " must be less than or equal to " + param
	case "oneof":
		return field + " must be one of: " + param
	case "username":
		return field + " must be a valid username (3-50 alphanumeric characters, _, -)"
	case "future_date":
		return field + " must be a future date"
	case "past_date":
		return field + " must be a past date"
	case "slug":
		return field + " must be a valid slug (lowercase alphanumeric + hyphens)"
	case "snake_case_key":
		return field + " must be a snake_case key (lowercase letter followed by lowercase alphanumeric + underscores)"
	case "billing_interval":
		return field + " must be a valid billing interval (month, year, week, day)"
	case "stripe_id":
		return field + " must be a valid Stripe ID"
	default:
		return field + " is invalid"
	}
}
