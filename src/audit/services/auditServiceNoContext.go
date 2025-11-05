package services

import (
	"encoding/json"
	"log"

	"soli/formations/src/audit/models"

	"github.com/google/uuid"
)

// LogBillingNoContext logs billing events without HTTP context (e.g., from webhooks)
func (as *auditService) LogBillingNoContext(eventType models.AuditEventType, userID *uuid.UUID, email string, targetID *uuid.UUID, targetType string, amount *float64, currency string, metadata map[string]interface{}, status string) {
	metadataJSON, _ := json.Marshal(metadata)

	severity := models.AuditSeverityInfo
	if status == "failed" {
		severity = models.AuditSeverityWarning
	}

	err := as.Log(models.AuditLogCreate{
		EventType:    eventType,
		Severity:     severity,
		ActorID:      userID,
		ActorEmail:   email,
		TargetID:     targetID,
		TargetType:   targetType,
		Action:       string(eventType),
		Status:       status,
		Metadata:     string(metadataJSON),
		Amount:       amount,
		Currency:     currency,
	})

	if err != nil {
		log.Printf("❌ [AUDIT] Failed to log billing event: %v", err)
	}
}

// LogOrganizationNoContext logs organization events without HTTP context
func (as *auditService) LogOrganizationNoContext(eventType models.AuditEventType, actorID *uuid.UUID, actorEmail string, orgID *uuid.UUID, targetID *uuid.UUID, targetType string, action string, metadata map[string]interface{}) {
	metadataJSON, _ := json.Marshal(metadata)

	err := as.Log(models.AuditLogCreate{
		EventType:      eventType,
		Severity:       models.AuditSeverityInfo,
		ActorID:        actorID,
		ActorEmail:     actorEmail,
		TargetID:       targetID,
		TargetType:     targetType,
		OrganizationID: orgID,
		Action:         action,
		Status:         "success",
		Metadata:       string(metadataJSON),
	})

	if err != nil {
		log.Printf("❌ [AUDIT] Failed to log organization event: %v", err)
	}
}
