// tests/terminalTrainer/orgTerminalUsageBudget_test.go
//
// Budget fields on GET /organizations/:id/terminal-usage.
//
// The response carries:
//
//   * Quota envelope (MaxCPU / MaxMemoryMB / Used* / Remaining* / Scope)
//   * RemainingBySize array (one entry per catalog size, XL → XS)
//   * Per-user ActiveCPU / ActiveMemoryMB
//
// For plans with zero CPU/RAM caps Quota.Scope is "unlimited" and
// RemainingBySize is empty — the dashboard renders an unconstrained view.
package terminalTrainer_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orgModels "soli/formations/src/organizations/models"
	paymentModels "soli/formations/src/payment/models"
	terminalController "soli/formations/src/terminalTrainer/routes"
)

// TestOrgTerminalUsage_BudgetMode_IncludesQuotaAndRemainingBySize covers the
// happy path: budget plan + one running M → Quota matches, RemainingBySize
// is populated (xl→xs), and per-user CPU/RAM is summed.
func TestOrgTerminalUsage_BudgetMode_IncludesQuotaAndRemainingBySize(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	plan := &paymentModels.SubscriptionPlan{
		Name:        "BudgetPro",
		MaxCPU:      8,
		MaxMemoryMB: 4096,
		IsActive:    true,
		IsCatalog:   true,
	}
	require.NoError(t, db.Create(plan).Error)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)
	createTestOrgMember(t, db, org.ID, "student1", orgModels.OrgRoleMember)

	orgSub := &paymentModels.OrganizationSubscription{
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		StripeCustomerID:   "cus_test_" + uuid.New().String()[:8],
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, db.Create(orgSub).Error)

	// Student has a running M (2c/1g). The hook would denormalise size_cpu
	// / size_memory_mb on insert; here we set them directly so the budget
	// sum picks them up.
	insertExistingTerminal(t, db, "student1", &org.ID, "running", "ephemeral", 2, 1024)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "owner1")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Top-level quota envelope.
	quota, ok := resp["quota"].(map[string]interface{})
	require.True(t, ok, "quota envelope must be present in budget mode")
	assert.Equal(t, float64(8), quota["max_cpu"])
	assert.Equal(t, float64(4096), quota["max_memory_mb"])
	assert.Equal(t, float64(2), quota["used_cpu"])
	assert.Equal(t, float64(1024), quota["used_memory_mb"])
	assert.Equal(t, float64(6), quota["remaining_cpu"])
	assert.Equal(t, float64(3072), quota["remaining_memory_mb"])
	assert.Equal(t, "organization", quota["scope"])

	// RemainingBySize: largest first (xl → xs).
	bySize, ok := resp["remaining_by_size"].([]interface{})
	require.True(t, ok, "remaining_by_size must be present in budget mode")
	require.NotEmpty(t, bySize)
	first := bySize[0].(map[string]interface{})
	assert.Equal(t, "xl", first["key"], "largest size must come first")

	// Per-user CPU/RAM.
	users, ok := resp["users"].([]interface{})
	require.True(t, ok)
	require.Equal(t, 1, len(users))
	student := users[0].(map[string]interface{})
	assert.Equal(t, "student1", student["user_id"])
	assert.Equal(t, float64(2), student["active_cpu"])
	assert.Equal(t, float64(1024), student["active_memory_mb"])
}

// TestOrgTerminalUsage_UnlimitedPlan_UnlimitedScope — plans with zero
// CPU/RAM caps emit Scope=unlimited and an empty RemainingBySize.
func TestOrgTerminalUsage_UnlimitedPlan_UnlimitedScope(t *testing.T) {
	db := setupTestDBWithOrgs(t)

	plan := &paymentModels.SubscriptionPlan{
		Name:      "Unlimited",
		IsActive:  true,
		IsCatalog: true,
		// MaxCPU=0 and MaxMemoryMB=0 → unlimited.
	}
	require.NoError(t, db.Create(plan).Error)

	org := createTestOrgForHistory(t, db, "owner1")
	createTestOrgMember(t, db, org.ID, "owner1", orgModels.OrgRoleOwner)

	orgSub := &paymentModels.OrganizationSubscription{
		OrganizationID:     org.ID,
		SubscriptionPlanID: plan.ID,
		StripeCustomerID:   "cus_test_" + uuid.New().String()[:8],
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(1, 0, 0),
	}
	require.NoError(t, db.Create(orgSub).Error)

	ctrl := terminalController.NewTerminalController(db)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userId", "owner1")
		c.Set("userRoles", []string{"member"})
		c.Next()
	})
	router.GET("/organizations/:id/terminal-usage", ctrl.GetOrgTerminalUsage)

	req := httptest.NewRequest("GET", "/organizations/"+org.ID.String()+"/terminal-usage", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	quota, ok := resp["quota"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "unlimited", quota["scope"],
		"plan with zero caps must emit Scope=unlimited")

	// remaining_by_size must be empty/nil so the frontend skips that block.
	bySize, present := resp["remaining_by_size"]
	if present {
		slice, ok := bySize.([]interface{})
		assert.True(t, ok && len(slice) == 0,
			"remaining_by_size must be empty for unlimited plan (got %v)", bySize)
	}
}
