package utils

// Validation tag constants for reusable validation patterns
// These can be composed together in struct tags

const (
	// ==========================================
	// Basic Validation Tags
	// ==========================================

	// Required - Field must be present and non-zero
	Required = "required"

	// Optional - Field can be omitted (default behavior, but explicit for clarity)
	Optional = "omitempty"

	// ==========================================
	// String Validation Tags
	// ==========================================

	// String length validation
	MinLength2   = "min=2"
	MinLength3   = "min=3"
	MinLength8   = "min=8"    // For passwords
	MinLength10  = "min=10"
	MaxLength50  = "max=50"
	MaxLength100 = "max=100"
	MaxLength255 = "max=255"
	MaxLength500 = "max=500"

	// Common string validation combinations
	ShortName     = "min=2,max=50"      // For names, titles
	MediumName    = "min=2,max=100"     // For display names
	LongName      = "min=2,max=255"     // For full names, descriptions
	ShortText     = "min=10,max=500"    // For short descriptions
	DescriptionLen = "min=0,max=1000"   // For longer descriptions

	// Format validation
	Email     = "email"
	UUID      = "uuid"
	UUIDv4    = "uuid4"
	URL       = "url"
	Alphanum  = "alphanum"
	Alpha     = "alpha"
	Numeric   = "numeric"

	// ==========================================
	// Numeric Validation Tags
	// ==========================================

	// Positive numbers
	Positive    = "gt=0"      // Greater than 0
	NonNegative = "gte=0"     // Greater than or equal to 0

	// Negative numbers
	Negative    = "lt=0"      // Less than 0
	NonPositive = "lte=0"     // Less than or equal to 0

	// Common numeric ranges
	Min1      = "min=1"
	Min0      = "min=0"
	Max100    = "max=100"
	Max1000   = "max=1000"
	Range0To100 = "gte=0,lte=100"

	// Price validation (in cents)
	PriceAmount = "gte=0"     // Prices can be 0 for free plans

	// Quantity validation
	QuantityMin1 = "min=1"    // At least 1
	QuantityGte0 = "gte=0"    // Can be 0 for unlimited

	// ==========================================
	// Enum/Choice Validation Tags
	// ==========================================

	// Common enums
	BillingInterval = "oneof=month year"
	Currency        = "oneof=usd eur gbp"
	Status          = "oneof=active inactive pending cancelled"

	// Role validation
	GroupMemberRole   = "oneof=member admin assistant owner"
	OrgMemberRole     = "oneof=owner manager member"
	AccessLevel       = "oneof=read write admin"

	// Boolean-like enums
	YesNo    = "oneof=yes no"
	TrueFalse = "oneof=true false"

	// ==========================================
	// Time/Date Validation Tags
	// ==========================================

	// Date formats (RFC3339 is Go's standard)
	DateTimeRFC3339 = "datetime=2006-01-02T15:04:05Z07:00"
	DateOnly        = "datetime=2006-01-02"

	// ==========================================
	// Composition Helpers (Common Combinations)
	// ==========================================

	// Required + Format
	RequiredEmail     = "required,email"
	RequiredUUID      = "required,uuid"
	RequiredURL       = "required,url"
	OptionalURL       = "omitempty,url"

	// Required + Length
	RequiredShortName  = "required,min=2,max=50"
	RequiredMediumName = "required,min=2,max=100"
	RequiredLongName   = "required,min=2,max=255"

	// Required + Numeric
	RequiredPositive    = "required,gt=0"
	RequiredNonNegative = "required,gte=0"
	RequiredQuantity    = "required,min=1"

	// Optional + Format
	OptionalEmail = "omitempty,email"
	OptionalUUID  = "omitempty,uuid"

	// Optional + Numeric
	OptionalPositive    = "omitempty,gt=0"
	OptionalNonNegative = "omitempty,gte=0"
)

// ==========================================
// Validation Tag Builders
// ==========================================

// Compose validation tags into a single string
func ComposeValidation(tags ...string) string {
	result := ""
	for i, tag := range tags {
		if i > 0 {
			result += ","
		}
		result += tag
	}
	return result
}

// RequiredEnum creates a validation tag for required enum fields
func RequiredEnum(values ...string) string {
	result := "required,oneof="
	for i, v := range values {
		if i > 0 {
			result += " "
		}
		result += v
	}
	return result
}

// OptionalEnum creates a validation tag for optional enum fields
func OptionalEnum(values ...string) string {
	result := "omitempty,oneof="
	for i, v := range values {
		if i > 0 {
			result += " "
		}
		result += v
	}
	return result
}

// RequiredStringRange creates a validation tag for required string with length range
func RequiredStringRange(min, max int) string {
	if max > 0 {
		return ComposeValidation(Required, "min="+string(rune(min+'0')), "max="+string(rune(max+'0')))
	}
	return ComposeValidation(Required, "min="+string(rune(min+'0')))
}

// OptionalStringRange creates a validation tag for optional string with length range
func OptionalStringRange(min, max int) string {
	if max > 0 {
		return ComposeValidation(Optional, "min="+string(rune(min+'0')), "max="+string(rune(max+'0')))
	}
	return ComposeValidation(Optional, "min="+string(rune(min+'0')))
}
