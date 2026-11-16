package payment_tests

// The generic DELETE soft-deletes an organization_members row and leaves
// is_active true: a removed member loses what the organization gave them.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	access "soli/formations/src/auth/access"

	entityManagementModels "soli/formations/src/entityManagement/models"
	orgModels "soli/formations/src/organizations/models"
	paymentMiddleware "soli/formations/src/payment/middleware"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func removeOrgMember(t *testing.T, db *gorm.DB, orgID uuid.UUID, userID string) {
	t.Helper()
	require.NoError(t, db.Where("organization_id = ? AND user_id = ?", orgID, userID).Delete(&orgModels.OrganizationMember{}).Error)
}

func TestGetUserOrganizationSubscriptions_RemovedMember_GetsNone(t *testing.T) {
	db := freshTestDB(t)
	org := seedSharedTeamOrg(t, db, "sub-owner", "sub-member", orgModels.OrgRoleMember)
	plan := livePlan(t, db, "Team Plan", 10)
	require.NoError(t, db.Create(&models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
	}).Error)
	repo := repositories.NewOrganizationSubscriptionRepository(db)

	subs, err := repo.GetUserOrganizationSubscriptions("sub-member")
	require.NoError(t, err)
	require.Len(t, subs, 1, "control: a member gets the organization's subscription")

	removeOrgMember(t, db, org.ID, "sub-member")
	subs, err = repo.GetUserOrganizationSubscriptions("sub-member")
	require.NoError(t, err)
	assert.Empty(t, subs, "a removed member no longer gets the organization's subscription")
}

func TestGetAccessibleBatchByID_RemovedMember_Denied(t *testing.T) {
	db := freshTestDB(t)
	_, batch, _ := seedBulkLicenseTestData(t, db, "batch-purchaser", 3, 0)
	org := seedSharedTeamOrg(t, db, "batch-purchaser", "batch-member", orgModels.OrgRoleMember)
	removeOrgMember(t, db, org.ID, "batch-member")

	_, err := services.NewBulkLicenseService(db).GetAccessibleBatchByID(batch.ID, "batch-member")
	assert.Error(t, err, "a removed member no longer reads the organization's batches")

	batches, err := repositories.NewSubscriptionBatchRepository(db).GetAccessibleByUser("batch-member")
	require.NoError(t, err)
	assert.Empty(t, *batches, "nor lists them")
}

// InjectOrgContext refuses a deleted organization as plan context, even to a
// member whose row is still active.
func TestInjectOrgContext_DeletedOrganization_Refused(t *testing.T) {
	db := freshTestDB(t)
	org := seedSharedTeamOrg(t, db, "ctx-owner", "ctx-member", orgModels.OrgRoleMember)
	require.NoError(t, db.Delete(&orgModels.Organization{}, "id = ?", org.ID).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/test", func(ctx *gin.Context) { ctx.Set("userId", "ctx-member") },
		paymentMiddleware.InjectOrgContext(access.NewGormMembershipChecker(db)),
		func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/test?organization_id="+org.ID.String(), nil))

	assert.Equal(t, http.StatusForbidden, w.Code, "a deleted organization is no plan context")
}
