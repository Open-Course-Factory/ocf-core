package terminalTrainer_tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	terminalController "soli/formations/src/terminalTrainer/routes"
)

// The manager of an archived class is refused its live view and controls, and
// is told why: the class is archived, not "you are not a manager". Anyone who
// does not manage it still gets the plain refusal, which reveals nothing.
func TestGroupManagerRefusal_ArchivedClass_SaysSoToItsManagerOnly(t *testing.T) {
	db := setupTestDB(t)
	group, _ := newSupervisedSession(t, db, "archived-class", "trainer-A", "learner-A")
	require.NoError(t, db.Model(group).Update("archived_at", time.Now()).Error)

	assert.Equal(t, "This class is archived: its live view and controls are closed",
		terminalController.GroupManagerRefusal(db, group.ID.String(), "trainer-A"))
	assert.Equal(t, "You are not a manager of this group",
		terminalController.GroupManagerRefusal(db, group.ID.String(), "learner-A"))
}

func TestGroupManagerRefusal_ActiveClass_IsThePlainRefusal(t *testing.T) {
	db := setupTestDB(t)
	group, _ := newSupervisedSession(t, db, "active-class", "trainer-A", "learner-A")

	assert.Equal(t, "You are not a manager of this group",
		terminalController.GroupManagerRefusal(db, group.ID.String(), "learner-A"))
	assert.Equal(t, "You are not a manager of this group",
		terminalController.GroupManagerRefusal(db, "not-a-class", "trainer-A"))
}
