// tests/auth/adminDeleteUser_test.go
//
// Admin user deletion (DELETE /users/:id) must run the SAME erasure flow as the
// self-service DeleteMyAccount: ownership pre-flight → identity/billing
// teardown → OCF-side cascade (GitLab issue #490). Before the fix the admin
// route only called userService.DeleteUser, leaving memberships, settings,
// scenario sessions and the personal organization behind.
//
// Fixtures (setupDeleteMyAccountDB, newUserID, happyMocks, seed*) live in
// deleteMyAccount_test.go; the Casdoor/payment mocks live in
// userDeletion_test.go. This file MUST NOT redefine them.
package auth_tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"soli/formations/src/auth/casdoor"
	"soli/formations/src/auth/mocks"
	authModels "soli/formations/src/auth/models"
	userController "soli/formations/src/auth/routes/usersRoutes"
	authServices "soli/formations/src/auth/services"
	entityManagementModels "soli/formations/src/entityManagement/models"
	groupModels "soli/formations/src/groups/models"
	organizationModels "soli/formations/src/organizations/models"
	scenarioModels "soli/formations/src/scenarios/models"
)

// invokeAdminDeleteUserHandler runs the real DELETE /users/:id gin handler as a
// platform administrator, with the deletion service composed from the mocked
// Casdoor/payment seams over the given DB. It swaps the global Casbin enforcer
// for a mock (the handler removes the user's policies) and restores it after.
func invokeAdminDeleteUserHandler(
	t *testing.T,
	db *gorm.DB,
	targetUserID string,
	casdoorMock *mockCasdoorUserClient,
	helperMock *mockPaymentDeletionHelper,
) (*httptest.ResponseRecorder, *mocks.MockEnforcer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	enforcer := mocks.NewMockEnforcer()
	prevEnforcer := casdoor.Enforcer
	casdoor.Enforcer = enforcer
	t.Cleanup(func() { casdoor.Enforcer = prevEnforcer })

	userSvc := authServices.NewUserService(casdoorMock, helperMock)
	controller := userController.NewUserControllerWithServices(
		userSvc,
		authServices.NewUserSettingsService(),
		authServices.NewUserDeletionService(db, userSvc),
	)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Set("userId", newUserID())
	ctx.Set("userRoles", []string{"administrator"})
	ctx.Params = gin.Params{{Key: "id", Value: targetUserID}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/users/"+targetUserID, nil)

	controller.DeleteUser(ctx)
	return w, enforcer
}

func TestAdminDeleteUser_RunsTheOcfCascade(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	teamOrgID := seedOrgMembership(t, db, userID, "member")
	seedGroupMembership(t, db, userID)
	personalOrgID := seedPersonalOrg(t, db, userID)
	scenarioID := seedScenario(t, db, "other-author")
	sessionID := seedScenarioSessionWithProgress(t, db, userID, scenarioID)
	require.NoError(t, db.Create(&authModels.UserSettings{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		DefaultLandingPage: "/dashboard",
	}).Error)

	casdoorMock, helperMock, _ := happyMocks(userID)
	w, enforcer := invokeAdminDeleteUserHandler(t, db, userID, casdoorMock, helperMock)

	assert.Equal(t, http.StatusNoContent, w.Code, "body=%s", w.Body.String())
	casdoorMock.AssertCalled(t, "DeleteUser", anyArg())

	var orgMemberCount, groupMemberCount, settingsCount, sessionCount, personalOrgCount, teamOrgCount int64
	db.Model(&organizationModels.OrganizationMember{}).Where("user_id = ?", userID).Count(&orgMemberCount)
	db.Model(&groupModels.GroupMember{}).Where("user_id = ?", userID).Count(&groupMemberCount)
	db.Model(&authModels.UserSettings{}).Where("user_id = ?", userID).Count(&settingsCount)
	db.Model(&scenarioModels.ScenarioSession{}).Where("id = ?", sessionID).Count(&sessionCount)
	db.Model(&organizationModels.Organization{}).Where("id = ?", personalOrgID).Count(&personalOrgCount)
	db.Model(&organizationModels.Organization{}).Where("id = ?", teamOrgID).Count(&teamOrgCount)

	assert.Equal(t, int64(0), orgMemberCount, "organization memberships must be removed by the admin path")
	assert.Equal(t, int64(0), groupMemberCount, "group memberships must be removed by the admin path")
	assert.Equal(t, int64(0), settingsCount, "user settings must be removed by the admin path")
	assert.Equal(t, int64(0), sessionCount, "scenario sessions must be removed by the admin path")
	assert.Equal(t, int64(0), personalOrgCount, "the personal organization must be deleted by the admin path")
	assert.Equal(t, int64(1), teamOrgCount, "an organization the user merely belonged to must survive")

	require.Len(t, enforcer.RemovePolicyCalls, 1, "the admin path must still remove the user's Casbin policies")
	assert.Equal(t, userID, enforcer.RemovePolicyCalls[0][0])
}

func TestAdminDeleteUser_RefusesWhenUserOwnsAnOrganization(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	seedOwnedOrg(t, db, userID)
	orgID := seedOrgMembership(t, db, userID, "member")

	casdoorMock, helperMock, _ := happyMocks(userID)
	w, _ := invokeAdminDeleteUserHandler(t, db, userID, casdoorMock, helperMock)

	assert.Equal(t, http.StatusConflict, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), authServices.ErrOwnsOrganizations.Error(),
		"the admin must be told why: ownership has to be transferred first")
	casdoorMock.AssertNotCalled(t, "DeleteUser", anyArg())

	var orgMemberCount int64
	db.Model(&organizationModels.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).Count(&orgMemberCount)
	assert.Equal(t, int64(1), orgMemberCount, "no OCF mutation may happen when the pre-flight refuses")
}

func TestAdminDeleteUser_UnknownUser_Returns404(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()

	casdoorMock := &mockCasdoorUserClient{}
	helperMock := &mockPaymentDeletionHelper{}
	casdoorMock.On("GetUserByUserId", userID).Return(nil, nil)

	w, _ := invokeAdminDeleteUserHandler(t, db, userID, casdoorMock, helperMock)

	assert.Equal(t, http.StatusNotFound, w.Code, "body=%s", w.Body.String())
	casdoorMock.AssertNotCalled(t, "DeleteUser", anyArg())
}

// Regression guard: DeleteMyAccount is now a thin wrapper over EraseUser and
// must still run the full flow (Casdoor delete + OCF cascade).
func TestDeleteMyAccount_StillErasesThroughTheSharedPath(t *testing.T) {
	db := setupDeleteMyAccountDB(t)
	userID := newUserID()
	seedOrgMembership(t, db, userID, "member")
	personalOrgID := seedPersonalOrg(t, db, userID)

	casdoorMock, helperMock, _ := happyMocks(userID)
	svc := composedUserDeletionService(db, casdoorMock, helperMock)

	require.NoError(t, svc.DeleteMyAccount(userID))

	casdoorMock.AssertCalled(t, "DeleteUser", anyArg())
	var orgMemberCount, personalOrgCount int64
	db.Model(&organizationModels.OrganizationMember{}).Where("user_id = ?", userID).Count(&orgMemberCount)
	db.Model(&organizationModels.Organization{}).Where("id = ?", personalOrgID).Count(&personalOrgCount)
	assert.Equal(t, int64(0), orgMemberCount, "self-service deletion must still remove memberships")
	assert.Equal(t, int64(0), personalOrgCount, "self-service deletion must still delete the personal organization")
}
