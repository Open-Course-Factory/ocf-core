package terminalTrainer_tests

// Learner names on the trainer surfaces that poll — the participant-controls
// bulk read and the live wall — come from the shared, cached identity lookup
// (#586): before it, every request asked Casdoor once per class member.
//
// The tests count the Casdoor calls really made, through a fake Casdoor HTTP
// server (testTools.StartFakeCasdoorUsers), so they hold whatever seam the code
// goes through. The display-name seam is reset to the real SDK call for the
// test, and member ids are unique per test: the cache is shared process-wide.

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	groupModels "soli/formations/src/groups/models"
	"soli/formations/src/terminalTrainer/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
	"soli/formations/src/terminalTrainer/services"
	testTools "soli/formations/tests/testTools"
)

type identityClass struct {
	db      *gorm.DB
	groupID uuid.UUID
	owner   string
	members []string // owner first, then the learners
}

// newIdentityClass seeds a class with a unique owner and three unique
// learners, each with a running session in the class's org and known to the
// fake Casdoor.
func newIdentityClass(t *testing.T, casdoor *testTools.FakeCasdoorUsers) *identityClass {
	t.Helper()
	db := freshTestDB(t)
	original := services.LookupCasdoorUserForOrgUsage
	services.LookupCasdoorUserForOrgUsage = casdoorsdk.GetUserByUserId
	t.Cleanup(func() { services.LookupCasdoorUserForOrgUsage = original })

	suffix := uuid.NewString()[:8]
	orgID := uuid.New()
	owner := "id-owner-" + suffix
	group := &groupModels.ClassGroup{Name: "id-class-" + suffix, DisplayName: "id-class", OwnerUserID: owner, MaxMembers: 50, OrganizationID: &orgID}
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	c := &identityClass{db: db, groupID: group.ID, owner: owner, members: []string{owner}}
	createTestGroupMember(t, db, group.ID, owner, groupModels.GroupMemberRoleOwner)
	for _, n := range []string{"a", "b", "c"} {
		learner := "id-learner-" + n + "-" + suffix
		createTestGroupMember(t, db, group.ID, learner, groupModels.GroupMemberRoleMember)
		c.members = append(c.members, learner)
	}
	for _, id := range c.members {
		pcTerminal(t, db, id, &orgID, models.StateRunning, "")
		casdoor.AddUser(id, "Display "+id, "login-"+id, id+"@example.com")
	}
	return c
}

func (c *identityClass) readControls(t *testing.T) map[string]pcParticipantView {
	t.Helper()
	w := pcDo(t, c.db, member(c.owner), http.MethodGet, controlsPath(c.groupID), nil)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var rows []pcParticipantView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	byUser := make(map[string]pcParticipantView, len(rows))
	for _, row := range rows {
		byUser[row.UserID] = row
	}
	return byUser
}

// TestGetParticipantControls_SecondRead_MakesNoCasdoorCall: the bulk read is
// polled; a second read inside the TTL must not reach Casdoor at all.
func TestGetParticipantControls_SecondRead_MakesNoCasdoorCall(t *testing.T) {
	casdoor := testTools.StartFakeCasdoorUsers(t, 0)
	c := newIdentityClass(t, casdoor)

	first := c.readControls(t)
	require.Len(t, first, len(c.members))
	require.Equal(t, len(c.members), casdoor.Calls(c.members...), "the cold read looks each member up once")

	second := c.readControls(t)
	assert.Equal(t, len(c.members), casdoor.Calls(c.members...), "the second read makes zero Casdoor calls")
	for _, id := range c.members {
		assert.Equal(t, "Display "+id, second[id].UserName, "user_name unchanged")
	}
}

// TestListGroupSupervisionSessions_SecondPoll_MakesNoCasdoorCall: the live wall
// polls every 30 s; only the first poll may ask Casdoor.
func TestListGroupSupervisionSessions_SecondPoll_MakesNoCasdoorCall(t *testing.T) {
	casdoor := testTools.StartFakeCasdoorUsers(t, 0)
	c := newIdentityClass(t, casdoor)

	_, ok := terminalController.ListGroupSupervisionSessions(c.db, c.groupID.String(), c.owner, false)
	require.True(t, ok)
	require.Equal(t, len(c.members), casdoor.Calls(c.members...))

	sessions, ok := terminalController.ListGroupSupervisionSessions(c.db, c.groupID.String(), c.owner, false)
	require.True(t, ok)
	require.Len(t, sessions, len(c.members))
	assert.Equal(t, len(c.members), casdoor.Calls(c.members...), "the second poll makes zero Casdoor calls")
	for _, s := range sessions {
		assert.Equal(t, "Display "+s.Terminal.UserID, s.UserName)
		assert.Equal(t, s.Terminal.UserID+"@example.com", s.UserEmail)
	}
}

// TestGetParticipantControls_ColdClass_ResolvesNamesConcurrently: the cold read
// of a class must not cost one Casdoor round trip per member in sequence.
func TestGetParticipantControls_ColdClass_ResolvesNamesConcurrently(t *testing.T) {
	casdoor := testTools.StartFakeCasdoorUsers(t, 5*time.Second)
	c := newIdentityClass(t, casdoor)

	rows := c.readControls(t)

	require.Len(t, rows, len(c.members))
	assert.Greater(t, casdoor.MaxInFlight(), 1, "members must be fetched concurrently, not one by one")
}

// TestGetParticipantControls_EmptyDisplayName_FallsBackToLoginName: the wall
// and participant controls used DisplayName only (a blank name when unset);
// the shared lookup unifies them on the live view's rule, DisplayName else Name.
func TestGetParticipantControls_EmptyDisplayName_FallsBackToLoginName(t *testing.T) {
	casdoor := testTools.StartFakeCasdoorUsers(t, 0)
	c := newIdentityClass(t, casdoor)
	learner := c.members[1]
	casdoor.AddUser(learner, "", "login-"+learner, learner+"@example.com")

	rows := c.readControls(t)
	assert.Equal(t, "login-"+learner, rows[learner].UserName)
	assert.Equal(t, "Display "+c.owner, rows[c.owner].UserName)
}
