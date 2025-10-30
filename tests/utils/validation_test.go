package utils_test

import (
	"testing"
	"time"

	"soli/formations/src/utils"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// Test DTOs
type TestUserInput struct {
	Username  string    `validate:"required,username"`
	Email     string    `validate:"required,email"`
	Age       int       `validate:"required,gt=0,lt=150"`
	Website   string    `validate:"omitempty,url"`
	Bio       string    `validate:"omitempty,min=10,max=500"`
	CreatedAt time.Time `validate:"required,past_date"`
}

type TestPlanInput struct {
	Name            string `validate:"required,plan_name"`
	BillingInterval string `validate:"required,billing_interval"`
	Price           int64  `validate:"required,gte=0"`
	Slug            string `validate:"required,slug"`
}

type TestOrgInput struct {
	Name     string     `validate:"required,org_name"`
	OwnerID  uuid.UUID  `validate:"required,uuid"`
	ParentID string     `validate:"uuid_or_empty"`
	Launch   *time.Time `validate:"omitempty,future_date"`
}

func TestRegisterCustomValidators(t *testing.T) {
	v := validator.New()
	err := utils.RegisterCustomValidators(v)
	assert.NoError(t, err, "Should register custom validators without error")
}

// ==========================================
// Username Validation Tests
// ==========================================

func TestUsernameValidator(t *testing.T) {
	v := validator.New()
	utils.RegisterCustomValidators(v)

	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"Valid username", "john_doe", false},
		{"Valid with hyphen", "john-doe", false},
		{"Valid with numbers", "user123", false},
		{"Too short", "ab", true},
		{"Too long", "a" + string(make([]byte, 50)), true},
		{"Invalid characters", "john@doe", true},
		{"Starts with underscore", "_johndoe", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := TestUserInput{
				Username:  tt.username,
				Email:     "test@example.com",
				Age:       25,
				CreatedAt: time.Now().Add(-24 * time.Hour),
			}

			err := v.Struct(input)
			if tt.wantErr {
				assert.Error(t, err, "Expected validation error for username: "+tt.username)
			} else {
				if err != nil {
					t.Logf("Validation error: %v", err)
				}
				// Check if error is specifically about username
				if err != nil {
					validationErrors := err.(validator.ValidationErrors)
					hasUsernameError := false
					for _, e := range validationErrors {
						if e.Field() == "Username" {
							hasUsernameError = true
							break
						}
					}
					assert.True(t, hasUsernameError, "Should have username validation error")
				}
			}
		})
	}
}

// ==========================================
// Plan Name Validation Tests
// ==========================================

func TestPlanNameValidator(t *testing.T) {
	v := validator.New()
	utils.RegisterCustomValidators(v)

	tests := []struct {
		name     string
		planName string
		wantErr  bool
	}{
		{"Valid plan name", "Basic Plan", false},
		{"Valid with hyphen", "Premium-Plus", false},
		{"Valid with numbers", "Plan 2024", false},
		{"Too short", "A", true},
		{"Too long", string(make([]byte, 101)), true},
		{"Invalid characters", "Plan@2024", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := TestPlanInput{
				Name:            tt.planName,
				BillingInterval: "month",
				Price:           1000,
				Slug:            "test-plan",
			}

			err := v.Struct(input)
			if tt.wantErr {
				assert.Error(t, err, "Expected validation error for plan name: "+tt.planName)
			} else {
				if err != nil {
					validationErrors := err.(validator.ValidationErrors)
					for _, e := range validationErrors {
						if e.Field() == "Name" {
							t.Errorf("Unexpected validation error for Name: %v", e)
						}
					}
				}
			}
		})
	}
}

// ==========================================
// Slug Validation Tests
// ==========================================

