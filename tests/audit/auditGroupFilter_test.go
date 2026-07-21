package audit_tests

// RED tests for the final piece of #430: making the indexed audit_logs.group_id
// column QUERYABLE. The column + write mapping already exist (see
// supervisionGroupField_test.go); this pins the READ side.
//
// CONTRACT (pinned):
//   - AuditLogFilter gains `GroupID *uuid.UUID` (matching the struct's existing
//     pointer-filter style, e.g. ActorID / TargetID / OrganizationID).
//   - GetAuditLogs applies `WHERE group_id = ?` when the filter's GroupID is set,
//     mirroring the OrganizationID filter at auditService.go:~271; nil GroupID
//     applies no group filtering.
//
// Until AuditLogFilter.GroupID exists this file does NOT compile (it references
// AuditLogFilter{GroupID: ...}) — the expected compile-red — which blocks the
// whole audit test package. Pre-existing audit tests are otherwise unaffected.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditModels "soli/formations/src/audit/models"
	"soli/formations/src/audit/services"
)

// seedGroupScopedLogs writes three login rows through the real audit service:
// one for groupA, one for groupB, one with no group. Returns the two group ids.
func seedGroupScopedLogs(t *testing.T, svc services.AuditService) (groupA, groupB uuid.UUID) {
	t.Helper()
	groupA = uuid.New()
	groupB = uuid.New()

	rows := []auditModels.AuditLogCreate{
		{EventType: auditModels.AuditEventSupervisionStarted, Severity: auditModels.AuditSeverityInfo, GroupID: &groupA, Action: "group A row", Status: "success"},
		{EventType: auditModels.AuditEventSupervisionStarted, Severity: auditModels.AuditSeverityInfo, GroupID: &groupB, Action: "group B row", Status: "success"},
		{EventType: auditModels.AuditEventSupervisionStarted, Severity: auditModels.AuditSeverityInfo, GroupID: nil, Action: "no group row", Status: "success"},
	}
	for _, r := range rows {
		require.NoError(t, svc.Log(r))
	}
	return groupA, groupB
}

// TestAuditService_GetAuditLogs_FilterByGroupID pins that filtering by GroupID
// returns ONLY that group's rows — not the other group's, not the group-less one.
func TestAuditService_GetAuditLogs_FilterByGroupID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	groupA, _ := seedGroupScopedLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{GroupID: &groupA})
	require.NoError(t, err)

	assert.Equal(t, int64(1), total, "only group A's single row must match")
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].GroupID)
	assert.Equal(t, groupA, *logs[0].GroupID, "the returned row must belong to group A")
}

// TestAuditService_GetAuditLogs_NilGroupID_NoGroupFilter pins that a nil GroupID
// filter applies NO group scoping — all rows are returned regardless of group.
func TestAuditService_GetAuditLogs_NilGroupID_NoGroupFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)
	seedGroupScopedLogs(t, svc)

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{}) // GroupID nil
	require.NoError(t, err)

	assert.Equal(t, int64(3), total, "a nil GroupID filter must not scope by group — all rows returned")
	assert.Len(t, logs, 3)
}

// TestAuditService_GetAuditLogs_GroupIDCombinedWithEventType pins that the group
// filter composes with another dimension: GroupID + EventType both apply (AND).
func TestAuditService_GetAuditLogs_GroupIDCombinedWithEventType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB test in short mode")
	}

	db := freshTestDB(t)
	svc := services.NewAuditService(db)

	groupA := uuid.New()
	groupB := uuid.New()

	// group A has two rows of different event types; group B has one that shares
	// the target event type. Only group A + take_hand must match.
	require.NoError(t, svc.Log(auditModels.AuditLogCreate{EventType: auditModels.AuditEventSupervisionTakeHand, Severity: auditModels.AuditSeverityInfo, GroupID: &groupA, Action: "A take_hand", Status: "success"}))
	require.NoError(t, svc.Log(auditModels.AuditLogCreate{EventType: auditModels.AuditEventSupervisionStopped, Severity: auditModels.AuditSeverityInfo, GroupID: &groupA, Action: "A stopped", Status: "success"}))
	require.NoError(t, svc.Log(auditModels.AuditLogCreate{EventType: auditModels.AuditEventSupervisionTakeHand, Severity: auditModels.AuditSeverityInfo, GroupID: &groupB, Action: "B take_hand", Status: "success"}))

	logs, total, err := svc.GetAuditLogs(services.AuditLogFilter{
		GroupID:   &groupA,
		EventType: auditModels.AuditEventSupervisionTakeHand,
	})
	require.NoError(t, err)

	assert.Equal(t, int64(1), total, "only group A's take_hand row must match both filters")
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].GroupID)
	assert.Equal(t, groupA, *logs[0].GroupID)
	assert.Equal(t, auditModels.AuditEventSupervisionTakeHand, logs[0].EventType)
}
