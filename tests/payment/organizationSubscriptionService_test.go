// tests/payment/organizationSubscriptionService_test.go
package payment_tests

import (
	"testing"
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"
	organizationModels "soli/formations/src/organizations/models"
	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// seedTestData creates test data for organization subscription tests
func seedTestData(t *testing.T, db *gorm.DB) (
	*models.SubscriptionPlan,
	*models.SubscriptionPlan,
	*organizationModels.Organization,
	*organizationModels.Organization,
	string,
) {
	// Create free plan
	freePlan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Free",
		Priority:  0,
		PriceAmount: 0,
		Currency:  "eur",
		BillingInterval: "month",
		Features: []string{"basic_features"},
		MaxConcurrentTerminals: 1,
		MaxCourses: 3,
		IsActive: true,
	}
	err := db.Create(freePlan).Error
	assert.NoError(t, err)

	// Create pro plan
	proPlan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Pro",
		Priority:  20,
		PriceAmount: 1200,
		Currency:  "eur",
		BillingInterval: "month",
		Features: []string{"basic_features", "advanced_labs", "custom_themes"},
		MaxConcurrentTerminals: 10,
		MaxCourses: -1,
		IsActive: true,
	}
	err = db.Create(proPlan).Error
	assert.NoError(t, err)

	userID := "test_user_123"

	// Create organizations (omit Metadata to avoid JSONB issues with SQLite)
	org1 := &organizationModels.Organization{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "org1",
		DisplayName: "Test Organization 1",
		OwnerUserID: userID,
		IsActive: true,
	}
	err = db.Omit("Metadata").Create(org1).Error
	assert.NoError(t, err)

	org2 := &organizationModels.Organization{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "org2",
		DisplayName: "Test Organization 2",
		OwnerUserID: "other_user",
		IsActive: true,
	}
	err = db.Omit("Metadata").Create(org2).Error
	assert.NoError(t, err)

	// Create organization members (omit Metadata)
	member1 := &organizationModels.OrganizationMember{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org1.ID,
		UserID: userID,
		Role: organizationModels.OrgRoleOwner,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	err = db.Omit("Metadata").Create(member1).Error
	assert.NoError(t, err)

	member2 := &organizationModels.OrganizationMember{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		OrganizationID: org2.ID,
		UserID: userID,
		Role: organizationModels.OrgRoleMember,
		JoinedAt: time.Now(),
		IsActive: true,
	}
	err = db.Omit("Metadata").Create(member2).Error
	assert.NoError(t, err)

	return freePlan, proPlan, org1, org2, userID
}

func TestOrganizationSubscriptionService_CreateFreePlan(t *testing.T) {
	t.Run("Create free organization subscription", func(t *testing.T) {
		db := freshTestDB(t)
		freePlan, _, org1, _, userID := seedTestData(t, db)
		service := services.NewOrganizationSubscriptionService(db)

		sub, err := service.CreateOrganizationSubscription(org1.ID, freePlan.ID, userID, 1, false)

		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.Equal(t, org1.ID, sub.OrganizationID)
		assert.Equal(t, freePlan.ID, sub.SubscriptionPlanID)
		assert.Equal(t, "active", sub.Status)
		assert.Equal(t, 1, sub.Quantity)

		// Free plans should be active immediately
		assert.False(t, sub.CurrentPeriodStart.IsZero())
		assert.False(t, sub.CurrentPeriodEnd.IsZero())
	})

	t.Run("Create paid organization subscription", func(t *testing.T) {
		db := freshTestDB(t)
		_, proPlan, org2, _, userID := seedTestData(t, db)
		service := services.NewOrganizationSubscriptionService(db)

		sub, err := service.CreateOrganizationSubscription(org2.ID, proPlan.ID, userID, 1, false)

		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.Equal(t, "incomplete", sub.Status) // Paid plans start incomplete
	})

	t.Run("Create subscription for non-existent organization", func(t *testing.T) {
		db := freshTestDB(t)
		freePlan, _, _, _, userID := seedTestData(t, db)
		service := services.NewOrganizationSubscriptionService(db)

		fakeOrgID := uuid.New()

		sub, err := service.CreateOrganizationSubscription(fakeOrgID, freePlan.ID, userID, 1, false)

		assert.Error(t, err)
		assert.Nil(t, sub)
		assert.Contains(t, err.Error(), "organization not found")
	})

	t.Run("Create subscription with invalid plan", func(t *testing.T) {
		db := freshTestDB(t)
		_, _, org1, _, userID := seedTestData(t, db)
		service := services.NewOrganizationSubscriptionService(db)

		fakePlanID := uuid.New()

		sub, err := service.CreateOrganizationSubscription(org1.ID, fakePlanID, userID, 1, false)

		assert.Error(t, err)
		assert.Nil(t, sub)
		assert.Contains(t, err.Error(), "invalid plan ID")
	})
}

