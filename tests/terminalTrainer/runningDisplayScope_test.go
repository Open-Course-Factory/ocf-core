package terminalTrainer_tests

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/terminalTrainer/models"
	"soli/formations/src/terminalTrainer/repositories"
	terminalServices "soli/formations/src/terminalTrainer/services"
)

// These tests pin the SSOT for the "is this terminal alive RIGHT NOW?"
// predicate (RunningDisplayScope in models/terminal.go). Every display /
// listing call that asks for "active terminals" must route through the
// scope; before the consolidation, callers expressed the rule inline as
// `status = 'active'` with no `expires_at` filter, drifting away from
// OccupiesSlotScope and the per-second-aware UI.
//
// A zombie terminal here means: status = 'active' but expires_at is in
// the past. Pre-fix, callers leaked these rows into "active" results;
// post-fix, every display caller excludes them via the scope.

// TestGetTerminalSessionsByUserID_ExcludesPastExpiry ensures the repository
// only returns alive sessions when isActive=true. Pre-fix the inline query
// `status = "active"` returned past-expiry rows too, drifting from the
// slot scope.
func TestGetTerminalSessionsByUserID_ExcludesPastExpiry(t *testing.T) {
	db := freshTestDB(t)

	userKey, err := createTestUserKey(db, "user-rds-1")
	require.NoError(t, err)

	// Live terminal (status=active, future expiry)
	live := &models.Terminal{
		SessionID:         "live-" + uuid.New().String(),
		UserID:            "user-rds-1",
		State:            "running",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(live).Error)

	// Zombie terminal (status=active but expired)
	zombie := &models.Terminal{
		SessionID:         "zombie-" + uuid.New().String(),
		UserID:            "user-rds-1",
		State:            "running",
		ExpiresAt:         time.Now().Add(-30 * time.Minute),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(zombie).Error)

	repo := repositories.NewTerminalRepository(db)
	got, err := repo.GetTerminalSessionsByUserID("user-rds-1", true)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, 1, len(*got),
		"GetTerminalSessionsByUserID(isActive=true) must exclude past-expiry rows (SSOT alignment with RunningDisplayScope)")
	if len(*got) == 1 {
		assert.Equal(t, live.SessionID, (*got)[0].SessionID,
			"only the live (future-expiry) terminal should be returned")
	}
}

// TestGetTerminalSessionsByUserID_IsActiveFalseUnchanged is the
// regression guard for the isActive=false branch: callers asking for
// "every terminal regardless of status" must keep getting every row
// (active + stopped + expired), so the scope must NOT apply on that
// branch.
func TestGetTerminalSessionsByUserID_IsActiveFalseUnchanged(t *testing.T) {
	db := freshTestDB(t)

	userKey, err := createTestUserKey(db, "user-rds-2")
	require.NoError(t, err)

	live := &models.Terminal{
		SessionID:         "live2-" + uuid.New().String(),
		UserID:            "user-rds-2",
		State:            "running",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(live).Error)

	stopped := &models.Terminal{
		SessionID:         "stopped2-" + uuid.New().String(),
		UserID:            "user-rds-2",
		State:            "stopped",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(stopped).Error)

	zombie := &models.Terminal{
		SessionID:         "zombie2-" + uuid.New().String(),
		UserID:            "user-rds-2",
		State:            "running",
		ExpiresAt:         time.Now().Add(-30 * time.Minute),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
	}
	require.NoError(t, db.Create(zombie).Error)

	repo := repositories.NewTerminalRepository(db)
	got, err := repo.GetTerminalSessionsByUserID("user-rds-2", false)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, 3, len(*got),
		"GetTerminalSessionsByUserID(isActive=false) must return every terminal regardless of status/expiry")
}

// TestGetTerminalSessionsByUserIDAndOrg_ExcludesPastExpiry pins the same
// rule for the org-scoped variant.
func TestGetTerminalSessionsByUserIDAndOrg_ExcludesPastExpiry(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	org := createTestOrgForHistory(t, db, "owner-rds-3")
	userKey, err := createTestUserKey(db, "user-rds-3")
	require.NoError(t, err)

	live := &models.Terminal{
		SessionID:         "live3-" + uuid.New().String(),
		UserID:            "user-rds-3",
		State:            "running",
		ExpiresAt:         time.Now().Add(1 * time.Hour),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(live).Error)

	zombie := &models.Terminal{
		SessionID:         "zombie3-" + uuid.New().String(),
		UserID:            "user-rds-3",
		State:            "running",
		ExpiresAt:         time.Now().Add(-30 * time.Minute),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(zombie).Error)

	repo := repositories.NewTerminalRepository(db)
	got, err := repo.GetTerminalSessionsByUserIDAndOrg("user-rds-3", &org.ID, true)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, 1, len(*got),
		"GetTerminalSessionsByUserIDAndOrg(isActive=true) must exclude past-expiry rows")
}

