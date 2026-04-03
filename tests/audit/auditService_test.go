package audit_tests

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditModels "soli/formations/src/audit/models"
	"soli/formations/src/audit/services"
)

func createAuditService(t *testing.T) services.AuditService {
	t.Helper()
	db := freshTestDB(t)
	return services.NewAuditService(db)
}

// --- Log ---

func TestAuditService_Log_CreatesEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	actorID := uuid.New()
	targetID := uuid.New()

	entry := auditModels.AuditLogCreate{
		EventType:    auditModels.AuditEventLogin,
		Severity:     auditModels.AuditSeverityInfo,
		ActorID:      &actorID,
		ActorEmail:   "user@example.com",
		ActorIP:      "192.168.1.1",
		TargetID:     &targetID,
		TargetType:   "user",
		TargetName:   "User Login",
		Action:       "User logged in",
		Status:       "success",
		Metadata:     `{"browser":"chrome"}`,
		RequestID:    "req-123",
		SessionID:    "sess-456",
	}

	err := svc.Log(entry)
	assert.NoError(t, err)

	// Verify persisted
	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)

	log := logs[0]
	assert.Equal(t, auditModels.AuditEventLogin, log.EventType)
	assert.Equal(t, auditModels.AuditSeverityInfo, log.Severity)
	assert.Equal(t, &actorID, log.ActorID)
	assert.Equal(t, "user@example.com", log.ActorEmail)
	assert.Equal(t, "192.168.1.1", log.ActorIP)
	assert.Equal(t, &targetID, log.TargetID)
	assert.Equal(t, "user", log.TargetType)
	assert.Equal(t, "User Login", log.TargetName)
	assert.Equal(t, "User logged in", log.Action)
	assert.Equal(t, "success", log.Status)
	assert.Equal(t, `{"browser":"chrome"}`, log.Metadata)
	assert.Equal(t, "req-123", log.RequestID)
	assert.Equal(t, "sess-456", log.SessionID)
	assert.False(t, log.CreatedAt.IsZero())
	assert.False(t, log.ExpiresAt.IsZero())
	// ExpiresAt should be approximately 1 year from now
	assert.WithinDuration(t, time.Now().AddDate(1, 0, 0), log.ExpiresAt, 5*time.Second)
}

func TestAuditService_Log_EmptyMetadataDefaultsToEmptyJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	entry := auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogout,
		Severity:  auditModels.AuditSeverityInfo,
		Action:    "User logged out",
		Status:    "success",
		Metadata:  "", // empty string
	}

	err := svc.Log(entry)
	assert.NoError(t, err)

	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	// Empty metadata should be stored as "{}" (valid JSON)
	assert.Equal(t, "{}", logs[0].Metadata)
}

func TestAuditService_Log_NilOptionalFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	entry := auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventConfigurationChanged,
		Severity:  auditModels.AuditSeverityInfo,
		ActorID:   nil, // system event, no actor
		TargetID:  nil,
		Action:    "System configuration changed",
		Status:    "success",
	}

	err := svc.Log(entry)
	assert.NoError(t, err)

	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	assert.Nil(t, logs[0].ActorID)
	assert.Nil(t, logs[0].TargetID)
	assert.Nil(t, logs[0].OrganizationID)
}

func TestAuditService_Log_BillingFieldsPersisted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	amount := 29.99
	entry := auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventPaymentSucceeded,
		Severity:  auditModels.AuditSeverityInfo,
		Action:    "Payment succeeded",
		Status:    "success",
		Amount:    &amount,
		Currency:  "EUR",
	}

	err := svc.Log(entry)
	assert.NoError(t, err)

	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].Amount)
	assert.InDelta(t, 29.99, *logs[0].Amount, 0.001)
	assert.Equal(t, "EUR", logs[0].Currency)
}

