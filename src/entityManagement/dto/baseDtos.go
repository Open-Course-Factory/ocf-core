package dto

import (
	"time"

	"github.com/google/uuid"
)

// BaseEditDto contains common fields for PATCH operations
// Embed this in your Edit DTOs to ensure consistency
//
// Example:
//
//	type EditEntityInput struct {
//	    BaseEditDto
//	    Name        *string `json:"name,omitempty" mapstructure:"name"`
//	    DisplayName *string `json:"display_name,omitempty" mapstructure:"display_name"`
//	}
type BaseEditDto struct {
	IsActive *bool                   `json:"is_active,omitempty" mapstructure:"is_active"`
	Metadata *map[string]interface{} `json:"metadata,omitempty" mapstructure:"metadata"`
}

// BaseOutputDto contains common fields for response DTOs
// Embed this in your Output DTOs to ensure consistency
//
// Example:
//
//	type EntityOutput struct {
//	    BaseOutputDto
//	    Name        string `json:"name"`
//	    DisplayName string `json:"display_name"`
//	}
type BaseOutputDto struct {
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// BaseEntityDto contains common fields for entities with UUID
// Includes both output fields and basic identification
type BaseEntityDto struct {
	ID uuid.UUID `json:"id"`
	BaseOutputDto
}

// BaseCreateDto can be used for common create fields
// though most entities have unique create requirements
type BaseCreateDto struct {
	Metadata map[string]interface{} `json:"metadata,omitempty" mapstructure:"metadata"`
}

// OwnedEntityOutput extends BaseEntityDto with owner information
// Use for entities that have an owner
type OwnedEntityOutput struct {
	BaseEntityDto
	OwnerUserID string `json:"owner_user_id"`
}

// NamedEntityOutput extends BaseEntityDto with name/display_name
// Use for entities that have both name and display_name
type NamedEntityOutput struct {
	BaseEntityDto
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
}

// FullEntityOutput combines all common fields
// For entities that have all standard fields
type FullEntityOutput struct {
	BaseEntityDto
	Name        string                 `json:"name"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description,omitempty"`
	OwnerUserID string                 `json:"owner_user_id"`
	IsActive    bool                   `json:"is_active"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}
