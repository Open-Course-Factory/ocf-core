package scenarios_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/models"
	"soli/formations/src/scenarios/services"
)

// --- Service-level tests for TeacherDashboardService.GetSessionDetails (bulk) ---
//
// Issue #320. The implementer will add:
//   - TeacherDashboardService.GetSessionDetails(groupID uuid.UUID, sessionIDs []uuid.UUID) ([]*SessionDetailResponse, error)
//   - HTTP handler at POST /teacher/groups/:groupId/sessions/details
//   - Constant: maxSessionDetailsBulkSize = 200
//
// The KISS implementation loops calling the existing GetSessionDetail; these
// tests pin the contract: ordering, empty input, limit, IDOR, missing session.

// seedBulkSessionDetailsScenario creates a group, an assigned scenario with one
// step, and `n` sessions (one per fresh member). Returns the group ID and the
// ordered session IDs.
func seedBulkSessionDetailsScenario(t *testing.T, prefix string, n int) (uuid.UUID, []uuid.UUID) {
	t.Helper()
	db := freshTestDB(t)

	groupID := uuid.New()
	scenario := models.Scenario{
		Name: prefix + "-bulk-detail", Title: "Bulk Detail", InstanceType: "ubuntu:22.04", CreatedByID: "c1",
	}
	require.NoError(t, db.Create(&scenario).Error)

	require.NoError(t, db.Create(&models.ScenarioAssignment{
		ScenarioID: scenario.ID, GroupID: &groupID, Scope: "group", CreatedByID: "c1", IsActive: true,
	}).Error)

	require.NoError(t, db.Create(&models.ScenarioStep{
		ScenarioID: scenario.ID, Order: 0, Title: "Step 1",
	}).Error)

	sessionIDs := make([]uuid.UUID, 0, n)
	for i := 0; i < n; i++ {
		userID := prefix + "-student-" + uuid.New().String()
		require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
			GroupID: groupID, UserID: userID, Role: "member", JoinedAt: time.Now(), IsActive: true,
		}).Error)

		session := models.ScenarioSession{
			ScenarioID: scenario.ID, UserID: userID, Status: "active", StartedAt: time.Now(),
		}
		require.NoError(t, db.Create(&session).Error)

		require.NoError(t, db.Create(&models.ScenarioStepProgress{
			SessionID: session.ID, StepOrder: 0, Status: "active",
		}).Error)

		sessionIDs = append(sessionIDs, session.ID)
	}

	return groupID, sessionIDs
}

func TestGetSessionDetails_ReturnsItemsInInputOrder(t *testing.T) {
	groupID, ids := seedBulkSessionDetailsScenario(t, "order", 3)
	// Reuse the shared DB (seed helper used freshTestDB; sharedTestDB still points there).
	svc := services.NewTeacherDashboardService(sharedTestDB, nil, nil)

	// Reorder: [s2, s0, s1]
	input := []uuid.UUID{ids[2], ids[0], ids[1]}
	details, err := svc.GetSessionDetails(groupID, input)
	require.NoError(t, err)
	require.Len(t, details, 3)

	// Verify input order is preserved
	assert.Equal(t, ids[2], details[0].SessionID, "first item must match first input id")
	assert.Equal(t, ids[0], details[1].SessionID, "second item must match second input id")
	assert.Equal(t, ids[1], details[2].SessionID, "third item must match third input id")

	// Basic field sanity — each item carries its own UserID (non-empty)
	for i, d := range details {
		assert.NotEmpty(t, d.UserID, "details[%d].UserID should be populated", i)
	}
}

func TestGetSessionDetails_EmptyInput_ReturnsEmpty(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTeacherDashboardService(db, nil, nil)

	details, err := svc.GetSessionDetails(uuid.New(), []uuid.UUID{})
	require.NoError(t, err)
	assert.Len(t, details, 0)
}

func TestGetSessionDetails_ExceedsLimit_ReturnsError(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTeacherDashboardService(db, nil, nil)

	// 201 random UUIDs — limit check should fire BEFORE any DB lookup, so no
	// seeding is needed. Implementer's limit is 200.
	ids := make([]uuid.UUID, 201)
	for i := range ids {
		ids[i] = uuid.New()
	}

	details, err := svc.GetSessionDetails(uuid.New(), ids)
	require.Error(t, err)
	assert.Nil(t, details)
	// The error message should mention either "too many" or the limit "200".
	msg := err.Error()
	assert.True(t, contains(msg, "too many") || contains(msg, "200"),
		"error message %q should mention the limit (substring 'too many' or '200')", msg)
}

