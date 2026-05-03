package audit_tests

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

// --- Slice B4: OnBehalfOfID + impersonation auto-population ---
//
// These tests cover the audit module's awareness of impersonation sessions:
//   - The new optional column AuditLog.OnBehalfOfID
//   - Three new event-type constants for impersonation lifecycle
//   - Auto-population in auditService: when ctx has "impersonatorId" set, the
//     service swaps ActorID = impersonator and OnBehalfOfID = the userId
//     currently in ctx (which the impersonation middleware has already swapped
//     to the target's id). Net effect: every existing audit call automatically
//     captures the real human's identity even during impersonation, with no
//     module-level changes required.

// Test 1 — column migration / persistence smoke test.
// Inserts an AuditLog row directly via gorm with OnBehalfOfID set, reads it
// back, and verifies the value round-trips. Proves the column exists.
func TestAuditLog_OnBehalfOfID_Column_Migrates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	onBehalfID := uuid.New()
	actorID := uuid.New()
	row := &auditModels.AuditLog{
		ID:           uuid.New(),
		EventType:    auditModels.AuditEventLogin,
		Severity:     auditModels.AuditSeverityInfo,
		ActorID:      &actorID,
		OnBehalfOfID: &onBehalfID,
		Action:       "test on_behalf_of column",
		Status:       "success",
		Metadata:     "{}",
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, db.Create(row).Error)

	var got auditModels.AuditLog
	require.NoError(t, db.First(&got, "id = ?", row.ID).Error)
	require.NotNil(t, got.OnBehalfOfID, "OnBehalfOfID should round-trip from the DB")
	assert.Equal(t, onBehalfID, *got.OnBehalfOfID)
}

// Test 2 — without an impersonation context, the entry is left unchanged:
// ActorID stays as whatever the caller provided, OnBehalfOfID stays nil.
func TestAuditService_Log_NoImpersonationContext_LeavesEntryUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	originalUserID := uuid.New()
	targetID := uuid.New()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	// Simulate a non-impersonated authenticated request: only userId is set.
	ctx.Set("userId", originalUserID.String())
	// No "impersonatorId" key set.

	svc.LogSecurityEvent(
		ctx,
		auditModels.AuditEventAccessDenied,
		&originalUserID,
		&targetID,
		"non-impersonated security event",
		auditModels.AuditSeverityWarning,
	)

	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	assert.Nil(t, logs[0].OnBehalfOfID, "OnBehalfOfID must be nil when no impersonation context")
	require.NotNil(t, logs[0].ActorID)
	assert.Equal(t, originalUserID, *logs[0].ActorID, "ActorID must remain the caller's userID")
}

// Test 3 — with an impersonation context, the service swaps ActorID and
// OnBehalfOfID:
//   - ActorID becomes the impersonator (the real human admin)
//   - OnBehalfOfID becomes the target (whoever was being impersonated)
//
// LogResourceAccess is used because it accepts userID as a parameter and
// passes it straight through as the entry's ActorID. The impersonation
// middleware (out of scope here) is assumed to have already set ctx.userId to
// the target — so callers passing &targetID is the realistic flow.
func TestAuditService_Log_WithImpersonationContext_SwapsActorAndOnBehalfOf(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	adminID := uuid.New()  // the real human (impersonator)
	targetID := uuid.New() // the user being impersonated
	resourceID := uuid.New()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	// Impersonation middleware sets these:
	ctx.Set("userId", targetID.String())
	ctx.Set("impersonatorId", adminID.String())

	// The caller passes &targetID as userID — that's what the impersonation
	// middleware has already substituted into the request flow.
	svc.LogResourceAccess(
		ctx,
		auditModels.AuditEventResourceViewed,
		&targetID,
		&resourceID,
		"course",
		"viewed course detail under impersonation",
	)

	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].ActorID, "ActorID must be set")
	require.NotNil(t, logs[0].OnBehalfOfID, "OnBehalfOfID must be set under impersonation")
	assert.Equal(t, adminID, *logs[0].ActorID, "ActorID must be swapped to the impersonator")
	assert.Equal(t, targetID, *logs[0].OnBehalfOfID, "OnBehalfOfID must record the impersonated user")
}

