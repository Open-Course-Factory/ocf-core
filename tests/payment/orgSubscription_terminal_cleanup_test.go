// tests/payment/orgSubscription_terminal_cleanup_test.go
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
	terminalModels "soli/formations/src/terminalTrainer/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTerminateUserTerminals_FreesQuotaSlotViaOccupiesSlotScope asserts the
// SSOT-correct outcome: after TerminateUserTerminals, the user's occupied slot
// count (as reported by models.CountUserOccupiedSlots — the canonical real-time
// counter from MR !218) drops to zero. A "stopped" terminal still occupies a
// slot per TerminalStatusesOccupyingSlot, so the cleanup must mark terminals
// as "deleted" to free the slot. This guards against regressing to the old
// stop-semantic which left users appearing over-quota after subscription
// cancellation.
func TestTerminateUserTerminals_FreesQuotaSlotViaOccupiesSlotScope(t *testing.T) {
	db := freshTestDB(t)
	userID := "user-frees-slot"

	db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, status, state, expires_at) VALUES (?, ?, ?, ?, 'active', 'running', ?)`,
		uuid.New().String(), uuid.New().String(), userID, "term-frees-slot", time.Now().Add(time.Hour),
	)

	// Sanity: before cleanup, the user occupies 1 slot.
	before, err := terminalModels.CountUserOccupiedSlots(db, userID, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), before, "precondition: user must occupy 1 slot before cleanup")

	// Execute
	require.NoError(t, services.TerminateUserTerminals(db, userID))

	// SSOT assertion: real-time counter returns 0 — the slot has been freed.
	after, err := terminalModels.CountUserOccupiedSlots(db, userID, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), after, "after TerminateUserTerminals, the user must occupy 0 slots (terminal must be 'deleted', not 'stopped')")
}

func TestTerminateOrganizationMemberTerminals_DeletesActiveTerminals(t *testing.T) {
	db := freshTestDB(t)

	// Create organization
	orgID := uuid.New()
	org := &organizationModels.Organization{
		BaseModel:   entityManagementModels.BaseModel{ID: orgID},
		Name:        "test-org",
		DisplayName: "Test Organization",
		OwnerUserID: "owner_user",
		IsActive:    true,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	// Create org members
	userA := "user_a"
	userB := "user_b"
	for _, userID := range []string{userA, userB} {
		member := &organizationModels.OrganizationMember{
			BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: orgID,
			UserID:         userID,
			Role:           organizationModels.OrgRoleMember,
			JoinedAt:       time.Now(),
			IsActive:       true,
		}
		require.NoError(t, db.Omit("Metadata").Create(member).Error)
	}

	// Create active terminals for both members
	terminalIDs := make([]uuid.UUID, 0, 3)
	for _, userID := range []string{userA, userA, userB} {
		termID := uuid.New()
		terminalIDs = append(terminalIDs, termID)
		db.Exec(
			`INSERT INTO terminals (id, session_id, user_id, name, status, state, expires_at) VALUES (?, ?, ?, ?, 'active', 'running', ?)`,
			termID.String(), uuid.New().String(), userID, "term-"+userID, time.Now().Add(time.Hour),
		)
	}

	// Verify active terminals exist
	var activeCount int64
	db.Raw("SELECT COUNT(*) FROM terminals WHERE status = 'active'").Scan(&activeCount)
	assert.Equal(t, int64(3), activeCount)

	// Execute
	services.TerminateOrganizationMemberTerminals(db, orgID)

	// Assert: all terminals should be deleted (both status and state in sync).
	// This frees the quota slot — stopped sessions still occupy slots per
	// models.TerminalStatusesOccupyingSlot.
	var deletedCount int64
	db.Raw("SELECT COUNT(*) FROM terminals WHERE status = 'deleted' AND state = 'deleted'").Scan(&deletedCount)
	assert.Equal(t, int64(3), deletedCount)

	var remainingActive int64
	db.Raw("SELECT COUNT(*) FROM terminals WHERE status = 'active'").Scan(&remainingActive)
	assert.Equal(t, int64(0), remainingActive)
}

func TestTerminateOrganizationMemberTerminals_IgnoresInactiveMembers(t *testing.T) {
	db := freshTestDB(t)

	orgID := uuid.New()
	org := &organizationModels.Organization{
		BaseModel:   entityManagementModels.BaseModel{ID: orgID},
		Name:        "test-org-2",
		DisplayName: "Test Organization 2",
		OwnerUserID: "owner_user",
		IsActive:    true,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	// Active member
	activeMember := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		UserID:         "active_user",
		Role:           organizationModels.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	require.NoError(t, db.Omit("Metadata").Create(activeMember).Error)

	// Inactive member (left the org)
	// Note: GORM's default:true treats IsActive=false as zero-value and applies the default,
	// so we create the member first then explicitly update is_active to false.
	inactiveMemberID := uuid.New()
	inactiveMember := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: inactiveMemberID},
		OrganizationID: orgID,
		UserID:         "inactive_user",
		Role:           organizationModels.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	require.NoError(t, db.Omit("Metadata").Create(inactiveMember).Error)
	require.NoError(t, db.Model(&organizationModels.OrganizationMember{}).Where("id = ?", inactiveMemberID).Update("is_active", false).Error)

	// Create terminals for both
	db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, status, expires_at) VALUES (?, ?, 'active_user', 'term-active', 'active', ?)`,
		uuid.New().String(), uuid.New().String(), time.Now().Add(time.Hour),
	)
	db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, status, expires_at) VALUES (?, ?, 'inactive_user', 'term-inactive', 'active', ?)`,
		uuid.New().String(), uuid.New().String(), time.Now().Add(time.Hour),
	)

	// Execute
	services.TerminateOrganizationMemberTerminals(db, orgID)

	// Active member's terminal should be deleted (frees the quota slot)
	var deletedStatus string
	db.Raw("SELECT status FROM terminals WHERE user_id = 'active_user'").Scan(&deletedStatus)
	assert.Equal(t, "deleted", deletedStatus)

	// Inactive member's terminal should remain active (member was not queried)
	var activeStatus string
	db.Raw("SELECT status FROM terminals WHERE user_id = 'inactive_user'").Scan(&activeStatus)
	assert.Equal(t, "active", activeStatus)
}

func TestTerminateOrganizationMemberTerminals_NoMembers(t *testing.T) {
	db := freshTestDB(t)

	orgID := uuid.New()
	org := &organizationModels.Organization{
		BaseModel:   entityManagementModels.BaseModel{ID: orgID},
		Name:        "empty-org",
		DisplayName: "Empty Organization",
		OwnerUserID: "owner_user",
		IsActive:    true,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	// Should not panic or error with zero members
	services.TerminateOrganizationMemberTerminals(db, orgID)
}

func TestTerminateUserTerminals_DeletesActiveTerminals(t *testing.T) {
	db := freshTestDB(t)

	userID := "test_user_term"

	// Create a subscription (the existence of a usage metric is no longer
	// part of the contract — quota is computed in real time from terminal
	// rows via models.OccupiesSlotScope. The stored counter is left alone:
	// it self-heals on the next reconcile pass and is not consulted by gating).
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Test Plan",
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxConcurrentTerminals: 5,
		IsActive:               true,
	}
	require.NoError(t, db.Create(plan).Error)

	sub := &models.UserSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:             userID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
	}
	require.NoError(t, db.Create(sub).Error)

	// Create 2 active terminals
	for i := 0; i < 2; i++ {
		db.Exec(
			`INSERT INTO terminals (id, session_id, user_id, name, status, state, expires_at) VALUES (?, ?, ?, ?, 'active', 'running', ?)`,
			uuid.New().String(), uuid.New().String(), userID, "term", time.Now().Add(time.Hour),
		)
	}

	// Execute
	err := services.TerminateUserTerminals(db, userID)
	require.NoError(t, err)

	// All terminals deleted (status AND state — kept in sync per the canonical
	// terminalTrainerService.DeleteSession pattern).
	var deletedCount int64
	db.Raw("SELECT COUNT(*) FROM terminals WHERE user_id = ? AND status = 'deleted' AND state = 'deleted'", userID).Scan(&deletedCount)
	assert.Equal(t, int64(2), deletedCount)

	// SSOT check: real-time occupied-slot count is now 0.
	occupied, err := terminalModels.CountUserOccupiedSlots(db, userID, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), occupied)
}

func TestCancelOrganizationSubscription_ImmediateTerminatesTerminals(t *testing.T) {
	db := freshTestDB(t)

	// Create plan
	plan := &models.SubscriptionPlan{
		BaseModel:              entityManagementModels.BaseModel{ID: uuid.New()},
		Name:                   "Org Plan",
		PriceAmount:            0,
		Currency:               "eur",
		BillingInterval:        "month",
		MaxConcurrentTerminals: 10,
		IsActive:               true,
	}
	require.NoError(t, db.Create(plan).Error)

	// Create organization
	orgID := uuid.New()
	org := &organizationModels.Organization{
		BaseModel:          entityManagementModels.BaseModel{ID: orgID},
		Name:               "cancel-org",
		DisplayName:        "Cancel Test Org",
		OwnerUserID:        "org_owner",
		SubscriptionPlanID: &plan.ID,
		IsActive:           true,
	}
	require.NoError(t, db.Omit("Metadata").Create(org).Error)

	// Create org subscription
	orgSub := &models.OrganizationSubscription{
		BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID:     orgID,
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		Quantity:           5,
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour),
	}
	require.NoError(t, db.Create(orgSub).Error)

	// Create org member
	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		UserID:         "org_member_1",
		Role:           organizationModels.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	require.NoError(t, db.Omit("Metadata").Create(member).Error)

	// Create active terminal for the member
	db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, status, state, expires_at) VALUES (?, ?, 'org_member_1', 'org-term', 'active', 'running', ?)`,
		uuid.New().String(), uuid.New().String(), time.Now().Add(time.Hour),
	)

	// Cancel immediately (not at period end)
	svc := services.NewOrganizationSubscriptionService(db)
	err := svc.CancelOrganizationSubscription(orgID, false)
	require.NoError(t, err)

	// Terminal should be deleted (frees the quota slot — stopped sessions
	// still occupy slots per models.TerminalStatusesOccupyingSlot).
	var status string
	db.Raw("SELECT status FROM terminals WHERE user_id = 'org_member_1'").Scan(&status)
	assert.Equal(t, "deleted", status)
}

