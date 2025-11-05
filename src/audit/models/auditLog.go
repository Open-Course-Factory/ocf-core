package models

import (
	"time"

	"github.com/google/uuid"
)

// AuditEventType represents different categories of auditable events
type AuditEventType string

const (
	// Authentication Events
	AuditEventLogin              AuditEventType = "auth.login"
	AuditEventLoginFailed        AuditEventType = "auth.login.failed"
	AuditEventLogout             AuditEventType = "auth.logout"
	AuditEventPasswordChange     AuditEventType = "auth.password.change"
	AuditEventPasswordReset      AuditEventType = "auth.password.reset"
	AuditEventMFAEnabled         AuditEventType = "auth.mfa.enabled"
	AuditEventMFADisabled        AuditEventType = "auth.mfa.disabled"
	AuditEventTokenRefresh       AuditEventType = "auth.token.refresh"
	AuditEventTokenRevoke        AuditEventType = "auth.token.revoke"
	AuditEventAPIKeyCreated      AuditEventType = "auth.apikey.created"
	AuditEventAPIKeyDeleted      AuditEventType = "auth.apikey.deleted"
	AuditEventSSHKeyAdded        AuditEventType = "auth.sshkey.added"
	AuditEventSSHKeyDeleted      AuditEventType = "auth.sshkey.deleted"

	// User Management Events
	AuditEventUserCreated        AuditEventType = "user.created"
	AuditEventUserUpdated        AuditEventType = "user.updated"
	AuditEventUserDeleted        AuditEventType = "user.deleted"
	AuditEventUserSuspended      AuditEventType = "user.suspended"
	AuditEventUserReactivated    AuditEventType = "user.reactivated"
	AuditEventUserRoleAssigned   AuditEventType = "user.role.assigned"
	AuditEventUserRoleRevoked    AuditEventType = "user.role.revoked"

	// Billing Events
	AuditEventSubscriptionCreated   AuditEventType = "billing.subscription.created"
	AuditEventSubscriptionUpdated   AuditEventType = "billing.subscription.updated"
	AuditEventSubscriptionCanceled  AuditEventType = "billing.subscription.canceled"
	AuditEventSubscriptionRenewed   AuditEventType = "billing.subscription.renewed"
	AuditEventPaymentSucceeded      AuditEventType = "billing.payment.succeeded"
	AuditEventPaymentFailed         AuditEventType = "billing.payment.failed"
	AuditEventRefundIssued          AuditEventType = "billing.refund.issued"
	AuditEventInvoiceGenerated      AuditEventType = "billing.invoice.generated"
	AuditEventBulkPurchase          AuditEventType = "billing.bulk.purchase"
	AuditEventLicenseAssigned       AuditEventType = "billing.license.assigned"
	AuditEventLicenseRevoked        AuditEventType = "billing.license.revoked"

	// Organization Events
	AuditEventOrganizationCreated   AuditEventType = "organization.created"
	AuditEventOrganizationUpdated   AuditEventType = "organization.updated"
	AuditEventOrganizationDeleted   AuditEventType = "organization.deleted"
	AuditEventMemberAdded           AuditEventType = "organization.member.added"
	AuditEventMemberRemoved         AuditEventType = "organization.member.removed"
	AuditEventMemberRoleChanged     AuditEventType = "organization.member.role.changed"
	AuditEventOrganizationSettingsChanged AuditEventType = "organization.settings.changed"

	// Group Events
	AuditEventGroupCreated       AuditEventType = "group.created"
	AuditEventGroupUpdated       AuditEventType = "group.updated"
	AuditEventGroupDeleted       AuditEventType = "group.deleted"
	AuditEventGroupMemberAdded   AuditEventType = "group.member.added"
	AuditEventGroupMemberRemoved AuditEventType = "group.member.removed"

	// Resource Access Events
	AuditEventResourceCreated    AuditEventType = "resource.created"
	AuditEventResourceViewed     AuditEventType = "resource.viewed"
	AuditEventResourceUpdated    AuditEventType = "resource.updated"
	AuditEventResourceDeleted    AuditEventType = "resource.deleted"
	AuditEventResourceShared     AuditEventType = "resource.shared"
	AuditEventResourceUnshared   AuditEventType = "resource.unshared"

	// Security Events
	AuditEventPermissionGranted  AuditEventType = "security.permission.granted"
	AuditEventPermissionRevoked  AuditEventType = "security.permission.revoked"
	AuditEventAccessDenied       AuditEventType = "security.access.denied"
	AuditEventSuspiciousActivity AuditEventType = "security.suspicious.activity"

	// System Events
	AuditEventConfigurationChanged AuditEventType = "system.configuration.changed"
	AuditEventMaintenanceStarted   AuditEventType = "system.maintenance.started"
	AuditEventMaintenanceEnded     AuditEventType = "system.maintenance.ended"
)

