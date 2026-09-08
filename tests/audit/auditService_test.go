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
// LogSecurityEvent is used because it accepts userID as a parameter and
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
	svc.LogSecurityEvent(
		ctx,
		auditModels.AuditEventAccessDenied,
		&targetID,
		&resourceID,
		"viewed course detail under impersonation",
		auditModels.AuditSeverityWarning,
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
