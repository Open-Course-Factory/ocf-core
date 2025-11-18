package services

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"soli/formations/src/audit/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditService provides methods for creating audit log entries
type AuditService interface {
	LogAuthentication(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, email string, status string, errorMsg string)
	LogBilling(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, targetID *uuid.UUID, targetType string, amount *float64, currency string, metadata map[string]interface{})
	LogOrganization(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, orgID *uuid.UUID, targetID *uuid.UUID, targetType string, action string, metadata map[string]interface{})
	LogUserManagement(ctx *gin.Context, eventType models.AuditEventType, actorID *uuid.UUID, targetUserID *uuid.UUID, targetEmail string, action string, metadata map[string]interface{})
	LogSecurityEvent(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, targetID *uuid.UUID, action string, severity models.AuditSeverity)
	LogResourceAccess(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, resourceID *uuid.UUID, resourceType string, action string)
	Log(entry models.AuditLogCreate) error
	GetAuditLogs(filter AuditLogFilter) ([]models.AuditLog, int64, error)
}

type auditService struct {
	db *gorm.DB
}

// NewAuditService creates a new audit logging service
func NewAuditService(db *gorm.DB) AuditService {
	return &auditService{db: db}
}

// AuditLogFilter provides filtering options for querying audit logs
type AuditLogFilter struct {
	ActorID        *uuid.UUID
	TargetID       *uuid.UUID
	OrganizationID *uuid.UUID
	EventType      models.AuditEventType
	Severity       models.AuditSeverity
	Status         string
	StartDate      *time.Time
	EndDate        *time.Time
	Limit          int
	Offset         int
}

// Log creates a new audit log entry with the provided details
func (as *auditService) Log(entry models.AuditLogCreate) error {
	// Ensure Metadata is valid JSON (empty string is invalid for jsonb)
	metadata := entry.Metadata
	if metadata == "" {
		metadata = "{}" // Use empty JSON object instead of empty string
	}

	auditLog := &models.AuditLog{
		EventType:      entry.EventType,
		Severity:       entry.Severity,
		ActorID:        entry.ActorID,
		ActorEmail:     entry.ActorEmail,
		ActorIP:        entry.ActorIP,
		ActorUserAgent: entry.ActorUserAgent,
		TargetID:       entry.TargetID,
		TargetType:     entry.TargetType,
		TargetName:     entry.TargetName,
		OrganizationID: entry.OrganizationID,
		Action:         entry.Action,
		Status:         entry.Status,
		ErrorMessage:   entry.ErrorMessage,
		Metadata:       metadata,
		Amount:         entry.Amount,
		Currency:       entry.Currency,
		RequestID:      entry.RequestID,
		SessionID:      entry.SessionID,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().AddDate(1, 0, 0), // Default: 1 year retention
	}

	result := as.db.Create(auditLog)
	if result.Error != nil {
		log.Printf("❌ [AUDIT] Failed to create audit log: %v", result.Error)
		return result.Error
	}

	// Log to console for immediate visibility (can be disabled in production)
	logLevel := "INFO"
	switch entry.Severity {
	case models.AuditSeverityWarning:
		logLevel = "WARN"
	case models.AuditSeverityError, models.AuditSeverityCritical:
		logLevel = "ERROR"
	}

	log.Printf("[AUDIT:%s] %s | Actor: %s | Target: %s (%s) | Status: %s",
		logLevel,
		entry.EventType,
		entry.ActorEmail,
		entry.TargetName,
		entry.TargetType,
		entry.Status,
	)

	return nil
}

// LogAuthentication logs authentication-related events
func (as *auditService) LogAuthentication(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, email string, status string, errorMsg string) {
	severity := models.AuditSeverityInfo
	if status == "failed" {
		severity = models.AuditSeverityWarning
	}

	as.Log(models.AuditLogCreate{
		EventType:      eventType,
		Severity:       severity,
		ActorID:        userID,
		ActorEmail:     email,
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: ctx.Request.UserAgent(),
		Action:         fmt.Sprintf("User %s", eventType),
		Status:         status,
		ErrorMessage:   errorMsg,
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	})
}

// LogBilling logs billing and payment-related events
func (as *auditService) LogBilling(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, targetID *uuid.UUID, targetType string, amount *float64, currency string, metadata map[string]interface{}) {
	metadataJSON, _ := json.Marshal(metadata)

	severity := models.AuditSeverityInfo
	if eventType == models.AuditEventPaymentFailed {
		severity = models.AuditSeverityWarning
	}

	as.Log(models.AuditLogCreate{
		EventType:      eventType,
		Severity:       severity,
		ActorID:        userID,
		ActorEmail:     getActorEmail(ctx),
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: ctx.Request.UserAgent(),
		TargetID:       targetID,
		TargetType:     targetType,
		Action:         fmt.Sprintf("Billing event: %s", eventType),
		Status:         "success",
		Metadata:       string(metadataJSON),
		Amount:         amount,
		Currency:       currency,
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	})
}