// AuditSeverity represents the importance level of an audit event
type AuditSeverity string

const (
	AuditSeverityInfo     AuditSeverity = "info"
	AuditSeverityWarning  AuditSeverity = "warning"
	AuditSeverityError    AuditSeverity = "error"
	AuditSeverityCritical AuditSeverity = "critical"
)

// AuditLog represents a single audit trail entry
// This provides compliance-ready logging for security, authentication, billing, and organizational events
type AuditLog struct {
	ID               uuid.UUID      `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	EventType        AuditEventType `gorm:"type:varchar(100);not null;index"` // Type of event (see constants above)
	Severity         AuditSeverity  `gorm:"type:varchar(20);not null;index"`  // Severity level

	// Actor Information (who performed the action)
	ActorID          *uuid.UUID     `gorm:"type:uuid;index"`                  // User ID who performed the action (null for system events)
	ActorEmail       string         `gorm:"type:varchar(255)"`                // User email for quick reference
	ActorIP          string         `gorm:"type:varchar(45)"`                 // IP address of the actor (supports IPv6)
	ActorUserAgent   string         `gorm:"type:text"`                        // Browser/client user agent

	// Target Information (what was affected)
	TargetID         *uuid.UUID     `gorm:"type:uuid;index"`                  // ID of the affected resource
	TargetType       string         `gorm:"type:varchar(100);index"`          // Type of resource (user, organization, subscription, etc.)
	TargetName       string         `gorm:"type:varchar(255)"`                // Name/identifier of the target for display

	// Organization Context (for multi-tenant filtering)
	OrganizationID   *uuid.UUID     `gorm:"type:uuid;index"`                  // Organization context (null for personal actions)

	// Event Details
	Action           string         `gorm:"type:varchar(255);not null"`       // Human-readable action description
	Status           string         `gorm:"type:varchar(50);not null;index"`  // success, failed, pending
	ErrorMessage     string         `gorm:"type:text"`                        // Error details if status is failed
	Metadata         string         `gorm:"type:jsonb"`                       // Additional context as JSON (e.g., changed fields, amounts)

	// Billing-Specific Fields (for financial audit trail)
	Amount           *float64       `gorm:"type:decimal(10,2)"`               // Transaction amount (for billing events)
	Currency         string         `gorm:"type:varchar(3)"`                  // ISO currency code (USD, EUR, etc.)

	// Security Context
	RequestID        string         `gorm:"type:varchar(100);index"`          // Correlation ID for request tracing
	SessionID        string         `gorm:"type:varchar(100);index"`          // Session identifier

	// Timestamps
	CreatedAt        time.Time      `gorm:"not null;index"`                   // When the event occurred
	ExpiresAt        time.Time      `gorm:"index;not null"`                   // When to delete this record (for retention policy)
}

func (AuditLog) TableName() string {
	return "audit_logs"
}

// AuditLogCreate is a helper struct for creating audit logs
type AuditLogCreate struct {
	EventType      AuditEventType
	Severity       AuditSeverity
	ActorID        *uuid.UUID
	ActorEmail     string
	ActorIP        string
	ActorUserAgent string
	TargetID       *uuid.UUID
	TargetType     string
	TargetName     string
	OrganizationID *uuid.UUID
	Action         string
	Status         string
	ErrorMessage   string
	Metadata       string
	Amount         *float64
	Currency       string
	RequestID      string
	SessionID      string
}
