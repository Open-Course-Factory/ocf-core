package organizations_tests

// Tests for the subscription_plan_id bypass vulnerability (issue #240).
//
// The CreateOrganizationInput and EditOrganizationInput DTOs expose
// subscription_plan_id as a writable JSON field. Without protection, any
// authenticated Member can assign a paid plan to their organization for free.
//
// The fix registers OrganizationPlanProtectionHook (BeforeCreate + BeforeUpdate)
// which strips the field when the caller is not an Administrator.
//
// NOTE: The Organization model uses PostgreSQL-specific types (jsonb, pq.StringArray)
// that are incompatible with in-memory SQLite. Following the established pattern in
// this package (see importController_test.go), these tests operate at the DTO and
// hook layer — exactly where the protection is enforced — without requiring a DB.

import (
	"context"
	"encoding/json"
	"testing"

	hookpkg "soli/formations/src/entityManagement/hooks"
	organizationHooks "soli/formations/src/organizations/hooks"
	"soli/formations/src/organizations/dto"
	"soli/formations/src/organizations/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// BeforeCreate protection — member caller
// ---------------------------------------------------------------------------

// TestOrganizationCreate_WithSubscriptionPlanID_ShouldIgnoreForNonAdmin verifies
// that the plan protection hook strips subscription_plan_id from the Organization
// model when the caller holds only the "Member" role.
//
// Without the hook, a Member who POSTs {"subscription_plan_id": "<paid-plan-uuid>"}
// would have that value faithfully copied into the model and persisted to the DB.
// After the fix the hook zeros the field before the model reaches the repository.
func TestOrganizationCreate_WithSubscriptionPlanID_ShouldIgnoreForNonAdmin(t *testing.T) {
	paidPlanID := uuid.New()

	// Simulate the Organization model as produced by DtoToModel after a Member
	// submits a create payload containing a paid plan UUID.
	org := &models.Organization{
		Name:               "my-org",
		DisplayName:        "My Org",
		SubscriptionPlanID: &paidPlanID, // member-injected value
		IsActive:           true,
	}

	ctx := &hookpkg.HookContext{
		EntityName: "Organization",
		HookType:   hookpkg.BeforeCreate,
		NewEntity:  org,
		UserID:     "user-123",
		UserRoles:  []string{"Member"},
		Context:    context.Background(),
	}

	hook := organizationHooks.NewOrganizationPlanProtectionHook()
	err := hook.Execute(ctx)
	require.NoError(t, err)

	// EXPECTED (after fix): the hook strips the field for non-admin callers.
	assert.Nil(t, org.SubscriptionPlanID,
		"Member-supplied subscription_plan_id must not reach the Organization model; "+
			"got %v — the BeforeCreate hook must strip this field for non-admin callers",
		org.SubscriptionPlanID)
}

// ---------------------------------------------------------------------------
// BeforeUpdate protection — member caller
// ---------------------------------------------------------------------------

// TestOrganizationUpdate_WithSubscriptionPlanID_ShouldIgnoreForNonAdmin verifies
// that the plan protection hook removes subscription_plan_id from the update map
// when the caller holds only the "Member" role.
//
// Without the hook, a Member who PATCHes {"subscription_plan_id": "<paid-plan-uuid>"}
// would have that key included in the UPDATE map and written to the DB unconditionally.
// After the fix the hook deletes the key before the map reaches the repository.
func TestOrganizationUpdate_WithSubscriptionPlanID_ShouldIgnoreForNonAdmin(t *testing.T) {
	paidPlanID := uuid.New()

	// Simulate the update map as produced by DtoToMap when a Member sends a PATCH
	// request containing subscription_plan_id.
	updateMap := map[string]any{
		"subscription_plan_id": paidPlanID,
		"display_name":         "New Name",
	}

	ctx := &hookpkg.HookContext{
		EntityName: "Organization",
		HookType:   hookpkg.BeforeUpdate,
		NewEntity:  updateMap,
		UserID:     "user-123",
		UserRoles:  []string{"Member"},
		Context:    context.Background(),
	}

	hook := organizationHooks.NewOrganizationPlanProtectionHook()
	err := hook.Execute(ctx)
	require.NoError(t, err)

	// EXPECTED (after fix): subscription_plan_id must not appear in the update map.
	_, containsPlanID := updateMap["subscription_plan_id"]
	assert.False(t, containsPlanID,
		"subscription_plan_id must not appear in the update map for non-admin callers; "+
			"the BeforeUpdate hook must strip it, but it was included")

	// Other fields must not be affected.
	assert.Equal(t, "New Name", updateMap["display_name"])
}

// ---------------------------------------------------------------------------
// Admin caller — plan ID must be preserved (BeforeCreate)
// ---------------------------------------------------------------------------

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
	// ctx.UserRoles containing "administrator"), the hook must NOT strip the field.
	// The model should be created with the supplied plan ID.
	org := &models.Organization{
		Name:               input.Name,
		DisplayName:        input.DisplayName,
		SubscriptionPlanID: input.SubscriptionPlanID, // retained for admins
		IsActive:           true,
	}

	ctx := &hookpkg.HookContext{
		EntityName: "Organization",
		HookType:   hookpkg.BeforeCreate,
		NewEntity:  org,
		UserID:     "admin-456",
		UserRoles:  []string{"Administrator"},
		Context:    context.Background(),
	}

	hook := organizationHooks.NewOrganizationPlanProtectionHook()
	err = hook.Execute(ctx)
	require.NoError(t, err)

	// This assertion documents and enforces the admin privilege:
	require.NotNil(t, org.SubscriptionPlanID,
		"Administrator-supplied subscription_plan_id must be preserved in the model")
	assert.Equal(t, targetPlanID, *org.SubscriptionPlanID,
		"Administrator-supplied plan ID must reach the Organization model unchanged")
}

// ---------------------------------------------------------------------------
// Baseline — nil plan ID is a no-op
// ---------------------------------------------------------------------------

// TestOrganizationCreate_NoSubscriptionPlanID_ShouldAssignTrialPlan documents
// the baseline: when no subscription_plan_id is supplied, the DTO field is nil
// and the hook is a no-op, allowing the Trial plan assignment logic to run downstream.
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

// ---------------------------------------------------------------------------
// DTO round-trip — subscription_plan_id is accepted in EditOrganizationInput
// ---------------------------------------------------------------------------

// TestOrganizationUpdate_WithSubscriptionPlanID_DTODeserialization documents
// that subscription_plan_id is fully functional in the EditOrganizationInput DTO.
// This is the prerequisite for the admin PATCH path to work.
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
