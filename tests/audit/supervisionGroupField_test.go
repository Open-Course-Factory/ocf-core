package audit_tests

// RED tests for issue #430 — dedicated managing-group field on supervision audit
// events.
//
// Today supervision events (terminal.supervision.*) OVERLOAD the audit model's
// OrganizationID column to carry the MANAGING CLASS-GROUP id — see
// buildSupervisionAuditStatus in src/terminalTrainer/routes/supervision.go
// (`entry.OrganizationID = &id // reuse the org column to index by managing
// group`). That conflates two different concepts on one indexed column: a real
// org context vs the class-group through which a trainer supervises. These tests
// pin the target contract.
//
// FIELD-NAME DECISION (pinned): the audit model gains `GroupID *uuid.UUID`
// (column `group_id`, `gorm:"type:uuid;index"`). Chosen over `ContextGroupID`
// to match the model's existing `<Noun>ID` naming (`ActorID`, `TargetID`,
// `OrganizationID`, `OnBehalfOfID`) — a parallel group-context column sitting
// beside OrganizationID. If backend-dev prefers `ContextGroupID`, rename here;
// the CONTRACT these tests pin is "a dedicated, nullable, indexed uuid column
// carrying the managing group, distinct from OrganizationID".
//
// DE-OVERLOAD DIRECTION (pinned): on supervision events OrganizationID becomes
// NIL, not "a real org id". buildSupervisionAudit is a pure builder whose only
// inputs are actorUserID / sessionID / groupID — it has NO way to know an org
// id (no DB handle, no org param), so the only honest de-overload is to stop
// populating OrganizationID entirely and move the group id to GroupID. Threading
// a real org id in is a separate future change, out of scope here.
//
// SCHEMA-SETUP NOTES for backend-dev:
//   - The AuditLog model uses Postgres-only column defaults, so tests/audit
//     hardcodes the audit_logs schema in main_test.go via CREATE TABLE (NOT
//     AutoMigrate). This file's `group_id TEXT` column + idx_audit_logs_group_id
//     index were added there, mirroring the on_behalf_of_id precedent.
//   - The fix must add `GroupID *uuid.UUID gorm:"type:uuid;index"` to
//     models.AuditLog AND models.AuditLogCreate, map it through auditService.Log
//     (like OrganizationID at auditService.go:69), add a real Postgres migration
//     for the column, and change buildSupervisionAuditStatus to set GroupID
//     (leaving OrganizationID nil).
//   - Until the GroupID field exists on the model, THIS FILE DOES NOT COMPILE
//     (it references row.GroupID), which blocks the whole audit test package —
//     the expected compile-red. The pre-existing audit tests are otherwise
//     unaffected (the extra SQL column is inert for them).

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditModels "soli/formations/src/audit/models"
	"soli/formations/src/audit/services"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// runSupervisionStopped drives a REAL supervision audit write through the real
// audit service and returns the persisted row. EndSupervision (handHeld=false)
// emits exactly one terminal.supervision.stopped row via the same
// buildSupervisionAudit mapping that started/take_hand/released all use, so
// pinning stopped pins the shared mapping. EndSupervision does not read `db` on
// this path — it only logs — so no terminal/group tables are needed here.
func runSupervisionStopped(t *testing.T, actorUUID, groupUUID uuid.UUID) auditModels.AuditLog {
	t.Helper()
	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	err := terminalController.EndSupervision(db, svc, actorUUID.String(), false, "sess-1", groupUUID.String(), false)
	require.NoError(t, err)

	var logs []auditModels.AuditLog
	require.NoError(t, db.Where("event_type = ?", auditModels.AuditEventSupervisionStopped).Find(&logs).Error)
	require.Len(t, logs, 1, "EndSupervision(handHeld=false) must emit exactly one stopped row")
	return logs[0]
}

// TestAuditLog_GroupID_Column_RoundTrips is the column smoke test (mirrors
// TestAuditLog_OnBehalfOfID_Column_Migrates): a row written with GroupID set
// reads back with the same value, proving the dedicated column exists and
// persists independently of any supervision mapping.
func TestAuditLog_GroupID_Column_RoundTrips(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)

	groupID := uuid.New()
	actorID := uuid.New()
	row := &auditModels.AuditLog{
		ID:        uuid.New(),
		EventType: auditModels.AuditEventSupervisionStarted,
		Severity:  auditModels.AuditSeverityInfo,
		ActorID:   &actorID,
		GroupID:   &groupID,
		Action:    "test group_id column",
		Status:    "success",
		Metadata:  "{}",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, db.Create(row).Error)

	var got auditModels.AuditLog
	require.NoError(t, db.First(&got, "id = ?", row.ID).Error)
	require.NotNil(t, got.GroupID, "GroupID must round-trip from the DB")
	assert.Equal(t, groupID, *got.GroupID)
}

// TestSupervisionAudit_StoppedEvent_CarriesManagingGroupInGroupField pins item
// (1): a real supervision audit row, written through the real audit service,
// carries the managing class-group id in the NEW dedicated GroupID field.
// Currently RED: buildSupervisionAudit puts the group id on OrganizationID and
// never populates GroupID.
func TestSupervisionAudit_StoppedEvent_CarriesManagingGroupInGroupField(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	actorID := uuid.New()
	groupID := uuid.New()

	got := runSupervisionStopped(t, actorID, groupID)

	require.NotNil(t, got.GroupID,
		"a supervision event must record the managing group in the dedicated GroupID field")
	assert.Equal(t, groupID, *got.GroupID,
		"GroupID must be the managing class-group derived server-side")
}

// TestSupervisionAudit_StoppedEvent_DoesNotOverloadOrganizationID pins item (3),
// the de-overload direction: a supervision event must NOT stuff the managing
// group id into OrganizationID anymore. Because the pure builder cannot know a
// real org id, OrganizationID must be nil on supervision events. Currently RED:
// buildSupervisionAudit sets OrganizationID = the group id.
func TestSupervisionAudit_StoppedEvent_DoesNotOverloadOrganizationID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	actorID := uuid.New()
	groupID := uuid.New()

	got := runSupervisionStopped(t, actorID, groupID)

	assert.Nil(t, got.OrganizationID,
		"supervision events must no longer overload OrganizationID with the managing group id")
	if got.OrganizationID != nil {
		assert.NotEqual(t, groupID, *got.OrganizationID,
			"the managing group id must never appear on the OrganizationID column")
	}
}

// TestAuditLog_GroupID_NilForNonSupervisionEvent pins item (2), the zero-input
// case: a non-supervision event that supplies no group leaves GroupID nil (the
// column is nullable / absent). Mirrors TestAuditService_Log_NilOptionalFields.
func TestAuditLog_GroupID_NilForNonSupervisionEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	err := svc.Log(auditModels.AuditLogCreate{
		EventType: auditModels.AuditEventLogin,
		Severity:  auditModels.AuditSeverityInfo,
		Action:    "User logged in",
		Status:    "success",
	})
	require.NoError(t, err)

	var logs []auditModels.AuditLog
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Nil(t, logs[0].GroupID,
		"a non-supervision event that supplies no group must leave GroupID nil")
}
