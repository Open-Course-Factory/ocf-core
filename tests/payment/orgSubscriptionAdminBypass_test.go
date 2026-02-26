// tests/payment/orgSubscriptionAdminBypass_test.go
// Tests that administrators can manage organization subscriptions
// without being a member of the organization.
package payment_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	paymentController "soli/formations/src/payment/routes"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupOrgAndPlan creates an org (via raw SQL to avoid SQLite jsonb issues) and a plan.
func setupOrgAndPlan(t *testing.T) (orgID uuid.UUID, planID uuid.UUID) {
	t.Helper()
	db := freshTestDB(t)

	orgID = uuid.New()
	err := db.Exec(
		"INSERT INTO organizations (id, name, display_name, owner_user_id, organization_type, is_active, max_groups, max_members) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		orgID, "Test Org", "Test Org", uuid.New().String(), "team", true, 30, 100,
	).Error
	require.NoError(t, err, "Failed to create test org")

	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Org Plan",
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxCourses:             -1,
		MaxConcurrentTerminals: 5,
		MaxConcurrentUsers:     15,
		IsActive:               true,
	}
	require.NoError(t, db.Create(plan).Error, "Failed to create test plan")

	return orgID, plan.ID
}

// newTestContext creates a gin test context with user info set.
func newTestContext(method, path string, body interface{}, userID string, roles []string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	var req *http.Request
	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewBuffer(jsonBytes))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = req
	ctx.Set("userId", userID)
	ctx.Set("userRoles", roles)
	return ctx, w
}

func TestCreateOrgSubscription_AdminNonMember_Succeeds(t *testing.T) {
	orgID, planID := setupOrgAndPlan(t)
	controller := paymentController.NewOrganizationSubscriptionController(sharedTestDB)

	adminUserID := uuid.New().String() // Not a member of the org

	body := map[string]interface{}{
		"subscription_plan_id": planID.String(),
		"quantity":             15,
	}
	ctx, w := newTestContext("POST", "/organizations/"+orgID.String()+"/subscribe", body, adminUserID, []string{"administrator"})
	ctx.Params = gin.Params{{Key: "id", Value: orgID.String()}}

	controller.CreateOrganizationSubscription(ctx)

	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Admin should NOT get 403 Forbidden when creating org subscription, got %d: %s", w.Code, w.Body.String())
}

func TestCreateOrgSubscription_NonMemberNonAdmin_Forbidden(t *testing.T) {
	orgID, planID := setupOrgAndPlan(t)
	controller := paymentController.NewOrganizationSubscriptionController(sharedTestDB)

	regularUserID := uuid.New().String() // Not a member, not an admin

	body := map[string]interface{}{
		"subscription_plan_id": planID.String(),
		"quantity":             1,
	}
	ctx, w := newTestContext("POST", "/organizations/"+orgID.String()+"/subscribe", body, regularUserID, []string{"member"})
	ctx.Params = gin.Params{{Key: "id", Value: orgID.String()}}

	controller.CreateOrganizationSubscription(ctx)

	assert.Equal(t, http.StatusForbidden, w.Code,
		"Non-member non-admin should get 403 Forbidden")
}

func TestGetOrgSubscription_AdminNonMember_NotForbidden(t *testing.T) {
	orgID, _ := setupOrgAndPlan(t)
	controller := paymentController.NewOrganizationSubscriptionController(sharedTestDB)

	adminUserID := uuid.New().String()
	ctx, w := newTestContext("GET", "/organizations/"+orgID.String()+"/subscription", nil, adminUserID, []string{"administrator"})
	ctx.Params = gin.Params{{Key: "id", Value: orgID.String()}}

	controller.GetOrganizationSubscription(ctx)

	// Should get 404 (no subscription yet) but NOT 403 (forbidden)
	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Admin should NOT get 403 Forbidden when viewing org subscription")
}

func TestGetOrgFeatures_AdminNonMember_NotForbidden(t *testing.T) {
	orgID, _ := setupOrgAndPlan(t)
	controller := paymentController.NewOrganizationSubscriptionController(sharedTestDB)

	adminUserID := uuid.New().String()
	ctx, w := newTestContext("GET", "/organizations/"+orgID.String()+"/features", nil, adminUserID, []string{"administrator"})
	ctx.Params = gin.Params{{Key: "id", Value: orgID.String()}}

	controller.GetOrganizationFeatures(ctx)

	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Admin should NOT get 403 Forbidden when viewing org features")
}

func TestGetOrgUsageLimits_AdminNonMember_NotForbidden(t *testing.T) {
	orgID, _ := setupOrgAndPlan(t)
	controller := paymentController.NewOrganizationSubscriptionController(sharedTestDB)

	adminUserID := uuid.New().String()
	ctx, w := newTestContext("GET", "/organizations/"+orgID.String()+"/usage-limits", nil, adminUserID, []string{"administrator"})
	ctx.Params = gin.Params{{Key: "id", Value: orgID.String()}}

	controller.GetOrganizationUsageLimits(ctx)

	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Admin should NOT get 403 Forbidden when viewing org usage limits")
}

func TestCancelOrgSubscription_AdminNonMember_NotForbidden(t *testing.T) {
	orgID, _ := setupOrgAndPlan(t)
	controller := paymentController.NewOrganizationSubscriptionController(sharedTestDB)

	adminUserID := uuid.New().String()
	body := map[string]interface{}{
		"cancel_at_period_end": true,
	}
	ctx, w := newTestContext("DELETE", "/organizations/"+orgID.String()+"/subscription", body, adminUserID, []string{"administrator"})
	ctx.Params = gin.Params{{Key: "id", Value: orgID.String()}}

	controller.CancelOrganizationSubscription(ctx)

	assert.NotEqual(t, http.StatusForbidden, w.Code,
		"Admin should NOT get 403 Forbidden when cancelling org subscription")
}