func TestSlugValidator(t *testing.T) {
	v := validator.New()
	utils.RegisterCustomValidators(v)

	tests := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{"Valid slug", "test-plan", false},
		{"Valid with numbers", "plan-2024", false},
		{"Too short", "a", true},
		{"Too long", string(make([]byte, 101)), true},
		{"Uppercase not allowed", "Test-Plan", true},
		{"Spaces not allowed", "test plan", true},
		{"Special chars not allowed", "test_plan", true},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := TestPlanInput{
				Name:            "Test Plan",
				BillingInterval: "month",
				Price:           1000,
				Slug:            tt.slug,
			}

			err := v.Struct(input)
			if tt.wantErr {
				assert.Error(t, err, "Expected validation error for slug: "+tt.slug)
			} else {
				if err != nil {
					validationErrors := err.(validator.ValidationErrors)
					for _, e := range validationErrors {
						if e.Field() == "Slug" {
							t.Errorf("Unexpected validation error for Slug: %v", e)
						}
					}
				}
			}
		})
	}
}

// ==========================================
// Date Validation Tests
// ==========================================

func TestFutureDateValidator(t *testing.T) {
	v := validator.New()
	utils.RegisterCustomValidators(v)

	futureDate := time.Now().Add(24 * time.Hour)
	pastDate := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name    string
		date    *time.Time
		wantErr bool
	}{
		{"Future date valid", &futureDate, false},
		{"Past date invalid", &pastDate, true},
		{"Nil date (optional)", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := TestOrgInput{
				Name:    "Test Org",
				OwnerID: uuid.New(),
				Launch:  tt.date,
			}

			err := v.Struct(input)
			if tt.wantErr {
				assert.Error(t, err, "Expected validation error for date")
			} else {
				if err != nil {
					validationErrors := err.(validator.ValidationErrors)
					for _, e := range validationErrors {
						if e.Field() == "Launch" {
							t.Errorf("Unexpected validation error for Launch: %v", e)
						}
					}
				}
			}
		})
	}
}

func TestPastDateValidator(t *testing.T) {
	v := validator.New()
	utils.RegisterCustomValidators(v)

	pastDate := time.Now().Add(-24 * time.Hour)
	futureDate := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name    string
		date    time.Time
		wantErr bool
	}{
		{"Past date valid", pastDate, false},
		{"Future date invalid", futureDate, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := TestUserInput{
				Username:  "testuser",
				Email:     "test@example.com",
				Age:       25,
				CreatedAt: tt.date,
			}

			err := v.Struct(input)
			if tt.wantErr {
				assert.Error(t, err, "Expected validation error for date")
			} else {
				if err != nil {
					validationErrors := err.(validator.ValidationErrors)
					for _, e := range validationErrors {
						if e.Field() == "CreatedAt" {
							t.Errorf("Unexpected validation error for CreatedAt: %v", e)
						}
					}
				}
			}
		})
	}
}

// ==========================================
// UUID Validation Tests
// ==========================================

func TestUUIDOrEmptyValidator(t *testing.T) {
	v := validator.New()
	utils.RegisterCustomValidators(v)

	tests := []struct {
		name     string
		parentID string
		wantErr  bool
	}{
		{"Valid UUID", uuid.New().String(), false},
		{"Empty string", "", false},
		{"Invalid UUID", "not-a-uuid", true},
		{"Partial UUID", "12345", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := TestOrgInput{
				Name:     "Test Org",
				OwnerID:  uuid.New(),
				ParentID: tt.parentID,
			}

			err := v.Struct(input)
			if tt.wantErr {
				assert.Error(t, err, "Expected validation error for ParentID: "+tt.parentID)
			} else {
				if err != nil {
					validationErrors := err.(validator.ValidationErrors)
					for _, e := range validationErrors {
						if e.Field() == "ParentID" {
							t.Errorf("Unexpected validation error for ParentID: %v", e)
						}
					}
				}
			}
		})
	}
}

// ==========================================
// Billing Interval Validation Tests
// ==========================================

func TestBillingIntervalValidator(t *testing.T) {
	v := validator.New()
	utils.RegisterCustomValidators(v)

	tests := []struct {
		name     string
		interval string
		wantErr  bool
	}{
		{"Month valid", "month", false},
		{"Year valid", "year", false},
		{"Week valid", "week", false},
		{"Day valid", "day", false},
		{"Invalid interval", "quarterly", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := TestPlanInput{
				Name:            "Test Plan",
				BillingInterval: tt.interval,
				Price:           1000,
				Slug:            "test-plan",
			}

			err := v.Struct(input)
			if tt.wantErr {
				assert.Error(t, err, "Expected validation error for billing interval: "+tt.interval)
			} else {
				if err != nil {
					validationErrors := err.(validator.ValidationErrors)
					for _, e := range validationErrors {
						if e.Field() == "BillingInterval" {
							t.Errorf("Unexpected validation error for BillingInterval: %v", e)
						}
					}
				}
			}
		})
	}
}