// LogOrganization logs organization-related events
func (as *auditService) LogOrganization(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, orgID *uuid.UUID, targetID *uuid.UUID, targetType string, action string, metadata map[string]interface{}) {
	metadataJSON, _ := json.Marshal(metadata)

	as.Log(models.AuditLogCreate{
		EventType:      eventType,
		Severity:       models.AuditSeverityInfo,
		ActorID:        userID,
		ActorEmail:     getActorEmail(ctx),
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: ctx.Request.UserAgent(),
		TargetID:       targetID,
		TargetType:     targetType,
		OrganizationID: orgID,
		Action:         action,
		Status:         "success",
		Metadata:       string(metadataJSON),
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	})
}

// LogUserManagement logs user management events
func (as *auditService) LogUserManagement(ctx *gin.Context, eventType models.AuditEventType, actorID *uuid.UUID, targetUserID *uuid.UUID, targetEmail string, action string, metadata map[string]interface{}) {
	metadataJSON, _ := json.Marshal(metadata)

	severity := models.AuditSeverityInfo
	if eventType == models.AuditEventUserDeleted || eventType == models.AuditEventUserSuspended {
		severity = models.AuditSeverityWarning
	}

	as.Log(models.AuditLogCreate{
		EventType:      eventType,
		Severity:       severity,
		ActorID:        actorID,
		ActorEmail:     getActorEmail(ctx),
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: ctx.Request.UserAgent(),
		TargetID:       targetUserID,
		TargetType:     "user",
		TargetName:     targetEmail,
		Action:         action,
		Status:         "success",
		Metadata:       string(metadataJSON),
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	})
}

// LogSecurityEvent logs security-related events
func (as *auditService) LogSecurityEvent(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, targetID *uuid.UUID, action string, severity models.AuditSeverity) {
	as.Log(models.AuditLogCreate{
		EventType:      eventType,
		Severity:       severity,
		ActorID:        userID,
		ActorEmail:     getActorEmail(ctx),
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: ctx.Request.UserAgent(),
		TargetID:       targetID,
		Action:         action,
		Status:         "detected",
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	})
}

// LogResourceAccess logs access to resources
func (as *auditService) LogResourceAccess(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, resourceID *uuid.UUID, resourceType string, action string) {
	as.Log(models.AuditLogCreate{
		EventType:      eventType,
		Severity:       models.AuditSeverityInfo,
		ActorID:        userID,
		ActorEmail:     getActorEmail(ctx),
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: ctx.Request.UserAgent(),
		TargetID:       resourceID,
		TargetType:     resourceType,
		Action:         action,
		Status:         "success",
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	})
}

// GetAuditLogs retrieves audit logs based on the provided filter
func (as *auditService) GetAuditLogs(filter AuditLogFilter) ([]models.AuditLog, int64, error) {
	var logs []models.AuditLog
	var total int64

	query := as.db.Model(&models.AuditLog{})

	// Apply filters
	if filter.ActorID != nil {
		query = query.Where("actor_id = ?", filter.ActorID)
	}
	if filter.TargetID != nil {
		query = query.Where("target_id = ?", filter.TargetID)
	}
	if filter.OrganizationID != nil {
		query = query.Where("organization_id = ?", filter.OrganizationID)
	}
	if filter.EventType != "" {
		query = query.Where("event_type = ?", filter.EventType)
	}
	if filter.Severity != "" {
		query = query.Where("severity = ?", filter.Severity)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.StartDate != nil {
		query = query.Where("created_at >= ?", filter.StartDate)
	}
	if filter.EndDate != nil {
		query = query.Where("created_at <= ?", filter.EndDate)
	}

	// Get total count
	query.Count(&total)

	// Apply pagination
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	// Order by most recent first
	result := query.Order("created_at DESC").Find(&logs)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	return logs, total, nil
}

// Helper functions to extract context information

func getClientIP(ctx *gin.Context) string {
	// Try to get real IP from X-Forwarded-For or X-Real-IP headers
	ip := ctx.GetHeader("X-Forwarded-For")
	if ip == "" {
		ip = ctx.GetHeader("X-Real-IP")
	}
	if ip == "" {
		ip = ctx.ClientIP()
	}
	return ip
}

func getRequestID(ctx *gin.Context) string {
	// Try to get request ID from header (if set by reverse proxy/middleware)
	requestID := ctx.GetHeader("X-Request-ID")
	if requestID == "" {
		// Generate a new one if not present
		requestID = uuid.New().String()
		ctx.Set("request_id", requestID)
	}
	return requestID
}

func getSessionID(ctx *gin.Context) string {
	// Try to get session ID from context (set by auth middleware)
	if sessionID, exists := ctx.Get("session_id"); exists {
		return sessionID.(string)
	}
	return ""
}

func getActorEmail(ctx *gin.Context) string {
	// Try to get actor email from context (set by auth middleware)
	if email, exists := ctx.Get("user_email"); exists {
		return email.(string)
	}
	return ""
}
