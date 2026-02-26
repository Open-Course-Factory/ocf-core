package dto

import "time"

// CSV Row Structures

// UserImportRow represents a row in the users.csv file
type UserImportRow struct {
	Email             string `csv:"email"`
	FirstName         string `csv:"first_name"`
	LastName          string `csv:"last_name"`
	Password          string `csv:"password"`
	Role              string `csv:"role"`            // member, supervisor, admin
	ExternalID        string `csv:"external_id"`     // Optional reference to external system ID
	ForceReset        string `csv:"force_reset"`     // "true" or "false"
	UpdateIfExists    string `csv:"update_existing"` // "true" or "false"
	Name              string `csv:"name"`            // raw "name" column value
	GeneratedPassword string `csv:"-"`               // populated by import service, not from CSV
}

// GroupImportRow represents a row in the groups.csv file
type GroupImportRow struct {
	GroupName   string `csv:"group_name"`   // Unique identifier within organization
	DisplayName string `csv:"display_name"` // Human-readable name
	Description string `csv:"description"`  // Optional description
	ParentGroup string `csv:"parent_group"` // Optional parent group name for nesting
	MaxMembers  string `csv:"max_members"`  // Optional max members (default: 50)
	ExpiresAt   string `csv:"expires_at"`   // Optional expiration date (ISO8601)
	ExternalID  string `csv:"external_id"`  // Optional reference to external system ID
}

// MembershipImportRow represents a row in the memberships.csv file
type MembershipImportRow struct {
	UserEmail string `csv:"user_email"`
	GroupName string `csv:"group_name"`
	Role      string `csv:"role"` // member, admin, assistant, owner
}

// Request/Response Structures

// ImportOrganizationDataRequest represents the bulk import request
type ImportOrganizationDataRequest struct {
	DryRun         bool `form:"dry_run"`         // Validate only, don't persist
	UpdateExisting bool `form:"update_existing"` // Update existing users/groups vs skip
	SendInvites    bool `form:"send_invites"`    // Send email invitations (future)
}

// ImportOrganizationDataResponse represents the import operation result
type ImportOrganizationDataResponse struct {
	Success     bool              `json:"success"`
	DryRun      bool              `json:"dry_run"`
	Summary     ImportSummary     `json:"summary"`
	Errors      []ImportError     `json:"errors"`
	Warnings    []ImportWarning   `json:"warnings"`
	Credentials []UserCredential  `json:"credentials,omitempty"`
}

// UserCredential represents generated credentials for a newly created user
type UserCredential struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// ImportSummary provides statistics about the import operation
type ImportSummary struct {
	UsersCreated       int       `json:"users_created"`
	UsersUpdated       int       `json:"users_updated"`
	UsersSkipped       int       `json:"users_skipped"`
	GroupsCreated      int       `json:"groups_created"`
	GroupsUpdated      int       `json:"groups_updated"`
	GroupsSkipped      int       `json:"groups_skipped"`
	MembershipsCreated int       `json:"memberships_created"`
	MembershipsSkipped int       `json:"memberships_skipped"`
	TotalProcessed     int       `json:"total_processed"`
	ProcessingTime     string    `json:"processing_time"`
	StartTime          time.Time `json:"-"`
}

// ImportError represents a critical error during import
type ImportError struct {
	Row     int    `json:"row"`
	File    string `json:"file"` // users, groups, memberships
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code"` // VALIDATION_ERROR, DUPLICATE, LIMIT_EXCEEDED, etc.
}

// ImportWarning represents a non-critical issue during import
type ImportWarning struct {
	Row     int    `json:"row"`
	File    string `json:"file"`
	Message string `json:"message"`
}

// RegeneratePasswordsRequest represents a request to regenerate passwords for group members
type RegeneratePasswordsRequest struct {
	UserIDs []string `json:"user_ids" binding:"required,min=1"`
}

// RegeneratePasswordsSummary provides statistics about the password regeneration operation
type RegeneratePasswordsSummary struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// RegeneratePasswordsResponse represents the password regeneration result
type RegeneratePasswordsResponse struct {
	Success     bool                       `json:"success"`
	Credentials []UserCredential           `json:"credentials"`
	Errors      []ImportError              `json:"errors,omitempty"`
	Summary     RegeneratePasswordsSummary `json:"summary"`
}

// Error Codes
const (
	ErrCodeValidation    = "VALIDATION_ERROR"
	ErrCodeDuplicate     = "DUPLICATE"
	ErrCodeLimitExceeded = "LIMIT_EXCEEDED"
	ErrCodeNotFound      = "NOT_FOUND"
	ErrCodeInvalidRole   = "INVALID_ROLE"
	ErrCodeInvalidEmail  = "INVALID_EMAIL"
	ErrCodeInvalidDate   = "INVALID_DATE"
	ErrCodeCircularRef   = "CIRCULAR_REFERENCE"
	ErrCodeOrphanedGroup = "ORPHANED_GROUP"
)