// ==========================================
// ValidateStruct Helper Tests
// ==========================================

func TestValidateStructHelper(t *testing.T) {
	v := validator.New()
	utils.RegisterCustomValidators(v)

	tests := []struct {
		name           string
		input          any
		expectErrors   bool
		expectedFields []string
	}{
		{
			name: "Valid input - no errors",
			input: TestUserInput{
				Username:  "johndoe",
				Email:     "john@example.com",
				Age:       30,
				CreatedAt: time.Now().Add(-24 * time.Hour),
			},
			expectErrors: false,
		},
		{
			name: "Invalid username and email",
			input: TestUserInput{
				Username:  "ab",              // Too short
				Email:     "invalid-email",   // Invalid format
				Age:       30,
				CreatedAt: time.Now().Add(-24 * time.Hour),
			},
			expectErrors:   true,
			expectedFields: []string{"Username", "Email"},
		},
		{
			name: "Invalid age",
			input: TestUserInput{
				Username:  "johndoe",
				Email:     "john@example.com",
				Age:       0, // Must be > 0
				CreatedAt: time.Now().Add(-24 * time.Hour),
			},
			expectErrors:   true,
			expectedFields: []string{"Age"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := utils.ValidateStruct(v, tt.input)

			if tt.expectErrors {
				assert.NotNil(t, errors, "Expected validation errors")
				for _, field := range tt.expectedFields {
					_, exists := errors[field]
					assert.True(t, exists, "Expected error for field: "+field)
				}
			} else {
				assert.Nil(t, errors, "Expected no validation errors")
			}
		})
	}
}

// ==========================================
// Error Message Formatting Tests
// ==========================================

func TestFormatValidationError(t *testing.T) {
	tests := []struct {
		field    string
		tag      string
		param    string
		expected string
	}{
		{"Name", "required", "", "Name is required"},
		{"Email", "email", "", "Email must be a valid email address"},
		{"Name", "min", "2", "Name must be at least 2 characters"},
		{"Age", "gt", "0", "Age must be greater than 0"},
		{"Status", "oneof", "active inactive", "Status must be one of: active inactive"},
		{"Username", "username", "", "Username must be a valid username (3-50 alphanumeric characters, _, -)"},
	}

	for _, tt := range tests {
		t.Run(tt.field+"_"+tt.tag, func(t *testing.T) {
			result := utils.FormatValidationError(tt.field, tt.tag, tt.param)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ==========================================
// Integration Test
// ==========================================

func TestValidationIntegration(t *testing.T) {
	v := validator.New()
	err := utils.RegisterCustomValidators(v)
	assert.NoError(t, err)

	// Create a complex input with multiple validation rules
	input := TestUserInput{
		Username:  "john_doe-123",
		Email:     "john@example.com",
		Age:       25,
		Website:   "https://example.com",
		Bio:       "This is a valid bio that is longer than 10 characters",
		CreatedAt: time.Now().Add(-7 * 24 * time.Hour),
	}

	// Validate
	errors := utils.ValidateStruct(v, input)
	assert.Nil(t, errors, "Valid input should not produce errors")

	// Test invalid input
	invalidInput := TestUserInput{
		Username:  "ab",            // Too short
		Email:     "not-an-email",  // Invalid format
		Age:       -5,              // Negative
		Website:   "not-a-url",     // Invalid URL
		Bio:       "Short",         // Too short
		CreatedAt: time.Now().Add(24 * time.Hour), // Future date (should be past)
	}

	errors = utils.ValidateStruct(v, invalidInput)
	assert.NotNil(t, errors, "Invalid input should produce errors")
	assert.GreaterOrEqual(t, len(errors), 4, "Should have at least 4 validation errors")
}
