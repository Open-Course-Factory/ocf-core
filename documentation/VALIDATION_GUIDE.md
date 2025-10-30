# Validation Guide

This guide explains how to use the validation utilities in `src/utils/` to create consistent, reusable validation patterns across DTOs.

## Overview

The validation system provides:
- **Reusable validation tag constants** (`validation_tags.go`) - Composable validation patterns
- **Custom validators** (`custom_validators.go`) - Business-specific validation rules
- **Helper functions** - Format error messages and validate structs

## Quick Start

### 1. Using Validation Tag Constants

Instead of manually typing validation tags, use the predefined constants:

```go
// ❌ OLD - Manual validation tags
type CreateGroupInput struct {
    Name        string `json:"name" binding:"required,min=2,max=255"`
    DisplayName string `json:"display_name" binding:"required,min=2,max=255"`
    Email       string `json:"email" binding:"required,email"`
    MaxMembers  int    `json:"max_members" binding:"omitempty,gte=0"`
}

// ✅ NEW - Using validation constants
import "soli/formations/src/utils"

type CreateGroupInput struct {
    Name        string `json:"name" binding:"required,min=2,max=255"`  // Keep existing if already good
    DisplayName string `json:"display_name" binding:"required,min=2,max=100"`
    Email       string `json:"email" binding:"required,email"`
    MaxMembers  int    `json:"max_members" binding:"omitempty,gte=0"`
}
```

### 2. Common Validation Patterns

#### Required Fields
```go
// String fields
Name string `binding:"required,min=2,max=50"`  // Short names (usernames, titles)
DisplayName string `binding:"required,min=2,max=100"`  // Display names
Description string `binding:"omitempty,min=10,max=1000"`  // Optional descriptions

// Numeric fields
Quantity int `binding:"required,min=1"`  // Required positive quantity
Price int64 `binding:"required,gte=0"`  // Price can be 0 (free plans)
MaxMembers int `binding:"omitempty,gte=0"`  // Optional, 0 = unlimited

// Format validation
Email string `binding:"required,email"`
UserID uuid.UUID `binding:"required,uuid"`
WebsiteURL string `binding:"omitempty,url"`
```

#### Edit/Patch DTOs (Pointer Fields)
```go
type EditGroupInput struct {
    DisplayName *string `json:"display_name,omitempty" mapstructure:"display_name" binding:"omitempty,min=2,max=100"`
    MaxMembers  *int    `json:"max_members,omitempty" mapstructure:"max_members" binding:"omitempty,gte=0"`
    IsActive    *bool   `json:"is_active,omitempty" mapstructure:"is_active"`
}
```

**Important**: Edit DTOs should:
- Use pointer types for optional fields (`*string`, `*int`, `*bool`)
- Include both `json` and `mapstructure` tags
- Use `omitempty` in JSON tag and validation tag

#### Enum Validation
```go
// Role selection
Role string `binding:"required,oneof=member admin assistant owner"`

// Status selection
Status string `binding:"required,oneof=active inactive pending cancelled"`

// Access level
AccessLevel string `binding:"required,oneof=read write admin"`

// Billing interval
BillingInterval string `binding:"required,oneof=month year"`
```

### 3. Custom Validators

Register custom validators in your service initialization:

```go
import (
    "github.com/go-playground/validator/v10"
    "soli/formations/src/utils"
)

func InitializeValidation() {
    v := validator.New()

    // Register all custom validators
    if err := utils.RegisterCustomValidators(v); err != nil {
        log.Fatal("Failed to register custom validators:", err)
    }
}
```

#### Available Custom Validators

```go
// Username validation (3-50 chars, alphanumeric + _ -)
Username string `binding:"required,username"`

// Future/past date validation
ExpiresAt *time.Time `binding:"omitempty,future_date"`
BirthDate time.Time `binding:"required,past_date"`

// Slug validation (URL-safe)
Slug string `binding:"required,slug"`

// Stripe ID validation
StripeCustomerID string `binding:"required,stripe_id"`

// UUID or empty (for optional UUIDs)
ParentID string `binding:"uuid_or_empty"`
```

### 4. Validation Helper Functions

#### Validate a Struct
```go
import "soli/formations/src/utils"

func CreateGroup(input dto.CreateGroupInput) error {
    v := validator.New()
    utils.RegisterCustomValidators(v)

    // Validate and get formatted errors
    errors := utils.ValidateStruct(v, input)
    if errors != nil {
        // errors is map[string]string with field names and friendly messages
        return fmt.Errorf("validation failed: %v", errors)
    }

    // Proceed with creation
    return nil
}
```