func TestOrganizationSubscriptionService_GetSubscription(t *testing.T) {
	db := freshTestDB(t)
	freePlan, _, org1, _, userID := seedTestData(t, db)
	service := services.NewOrganizationSubscriptionService(db)

	// Create a subscription first
	createdSub, err := service.CreateOrganizationSubscription(org1.ID, freePlan.ID, userID, 1, false)
	assert.NoError(t, err)

	t.Run("Get subscription by organization ID", func(t *testing.T) {
		sub, err := service.GetOrganizationSubscription(org1.ID)

		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.Equal(t, createdSub.ID, sub.ID)
		assert.Equal(t, freePlan.Name, sub.SubscriptionPlan.Name)
	})

	t.Run("Get subscription by ID", func(t *testing.T) {
		sub, err := service.GetOrganizationSubscriptionByID(createdSub.ID)

		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.Equal(t, createdSub.ID, sub.ID)
	})

	t.Run("Get subscription for organization without subscription", func(t *testing.T) {
		_, _, _, org2, _ := seedTestData(t, db)

		sub, err := service.GetOrganizationSubscription(org2.ID)

		assert.Error(t, err)
		assert.Nil(t, sub)
	})
}

func TestOrganizationSubscriptionService_UpdateSubscription(t *testing.T) {
	db := freshTestDB(t)
	freePlan, proPlan, org1, _, userID := seedTestData(t, db)
	service := services.NewOrganizationSubscriptionService(db)

	// Create initial subscription
	_, err := service.CreateOrganizationSubscription(org1.ID, freePlan.ID, userID, 1, false)
	assert.NoError(t, err)

	t.Run("Upgrade subscription plan", func(t *testing.T) {
		updatedSub, err := service.UpdateOrganizationSubscription(org1.ID, proPlan.ID)

		assert.NoError(t, err)
		assert.NotNil(t, updatedSub)
		assert.Equal(t, proPlan.ID, updatedSub.SubscriptionPlanID)
		assert.Equal(t, proPlan.Name, updatedSub.SubscriptionPlan.Name)
	})

	t.Run("Update non-existent subscription", func(t *testing.T) {
		fakeOrgID := uuid.New()

		updatedSub, err := service.UpdateOrganizationSubscription(fakeOrgID, proPlan.ID)

		assert.Error(t, err)
		assert.Nil(t, updatedSub)
	})
}

