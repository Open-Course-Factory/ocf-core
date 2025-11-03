package dto

import (
	"github.com/google/uuid"
	groupDto "soli/formations/src/groups/dto"
	organizationDto "soli/formations/src/organizations/dto"
)

// PermissionRule represents a single Casbin permission rule
type PermissionRule struct {
	Resource string   `json:"resource"` // e.g., "/api/v1/groups/:id"
	Methods  []string `json:"methods"`  // e.g., ["GET", "POST"]
}

// OrganizationMembershipContext provides context about a user's organization membership
type OrganizationMembershipContext struct {
	OrganizationID   uuid.UUID `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	Role             string    `json:"role"`             // e.g., "owner", "member"
	IsOwner          bool      `json:"is_owner"`         // Quick check if user owns the organization
	Features         []string  `json:"features"`         // Features from org subscription
	HasSubscription  bool      `json:"has_subscription"` // Whether org has active subscription
}

// GroupMembershipContext provides context about a user's group membership
type GroupMembershipContext struct {
	GroupID   uuid.UUID `json:"group_id"`
	GroupName string    `json:"group_name"`
	Role      string    `json:"role"` // e.g., "owner", "member"
	IsOwner   bool      `json:"is_owner"`
}

// EntityMembershipContext is a generic membership context for any entity type
// This replaces specific contexts (OrganizationMembershipContext, GroupMembershipContext)
// with a unified approach that works for all entity types
type EntityMembershipContext struct {
	EntityID         uuid.UUID              `json:"entity_id"`
	EntityType       string                 `json:"entity_type"`        // e.g., "Organization", "ClassGroup", "Team"
	EntityName       string                 `json:"entity_name"`        // Display name of the entity
	Role             string                 `json:"role"`               // User's role in this entity
	IsOwner          bool                   `json:"is_owner"`           // Quick check if user owns the entity
	Features         []string               `json:"features,omitempty"` // Features from entity subscription (if applicable)
	HasSubscription  bool                   `json:"has_subscription"`   // Whether entity has active subscription (if applicable)
	CustomAttributes map[string]interface{} `json:"custom_attributes,omitempty"` // Entity-specific additional data
}

// UserPermissionsOutput is the comprehensive permission response
type UserPermissionsOutput struct {
	// User identity
	UserID string `json:"user_id"`

	// Casbin computed permissions
	Permissions []PermissionRule `json:"permissions"`

	// User context
	Roles         []string `json:"roles"`           // Casdoor roles
	IsSystemAdmin bool     `json:"is_system_admin"` // Quick check for system admin

	// DEPRECATED: Use EntityMemberships instead (kept for backward compatibility)
	// Organization context
	OrganizationMemberships []OrganizationMembershipContext `json:"organization_memberships,omitempty"`
	// Group context
	GroupMemberships []GroupMembershipContext `json:"group_memberships,omitempty"`

	// NEW: Generic entity memberships (replaces specific membership contexts)
	// Map of entity type to memberships: {"Organization": [...], "ClassGroup": [...]}
	EntityMemberships map[string][]EntityMembershipContext `json:"entity_memberships,omitempty"`

	// Aggregated features from all organizations
	AggregatedFeatures []string `json:"aggregated_features"`

	// Quick access flags (for common checks)
	CanCreateOrganization bool `json:"can_create_organization"`
	CanCreateGroup        bool `json:"can_create_group"`
	HasAnySubscription    bool `json:"has_any_subscription"`
}

// Helper function to convert Casbin permission array to PermissionRule
func CasbinPermissionToRule(permission []string) *PermissionRule {
	if len(permission) < 3 {
		return nil
	}

	// Permission format: [subject, resource, methods]
	// methods might be like "(GET|POST|DELETE)"
	resource := permission[1]
	methodStr := permission[2]

	// Parse methods - remove parentheses and split by |
	methods := parseMethods(methodStr)

	return &PermissionRule{
		Resource: resource,
		Methods:  methods,
	}
}

// parseMethods extracts individual methods from a Casbin methods string
// e.g., "(GET|POST|DELETE)" -> ["GET", "POST", "DELETE"]
func parseMethods(methodStr string) []string {
	// Remove parentheses
	if len(methodStr) > 0 && methodStr[0] == '(' {
		methodStr = methodStr[1:]
	}
	if len(methodStr) > 0 && methodStr[len(methodStr)-1] == ')' {
		methodStr = methodStr[:len(methodStr)-1]
	}

	// Split by |
	methods := []string{}
	if methodStr == "" {
		return methods
	}

	// Simple split by |
	current := ""
	for _, ch := range methodStr {
		if ch == '|' {
			if current != "" {
				methods = append(methods, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		methods = append(methods, current)
	}

	return methods
}

// OrganizationMemberToContext converts OrganizationMemberOutput to context
func OrganizationMemberToContext(member *organizationDto.OrganizationMemberOutput, orgName string, features []string, hasSubscription bool) *OrganizationMembershipContext {
	if member == nil {
		return nil
	}

	roleStr := string(member.Role)
	return &OrganizationMembershipContext{
		OrganizationID:   member.OrganizationID,
		OrganizationName: orgName,
		Role:             roleStr,
		IsOwner:          roleStr == "owner",
		Features:         features,
		HasSubscription:  hasSubscription,
	}
}

// GroupMemberToContext converts GroupMemberOutput to context
func GroupMemberToContext(member *groupDto.GroupMemberOutput, groupName string) *GroupMembershipContext {
	if member == nil {
		return nil
	}

	roleStr := string(member.Role)
	return &GroupMembershipContext{
		GroupID:   member.GroupID,
		GroupName: groupName,
		Role:      roleStr,
		IsOwner:   roleStr == "owner",
	}
}