#### Format Validation Errors
```go
// Automatically formats validation errors into user-friendly messages
errors := utils.ValidateStruct(v, input)
// Returns: map[string]string{
//     "Name": "Name must be at least 2 characters",
//     "Email": "Email must be a valid email address",
// }
```

## Validation Tag Reference

### String Validation

| Constant | Tag | Description | Example |
|----------|-----|-------------|---------|
| `RequiredShortName` | `required,min=2,max=50` | Short names, usernames | Group name |
| `RequiredMediumName` | `required,min=2,max=100` | Display names | Organization display name |
| `RequiredLongName` | `required,min=2,max=255` | Long names, descriptions | Full description |
| `RequiredEmail` | `required,email` | Email addresses | user@example.com |
| `RequiredURL` | `required,url` | Website URLs | https://example.com |
| `OptionalURL` | `omitempty,url` | Optional URLs | - |

### Numeric Validation

| Constant | Tag | Description | Example |
|----------|-----|-------------|---------|
| `RequiredPositive` | `required,gt=0` | Must be > 0 | Quantity: 5 |
| `RequiredNonNegative` | `required,gte=0` | Must be >= 0 | Price: 0 (free) |
| `RequiredQuantity` | `required,min=1` | At least 1 | Bulk purchase: 10 |
| `OptionalPositive` | `omitempty,gt=0` | Optional, > 0 if present | - |
| `NonNegative` | `gte=0` | >= 0 | MaxMembers: 0 (unlimited) |

### UUID Validation

| Constant | Tag | Description |
|----------|-----|-------------|
| `RequiredUUID` | `required,uuid` | Required UUID field |
| `OptionalUUID` | `omitempty,uuid` | Optional UUID field |

### Enum Validation

| Constant | Tag | Description |
|----------|-----|-------------|
| `GroupMemberRole` | `oneof=member admin assistant owner` | Group member roles |
| `AccessLevel` | `oneof=read write admin` | Terminal access levels |
| `BillingInterval` | `oneof=month year` | Subscription billing |

## Best Practices

### ✅ DO

1. **Use constants for common patterns**
   ```go
   Email string `binding:"required,email"`
   ```

2. **Always include mapstructure tags for Edit DTOs**
   ```go
   Name *string `json:"name,omitempty" mapstructure:"name" binding:"omitempty,min=2,max=50"`
   ```

3. **Use pointer types for Edit/Patch DTOs**
   ```go
   type EditInput struct {
       Name *string `json:"name,omitempty" mapstructure:"name"`
   }
   ```

4. **Validate numeric ranges**
   ```go
   MaxMembers int `binding:"omitempty,gte=0"`  // 0 = unlimited
   ```

5. **Use custom validators for business logic**
   ```go
   Username string `binding:"required,username"`
   ```

### ❌ DON'T

1. **Don't skip validation on Edit DTOs**
   ```go
   // ❌ BAD - No length validation on edit
   Name *string `json:"name,omitempty"`

   // ✅ GOOD - Validate even on edit
   Name *string `json:"name,omitempty" mapstructure:"name" binding:"omitempty,min=2,max=50"`
   ```

2. **Don't forget mapstructure tags**
   ```go
   // ❌ BAD - Missing mapstructure (PATCH will fail)
   Name *string `json:"name,omitempty" binding:"omitempty,min=2,max=50"`

   // ✅ GOOD
   Name *string `json:"name,omitempty" mapstructure:"name" binding:"omitempty,min=2,max=50"`
   ```

3. **Don't allow negative values for quantities/prices**
   ```go
   // ❌ BAD - Allows negative
   Price int64 `binding:"required"`

   // ✅ GOOD - Must be >= 0
   Price int64 `binding:"required,gte=0"`
   ```

4. **Don't use `required` with pointer fields in Edit DTOs**
   ```go
   // ❌ BAD - Required defeats the purpose of PATCH
   Name *string `binding:"required,min=2,max=50"`

   // ✅ GOOD - Use omitempty for optional updates
   Name *string `binding:"omitempty,min=2,max=50"`
   ```

## Migration Guide

### Updating Existing DTOs

1. **Import the utils package**
   ```go
   import "soli/formations/src/utils"
   ```

2. **Keep existing validation if already correct**
   - Don't change validation tags that are already working
   - Only update if you find inconsistencies or want to use custom validators

3. **Add missing mapstructure tags to Edit DTOs**
   - Edit DTOs need both `json` and `mapstructure` tags
   - Example: `json:"name,omitempty" mapstructure:"name"`

4. **Use custom validators for business rules**
   - Replace manual regex checks with custom validators
   - Example: `binding:"required,username"` instead of manual validation

