package terminalTrainer_tests

// RED tests for two confirmed bugs in the supervision WALL listing
// (ListGroupSupervisionSessions, src/terminalTrainer/routes/supervision.go),
// surfaced by GET /class-groups/:id/terminal-sessions.
//
// Bug A — expired running sessions leak onto the wall. The listing filters
// state='running' but never checks expires_at, so a zombie row (running state,
// tt-backend session long gone, expires_at in the past) renders as a dead tile
// the trainer cannot supervise. The fix must route through the existing SSOT
// scope models.RunningDisplayScope (which already encodes the per-second expiry
// rule), so these tests assert OBSERVABLE behaviour (expired row absent) rather
// than any specific SQL.
//
// Bug B — the response carries no learner identity, only the raw user_id, so a
// tile cannot show who it belongs to. The fix must enrich each session with the
// learner's display name and email (JSON user_name / user_email), resolved via
// the existing swappable Casdoor seam services.LookupCasdoorUserForOrgUsage —
// and MUST NOT drop a session when that resolver errors (the wall must not go
// blank because Casdoor hiccuped).
//
// SEAM CONTRACT pinned for backend-dev (assumed by the Bug B tests):
//   - ListGroupSupervisionSessions enriches each returned session by calling
//     services.LookupCasdoorUserForOrgUsage(userID) per unique learner id.
//   - The value it returns serialises to JSON with fields "user_name" and
//     "user_email" (e.g. a response struct embedding terminalModels.Terminal
//     plus UserName/UserEmail, OR transient fields on the row — the tests pin
//     only the JSON contract, not the Go shape).
//   - A resolver error for a user leaves that session in the result with an
//     empty/fallback name; it is never dropped.
//
// The Bug A/B tests compile against the CURRENT production surface (they touch
// only promoted fields and the JSON serialisation), so they fail on ASSERTION,
// not on compilation.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
)

// seedWallGroup creates an active class-group owned by "wall-owner" with the
// given learner as an active member. It creates NO terminal — each test builds
// the terminal it needs with an explicit expiry so the expiry contract is
// exercised directly rather than via the fixture default.
func seedWallGroup(t *testing.T, db *gorm.DB, groupName, learnerID string) *groupModels.ClassGroup {
	t.Helper()

	group := &groupModels.ClassGroup{
		Name:        groupName,
		DisplayName: groupName,
		OwnerUserID: "wall-owner",
		IsActive:    true,
		MaxMembers:  50,
	}
	require.NoError(t, db.Omit("Metadata").Create(group).Error)
	createTestGroupMember(t, db, group.ID, learnerID, groupModels.GroupMemberRoleMember)
	return group
}

// --- Bug A: expired running sessions must not leak onto the wall -------------

// TestSupervisionWall_ExpiredRunningSession_Excluded pins that a row with
// state='running' but expires_at in the past (a zombie whose tt-backend session
// is gone) is NOT listed on the wall. Currently RED: the listing filters only
// on state and returns the zombie as a dead, unsupervisable tile.
func TestSupervisionWall_ExpiredRunningSession_Excluded(t *testing.T) {
	db := setupTestDB(t)

	group := seedWallGroup(t, db, "group-A", "learner-A")

	expired, err := createTestTerminal(db, "learner-A", "active", time.Now().Add(-time.Hour))
	require.NoError(t, err)

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "admin", true)
	require.True(t, ok)

	present := make(map[string]bool)
	for _, s := range sessions {
		present[s.SessionID] = true
	}
	assert.False(t, present[expired.SessionID],
		"a running-but-expired zombie session must not appear on the supervision wall")
}

// TestSupervisionWall_LiveRunningSession_Included is the paired positive: a
// running session whose expires_at is in the future MUST still be listed. Guards
// against the Bug A fix over-correcting and dropping live, supervisable sessions.
func TestSupervisionWall_LiveRunningSession_Included(t *testing.T) {
	db := setupTestDB(t)

	group := seedWallGroup(t, db, "group-A", "learner-A")

	live, err := createTestTerminal(db, "learner-A", "active", time.Now().Add(time.Hour))
	require.NoError(t, err)

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "admin", true)
	require.True(t, ok)

	present := make(map[string]bool)
	for _, s := range sessions {
		present[s.SessionID] = true
	}
	assert.True(t, present[live.SessionID],
		"a live (non-expired) running session must appear on the supervision wall")
}

// --- Bug B: response must carry the learner's identity ----------------------

// TestSupervisionWall_CarriesLearnerNameAndEmail pins that each listed session
// serialises the learner's display name and email (JSON user_name / user_email),
// resolved through the swappable Casdoor seam. Currently RED: the listing returns
// raw Terminal rows with only user_id, so the marshalled output has no user_name.
func TestSupervisionWall_CarriesLearnerNameAndEmail(t *testing.T) {
	db := setupTestDB(t)

	group := seedWallGroup(t, db, "group-A", "learner-A")
	_, err := createTestTerminal(db, "learner-A", "active", time.Now().Add(time.Hour))
	require.NoError(t, err)

	original := services.LookupCasdoorUserForOrgUsage
	services.LookupCasdoorUserForOrgUsage = func(id string) (*casdoorsdk.User, error) {
		if id == "learner-A" {
			return &casdoorsdk.User{Id: "learner-A", DisplayName: "Alice Liddell", Email: "alice@example.com"}, nil
		}
		return nil, fmt.Errorf("user not found")
	}
	t.Cleanup(func() { services.LookupCasdoorUserForOrgUsage = original })

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "admin", true)
	require.True(t, ok)
	require.Len(t, sessions, 1)

	raw, err := json.Marshal(sessions)
	require.NoError(t, err)
	body := string(raw)

	assert.Contains(t, body, `"user_name":"Alice Liddell"`,
		"listed session must carry the learner's resolved display name")
	assert.Contains(t, body, `"user_email":"alice@example.com"`,
		"listed session must carry the learner's resolved email")
}

// TestSupervisionWall_ResolverError_SessionStillListed pins the fallback: when
// the Casdoor resolver errors for a learner, that learner's session is STILL
// returned (name empty or falling back to user_id). The wall must not go blank
// because Casdoor hiccuped.
func TestSupervisionWall_ResolverError_SessionStillListed(t *testing.T) {
	db := setupTestDB(t)

	group := seedWallGroup(t, db, "group-A", "learner-A")
	term, err := createTestTerminal(db, "learner-A", "active", time.Now().Add(time.Hour))
	require.NoError(t, err)

	original := services.LookupCasdoorUserForOrgUsage
	services.LookupCasdoorUserForOrgUsage = func(id string) (*casdoorsdk.User, error) {
		return nil, fmt.Errorf("casdoor unreachable")
	}
	t.Cleanup(func() { services.LookupCasdoorUserForOrgUsage = original })

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "admin", true)
	require.True(t, ok)

	require.Len(t, sessions, 1, "a resolver error must not drop the session — the wall must not go blank")
	assert.Equal(t, term.SessionID, sessions[0].SessionID,
		"the session must still be listed by its session id when identity resolution fails")
}
