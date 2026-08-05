// tests/payment/personalOrgClassroomGate_test.go
//
// #475: a plan is bought by a USER, but classroom capability only materializes in
// TEAM organizations. The personal organization is a workspace for one person —
// it holds no groups, so it must not advertise that it can run classes.
//
// GET /users/me/features?organization_id=<personal org> resolved the buyer's own
// plan (Formateur, group management granted) and read the entitlement straight off
// that plan, skipping the organization-shaped gate that CanRunClassrooms applies.
// The frontend (stores/permissions.ts calls exactly this URL with the current org)
// therefore offered class management inside a personal org, and the ClassGroup
// placement hook then refused the create — the two halves of one rule disagreeing,
// which is the drift this codebase keeps paying for.
//
// These tests drive the REAL controller and assert the decoded JSON body.
package payment_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	paymentController "soli/formations/src/payment/routes"
	"soli/formations/src/payment/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// classroomEntitlementResponse decodes only the verdict fields of
// dto.UserEffectiveFeaturesOutput — the contract these tests are about.
type classroomEntitlementResponse struct {
	CanRunClassrooms bool   `json:"can_run_classrooms"`
	DeniedReason     string `json:"classroom_denied_reason"`
}

// seedClassroomPlanFor gives userID a personal subscription on a plan that grants
// group management: the Formateur shape that opened #475.
func seedClassroomPlanFor(t *testing.T, db *gorm.DB, userID string, groupManagement bool) *models.SubscriptionPlan {
	t.Helper()

	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Formateur",
		Priority:               20,
		Currency:               "eur",
		BillingInterval:        "month",
		IsActive:               true,
		IsCatalog:              true,
		GroupManagementEnabled: groupManagement,
	}
	require.NoError(t, db.Create(plan).Error)
	createUserSubscription(t, db, userID, plan)
	return plan
}

// getFeatures calls GET /users/me/features for userID with the given raw query
// string (empty for none), driving the real controller.
func getFeatures(t *testing.T, db *gorm.DB, userID, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	controller := paymentController.NewOrganizationSubscriptionController(db)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Next()
	})
	r.GET("/users/me/features", controller.GetUserEffectiveFeatures)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/users/me/features"+rawQuery, nil))
	return w
}

// getFeaturesInOrg asks the same endpoint in orgID's context.
func getFeaturesInOrg(t *testing.T, db *gorm.DB, userID string, orgID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	return getFeatures(t, db, userID, "?organization_id="+orgID.String())
}

func decodeClassroomVerdict(t *testing.T, w *httptest.ResponseRecorder) classroomEntitlementResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, "endpoint should succeed; body=%s", w.Body.String())

	var resp classroomEntitlementResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// The case that opened the issue.
func TestUsersMeFeatures_PersonalOrgContext_RefusesClassrooms(t *testing.T) {
	db := freshTestDB(t)

	const userID = "formateur-personal-475"
	seedClassroomPlanFor(t, db, userID, true)
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "formateur-personal-workspace", userID)

	verdict := decodeClassroomVerdict(t, getFeaturesInOrg(t, db, userID, personalOrg.ID))

	assert.False(t, verdict.CanRunClassrooms,
		"a personal organization holds no groups, whatever plan the user bought")
	assert.Equal(t, services.ClassroomDeniedPersonalOrg, verdict.DeniedReason,
		"the refusal must name the organization's shape, not the plan — a user told to upgrade will upgrade and still be stuck")
}

// The same buyer, in the team organization they created for their classes. The team
// org owns no subscription, so the plan falls back to the one they hold personally:
// this is the trainer path, and it must stay open.
func TestUsersMeFeatures_TeamOrgContext_GrantsClassroomsOnPersonalPlan(t *testing.T) {
	db := freshTestDB(t)

	const userID = "formateur-team-475"
	seedClassroomPlanFor(t, db, userID, true)
	teamOrg := createTeamOrgWithoutSubscription(t, db, "formateur-classes", userID)

	verdict := decodeClassroomVerdict(t, getFeaturesInOrg(t, db, userID, teamOrg.ID))

	assert.True(t, verdict.CanRunClassrooms,
		"the plan a trainer holds must still grant classrooms in the team org they own")
	assert.Empty(t, verdict.DeniedReason)
}