func TestOrganizationSubscriptionService_CancelSubscription(t *testing.T) {
	db := freshTestDB(t)
	freePlan, _, org1, _, userID := seedTestData(t, db)
	service := services.NewOrganizationSubscriptionService(db)

	// Create subscription
	_, err := service.CreateOrganizationSubscription(org1.ID, freePlan.ID, userID, 1, false)
	assert.NoError(t, err)

	t.Run("Cancel subscription at period end", func(t *testing.T) {
		err := service.CancelOrganizationSubscription(org1.ID, true)

		assert.NoError(t, err)

		// Verify cancellation flag
		sub, err := service.GetOrganizationSubscription(org1.ID)
		assert.NoError(t, err)
		assert.True(t, sub.CancelAtPeriodEnd)
		assert.Equal(t, "active", sub.Status) // Still active until period end
		assert.Nil(t, sub.CancelledAt)
	})

	t.Run("Cancel subscription immediately", func(t *testing.T) {
		// Create new org and subscription for this test
		db2 := freshTestDB(t)
		_, _, org2, _, userID2 := seedTestData(t, db2)
		service2 := services.NewOrganizationSubscriptionService(db2)

		// Get the free plan from db2
		var freePlan2 models.SubscriptionPlan
		db2.Where("name = ?", "Free").First(&freePlan2)

		_, err := service2.CreateOrganizationSubscription(org2.ID, freePlan2.ID, userID2, 1, false)
		assert.NoError(t, err)

		err = service2.CancelOrganizationSubscription(org2.ID, false)

		assert.NoError(t, err)

		// Verify immediate cancellation
		_, err = service2.GetOrganizationSubscription(org2.ID)
		assert.Error(t, err) // Should error because status is "cancelled", not "active"

		// Get by ID to check status
		var cancelledSub models.OrganizationSubscription
		db2.Where("organization_id = ?", org2.ID).First(&cancelledSub)
		assert.Equal(t, "cancelled", cancelledSub.Status)
		assert.NotNil(t, cancelledSub.CancelledAt)
	})
}

func TestOrganizationSubscriptionService_FeatureAccess(t *testing.T) {
	db := freshTestDB(t)
	freePlan, _, org1, _, userID := seedTestData(t, db)
	service := services.NewOrganizationSubscriptionService(db)

	// Create subscription with free plan (will be active immediately)
	_, err := service.CreateOrganizationSubscription(org1.ID, freePlan.ID, userID, 1, false)
	assert.NoError(t, err)

	t.Run("Get organization features", func(t *testing.T) {
		features, err := service.GetOrganizationFeatures(org1.ID)

		assert.NoError(t, err)
		assert.NotNil(t, features)
		assert.Equal(t, freePlan.Name, features.Name)
		assert.Equal(t, freePlan.Priority, features.Priority)
		assert.Contains(t, features.Features, "basic_features")
	})

	t.Run("Check organization has feature", func(t *testing.T) {
		hasFeature, err := service.CanOrganizationAccessFeature(org1.ID, "basic_features")

		assert.NoError(t, err)
		assert.True(t, hasFeature)
	})

	t.Run("Check organization does not have feature", func(t *testing.T) {
		hasFeature, err := service.CanOrganizationAccessFeature(org1.ID, "non_existent_feature")

		assert.NoError(t, err)
		assert.False(t, hasFeature)
	})

	t.Run("Get organization usage limits", func(t *testing.T) {
		limits, err := service.GetOrganizationUsageLimits(org1.ID)

		assert.NoError(t, err)
		assert.NotNil(t, limits)
		assert.Equal(t, freePlan.MaxConcurrentTerminals, limits.MaxConcurrentTerminals)
		assert.Equal(t, freePlan.MaxCourses, limits.MaxCourses)
	})
}

