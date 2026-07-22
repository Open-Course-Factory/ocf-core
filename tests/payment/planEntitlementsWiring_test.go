// tests/payment/planEntitlementsWiring_test.go
//
// RED tests for the entitlement WIRING migration: the feature-resolution paths
// must project the plan's TYPED fields (via derivePlanEntitlements) instead of
// returning the raw plan.Features array.
//
//   - OrganizationFeatureProvider.GetFeatures (organizations/services)
//   - organizationSubscriptionService.GetUserEffectiveFeatures (AllFeatures)
//
// Two directions, both asserted at the service boundary:
//   (1) typed fields set + EMPTY features[] → the derived keys appear.
//       RED today: these paths read plan.Features, which is empty → no keys.
//   (2) legacy strings in features[] + typed fields FALSE → NOTHING is derived
//       from the strings. RED today: these paths return the raw strings.
//
// plan.Features is NOT removed in this phase — only its consumers migrate.
//
// Reuses createOrgWithSubscription from effectivePlanService_test.go.
package payment_tests

import (
	"testing"

	orgServices "soli/formations/src/organizations/services"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedTypedGroupPlan creates a plan whose TYPED entitlement fields are set but
// whose legacy Features array is empty.
func seedTypedGroupPlan(t *testing.T, db *gorm.DB) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:            entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                 "Typed Entitlement Plan",
		Priority:             20,
		Currency:             "eur",
		BillingInterval:      "month",
		IsActive:             true,
		IsCatalog:            true,
		GroupManagementEnabled: true,
		NetworkAccessEnabled: true,
	}
	require.NoError(t, db.Create(plan).Error)
	return plan
}

// seedLegacyStringPlan creates a plan whose legacy "group_management" /
// "legacy_only_string" live ONLY in the raw features column (the model field is
// gone), with all typed entitlement fields false — so the derived paths must
// surface nothing from those strings.
func seedLegacyStringPlan(t *testing.T, db *gorm.DB) *models.SubscriptionPlan {
	t.Helper()
	plan := &models.SubscriptionPlan{
		BaseModel:       entityManagementModels.BaseModel{ID: uuid.New()},
		Name:            "Legacy String Plan",
		Priority:        20,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
		IsCatalog:       true,
		// all typed entitlement fields default false
	}
	require.NoError(t, db.Create(plan).Error)
	seedLegacyFeaturesColumn(t, db, plan.ID, `["group_management","legacy_only_string"]`)
	return plan
}

// (d1) provider: typed fields set, empty features[] → derived keys appear.
func TestOrganizationFeatureProvider_ProjectsTypedEntitlements(t *testing.T) {
	db := freshTestDB(t)
	plan := seedTypedGroupPlan(t, db)
	org, _ := createOrgWithSubscription(t, db, "typed-org", "typed-user", plan)

	provider := orgServices.NewOrganizationFeatureProvider(db)
	features, hasSub, err := provider.GetFeatures(org.ID.String())
	require.NoError(t, err)
	assert.True(t, hasSub, "an active org subscription must report hasSubscription=true")

	assert.Contains(t, features, "group_management",
		"GetFeatures must project GroupManagementEnabled even when features[] is empty")
	assert.Contains(t, features, "multiple_groups",
		"GroupManagementEnabled must project multiple_groups")
	assert.Contains(t, features, "network_access",
		"GetFeatures must project NetworkAccessEnabled from the typed field")
}

// (d2) provider: legacy strings, typed false → NOTHING derived from strings.
func TestOrganizationFeatureProvider_IgnoresLegacyFeatureStrings(t *testing.T) {
	db := freshTestDB(t)
	plan := seedLegacyStringPlan(t, db)
	org, _ := createOrgWithSubscription(t, db, "legacy-org", "legacy-user", plan)

	provider := orgServices.NewOrganizationFeatureProvider(db)
	features, _, err := provider.GetFeatures(org.ID.String())
	require.NoError(t, err)

	assert.NotContains(t, features, "group_management",
		"GetFeatures must derive from typed fields, not the legacy features[] string")
	assert.NotContains(t, features, "legacy_only_string",
		"a legacy-only features[] string must NOT survive the typed projection")
}

// (d1-service) AllFeatures: typed fields set, empty features[] → derived keys.
func TestGetUserEffectiveFeatures_ProjectsTypedEntitlements(t *testing.T) {
	db := freshTestDB(t)
	plan := seedTypedGroupPlan(t, db)
	_, _ = createOrgWithSubscription(t, db, "typed-svc-org", "typed-svc-user", plan)

	svc := services.NewOrganizationSubscriptionService(db)
	features, err := svc.GetUserEffectiveFeatures("typed-svc-user")
	require.NoError(t, err)
	require.NotNil(t, features)

	assert.Contains(t, features.AllFeatures, "group_management",
		"AllFeatures must project GroupManagementEnabled from the typed field, not features[]")
	assert.Contains(t, features.AllFeatures, "network_access",
		"AllFeatures must project NetworkAccessEnabled from the typed field")
}

// (d2-service) AllFeatures: legacy strings, typed false → nothing from strings.
func TestGetUserEffectiveFeatures_IgnoresLegacyFeatureStrings(t *testing.T) {
	db := freshTestDB(t)
	plan := seedLegacyStringPlan(t, db)
	_, _ = createOrgWithSubscription(t, db, "legacy-svc-org", "legacy-svc-user", plan)

	svc := services.NewOrganizationSubscriptionService(db)
	features, err := svc.GetUserEffectiveFeatures("legacy-svc-user")
	require.NoError(t, err)
	require.NotNil(t, features)

	assert.NotContains(t, features.AllFeatures, "legacy_only_string",
		"AllFeatures must derive from typed fields; a legacy-only features[] string must not survive")
}
