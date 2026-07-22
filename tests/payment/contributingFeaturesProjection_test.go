// tests/payment/contributingFeaturesProjection_test.go
//
// RED test for the deletion phase, review follow-up: the /users/me/features
// aggregated endpoint builds each source org's ContributingFeatures from the RAW
// plan.Features array (organizationSubscriptionController.go:~378). Once the
// features[] column empties, that view goes blank while EffectiveFeatures (which
// now projects the typed fields via DerivePlanEntitlements) stays populated — the
// two views of the same endpoint would disagree.
//
// Contract: ContributingFeatures must ALSO come from DerivePlanEntitlements, so
// an org on a typed-fields plan with EMPTY features[] reports the derived keys,
// and AllFeatures (EffectiveFeatures.features) stays a superset of every org's
// ContributingFeatures.
//
// RED today: the controller reads the raw (empty) Features, so
// contributing_features is empty.
//
// Drives the REAL controller handler with a gin context carrying userId, and
// asserts the decoded JSON — never a mock call. Reuses createOrgWithSubscription.
package payment_tests

import (
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

type userEffectiveFeaturesResponse struct {
	EffectiveFeatures struct {
		Features []string `json:"features"`
	} `json:"effective_features"`
	SourceOrganizations []struct {
		OrganizationID       string   `json:"organization_id"`
		ContributingFeatures []string `json:"contributing_features"`
	} `json:"source_organizations"`
}

func TestUsersMeFeatures_ContributingFeatures_ProjectsTypedEntitlements(t *testing.T) {
	db := freshTestDB(t)

	// Plan with typed entitlements set but an EMPTY legacy features[] array.
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Typed Contributing Plan",
		Priority:               20,
		Currency:               "eur",
		BillingInterval:        "month",
		IsActive:               true,
		IsCatalog:              true,
		GroupManagementEnabled: true,
		NetworkAccessEnabled:   true,
	}
	require.NoError(t, db.Create(plan).Error)

	const userID = "contrib-user-1"
	createOrgWithSubscription(t, db, "contrib-org", userID, plan)

	gin.SetMode(gin.TestMode)
	controller := paymentController.NewOrganizationSubscriptionController(db)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userId", userID)
		c.Next()
	})
	r.GET("/users/me/features", controller.GetUserEffectiveFeatures)

	req := httptest.NewRequest("GET", "/users/me/features", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "endpoint should succeed; body=%s", w.Body.String())

	var resp userEffectiveFeaturesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.SourceOrganizations, 1, "one contributing org expected")

	contributing := resp.SourceOrganizations[0].ContributingFeatures
	assert.Contains(t, contributing, "group_management",
		"contributing_features must project GroupManagementEnabled even with an empty features[]")
	assert.Contains(t, contributing, "multiple_groups",
		"GroupManagementEnabled must project multiple_groups into contributing_features")
	assert.Contains(t, contributing, "network_access",
		"contributing_features must project NetworkAccessEnabled from the typed field")

	// Coherence pin: the aggregated AllFeatures must be a superset of every org's
	// ContributingFeatures — the two views of the same endpoint stay consistent.
	for _, f := range contributing {
		assert.Contains(t, resp.EffectiveFeatures.Features, f,
			"EffectiveFeatures.features must contain every contributing feature %q", f)
	}
}
