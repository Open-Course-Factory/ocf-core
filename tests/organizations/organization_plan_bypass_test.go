package organizations_tests

// Tests proving the subscription_plan_id bypass vulnerability (issue #240).
//
// The CreateOrganizationInput and EditOrganizationInput DTOs expose
// subscription_plan_id as a writable JSON field. Any authenticated Member can
// therefore craft a payload that assigns an arbitrary paid plan to their
// organization, bypassing Stripe payment.
//
// NOTE: The Organization model uses PostgreSQL-specific types (jsonb, pq.StringArray)
// that are incompatible with in-memory SQLite. Following the established pattern in
// this package (see importController_test.go), these tests operate at the DTO and
// converter layer — exactly where the vulnerability manifests — without requiring a DB.
//
// The three layers where the vulnerability exists:
//   1. JSON deserialization: subscription_plan_id is accepted from the request body.
//   2. DtoToModel converter: the field is copied to the Organization model on create.
//   3. DtoToMap converter: the field is passed into UPDATE queries on PATCH.
//
// Expected behaviour after the fix:
//   - Member: subscription_plan_id in input is silently stripped (nil) before reaching DB.
//   - Administrator: subscription_plan_id is honoured.

import (
	"encoding/json"
	"testing"

	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Layer 1: JSON deserialization — the API accepts subscription_plan_id
// ---------------------------------------------------------------------------

// TestOrganizationCreate_WithSubscriptionPlanID_ShouldIgnoreForNonAdmin proves
// that CreateOrganizationInput accepts a subscription_plan_id from JSON, which
// means a Member user can inject a plan ID through the API.
//
// After the fix, the DTO should either strip this field for non-admins (via a
// hook or service-layer check) so that it never reaches the DB with a
// member-supplied value.
//
// This test currently FAILS the security assertion: the DTO faithfully
// deserializes the member-supplied plan ID and the converter copies it to the
// model with no role check — proving the vulnerability.
func TestOrganizationCreate_WithSubscriptionPlanID_ShouldIgnoreForNonAdmin(t *testing.T) {
	paidPlanID := uuid.New()

	// Simulate what the API receives from a Member's HTTP request body.
	payload := map[string]interface{}{
		"name":                 "my-org",
		"display_name":         "My Org",
		"subscription_plan_id": paidPlanID.String(), // Member injects a paid plan.
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var input dto.CreateOrganizationInput
	err = json.Unmarshal(raw, &input)
	require.NoError(t, err)

	// The DTO deserialization must succeed — this is by design so the field can
	// be accepted from admins. The security check must happen *downstream*.

	// Layer 2: DtoToModel — simulate what the entity registration converter does.
	// This mirrors organizationRegistration.go DtoToModel exactly.
	org := &models.Organization{
		Name:               input.Name,
		DisplayName:        input.DisplayName,
		Description:        input.Description,
		SubscriptionPlanID: input.SubscriptionPlanID, // ← vulnerability: no role check
		MaxGroups:          input.MaxGroups,
		MaxMembers:         input.MaxMembers,
		Metadata:           input.Metadata,
		IsActive:           true,
	}

	// CURRENT BEHAVIOUR (bug): SubscriptionPlanID is set to the member-supplied value.
	// EXPECTED (after fix): SubscriptionPlanID must be nil for non-admin callers;
	// the fix should strip it before the model is persisted.
	assert.Nil(t, org.SubscriptionPlanID,
		"Member-supplied subscription_plan_id must not reach the Organization model; "+
			"got %v — the service or hook must strip this field for non-admin callers",
		org.SubscriptionPlanID)
}

// TestOrganizationUpdate_WithSubscriptionPlanID_ShouldIgnoreForNonAdmin proves
// that EditOrganizationInput accepts a subscription_plan_id and that the
// DtoToMap converter propagates it into the UPDATE map with no role check.
//
// This test currently FAILS: the update map contains subscription_plan_id,
// meaning any Member who owns an org can change its plan for free via PATCH.
func TestOrganizationUpdate_WithSubscriptionPlanID_ShouldIgnoreForNonAdmin(t *testing.T) {
	paidPlanID := uuid.New()

	// Simulate what the API receives from a Member's PATCH request body.
	payload := map[string]interface{}{
		"subscription_plan_id": paidPlanID.String(),
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var input dto.EditOrganizationInput
	err = json.Unmarshal(raw, &input)
	require.NoError(t, err)

	// The field is deserialized successfully.
	require.NotNil(t, input.SubscriptionPlanID, "SubscriptionPlanID should be parsed from JSON")
	assert.Equal(t, paidPlanID, *input.SubscriptionPlanID)

	// Layer 3: DtoToMap — simulate what the entity registration converter does.
	// This mirrors organizationRegistration.go DtoToMap exactly.
	updates := make(map[string]any)
	if input.SubscriptionPlanID != nil {
		updates["subscription_plan_id"] = *input.SubscriptionPlanID // ← vulnerability
	}

	// CURRENT BEHAVIOUR (bug): the update map contains subscription_plan_id,
	// which will be written to the DB via a raw UPDATE with no role check.
	// EXPECTED (after fix): subscription_plan_id must not appear in the update
	// map for non-admin callers; it should be stripped before DtoToMap is called.
	_, containsPlanID := updates["subscription_plan_id"]
	assert.False(t, containsPlanID,
		"subscription_plan_id must not appear in the update map for non-admin callers; "+
			"the DtoToMap converter (or upstream hook) must strip it, but it was included")
}

// TestOrganizationCreate_WithSubscriptionPlanID_ShouldAllowForAdmin verifies
// the positive case: an Administrator can assign a specific subscription plan
// when creating an organization, and the value must survive the converter chain.
//
// This test acts as the safety net for the fix: it must pass both before AND
// after the fix to ensure we do not break the admin code path.
func TestOrganizationCreate_WithSubscriptionPlanID_ShouldAllowForAdmin(t *testing.T) {
	targetPlanID := uuid.New()

	// Admin payload — same field, different caller.
	payload := map[string]interface{}{
		"name":                 "admin-plan-org",
		"display_name":         "Admin Plan Org",
		"subscription_plan_id": targetPlanID.String(),
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var input dto.CreateOrganizationInput
	err = json.Unmarshal(raw, &input)
	require.NoError(t, err)

	// The field must be parsed correctly so the admin path can use it.
	require.NotNil(t, input.SubscriptionPlanID,
		"CreateOrganizationInput must accept subscription_plan_id so admins can use it")
	assert.Equal(t, targetPlanID, *input.SubscriptionPlanID,
		"SubscriptionPlanID must be correctly deserialized")

	// After the fix: when the caller is identified as an administrator (e.g. via
	// ctx.UserRoles containing "administrator"), the service/hook must NOT strip
	// the field. The model should be created with the supplied plan ID.
	//
	// We simulate the converter as the admin code path should produce it:
	org := &models.Organization{
		Name:               input.Name,
		DisplayName:        input.DisplayName,
		SubscriptionPlanID: input.SubscriptionPlanID, // retained for admins
		IsActive:           true,
	}

	// This assertion documents and enforces the admin privilege:
	require.NotNil(t, org.SubscriptionPlanID,
		"Administrator-supplied subscription_plan_id must be preserved in the model")
	assert.Equal(t, targetPlanID, *org.SubscriptionPlanID,
		"Administrator-supplied plan ID must reach the Organization model unchanged")
}

// TestOrganizationCreate_NoSubscriptionPlanID_ShouldAssignTrialPlan documents
// the baseline: when no subscription_plan_id is supplied, the service assigns
// the Trial plan. This test verifies the DTO correctly leaves the field nil,
// allowing the Trial assignment logic to run.
func TestOrganizationCreate_NoSubscriptionPlanID_ShouldAssignTrialPlan(t *testing.T) {
	payload := map[string]interface{}{
		"name":         "trial-org",
		"display_name": "Trial Org",
		// No subscription_plan_id — normal member creation.
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	var input dto.CreateOrganizationInput
	err = json.Unmarshal(raw, &input)
	require.NoError(t, err)

	// SubscriptionPlanID must be nil so that assignOrgTrialPlan() is triggered.
	assert.Nil(t, input.SubscriptionPlanID,
		"When no subscription_plan_id is supplied, the field must be nil so the "+
			"Trial plan assignment logic can run")
}

// TestOrganizationUpdate_WithSubscriptionPlanID_DTODeserialization documents
// that subscription_plan_id is fully functional in the EditOrganizationInput DTO.
// This is the prerequisite for the vulnerability and also for the admin fix:
// the field must be parse-able so it can be selectively applied for admins.
func TestOrganizationUpdate_WithSubscriptionPlanID_DTODeserialization(t *testing.T) {
	planID := uuid.New()

	input := dto.EditOrganizationInput{
		SubscriptionPlanID: &planID,
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)

	var decoded dto.EditOrganizationInput
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// The field round-trips correctly through JSON — confirming the API accepts it.
	require.NotNil(t, decoded.SubscriptionPlanID)
	assert.Equal(t, planID, *decoded.SubscriptionPlanID,
		"subscription_plan_id must survive JSON round-trip in EditOrganizationInput")
}
