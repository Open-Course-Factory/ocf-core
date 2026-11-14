package scenarios_test

// Learner names on the live class view come from the shared, cached identity
// lookup (#586). These tests count the Casdoor calls really made, through a fake
// Casdoor HTTP server (testTools.StartFakeCasdoorUsers), so they hold whatever
// seam the code uses. Learner ids are unique per test: the cache is shared
// process-wide.

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/scenarios/services"
	terminalController "soli/formations/src/terminalTrainer/routes"
	terminalModels "soli/formations/src/terminalTrainer/models"
	testTools "soli/formations/tests/testTools"
)

// seedIdentityClass (on a fresh DB) creates a class of n learners with unique ids, each known
// to the fake Casdoor and connected (a running terminal in the class's org).
func seedIdentityClass(t *testing.T, casdoor *testTools.FakeCasdoorUsers, n int) (*gorm.DB, groupModels.ClassGroup, []string) {
	t.Helper()
	db := freshTestDB(t)
	orgID := uuid.New()
	suffix := uuid.NewString()[:8]
	group := createClassGroup(t, db, "identity-class-"+suffix, "teacher-"+suffix, &orgID)
	learners := make([]string, n)
	for i := range learners {
		id := fmt.Sprintf("learner-%s-%02d", suffix, i)
		learners[i] = id
		addGroupMember(t, db, group.ID, id, groupModels.GroupMemberRoleMember)
		createTerminalSession(t, db, id, terminalModels.StateRunning, &orgID, time.Now().Add(time.Hour))
		casdoor.AddUser(id, "Display "+id, "login-"+id, id+"@example.com")
	}
	return db, group, learners
}

// TestGetGroupLiveProgress_ColdClass_ResolvesNamesConcurrently is the reported
// slowness: 40 learners at 50 ms per Casdoor call took 2 s, one call after the
// other. With concurrent fetches the cold load stays under 10 calls' worth.
func TestGetGroupLiveProgress_ColdClass_ResolvesNamesConcurrently(t *testing.T) {
	casdoor := testTools.StartFakeCasdoorUsers(t, 50*time.Millisecond)
	db, group, learners := seedIdentityClass(t, casdoor, 40)
	svc := services.NewTeacherDashboardService(db, nil, nil)

	start := time.Now()
	rows, err := svc.GetGroupLiveProgress(group.ID)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, rows, 40)
	assert.Less(t, elapsed, 10*50*time.Millisecond, "names of a cold class must be fetched concurrently")
	assert.Equal(t, 40, casdoor.Calls(learners...), "each learner is looked up once")
	for _, row := range liveProgressByUserID(rows) {
		assert.Equal(t, "Display "+row.UserID, row.UserName)
		assert.Equal(t, row.UserID+"@example.com", row.UserEmail)
	}
}

// TestGetGroupLiveProgress_SecondLoad_MakesNoCasdoorCall: reloading the class
// view inside the TTL is served from the cache.
func TestGetGroupLiveProgress_SecondLoad_MakesNoCasdoorCall(t *testing.T) {
	casdoor := testTools.StartFakeCasdoorUsers(t, 0)
	db, group, learners := seedIdentityClass(t, casdoor, 5)
	svc := services.NewTeacherDashboardService(db, nil, nil)

	_, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Equal(t, 5, casdoor.Calls(learners...))

	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, casdoor.Calls(learners...), "the second load makes zero Casdoor calls")
	assert.Equal(t, "Display "+rows[0].UserID, rows[0].UserName)
}

// TestGetGroupLiveProgress_EmptyDisplayName_FallsBackToLoginName pins the name
// rule the live view already had (DisplayName, else Name).
func TestGetGroupLiveProgress_EmptyDisplayName_FallsBackToLoginName(t *testing.T) {
	casdoor := testTools.StartFakeCasdoorUsers(t, 0)
	db, group, learners := seedIdentityClass(t, casdoor, 1)
	casdoor.AddUser(learners[0], "", "jdoe-"+learners[0], "jdoe@example.com")
	svc := services.NewTeacherDashboardService(db, nil, nil)

	rows, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "jdoe-"+learners[0], rows[0].UserName)
	assert.Equal(t, "jdoe@example.com", rows[0].UserEmail)
}

// TestLearnerNames_LiveProgressAndLiveWall_ShareOneCache: the live class view
// and the live wall show the same learners; once one has resolved them, the
// other must not ask Casdoor again.
func TestLearnerNames_LiveProgressAndLiveWall_ShareOneCache(t *testing.T) {
	casdoor := testTools.StartFakeCasdoorUsers(t, 0)
	db, group, learners := seedIdentityClass(t, casdoor, 5)
	svc := services.NewTeacherDashboardService(db, nil, nil)

	_, err := svc.GetGroupLiveProgress(group.ID)
	require.NoError(t, err)
	require.Equal(t, 5, casdoor.Calls(learners...))

	sessions, ok := terminalController.ListGroupSupervisionSessions(db, group.ID.String(), "platform-admin", true)
	require.True(t, ok)
	require.Len(t, sessions, 5)
	assert.Equal(t, 5, casdoor.Calls(learners...), "the wall reuses the names live progress just resolved")
	for _, s := range sessions {
		assert.Equal(t, "Display "+s.Terminal.UserID, s.UserName)
		assert.Equal(t, s.Terminal.UserID+"@example.com", s.UserEmail)
	}
}