// TestTerminateOrganizationMemberTerminals_DoesNotTouchPersonalTerminals
// asserts that when an organization subscription is cancelled, only the
// terminals tied to THAT organization are terminated. Personal-plan
// terminals (organization_id IS NULL) and terminals for other orgs must
// remain untouched.
//
// Without orgID scoping, TerminateUserTerminals wipes ALL of a member's
// terminals — causing permanent data loss for users on a personal plan
// who happen to be members of a cancelled organization (closes #314).
func TestTerminateOrganizationMemberTerminals_DoesNotTouchPersonalTerminals(t *testing.T) {
	db := freshTestDB(t)

	// Two orgs — the cancelled one (orgA) and an unrelated one (orgB)
	orgA := uuid.New()
	orgB := uuid.New()
	for _, oid := range []uuid.UUID{orgA, orgB} {
		org := &organizationModels.Organization{
			BaseModel:   entityManagementModels.BaseModel{ID: oid},
			Name:        "org-" + oid.String()[:8],
			DisplayName: "Org " + oid.String()[:8],
			OwnerUserID: "owner",
			IsActive:    true,
		}
		require.NoError(t, db.Omit("Metadata").Create(org).Error)
	}

	// One user is a member of orgA only
	userID := "shared_user"
	member := &organizationModels.OrganizationMember{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: orgA,
		UserID:         userID,
		Role:           organizationModels.OrgRoleMember,
		JoinedAt:       time.Now(),
		IsActive:       true,
	}
	require.NoError(t, db.Omit("Metadata").Create(member).Error)

	// User has THREE terminals:
	//  - one tied to orgA (the org being cancelled)
	//  - one tied to orgB (an unrelated org, but user happens to share user_id)
	//  - one personal (organization_id IS NULL)
	termOrgA := uuid.New()
	termOrgB := uuid.New()
	termPersonal := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, status, state, organization_id, expires_at) VALUES (?, ?, ?, 'term-orgA', 'active', 'running', ?, ?)`,
		termOrgA.String(), uuid.New().String(), userID, orgA, time.Now().Add(time.Hour),
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, status, state, organization_id, expires_at) VALUES (?, ?, ?, 'term-orgB', 'active', 'running', ?, ?)`,
		termOrgB.String(), uuid.New().String(), userID, orgB, time.Now().Add(time.Hour),
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO terminals (id, session_id, user_id, name, status, state, organization_id, expires_at) VALUES (?, ?, ?, 'term-personal', 'active', 'running', NULL, ?)`,
		termPersonal.String(), uuid.New().String(), userID, time.Now().Add(time.Hour),
	).Error)

	// Cancel orgA's subscription -> should terminate ONLY orgA's terminal
	services.TerminateOrganizationMemberTerminals(db, orgA)

	// orgA terminal: deleted
	var statusOrgA string
	db.Raw("SELECT status FROM terminals WHERE id = ?", termOrgA.String()).Scan(&statusOrgA)
	assert.Equal(t, "deleted", statusOrgA, "orgA terminal must be deleted")

	// orgB terminal: untouched
	var statusOrgB string
	db.Raw("SELECT status FROM terminals WHERE id = ?", termOrgB.String()).Scan(&statusOrgB)
	assert.Equal(t, "active", statusOrgB, "orgB terminal must NOT be touched (different org)")

	// Personal terminal: untouched — this is the critical data-loss prevention
	var statusPersonal string
	db.Raw("SELECT status FROM terminals WHERE id = ?", termPersonal.String()).Scan(&statusPersonal)
	assert.Equal(t, "active", statusPersonal, "personal terminal must NOT be touched (organization_id is NULL)")

	// SSOT checks via CountUserOccupiedSlots
	// All-scope: user still occupies 2 slots (personal + orgB)
	totalSlots, err := terminalModels.CountUserOccupiedSlots(db, userID, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(2), totalSlots, "user should still occupy 2 slots (personal + orgB) after orgA cancellation")

	// orgA-scope: 0 slots
	orgASlots, err := terminalModels.CountUserOccupiedSlots(db, userID, &orgA)
	require.NoError(t, err)
	assert.Equal(t, int64(0), orgASlots, "user should occupy 0 slots within orgA after cancellation")

	// orgB-scope: 1 slot (unchanged)
	orgBSlots, err := terminalModels.CountUserOccupiedSlots(db, userID, &orgB)
	require.NoError(t, err)
	assert.Equal(t, int64(1), orgBSlots, "user should still occupy 1 slot in orgB")
}