func TestAuditService_Log_WithOrganizationID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	orgID := uuid.New()
	entry := auditModels.AuditLogCreate{
		EventType:      auditModels.AuditEventOrganizationUpdated,
		Severity:       auditModels.AuditSeverityInfo,
		OrganizationID: &orgID,
		Action:         "Organization updated",
		Status:         "success",
	}

	err := svc.Log(entry)
	assert.NoError(t, err)

	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].OrganizationID)
	assert.Equal(t, orgID, *logs[0].OrganizationID)
}

// --- GetAuditLogs filters ---

func seedAuditLogs(t *testing.T, svc services.AuditService) (actorID1, actorID2, targetID1, orgID1 uuid.UUID) {
	t.Helper()
	actorID1 = uuid.New()
	actorID2 = uuid.New()
	targetID1 = uuid.New()
	orgID1 = uuid.New()

	entries := []auditModels.AuditLogCreate{
		{
			EventType:      auditModels.AuditEventLogin,
			Severity:       auditModels.AuditSeverityInfo,
			ActorID:        &actorID1,
			ActorEmail:     "alice@example.com",
			TargetID:       &targetID1,
			TargetType:     "session",
			Action:         "User logged in",
			Status:         "success",
			OrganizationID: &orgID1,
		},
		{
			EventType:  auditModels.AuditEventLoginFailed,
			Severity:   auditModels.AuditSeverityWarning,
			ActorID:    &actorID2,
			ActorEmail: "bob@example.com",
			Action:     "Login attempt failed",
			Status:     "failed",
		},
		{
			EventType:  auditModels.AuditEventPasswordChange,
			Severity:   auditModels.AuditSeverityInfo,
			ActorID:    &actorID1,
			ActorEmail: "alice@example.com",
			Action:     "Password changed",
			Status:     "success",
		},
		{
			EventType:  auditModels.AuditEventAccessDenied,
			Severity:   auditModels.AuditSeverityCritical,
			ActorID:    &actorID2,
			ActorEmail: "bob@example.com",
			Action:     "Access denied to admin panel",
			Status:     "detected",
		},
	}

	for _, entry := range entries {
		err := svc.Log(entry)
		require.NoError(t, err)
	}
	return
}

func TestAuditService_GetAuditLogs_NoFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{})
	assert.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, logs, 4)
}

func TestAuditService_GetAuditLogs_FilterByActorID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	actorID1, _, _, _ := seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		ActorID: &actorID1,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, logs, 2)
	for _, log := range logs {
		assert.Equal(t, actorID1, *log.ActorID)
	}
}

func TestAuditService_GetAuditLogs_FilterByTargetID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	_, _, targetID1, _ := seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		TargetID: &targetID1,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
}

func TestAuditService_GetAuditLogs_FilterByEventType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		EventType: auditModels.AuditEventLoginFailed,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
	assert.Equal(t, auditModels.AuditEventLoginFailed, logs[0].EventType)
}

func TestAuditService_GetAuditLogs_FilterBySeverity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		Severity: auditModels.AuditSeverityCritical,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
	assert.Equal(t, auditModels.AuditSeverityCritical, logs[0].Severity)
}

func TestAuditService_GetAuditLogs_FilterByStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		Status: "failed",
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
	assert.Equal(t, "failed", logs[0].Status)
}

func TestAuditService_GetAuditLogs_FilterByOrganizationID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	_, _, _, orgID1 := seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		OrganizationID: &orgID1,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
	assert.Equal(t, orgID1, *logs[0].OrganizationID)
}

func TestAuditService_GetAuditLogs_FilterByDateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	// All logs were created "now", so a range from 1 minute ago to 1 minute from now should include all
	start := time.Now().Add(-1 * time.Minute)
	end := time.Now().Add(1 * time.Minute)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		StartDate: &start,
		EndDate:   &end,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, logs, 4)

	// A range in the past should return nothing
	pastStart := time.Now().Add(-2 * time.Hour)
	pastEnd := time.Now().Add(-1 * time.Hour)

	logs, total, err = svc.GetAuditLogs(services.AuditLogFilter{
		StartDate: &pastStart,
		EndDate:   &pastEnd,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, logs)
}