// TestGetGroupCommandHistory_ExcludesPastExpiry pins the rule for the
// group command history endpoint when includeStopped=false. Without
// routing through the scope, a zombie active terminal would inflate the
// query result and produce a downstream HTTP call to tt-backend for a
// dead session.
//
// The function returns empty results when no live terminals match, so we
// assert: with a single zombie + no live, the response should be empty
// (zero commands, zero sessions). Without the fix the zombie session
// would be discovered, the tt-backend call would happen, and either
// produce results from a stale endpoint or fail the test by HTTP error.
func TestGetGroupCommandHistory_ExcludesPastExpiry(t *testing.T) {
	db := setupTestDBWithGroups(t)

	org := createTestOrgForHistory(t, db, "trainer-rds-4")
	group := createTestGroupForHistory(t, db, "trainer-rds-4", &org.ID)
	createTestGroupMember(t, db, group.ID, "trainer-rds-4", groupModels.GroupMemberRoleOwner)
	createTestGroupMember(t, db, group.ID, "student-rds-4", groupModels.GroupMemberRoleMember)

	userKey, err := createTestUserKey(db, "student-rds-4")
	require.NoError(t, err)

	zombie := &models.Terminal{
		SessionID:         "zombie4-" + uuid.New().String(),
		UserID:            "student-rds-4",
		State:            "running",
		ExpiresAt:         time.Now().Add(-30 * time.Minute), // past expiry
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(zombie).Error)

	service := terminalServices.NewTerminalTrainerService(db)
	body, contentType, err := service.GetGroupCommandHistory(
		group.ID.String(), "trainer-rds-4", nil, "json", 50, 0, false /* includeStopped */, "",
	)
	require.NoError(t, err,
		"GetGroupCommandHistory with includeStopped=false must filter out the zombie terminal locally and return empty rather than hit tt-backend")
	assert.Equal(t, "application/json", contentType)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	total, ok := result["total"].(float64)
	require.True(t, ok, "Response must contain 'total'")
	assert.Equal(t, float64(0), total,
		"Past-expiry terminals must not contribute to group command history when includeStopped=false")
}

// TestGetGroupCommandHistoryStats_ExcludesPastExpiry pins the same rule
// for the stats variant.
func TestGetGroupCommandHistoryStats_ExcludesPastExpiry(t *testing.T) {
	db := setupTestDBWithGroups(t)

	org := createTestOrgForHistory(t, db, "trainer-rds-5")
	group := createTestGroupForHistory(t, db, "trainer-rds-5", &org.ID)
	createTestGroupMember(t, db, group.ID, "trainer-rds-5", groupModels.GroupMemberRoleOwner)
	createTestGroupMember(t, db, group.ID, "student-rds-5", groupModels.GroupMemberRoleMember)

	userKey, err := createTestUserKey(db, "student-rds-5")
	require.NoError(t, err)

	zombie := &models.Terminal{
		SessionID:         "zombie5-" + uuid.New().String(),
		UserID:            "student-rds-5",
		State:            "running",
		ExpiresAt:         time.Now().Add(-30 * time.Minute),
		InstanceType:      "test",
		MachineSize:       "S",
		UserTerminalKeyID: userKey.ID,
		OrganizationID:    &org.ID,
	}
	require.NoError(t, db.Create(zombie).Error)

	service := terminalServices.NewTerminalTrainerService(db)
	body, contentType, err := service.GetGroupCommandHistoryStats(
		group.ID.String(), "trainer-rds-5", false /* includeStopped */,
	)
	require.NoError(t, err,
		"GetGroupCommandHistoryStats with includeStopped=false must filter out the zombie terminal locally and return empty stats")
	assert.Equal(t, "application/json", contentType)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	summary, ok := result["summary"].(map[string]interface{})
	require.True(t, ok, "Response must contain 'summary'")
	assert.Equal(t, float64(0), summary["total_sessions"],
		"Past-expiry terminals must not contribute to total_sessions when includeStopped=false")
}