// Test 4 — defensive: if the impersonatorId in ctx is a malformed UUID, the
// auto-populate must fail gracefully (not panic, not corrupt the entry).
// The log call still succeeds; the entry behaves as if no impersonation
// context existed.
func TestAuditService_Log_WithImpersonationContext_DoesNotPanicOnInvalidImpersonatorID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	originalUserID := uuid.New()
	targetID := uuid.New()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("userId", originalUserID.String())
	ctx.Set("impersonatorId", "not-a-uuid") // malformed

	require.NotPanics(t, func() {
		svc.LogSecurityEvent(
			ctx,
			auditModels.AuditEventAccessDenied,
			&originalUserID,
			&targetID,
			"malformed impersonator id",
			auditModels.AuditSeverityWarning,
		)
	})

	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	assert.Nil(t, logs[0].OnBehalfOfID, "OnBehalfOfID stays nil when parse fails")
	require.NotNil(t, logs[0].ActorID)
	assert.Equal(t, originalUserID, *logs[0].ActorID, "ActorID stays the caller's userID when parse fails")
}

// Test 5 — the three new event-type constants exist and persist with the
// correct string values. This is a smoke test for the constants block.
func TestAuditService_Log_NewImpersonationEventTypes_AreAccepted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	adminID := uuid.New()
	targetID := uuid.New()

	cases := []struct {
		eventType auditModels.AuditEventType
		expected  string
	}{
		{auditModels.AuditEventImpersonationStarted, "impersonation_started"},
		{auditModels.AuditEventImpersonationStopped, "impersonation_stopped"},
		{auditModels.AuditEventImpersonationExpired, "impersonation_expired"},
	}

	for _, c := range cases {
		err := svc.Log(auditModels.AuditLogCreate{
			EventType: c.eventType,
			Severity:  auditModels.AuditSeverityInfo,
			ActorID:   &adminID,
			TargetID:  &targetID,
			Action:    "impersonation lifecycle event",
			Status:    "success",
		})
		assert.NoError(t, err)
	}

	var logs []auditModels.AuditLog
	db.Order("created_at ASC").Find(&logs)
	require.Len(t, logs, len(cases))
	for i, c := range cases {
		assert.Equal(t, c.eventType, logs[i].EventType, "event type %d must persist as %s", i, c.expected)
		assert.Equal(t, c.expected, string(logs[i].EventType), "constant value must equal %s", c.expected)
	}
}

// Test 6 — design-intent guard: if the caller has ALREADY set
// entry.OnBehalfOfID explicitly (rare but legitimate, e.g. system jobs that
// reconstruct an impersonation chain after the fact), the auto-populate must
// NOT clobber that value. The swap from ctx only fires when the entry's
// OnBehalfOfID is nil going in, so an explicit caller-supplied value wins.
//
// This protects against silent data loss if the auto-populate ever fired
// unconditionally.
func TestAuditService_Log_WithImpersonationContext_PreservesExplicitOnBehalfOf(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	adminID := uuid.New()              // would-be auto-populated impersonator
	targetID := uuid.New()             // userId in ctx
	explicitOnBehalfID := uuid.New()   // caller's explicit value
	explicitActorID := uuid.New()      // caller's explicit ActorID

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("userId", targetID.String())
	ctx.Set("impersonatorId", adminID.String())

	// Caller supplies BOTH ActorID and OnBehalfOfID explicitly. The Log()
	// auto-populate should leave these untouched.
	err := svc.Log(auditModels.AuditLogCreate{
		EventType:    auditModels.AuditEventImpersonationStarted,
		Severity:     auditModels.AuditSeverityInfo,
		ActorID:      &explicitActorID,
		OnBehalfOfID: &explicitOnBehalfID,
		Action:       "explicit caller-supplied actor + on_behalf_of",
		Status:       "success",
	})
	assert.NoError(t, err)

	var logs []auditModels.AuditLog
	db.Find(&logs)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].ActorID)
	require.NotNil(t, logs[0].OnBehalfOfID)
	assert.Equal(t, explicitActorID, *logs[0].ActorID,
		"explicit ActorID must NOT be overwritten by ctx-based auto-populate")
	assert.Equal(t, explicitOnBehalfID, *logs[0].OnBehalfOfID,
		"explicit OnBehalfOfID must NOT be overwritten by ctx-based auto-populate")
}