func TestOrganizationSubscriptionService_UserEffectiveFeatures(t *testing.T) {
	db := freshTestDB(t)
	freePlan, _, org1, org2, userID := seedTestData(t, db)
	service := services.NewOrganizationSubscriptionService(db)

	// Create a premium free plan (free but with advanced features) for testing aggregation
	premiumFreePlan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Premium Free",
		Priority:  20, // Higher priority than basic free (0)
		PriceAmount: 0, // Free so it will be active immediately
		Currency:  "eur",
		BillingInterval: "month",
		Features: []string{"basic_features", "advanced_labs", "custom_themes"},
		MaxConcurrentTerminals: 10,
		MaxCourses: -1, // Unlimited
		IsActive: true,
	}
	err := db.Create(premiumFreePlan).Error
	assert.NoError(t, err)

	// Create subscriptions for both organizations (both free so both will be active)
	_, err = service.CreateOrganizationSubscription(org1.ID, premiumFreePlan.ID, userID, 1, false)
	assert.NoError(t, err)

	_, err = service.CreateOrganizationSubscription(org2.ID, freePlan.ID, userID, 1, false)
	assert.NoError(t, err)

	t.Run("Get user effective features from multiple orgs", func(t *testing.T) {
		features, err := service.GetUserEffectiveFeatures(userID)

		assert.NoError(t, err)
		assert.NotNil(t, features)

		// Should get the highest priority plan (Premium Free)
		assert.Equal(t, premiumFreePlan.Name, features.HighestPlan.Name)
		assert.Equal(t, premiumFreePlan.Priority, features.HighestPlan.Priority)

		// Should aggregate features from all organizations
		assert.Contains(t, features.AllFeatures, "advanced_labs")
		assert.Contains(t, features.AllFeatures, "basic_features")

		// Should take maximum limits
		assert.Equal(t, premiumFreePlan.MaxConcurrentTerminals, features.MaxConcurrentTerminals)
		assert.Equal(t, premiumFreePlan.MaxCourses, features.MaxCourses)

		// Should include both organizations
		assert.Equal(t, 2, len(features.Organizations))
	})

	t.Run("Check user can access feature via any org", func(t *testing.T) {
		// Feature from premium free plan
		hasFeature, err := service.CanUserAccessFeature(userID, "advanced_labs")
		assert.NoError(t, err)
		assert.True(t, hasFeature)

		// Feature from both plans
		hasFeature, err = service.CanUserAccessFeature(userID, "basic_features")
		assert.NoError(t, err)
		assert.True(t, hasFeature)

		// Non-existent feature
		hasFeature, err = service.CanUserAccessFeature(userID, "non_existent")
		assert.NoError(t, err)
		assert.False(t, hasFeature)
	})

	t.Run("Get organization that provides specific feature", func(t *testing.T) {
		// Advanced labs only in premium free plan (org1)
		org, err := service.GetUserOrganizationWithFeature(userID, "advanced_labs")

		assert.NoError(t, err)
		assert.NotNil(t, org)
		assert.Equal(t, org1.ID, org.ID)
	})

	t.Run("User with no organization subscriptions", func(t *testing.T) {
		noSubUser := "user_no_subs"

		features, err := service.GetUserEffectiveFeatures(noSubUser)

		assert.Error(t, err)
		assert.Nil(t, features)
		assert.Contains(t, err.Error(), "no organization subscriptions")
	})
}

func TestOrganizationSubscriptionService_FeatureAggregation(t *testing.T) {
	db := freshTestDB(t)
	service := services.NewOrganizationSubscriptionService(db)
	userID := "multi_org_user"

	// Create three plans with different priorities and features
	basicPlan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Basic",
		Priority:  10,
		PriceAmount: 500,
		Features: []string{"feature_a", "feature_b"},
		MaxConcurrentTerminals: 2,
		MaxCourses: 10,
		IsActive: true,
	}
	db.Create(basicPlan)

	proPlan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Pro",
		Priority:  20,
		PriceAmount: 1000,
		Features: []string{"feature_b", "feature_c"},
		MaxConcurrentTerminals: 5,
		MaxCourses: 50,
		IsActive: true,
	}
	db.Create(proPlan)

	enterprisePlan := &models.SubscriptionPlan{
		BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
		Name:      "Enterprise",
		Priority:  30,
		PriceAmount: 5000,
		Features: []string{"feature_c", "feature_d", "feature_e"},
		MaxConcurrentTerminals: -1, // Unlimited
		MaxCourses: -1,
		IsActive: true,
	}
	db.Create(enterprisePlan)

	// Create three organizations with different subscriptions
	createOrgWithSubscription := func(name string, plan *models.SubscriptionPlan) uuid.UUID {
		org := &organizationModels.Organization{
			BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
			Name:      name,
			DisplayName: name + " Org",
			OwnerUserID: "owner",
			IsActive: true,
		}
		db.Omit("Metadata").Create(org)

		member := &organizationModels.OrganizationMember{
			BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			UserID: userID,
			Role: organizationModels.OrgRoleMember,
			JoinedAt: time.Now(),
			IsActive: true,
		}
		db.Omit("Metadata").Create(member)

		sub := &models.OrganizationSubscription{
			BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID: org.ID,
			SubscriptionPlanID: plan.ID,
			Status: "active",
			CurrentPeriodStart: time.Now(),
			CurrentPeriodEnd: time.Now().AddDate(0, 1, 0),
			Quantity: 1,
		}
		db.Create(sub)

		return org.ID
	}

	createOrgWithSubscription("basic_org", basicPlan)
	createOrgWithSubscription("pro_org", proPlan)
	createOrgWithSubscription("enterprise_org", enterprisePlan)

	t.Run("Aggregate features across all plans", func(t *testing.T) {
		features, err := service.GetUserEffectiveFeatures(userID)

		assert.NoError(t, err)
		assert.NotNil(t, features)

		// Highest priority plan should be Enterprise
		assert.Equal(t, "Enterprise", features.HighestPlan.Name)
		assert.Equal(t, 30, features.HighestPlan.Priority)

		// Should have union of all features
		assert.Contains(t, features.AllFeatures, "feature_a") // from Basic
		assert.Contains(t, features.AllFeatures, "feature_b") // from Basic and Pro
		assert.Contains(t, features.AllFeatures, "feature_c") // from Pro and Enterprise
		assert.Contains(t, features.AllFeatures, "feature_d") // from Enterprise
		assert.Contains(t, features.AllFeatures, "feature_e") // from Enterprise

		// Should take maximum limits (Enterprise has unlimited)
		assert.Equal(t, -1, features.MaxConcurrentTerminals)
		assert.Equal(t, -1, features.MaxCourses)

		// Should include all three organizations
		assert.Equal(t, 3, len(features.Organizations))
	})
}

