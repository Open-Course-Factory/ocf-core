package payment_tests

import (
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"

	emm "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func ptrString(s string) *string { return &s }

func TestConversionService_SubscriptionPlanToDTO(t *testing.T) {
	conversionService := services.NewConversionService()

	planID := uuid.New()
	createdAt := time.Now()
	updatedAt := time.Now()

	plan := &models.SubscriptionPlan{

		BaseModel: emm.BaseModel{
			ID: planID,
			Model: gorm.Model{
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
		},
		Name:               "Pro Plan",
		Description:        "Professional plan with advanced features",
		StripeProductID:    ptrString("prod_123"),
		StripePriceID:      ptrString("price_123"),
		PriceAmount:        2999, // 29.99 EUR
		Currency:           "eur",
		BillingInterval:    "month",
		TrialDays:          14,
		Features:           []string{"advanced_labs", "api_access", "custom_themes"},
		MaxConcurrentUsers: 10,
		MaxCourses:         -1, // Unlimited
		MaxLabSessions:     100,
		IsActive:           true,
		RequiredRole:       "member_pro",
	}

	result, err := conversionService.SubscriptionPlanToDTO(plan)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, planID, result.ID)
	assert.Equal(t, "Pro Plan", result.Name)
	assert.Equal(t, "Professional plan with advanced features", result.Description)
	assert.Equal(t, ptrString("prod_123"), result.StripeProductID)
	assert.Equal(t, ptrString("price_123"), result.StripePriceID)
	assert.Equal(t, int64(2999), result.PriceAmount)
	assert.Equal(t, "eur", result.Currency)
	assert.Equal(t, "month", result.BillingInterval)
	assert.Equal(t, 14, result.TrialDays)
	assert.Equal(t, []string{"advanced_labs", "api_access", "custom_themes"}, result.Features)
	assert.Equal(t, 10, result.MaxConcurrentUsers)
	assert.Equal(t, -1, result.MaxCourses)
	assert.Equal(t, 100, result.MaxLabSessions)
	assert.True(t, result.IsActive)
	assert.Equal(t, "member_pro", result.RequiredRole)
	assert.Equal(t, createdAt, result.CreatedAt)
	assert.Equal(t, updatedAt, result.UpdatedAt)
}

func TestConversionService_SubscriptionPlanToDTO_Nil(t *testing.T) {
	conversionService := services.NewConversionService()

	result, err := conversionService.SubscriptionPlanToDTO(nil)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestConversionService_UserSubscriptionToDTO(t *testing.T) {
	conversionService := services.NewConversionService()

	subscriptionID := uuid.New()
	planID := uuid.New()
	now := time.Now()
	trialEnd := now.AddDate(0, 0, 14)

	subscription := &models.UserSubscription{
		BaseModel: emm.BaseModel{
			ID: subscriptionID,
			Model: gorm.Model{
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
		UserID:               "user123",
		SubscriptionPlanID:   planID,
		StripeSubscriptionID: "sub_123",
		StripeCustomerID:     "cus_123",
		Status:               "active",
		CurrentPeriodStart:   now,
		CurrentPeriodEnd:     now.AddDate(0, 1, 0),
		TrialEnd:             &trialEnd,
		CancelAtPeriodEnd:    false,
	}

	result, err := conversionService.UserSubscriptionToDTO(subscription)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, subscriptionID, result.ID)
	assert.Equal(t, "user123", result.UserID)
	assert.Equal(t, "sub_123", result.StripeSubscriptionID)
	assert.Equal(t, "cus_123", result.StripeCustomerID)
	assert.Equal(t, "active", result.Status)
	assert.Equal(t, &trialEnd, result.TrialEnd)
	assert.False(t, result.CancelAtPeriodEnd)
	assert.Nil(t, result.CancelledAt)

	// Vérifier le plan inclus
	assert.Equal(t, planID, result.SubscriptionPlanID)
	// assert.Equal(t, "Test Plan", result.SubscriptionPlan.Name)
	// assert.Equal(t, int64(1999), result.SubscriptionPlan.PriceAmount)
}

func TestConversionService_UsageMetricsToDTO(t *testing.T) {
	conversionService := services.NewConversionService()

	metricsID := uuid.New()
	now := time.Now()

	metrics := &models.UsageMetrics{
		BaseModel: emm.BaseModel{
			ID: metricsID,
		},
		UserID:       "user123",
		MetricType:   "courses_created",
		CurrentValue: 7,
		LimitValue:   10,
		PeriodStart:  now.AddDate(0, 0, -15),
		PeriodEnd:    now.AddDate(0, 0, 15),
		LastUpdated:  now,
	}

	result, err := conversionService.UsageMetricsToDTO(metrics)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, metricsID, result.ID)
	assert.Equal(t, "user123", result.UserID)
	assert.Equal(t, "courses_created", result.MetricType)
	assert.Equal(t, int64(7), result.CurrentValue)
	assert.Equal(t, int64(10), result.LimitValue)
	assert.Equal(t, float64(70), result.UsagePercent) // 7/10 * 100
	assert.Equal(t, now, result.LastUpdated)
}

func TestConversionService_UsageMetricsToDTO_UnlimitedUsage(t *testing.T) {
	conversionService := services.NewConversionService()

	metrics := &models.UsageMetrics{
		UserID:       "user123",
		MetricType:   "lab_sessions",
		CurrentValue: 50,
		LimitValue:   -1, // Unlimited
	}

	result, err := conversionService.UsageMetricsToDTO(metrics)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(50), result.CurrentValue)
	assert.Equal(t, int64(-1), result.LimitValue)
	assert.Equal(t, float64(0), result.UsagePercent) // 0% pour unlimited
}

func TestConversionService_UsageLimitCheckToDTO(t *testing.T) {
	conversionService := services.NewConversionService()

	check := &services.UsageLimitCheck{
		Allowed:        false,
		CurrentUsage:   8,
		Limit:          10,
		RemainingUsage: 2,
		Message:        "Approaching limit",
		UserID:         "user123",
		MetricType:     "courses_created",
	}

	result := conversionService.UsageLimitCheckToDTO(check)

	assert.NotNil(t, result)
	assert.False(t, result.Allowed)
	assert.Equal(t, int64(8), result.CurrentUsage)
	assert.Equal(t, int64(10), result.Limit)
	assert.Equal(t, int64(2), result.RemainingUsage)
	assert.Equal(t, "Approaching limit", result.Message)
}

func TestConversionService_UsageLimitCheckToDTO_Nil(t *testing.T) {
	conversionService := services.NewConversionService()

	result := conversionService.UsageLimitCheckToDTO(nil)

	assert.Nil(t, result)
}

func TestConversionService_PaymentMethodToDTO(t *testing.T) {
	conversionService := services.NewConversionService()

	pmID := uuid.New()
	createdAt := time.Now()

	pm := &models.PaymentMethod{
		BaseModel: emm.BaseModel{
			ID: pmID,
			Model: gorm.Model{
				CreatedAt: createdAt,
			},
		},
		UserID:                "user123",
		StripePaymentMethodID: "pm_123",
		Type:                  "card",
		CardBrand:             "visa",
		CardLast4:             "4242",
		CardExpMonth:          12,
		CardExpYear:           2025,
		IsDefault:             true,
		IsActive:              true,
	}

	result, err := conversionService.PaymentMethodToDTO(pm)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, pmID, result.ID)
	assert.Equal(t, "user123", result.UserID)
	assert.Equal(t, "pm_123", result.StripePaymentMethodID)
	assert.Equal(t, "card", result.Type)
	assert.Equal(t, "visa", result.CardBrand)
	assert.Equal(t, "4242", result.CardLast4)
	assert.Equal(t, 12, result.CardExpMonth)
	assert.Equal(t, 2025, result.CardExpYear)
	assert.True(t, result.IsDefault)
	assert.True(t, result.IsActive)
	assert.Equal(t, createdAt, result.CreatedAt)
}