// A team organization that owns its own classroom plan — the school shape — is
// unaffected by the personal-org gate.
func TestUsersMeFeatures_TeamOrgWithOwnPlan_GrantsClassrooms(t *testing.T) {
	db := freshTestDB(t)

	const userID = "school-owner-475"
	schoolPlan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Ecole / OF",
		Priority:               30,
		Currency:               "eur",
		BillingInterval:        "month",
		IsActive:               true,
		IsCatalog:              true,
		GroupManagementEnabled: true,
	}
	require.NoError(t, db.Create(schoolPlan).Error)
	school, _ := createOrgWithSubscriptionAndType(t, db, "esitech", userID, schoolPlan, organizationModels.OrgTypeTeam)

	verdict := decodeClassroomVerdict(t, getFeaturesInOrg(t, db, userID, school.ID))

	assert.True(t, verdict.CanRunClassrooms)
	assert.Empty(t, verdict.DeniedReason)
}

// The organization's shape is decided before the plan is read, so the refusal is
// stable: a personal org refuses identically whether or not the plan would have
// granted classrooms. Without this the reason would flip between two codes for the
// same user depending on what they bought.
func TestUsersMeFeatures_PersonalOrgContext_RefusalDoesNotDependOnThePlan(t *testing.T) {
	db := freshTestDB(t)

	const userID = "solo-personal-475"
	seedClassroomPlanFor(t, db, userID, false)
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "solo-personal-workspace", userID)

	verdict := decodeClassroomVerdict(t, getFeaturesInOrg(t, db, userID, personalOrg.ID))

	assert.False(t, verdict.CanRunClassrooms)
	assert.Equal(t, services.ClassroomDeniedPersonalOrg, verdict.DeniedReason)
}

// A user with no subscription anywhere resolves no plan, so the endpoint has
// nothing to describe and answers 404 — the behaviour before the gate, unchanged.
// The gate must not turn "nothing to report" into a classroom verdict.
func TestUsersMeFeatures_PersonalOrgWithoutAnySubscription_Unchanged(t *testing.T) {
	db := freshTestDB(t)

	const userID = "planless-475"
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "planless-personal-workspace", userID)

	w := getFeaturesInOrg(t, db, userID, personalOrg.ID)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"no plan resolves, so there are no features to report; body=%s", w.Body.String())
}

// No organization_id at all is the "describe the user" question, not "describe a
// workspace": a Formateur who has not yet created a team org must still be told
// their plan runs classrooms, or they can never be invited to create one.
func TestUsersMeFeatures_WithoutOrgContext_ReportsThePlansVerdict(t *testing.T) {
	db := freshTestDB(t)

	const userID = "formateur-noctx-475"
	plan := seedClassroomPlanFor(t, db, userID, true)
	createOrgWithSubscription(t, db, "formateur-noctx-org", userID, plan)

	verdict := decodeClassroomVerdict(t, getFeatures(t, db, userID, ""))

	assert.True(t, verdict.CanRunClassrooms,
		"the org-less question is about the plan the user holds, not about a workspace")
	assert.Empty(t, verdict.DeniedReason)
}

// An empty organization_id is not an organization: it must fall through to the
// org-less answer rather than being parsed, and a blank string must not panic.
func TestUsersMeFeatures_EmptyOrgParam_FallsBackToOrgLessAnswer(t *testing.T) {
	db := freshTestDB(t)

	const userID = "formateur-blank-475"
	plan := seedClassroomPlanFor(t, db, userID, true)
	createOrgWithSubscription(t, db, "formateur-blank-org", userID, plan)

	verdict := decodeClassroomVerdict(t, getFeatures(t, db, userID, "?organization_id="))

	assert.True(t, verdict.CanRunClassrooms)
}

// SSOT pin: the endpoint's verdict and CanRunClassrooms — the owner of the rule,
// and what the ClassGroup placement hook consults before accepting a create — must
// return the same answer for the same user in the same organization. They diverged
// once (#475) because the endpoint applied only the plan half of the rule.
func TestPersonalOrgVerdict_EndpointAndCanRunClassroomsAgree(t *testing.T) {
	db := freshTestDB(t)

	const userID = "formateur-ssot-475"
	seedClassroomPlanFor(t, db, userID, true)
	personalOrg := createPersonalOrgWithoutSubscription(t, db, "ssot-personal-workspace", userID)
	teamOrg := createTeamOrgWithoutSubscription(t, db, "ssot-team", userID)

	svc := services.NewEffectivePlanService(db)

	for _, tc := range []struct {
		name  string
		orgID uuid.UUID
	}{
		{"personal organization", personalOrg.ID},
		{"team organization", teamOrg.ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := svc.CanRunClassrooms(userID, &tc.orgID)
			endpoint := decodeClassroomVerdict(t, getFeaturesInOrg(t, db, userID, tc.orgID))

			assert.Equal(t, owner.Allowed, endpoint.CanRunClassrooms,
				"the endpoint must not answer differently from the rule's owner")
			assert.Equal(t, owner.Reason, endpoint.DeniedReason,
				"the refusal code must match too, or the frontend explains a refusal the backend never made")
		})
	}
}
