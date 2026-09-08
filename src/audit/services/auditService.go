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
	LogSecurityEvent(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, targetID *uuid.UUID, action string, severity models.AuditSeverity)
	Log(entry models.AuditLogCreate) error
}

type auditService struct {
	db *gorm.DB
}

// NewAuditService creates a new audit logging service
func NewAuditService(db *gorm.DB) AuditService {
	return &auditService{db: db}
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
		GroupID:        entry.GroupID,
		OnBehalfOfID:   entry.OnBehalfOfID,
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

	entry := models.AuditLogCreate{
		EventType:      eventType,
		Severity:       severity,
		ActorID:        userID,
		ActorEmail:     email,
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: getUserAgent(ctx),
		Action:         fmt.Sprintf("User %s", eventType),
		Status:         status,
		ErrorMessage:   errorMsg,
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	}
	applyImpersonationFromContext(ctx, &entry)
	as.Log(entry)
}

// LogBilling logs billing and payment-related events
func (as *auditService) LogBilling(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, targetID *uuid.UUID, targetType string, amount *float64, currency string, metadata map[string]interface{}) {
	metadataJSON, _ := json.Marshal(metadata)

	severity := models.AuditSeverityInfo
	if eventType == models.AuditEventPaymentFailed {
		severity = models.AuditSeverityWarning
	}

	entry := models.AuditLogCreate{
		EventType:      eventType,
		Severity:       severity,
		ActorID:        userID,
		ActorEmail:     getActorEmail(ctx),
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: getUserAgent(ctx),
		TargetID:       targetID,
		TargetType:     targetType,
		Action:         fmt.Sprintf("Billing event: %s", eventType),
		Status:         "success",
		Metadata:       string(metadataJSON),
		Amount:         amount,
		Currency:       currency,
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	}
	applyImpersonationFromContext(ctx, &entry)
	as.Log(entry)
}

// LogSecurityEvent logs security-related events
func (as *auditService) LogSecurityEvent(ctx *gin.Context, eventType models.AuditEventType, userID *uuid.UUID, targetID *uuid.UUID, action string, severity models.AuditSeverity) {
	entry := models.AuditLogCreate{
		EventType:      eventType,
		Severity:       severity,
		ActorID:        userID,
		ActorEmail:     getActorEmail(ctx),
		ActorIP:        getClientIP(ctx),
		ActorUserAgent: getUserAgent(ctx),
		TargetID:       targetID,
		Action:         action,
		Status:         "detected",
		RequestID:      getRequestID(ctx),
		SessionID:      getSessionID(ctx),
	}
	applyImpersonationFromContext(ctx, &entry)
	as.Log(entry)
}

// Helper functions to extract context information

func getClientIP(ctx *gin.Context) string {
	if ctx == nil || ctx.Request == nil {
		return ""
	}
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
	if ctx == nil {
		return ""
	}
	// Try to get request ID from header (if set by reverse proxy/middleware)
	var requestID string
	if ctx.Request != nil {
		requestID = ctx.GetHeader("X-Request-ID")
	}
	if requestID == "" {
		// Generate a new one if not present
		requestID = uuid.New().String()
		ctx.Set("request_id", requestID)
	}
	return requestID
}

// getUserAgent returns the request User-Agent or "" if the request is unset
// (e.g. in tests that build a gin.Context without an HTTP request).
func getUserAgent(ctx *gin.Context) string {
	if ctx == nil || ctx.Request == nil {
		return ""
	}
	return ctx.Request.UserAgent()
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

// applyImpersonationFromContext rewrites the entry's ActorID to the real
// human (impersonator) and records the impersonated user in OnBehalfOfID,
// when the request context indicates an active impersonation.
//
// Behaviour:
//   - No "impersonatorId" in ctx → entry left unchanged.
//   - Malformed impersonatorId   → entry left unchanged (no panic).
//   - entry.OnBehalfOfID already set → entry left unchanged (explicit caller wins).
//   - Otherwise: OnBehalfOfID = current ActorID (the impersonation middleware
//     has already substituted ctx.userId to the target), ActorID = impersonator.
func applyImpersonationFromContext(ctx *gin.Context, entry *models.AuditLogCreate) {
	if ctx == nil || entry == nil {
		return
	}
	raw, exists := ctx.Get("impersonatorId")
	if !exists {
		return
	}
	impersonatorStr, ok := raw.(string)
	if !ok {
		return
	}
	impersonatorUUID, err := uuid.Parse(impersonatorStr)
	if err != nil {
		// Malformed value — fail gracefully, do not corrupt the entry.
		return
	}
	if entry.OnBehalfOfID != nil {
		// Caller explicitly supplied OnBehalfOfID — preserve it.
		return
	}
	// Move the existing ActorID (the target — already swapped by the
	// impersonation middleware into ctx.userId and propagated by the caller)
	// into OnBehalfOfID, and replace ActorID with the real human admin.
	entry.OnBehalfOfID = entry.ActorID
	id := impersonatorUUID
	entry.ActorID = &id
}