func TestGetSessionDetails_SessionFromAnotherGroup_ReturnsError(t *testing.T) {
	// Seed group g1 with a session, then query for g2 (which does not contain that session's user).
	_, ids := seedBulkSessionDetailsScenario(t, "idor", 1)

	// A second, unrelated group ID — no member, no assignment for g2, so
	// querying the seeded session under g2 must trip the IDOR check.
	g2 := uuid.New()
	svc := services.NewTeacherDashboardService(sharedTestDB, nil, nil)

	details, err := svc.GetSessionDetails(g2, []uuid.UUID{ids[0]})
	require.Error(t, err)
	assert.Nil(t, details)
	assert.Contains(t, err.Error(), "session does not belong to this group")
}

func TestGetSessionDetails_InvalidSessionID_ReturnsError(t *testing.T) {
	db := freshTestDB(t)
	svc := services.NewTeacherDashboardService(db, nil, nil)

	// Random ID, no seeded session.
	details, err := svc.GetSessionDetails(uuid.New(), []uuid.UUID{uuid.New()})
	require.Error(t, err)
	assert.Nil(t, details)
	assert.Contains(t, err.Error(), "session not found")
}

// --- HTTP handler test ---

func TestGetSessionDetailsBulkHandler_ReturnsItems(t *testing.T) {
	// Seed two sessions in a group; query the bulk endpoint as the group owner.
	groupID, ids := seedBulkSessionDetailsScenario(t, "http", 2)

	// Make the caller (handler-test user) a group owner so Layer 2 GroupRole
	// (manager) passes. Use a stable userID and add as owner.
	ownerID := "http-bulk-owner"
	require.NoError(t, sharedTestDB.Omit("Metadata").Create(&groupModels.GroupMember{
		GroupID: groupID, UserID: ownerID, Role: groupModels.GroupMemberRoleOwner,
		JoinedAt: time.Now(), IsActive: true,
	}).Error)

	router := setupRealTeacherRouter(t, sharedTestDB, ownerID, []string{"member"})

	body, _ := json.Marshal(map[string]interface{}{
		"session_ids": []uuid.UUID{ids[0], ids[1]},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(
		"POST",
		"/api/v1/teacher/groups/"+groupID.String()+"/sessions/details",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "response body: %s", w.Body.String())

	var resp struct {
		Items []services.SessionDetailResponse `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Items, 2)
	assert.Equal(t, ids[0], resp.Items[0].SessionID)
	assert.Equal(t, ids[1], resp.Items[1].SessionID)
}

// TestGetSessionDetails_Batch_MatchesIndividualCalls asserts that the batched
// GetSessionDetails produces the same output as N individual GetSessionDetail
// calls. Guards against merge bugs in the post-query mapping (scenarios by ID,
// progress by session ID, steps by (scenario_id, order)).
//
// This is a CONTRACT-LOCK test, not a RED→GREEN test. The current production
// implementation of GetSessionDetails is a thin loop over GetSessionDetail, so
// the equivalence is trivially satisfied today. The test exists to fail when
// the upcoming refactor swaps the loop for batched IN queries — any drift in
// nested fields (steps, quiz questions, timestamps, correct counts) will trip
// reflect.DeepEqual.
//
// The seed helper creates ONE scenario shared by N sessions, so the eventual
// batched implementation will naturally exercise scenario-de-dup (single
// scenario row fetched once, fanned out to N response items).
func TestGetSessionDetails_Batch_MatchesIndividualCalls(t *testing.T) {
	groupID, sessionIDs := seedBulkSessionDetailsScenario(t, "batch-equiv", 5)
	svc := services.NewTeacherDashboardService(sharedTestDB, nil, nil)

	// Call batched.
	batched, err := svc.GetSessionDetails(groupID, sessionIDs)
	if err != nil {
		t.Fatalf("GetSessionDetails returned error: %v", err)
	}
	if len(batched) != len(sessionIDs) {
		t.Fatalf("expected %d batched results, got %d", len(sessionIDs), len(batched))
	}

	// Call individual N times and compare every field via DeepEqual.
	for i, id := range sessionIDs {
		single, err := svc.GetSessionDetail(groupID, id)
		if err != nil {
			t.Fatalf("GetSessionDetail(%s) returned error: %v", id, err)
		}
		if !reflect.DeepEqual(batched[i], single) {
			t.Errorf("batched[%d] differs from individual:\n  batched:    %+v\n  individual: %+v", i, batched[i], single)
		}
	}
}

// contains is a tiny helper to avoid importing "strings" twice in a row;
// keeps the test file self-contained and explicit about substring intent.
func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
