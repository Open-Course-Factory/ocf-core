package payment_tests

// Assigning a seat from a batch linked to a group also enrols the learner in that
// group. That enrolment used to be a direct INSERT, which skipped everything the
// service path does: the group's MaxMembers, its expiry, whether the caller may
// manage it at all, and — worst — the Casbin grant, leaving a learner with a
// group_members row and no binding, a member of a group they could not see.
//
// It goes through GroupService.AddMembersToGroup now. These tests pin the
// behaviours that were being skipped; the Casbin grant itself rides on that same
// canonical path, which is exactly the point of delegating rather than
// re-implementing.

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	groupServices "soli/formations/src/groups/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedGroupWithOwner creates a group owned by ownerID, with the owner enrolled,
// and the given member cap and expiry.
func seedGroupWithOwner(
	t *testing.T,
	db *gorm.DB,
	ownerID string,
	maxMembers int,
	expiresAt *time.Time,
) uuid.UUID {
	t.Helper()

	group := &groupModels.ClassGroup{
		BaseModel:   entityManagementModels.BaseModel{ID: uuid.New()},
		Name:        "class-" + uuid.NewString()[:8],
		DisplayName: "Class",
		OwnerUserID: ownerID,
		MaxMembers:  maxMembers,
		ExpiresAt:   expiresAt,
	}
	require.NoError(t, db.Omit("Metadata").Create(group).Error)

	require.NoError(t, db.Omit("Metadata").Create(&groupModels.GroupMember{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		GroupID:   group.ID,
		UserID:    ownerID,
		Role:      groupModels.GroupMemberRoleOwner,
		JoinedAt:  time.Now(),
		IsActive:  true,
	}).Error)

	return group.ID
}

func countActiveMembers(t *testing.T, db *gorm.DB, groupID uuid.UUID) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&groupModels.GroupMember{}).
		Where("group_id = ? AND is_active = ?", groupID, true).Count(&n).Error)
	return n
}

func TestGroupEnrolment_AddsTheLearner(t *testing.T) {
	db := freshTestDB(t)
	const trainer = "trainer-marc"
	groupID := seedGroupWithOwner(t, db, trainer, 30, nil)

	err := groupServices.NewGroupService(db).AddMembersToGroup(
		groupID, trainer, []string{"learner-1"}, groupModels.GroupMemberRoleMember)

	require.NoError(t, err)
	assert.Equal(t, int64(2), countActiveMembers(t, db, groupID), "owner plus the new learner")
}

// The cap was ignored entirely by the direct insert, so a 30-seat class could end
// up with 40 learners and nothing would say so.
func TestGroupEnrolment_RespectsTheMemberCap(t *testing.T) {
	db := freshTestDB(t)
	const trainer = "trainer-marc"
	// Cap of 1, already filled by the owner.
	groupID := seedGroupWithOwner(t, db, trainer, 1, nil)

	err := groupServices.NewGroupService(db).AddMembersToGroup(
		groupID, trainer, []string{"learner-overflow"}, groupModels.GroupMemberRoleMember)

	require.Error(t, err, "a full group must refuse another member")
	assert.Equal(t, int64(1), countActiveMembers(t, db, groupID), "and must not have grown")
}

// An expired class is over. Adding learners to it produced members of something
// that no longer runs.
func TestGroupEnrolment_RefusesAnExpiredGroup(t *testing.T) {
	db := freshTestDB(t)
	const trainer = "trainer-marc"
	yesterday := time.Now().Add(-24 * time.Hour)
	groupID := seedGroupWithOwner(t, db, trainer, 30, &yesterday)

	err := groupServices.NewGroupService(db).AddMembersToGroup(
		groupID, trainer, []string{"learner-late"}, groupModels.GroupMemberRoleMember)

	require.Error(t, err, "an expired class must not take new members")
	assert.Equal(t, int64(1), countActiveMembers(t, db, groupID))
}

// The direct insert never asked whether the caller could manage the group, so a
// purchaser could enrol learners into a class belonging to someone else.
func TestGroupEnrolment_RefusesACallerWhoCannotManageTheGroup(t *testing.T) {
	db := freshTestDB(t)
	groupID := seedGroupWithOwner(t, db, "trainer-marc", 30, nil)

	err := groupServices.NewGroupService(db).AddMembersToGroup(
		groupID, "unrelated-purchaser", []string{"learner-1"}, groupModels.GroupMemberRoleMember)

	require.Error(t, err, "enrolling into someone else's class must be refused")
	assert.Equal(t, int64(1), countActiveMembers(t, db, groupID))
}

// Re-assigning a seat to someone already enrolled must not duplicate them.
func TestGroupEnrolment_IsIdempotentForAnExistingMember(t *testing.T) {
	db := freshTestDB(t)
	const trainer = "trainer-marc"
	groupID := seedGroupWithOwner(t, db, trainer, 30, nil)
	svc := groupServices.NewGroupService(db)

	require.NoError(t, svc.AddMembersToGroup(
		groupID, trainer, []string{"learner-1"}, groupModels.GroupMemberRoleMember))
	require.NoError(t, svc.AddMembersToGroup(
		groupID, trainer, []string{"learner-1"}, groupModels.GroupMemberRoleMember))

	assert.Equal(t, int64(2), countActiveMembers(t, db, groupID),
		"a second assignment must not enrol the same learner twice")
}
