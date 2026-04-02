// tests/payment/orgSubscription_terminal_cleanup_test.go
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminateOrganizationMemberTerminals_StopsActiveTerminals(t *testing.T) {
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
			`INSERT INTO terminals (id, session_id, user_id, name, status, expires_at) VALUES (?, ?, ?, ?, 'active', ?)`,
			termID.String(), uuid.New().String(), userID, "term-"+userID, time.Now().Add(time.Hour),
		)
	}

	// Verify active terminals exist
	var activeCount int64
	db.Raw("SELECT COUNT(*) FROM terminals WHERE status = 'active'").Scan(&activeCount)
	assert.Equal(t, int64(3), activeCount)

	// Execute
	services.TerminateOrganizationMemberTerminals(db, orgID)

	// Assert: all terminals should be stopped
	var stoppedCount int64
	db.Raw("SELECT COUNT(*) FROM terminals WHERE status = 'stopped'").Scan(&stoppedCount)
	assert.Equal(t, int64(3), stoppedCount)

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

	// Active member's terminal should be stopped
	var stoppedStatus string
	db.Raw("SELECT status FROM terminals WHERE user_id = 'active_user'").Scan(&stoppedStatus)
	assert.Equal(t, "stopped", stoppedStatus)

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

func TestTerminateUserTerminals_StopsActiveAndDecrementsMetrics(t *testing.T) {
	db := freshTestDB(t)

	userID := "test_user_term"

	// Create a subscription and usage metric
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

	usageMetric := &models.UsageMetrics{
		BaseModel:      entityManagementModels.BaseModel{ID: uuid.New()},
		UserID:         userID,
		SubscriptionID: sub.ID,
		MetricType:     "concurrent_terminals",
		CurrentValue:   2,
		LimitValue:     5,
		PeriodStart:    time.Now(),
		PeriodEnd:      time.Now().Add(30 * 24 * time.Hour),
		LastUpdated:    time.Now(),
	}
	require.NoError(t, db.Create(usageMetric).Error)

	// Create 2 active terminals
	for i := 0; i < 2; i++ {
		db.Exec(
			`INSERT INTO terminals (id, session_id, user_id, name, status, expires_at) VALUES (?, ?, ?, ?, 'active', ?)`,
			uuid.New().String(), uuid.New().String(), userID, "term", time.Now().Add(time.Hour),
		)
	}

	// Execute
	err := services.TerminateUserTerminals(db, userID)
	require.NoError(t, err)

	// All terminals stopped
	var stoppedCount int64
	db.Raw("SELECT COUNT(*) FROM terminals WHERE user_id = ? AND status = 'stopped'", userID).Scan(&stoppedCount)
	assert.Equal(t, int64(2), stoppedCount)

	// Usage metric decremented to 0
	var updatedMetric models.UsageMetrics
	db.Where("user_id = ? AND metric_type = ?", userID, "concurrent_terminals").First(&updatedMetric)
	assert.Equal(t, int64(0), updatedMetric.CurrentValue)
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
		`INSERT INTO terminals (id, session_id, user_id, name, status, expires_at) VALUES (?, ?, 'org_member_1', 'org-term', 'active', ?)`,
		uuid.New().String(), uuid.New().String(), time.Now().Add(time.Hour),
	)

	// Cancel immediately (not at period end)
	svc := services.NewOrganizationSubscriptionService(db)
	err := svc.CancelOrganizationSubscription(orgID, false)
	require.NoError(t, err)

	// Terminal should be stopped
	var status string
	db.Raw("SELECT status FROM terminals WHERE user_id = 'org_member_1'").Scan(&status)
	assert.Equal(t, "stopped", status)
}