func TestConversionService_SubscriptionPlansToDTO(t *testing.T) {
	conversionService := services.NewConversionService()

	plan1 := models.SubscriptionPlan{
		BaseModel: emm.BaseModel{ID: uuid.New()},
		Name:      "Basic Plan",
		IsActive:  true,
	}

	plan2 := models.SubscriptionPlan{
		BaseModel: emm.BaseModel{ID: uuid.New()},
		Name:      "Pro Plan",
		IsActive:  true,
	}

	plans := &[]models.SubscriptionPlan{plan1, plan2}

	result, err := conversionService.SubscriptionPlansToDTO(plans)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, *result, 2)
	assert.Equal(t, "Basic Plan", (*result)[0].Name)
	assert.Equal(t, "Pro Plan", (*result)[1].Name)
}

func TestConversionService_SubscriptionPlansToDTO_Nil(t *testing.T) {
	conversionService := services.NewConversionService()

	result, err := conversionService.SubscriptionPlansToDTO(nil)

	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestConversionService_SubscriptionAnalyticsToDTO(t *testing.T) {
	conversionService := services.NewConversionService()

	now := time.Now()

	// Créer quelques subscriptions pour les tests
	subscription1 := models.UserSubscription{
		BaseModel: emm.BaseModel{ID: uuid.New()},
		UserID:    "user1",
		Status:    "active",
	}

	subscription2 := models.UserSubscription{
		BaseModel: emm.BaseModel{ID: uuid.New()},
		UserID:    "user2",
		Status:    "cancelled",
	}

	analytics := &services.SubscriptionAnalytics{
		TotalSubscriptions:      100,
		ActiveSubscriptions:     80,
		CancelledSubscriptions:  15,
		TrialSubscriptions:      5,
		Revenue:                 50000, // 500.00 EUR en centimes
		MonthlyRecurringRevenue: 8000,  // 80.00 EUR en centimes
		ChurnRate:               15.0,  // 15%
		ByPlan:                  map[string]int{"Basic": 40, "Pro": 35, "Enterprise": 5},
		RecentSignups:           []models.UserSubscription{subscription1},
		RecentCancellations:     []models.UserSubscription{subscription2},
		GeneratedAt:             now,
	}

	result := conversionService.SubscriptionAnalyticsToDTO(analytics)

	assert.NotNil(t, result)
	assert.Equal(t, int64(100), result.TotalSubscriptions)
	assert.Equal(t, int64(80), result.ActiveSubscriptions)
	assert.Equal(t, int64(15), result.CancelledSubscriptions)
	assert.Equal(t, int64(5), result.TrialSubscriptions)
	assert.Equal(t, int64(50000), result.Revenue)
	assert.Equal(t, int64(8000), result.MonthlyRecurringRevenue)
	assert.Equal(t, 15.0, result.ChurnRate)
	assert.Equal(t, map[string]int{"Basic": 40, "Pro": 35, "Enterprise": 5}, result.ByPlan)
	assert.Len(t, result.RecentSignups, 1)
	assert.Len(t, result.RecentCancellations, 1)
	assert.Equal(t, now, result.GeneratedAt)
}

func TestConversionService_SubscriptionAnalyticsToDTO_Nil(t *testing.T) {
	conversionService := services.NewConversionService()

	result := conversionService.SubscriptionAnalyticsToDTO(nil)

	assert.Nil(t, result)
}

// Test de performance pour vérifier que les conversions sont rapides
func BenchmarkConversionService_SubscriptionPlanToDTO(b *testing.B) {
	conversionService := services.NewConversionService()

	plan := &models.SubscriptionPlan{
		BaseModel:       emm.BaseModel{ID: uuid.New()},
		Name:            "Benchmark Plan",
		Description:     "Plan for benchmarking",
		PriceAmount:     1999,
		Currency:        "eur",
		BillingInterval: "month",
		IsActive:        true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = conversionService.SubscriptionPlanToDTO(plan)
	}
}

func BenchmarkConversionService_UserSubscriptionToDTO(b *testing.B) {
	conversionService := services.NewConversionService()

	subscription := &models.UserSubscription{
		BaseModel:            emm.BaseModel{ID: uuid.New()},
		UserID:               "benchmark-user",
		SubscriptionPlanID:   uuid.New(),
		StripeSubscriptionID: "sub_benchmark",
		Status:               "active",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = conversionService.UserSubscriptionToDTO(subscription)
	}
}