func TestOrganizationSubscription_Create_RespectsQuantity(t *testing.T) {
	t.Run("Subscription quantity should match requested amount", func(t *testing.T) {
		db := freshTestDB(t)
		freePlan, _, org1, _, userID := seedTestData(t, db)
		service := services.NewOrganizationSubscriptionService(db)

		// Create subscription via the service.
		// The DTO CreateOrganizationSubscriptionInput has a Quantity field (e.g., 10 seats),
		// but the service method signature does not accept quantity as a parameter.
		// It hardcodes Quantity: 1 on line 105 of organizationSubscriptionService.go.
		requestedQuantity := 10

		sub, err := service.CreateOrganizationSubscription(org1.ID, freePlan.ID, userID, requestedQuantity, false)
		assert.NoError(t, err)
		assert.NotNil(t, sub)

		// Verify the service respects the requested quantity.
		assert.Equal(t, requestedQuantity, sub.Quantity,
			"Subscription quantity should be %d but service hardcodes it to 1", requestedQuantity)
	})
}

func TestOrganizationSubscriptionRepository_GetAllActiveOrganizationSubscriptions(t *testing.T) {
	t.Run("Returns active and trialing subscriptions with plan details preloaded", func(t *testing.T) {
		db := freshTestDB(t)
		freePlan, proPlan, org1, org2, _ := seedTestData(t, db)
		repo := repositories.NewOrganizationSubscriptionRepository(db)

		// Create an active subscription for org1
		activeSub := &models.OrganizationSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID:     org1.ID,
			SubscriptionPlanID: freePlan.ID,
			Status:             "active",
			CurrentPeriodStart: time.Now(),
			CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
			Quantity:           1,
		}
		err := db.Create(activeSub).Error
		assert.NoError(t, err)

		// Create a trialing subscription for org2
		trialingSub := &models.OrganizationSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID:     org2.ID,
			SubscriptionPlanID: proPlan.ID,
			Status:             "trialing",
			CurrentPeriodStart: time.Now(),
			CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
			Quantity:           5,
		}
		err = db.Create(trialingSub).Error
		assert.NoError(t, err)

		// Call the new method
		subscriptions, err := repo.GetAllActiveOrganizationSubscriptions()

		assert.NoError(t, err)
		assert.Len(t, subscriptions, 2)

		// Verify that SubscriptionPlan is preloaded for each subscription
		for _, sub := range subscriptions {
			assert.NotEqual(t, uuid.Nil, sub.SubscriptionPlan.ID,
				"SubscriptionPlan should be preloaded")
			assert.NotEmpty(t, sub.SubscriptionPlan.Name,
				"SubscriptionPlan.Name should be loaded")
		}

		// Verify we got both active and trialing
		statuses := make(map[string]bool)
		for _, sub := range subscriptions {
			statuses[sub.Status] = true
		}
		assert.True(t, statuses["active"], "Should include active subscriptions")
		assert.True(t, statuses["trialing"], "Should include trialing subscriptions")
	})

	t.Run("Excludes cancelled and incomplete subscriptions", func(t *testing.T) {
		db := freshTestDB(t)
		freePlan, proPlan, org1, org2, _ := seedTestData(t, db)
		repo := repositories.NewOrganizationSubscriptionRepository(db)

		now := time.Now()
		cancelledAt := now

		// Create a cancelled subscription for org1
		cancelledSub := &models.OrganizationSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID:     org1.ID,
			SubscriptionPlanID: freePlan.ID,
			Status:             "cancelled",
			CurrentPeriodStart: now,
			CurrentPeriodEnd:   now.AddDate(0, 1, 0),
			CancelledAt:        &cancelledAt,
			Quantity:           1,
		}
		err := db.Create(cancelledSub).Error
		assert.NoError(t, err)

		// Create an incomplete subscription for org2
		incompleteSub := &models.OrganizationSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID:     org2.ID,
			SubscriptionPlanID: proPlan.ID,
			Status:             "incomplete",
			Quantity:           1,
		}
		err = db.Create(incompleteSub).Error
		assert.NoError(t, err)

		// Call the new method — should return empty since only cancelled/incomplete exist
		subscriptions, err := repo.GetAllActiveOrganizationSubscriptions()

		assert.NoError(t, err)
		assert.Empty(t, subscriptions, "Should not include cancelled or incomplete subscriptions")
	})

	t.Run("Returns empty slice when no active subscriptions exist", func(t *testing.T) {
		db := freshTestDB(t)
		// seedTestData creates orgs and plans but NO subscriptions
		seedTestData(t, db)
		repo := repositories.NewOrganizationSubscriptionRepository(db)

		subscriptions, err := repo.GetAllActiveOrganizationSubscriptions()

		assert.NoError(t, err)
		assert.NotNil(t, subscriptions, "Should return empty slice, not nil")
		assert.Empty(t, subscriptions, "Should return no subscriptions when none exist")
	})
}

