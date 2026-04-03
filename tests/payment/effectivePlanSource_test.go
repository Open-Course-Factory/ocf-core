// tests/payment/effectivePlanSource_test.go
package payment_tests

import (
	"testing"

	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// --- CheckEffectiveUsageLimit source propagation tests ---

func TestCheckEffectiveUsageLimit_Source_Personal(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-source-personal"

	plan := createPlan(t, db, "Basic Personal", 5, 3)
	createUserSubscription(t, db, userID, plan)

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.Equal(t, services.PlanSourcePersonal, check.Source)
	assert.Equal(t, services.EffectivePlanSource("personal"), check.Source)
	assert.True(t, check.Allowed)
}

func TestCheckEffectiveUsageLimit_Source_Organization(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-source-org"

	orgPlan := createPlan(t, db, "Org Pro", 10, 5)
	createOrgWithSubscription(t, db, "source-test-org", userID, orgPlan)

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.Equal(t, services.PlanSourceOrganization, check.Source)
	assert.Equal(t, services.EffectivePlanSource("organization"), check.Source)
	assert.True(t, check.Allowed)
}

func TestCheckEffectiveUsageLimit_Source_OrgWinsOverPersonal(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-source-org-wins"

	// Personal plan with lower priority
	personalPlan := createPlan(t, db, "Trial", 1, 1)
	createUserSubscription(t, db, userID, personalPlan)

	// Org plan with higher priority
	orgPlan := createPlan(t, db, "Enterprise", 20, 10)
	createOrgWithSubscription(t, db, "source-enterprise-org", userID, orgPlan)

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.Equal(t, services.PlanSourceOrganization, check.Source)
	// The limit should come from the org plan (10 terminals)
	assert.Equal(t, int64(10), check.Limit)
}

func TestCheckEffectiveUsageLimit_Source_PersonalWinsOverOrg(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-source-personal-wins"

	// Personal plan with higher priority
	personalPlan := createPlan(t, db, "Premium", 30, 20)
	createUserSubscription(t, db, userID, personalPlan)

	// Org plan with lower priority
	orgPlan := createPlan(t, db, "Basic Org", 5, 3)
	createOrgWithSubscription(t, db, "source-basic-org", userID, orgPlan)

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.Equal(t, services.PlanSourcePersonal, check.Source)
	// The limit should come from the personal plan (20 terminals)
	assert.Equal(t, int64(20), check.Limit)
}

func TestCheckEffectiveUsageLimit_Source_LimitExceeded_Personal(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-source-exceeded-personal"

	plan := createPlan(t, db, "Starter", 5, 1)
	createUserSubscription(t, db, userID, plan)

	// Insert 1 active terminal to hit the limit
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.False(t, check.Allowed)
	assert.Equal(t, services.PlanSourcePersonal, check.Source)
	assert.Contains(t, check.Message, "Usage limit exceeded")
}

func TestCheckEffectiveUsageLimit_Source_LimitExceeded_Organization(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-source-exceeded-org"

	orgPlan := createPlan(t, db, "Org Basic", 10, 2)
	createOrgWithSubscription(t, db, "source-exceeded-org", userID, orgPlan)

	// Insert 2 active terminals to hit the limit
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")
	db.Exec("INSERT INTO terminals (id, user_id, status) VALUES (?, ?, ?)", uuid.New().String(), userID, "active")

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.NoError(t, err)
	assert.NotNil(t, check)
	assert.False(t, check.Allowed)
	assert.Equal(t, services.PlanSourceOrganization, check.Source)
	assert.Contains(t, check.Message, "Usage limit exceeded")
}

func TestCheckEffectiveUsageLimit_NoSubscription_ReturnsError(t *testing.T) {
	db := freshTestDB(t)
	ensureTerminalsTable(t, db)
	userID := "user-no-subscription"

	svc := services.NewEffectivePlanService(db)
	check, err := svc.CheckEffectiveUsageLimit(userID, "concurrent_terminals", 1)

	assert.Error(t, err)
	assert.Nil(t, check)
	assert.Contains(t, err.Error(), "failed to get effective plan")
}