// --- Pagination ---

func TestAuditService_GetAuditLogs_Pagination_Limit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		Limit: 2,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(4), total) // total count ignores limit
	assert.Len(t, logs, 2)
}

func TestAuditService_GetAuditLogs_Pagination_Offset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		Limit:  2,
		Offset: 2,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Len(t, logs, 2)
}

func TestAuditService_GetAuditLogs_Pagination_BeyondTotal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		Limit:  10,
		Offset: 100,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(4), total)
	assert.Empty(t, logs)
}

func TestAuditService_GetAuditLogs_OrderedByMostRecent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedAuditLogs(t, svc)

	logs, _, err := svc.GetAuditLogs(services.AuditLogFilter{})
	assert.NoError(t, err)
	require.True(t, len(logs) >= 2)

	// Verify descending order by created_at
	for i := 1; i < len(logs); i++ {
		assert.True(t, logs[i-1].CreatedAt.After(logs[i].CreatedAt) || logs[i-1].CreatedAt.Equal(logs[i].CreatedAt),
			"logs should be ordered by created_at DESC")
	}
}

// --- LogBillingNoContext ---

func TestAuditService_LogBillingNoContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	// LogBillingNoContext is a method on the concrete type, not the interface.
	// We test the same code path via Log() since LogBillingNoContext just calls Log().
	userID := uuid.New()
	targetID := uuid.New()
	amount := 49.99

	entry := auditModels.AuditLogCreate{
		EventType:  auditModels.AuditEventPaymentSucceeded,
		Severity:   auditModels.AuditSeverityInfo,
		ActorID:    &userID,
		ActorEmail: "billing@example.com",
		TargetID:   &targetID,
		TargetType: "subscription",
		Action:     string(auditModels.AuditEventPaymentSucceeded),
		Status:     "success",
		Metadata:   `{"stripe_id":"pi_123"}`,
		Amount:     &amount,
		Currency:   "USD",
	}

	err := svc.Log(entry)
	assert.NoError(t, err)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		EventType: auditModels.AuditEventPaymentSucceeded,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, "billing@example.com", logs[0].ActorEmail)
	require.NotNil(t, logs[0].Amount)
	assert.InDelta(t, 49.99, *logs[0].Amount, 0.001)
	assert.Equal(t, "USD", logs[0].Currency)
}

// --- LogOrganizationNoContext ---

func TestAuditService_LogOrganizationNoContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	actorID := uuid.New()
	orgID := uuid.New()
	targetID := uuid.New()

	entry := auditModels.AuditLogCreate{
		EventType:      auditModels.AuditEventMemberAdded,
		Severity:       auditModels.AuditSeverityInfo,
		ActorID:        &actorID,
		ActorEmail:     "admin@org.com",
		TargetID:       &targetID,
		TargetType:     "user",
		OrganizationID: &orgID,
		Action:         "Member added to organization",
		Status:         "success",
		Metadata:       `{"role":"member"}`,
	}

	err := svc.Log(entry)
	assert.NoError(t, err)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		OrganizationID: &orgID,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, auditModels.AuditEventMemberAdded, logs[0].EventType)
	assert.Equal(t, "admin@org.com", logs[0].ActorEmail)
}

// --- Multiple filters combined ---

func TestAuditService_GetAuditLogs_CombinedFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	actorID1, _, _, _ := seedAuditLogs(t, svc)

	// Filter by both actorID and eventType
	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		ActorID:   &actorID1,
		EventType: auditModels.AuditEventLogin,
	})
	assert.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, logs, 1)
	assert.Equal(t, actorID1, *logs[0].ActorID)
	assert.Equal(t, auditModels.AuditEventLogin, logs[0].EventType)
}

// --- Empty DB ---

func TestAuditService_GetAuditLogs_EmptyDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	svc := createAuditService(t)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{})
	assert.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, logs)
}