## Examples

### Complete DTO Example

```go
package dto

import (
    "time"
    "github.com/google/uuid"
)

// CreateGroupInput - Create DTO with comprehensive validation
type CreateGroupInput struct {
    Name               string                 `json:"name" mapstructure:"name" binding:"required,min=2,max=255"`
    DisplayName        string                 `json:"display_name" mapstructure:"display_name" binding:"required,min=2,max=100"`
    Description        string                 `json:"description,omitempty" mapstructure:"description" binding:"omitempty,min=10,max=1000"`
    OrganizationID     *uuid.UUID             `json:"organization_id,omitempty" mapstructure:"organization_id" binding:"omitempty,uuid"`
    MaxMembers         int                    `json:"max_members" mapstructure:"max_members" binding:"omitempty,gte=0"` // 0 = unlimited
    ExpiresAt          *time.Time             `json:"expires_at,omitempty" mapstructure:"expires_at" binding:"omitempty,future_date"`
    Email              string                 `json:"email" mapstructure:"email" binding:"required,email"`
    Metadata           map[string]interface{} `json:"metadata,omitempty" mapstructure:"metadata"`
}

// EditGroupInput - Edit DTO with pointer fields for partial updates
type EditGroupInput struct {
    DisplayName        *string                 `json:"display_name,omitempty" mapstructure:"display_name" binding:"omitempty,min=2,max=100"`
    Description        *string                 `json:"description,omitempty" mapstructure:"description" binding:"omitempty,min=10,max=1000"`
    OrganizationID     *uuid.UUID              `json:"organization_id,omitempty" mapstructure:"organization_id" binding:"omitempty,uuid"`
    MaxMembers         *int                    `json:"max_members,omitempty" mapstructure:"max_members" binding:"omitempty,gte=0"`
    ExpiresAt          *time.Time              `json:"expires_at,omitempty" mapstructure:"expires_at" binding:"omitempty,future_date"`
    IsActive           *bool                   `json:"is_active,omitempty" mapstructure:"is_active"`
    Metadata           *map[string]interface{} `json:"metadata,omitempty" mapstructure:"metadata"`
}

// GroupOutput - Output DTO (no validation needed)
type GroupOutput struct {
    ID          uuid.UUID              `json:"id"`
    Name        string                 `json:"name"`
    DisplayName string                 `json:"display_name"`
    Description string                 `json:"description,omitempty"`
    MaxMembers  int                    `json:"max_members"`
    IsActive    bool                   `json:"is_active"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
    CreatedAt   time.Time              `json:"created_at"`
    UpdatedAt   time.Time              `json:"updated_at"`
}
```

## Integration with Entity Management

The validation system integrates seamlessly with the entity management system. Validation occurs automatically in the PATCH/POST routes before conversion to models.

```go
// In your entity registration
func (g GroupRegistration) EntityInputDtoToEntityModel(input any) any {
    // Validation already happened in the route handler
    groupInput := input.(dto.CreateGroupInput)

    return &models.Group{
        Name:        groupInput.Name,
        DisplayName: groupInput.DisplayName,
        // ... map other fields
    }
}
```

## Testing Validation

Create tests to verify validation rules:

```go
func TestCreateGroupInput_Validation(t *testing.T) {
    v := validator.New()
    utils.RegisterCustomValidators(v)

    tests := []struct {
        name    string
        input   dto.CreateGroupInput
        wantErr bool
    }{
        {
            name: "Valid input",
            input: dto.CreateGroupInput{
                Name:        "test-group",
                DisplayName: "Test Group",
                Email:       "admin@test.com",
            },
            wantErr: false,
        },
        {
            name: "Name too short",
            input: dto.CreateGroupInput{
                Name:        "t",  // Only 1 char
                DisplayName: "Test Group",
                Email:       "admin@test.com",
            },
            wantErr: true,
        },
        {
            name: "Invalid email",
            input: dto.CreateGroupInput{
                Name:        "test-group",
                DisplayName: "Test Group",
                Email:       "not-an-email",
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            errors := utils.ValidateStruct(v, tt.input)
            if (errors != nil) != tt.wantErr {
                t.Errorf("ValidateStruct() error = %v, wantErr %v", errors, tt.wantErr)
            }
        })
    }
}
```

## Further Reading

- [go-playground/validator Documentation](https://pkg.go.dev/github.com/go-playground/validator/v10)
- [Entity Management System Guide](CLAUDE.md#entity-management-system)
- [DTO Tag Requirements](CLAUDE.md#important---dto-tag-requirements)