func TestOrganizationSubscriptionService_GetAllActiveOrganizationSubscriptions(t *testing.T) {
	t.Run("Service delegates to repository and returns results", func(t *testing.T) {
		db := freshTestDB(t)
		freePlan, proPlan, org1, org2, _ := seedTestData(t, db)
		service := services.NewOrganizationSubscriptionService(db)

		// Create an active subscription for org1
		activeSub := &models.OrganizationSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID:     org1.ID,
			SubscriptionPlanID: freePlan.ID,
			Status:             "active",
			CurrentPeriodStart: time.Now(),
			CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
			Quantity:           1,
		}
		err := db.Create(activeSub).Error
		assert.NoError(t, err)

		// Create a trialing subscription for org2
		trialingSub := &models.OrganizationSubscription{
			BaseModel:          entityManagementModels.BaseModel{ID: uuid.New()},
			OrganizationID:     org2.ID,
			SubscriptionPlanID: proPlan.ID,
			Status:             "trialing",
			CurrentPeriodStart: time.Now(),
			CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
			Quantity:           3,
		}
		err = db.Create(trialingSub).Error
		assert.NoError(t, err)

		// Call the new service method
		subscriptions, err := service.GetAllActiveOrganizationSubscriptions()

		assert.NoError(t, err)
		assert.Len(t, subscriptions, 2)

		// Verify plan details are preloaded
		for _, sub := range subscriptions {
			assert.NotEmpty(t, sub.SubscriptionPlan.Name,
				"SubscriptionPlan should be preloaded via service")
		}
	})
}
