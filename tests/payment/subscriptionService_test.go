// tests/payment/subscriptionService_test.go
package payment_tests

import (
	"fmt"
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	emm "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Mock repository pour les tests
type MockPaymentRepository struct {
	mock.Mock
}

func (m *MockPaymentRepository) GetActiveUserSubscription(userID string) (*models.UserSubscription, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *MockPaymentRepository) GetUserSubscription(id uuid.UUID) (*models.UserSubscription, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *MockPaymentRepository) CreateUserSubscription(subscription *models.UserSubscription) error {
	args := m.Called(subscription)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetUserUsageMetrics(userID, metricType string) (*models.UsageMetrics, error) {
	args := m.Called(userID, metricType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UsageMetrics), args.Error(1)
}

func (m *MockPaymentRepository) GetAllUserUsageMetrics(userID string) (*[]models.UsageMetrics, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.UsageMetrics), args.Error(1)
}

func (m *MockPaymentRepository) IncrementUsageMetric(userID, metricType string, increment int64) error {
	args := m.Called(userID, metricType, increment)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetUserPaymentMethods(userID string, activeOnly bool) (*[]models.PaymentMethod, error) {
	args := m.Called(userID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.PaymentMethod), args.Error(1)
}

func (m *MockPaymentRepository) SetDefaultPaymentMethod(userID string, pmID uuid.UUID) error {
	args := m.Called(userID, pmID)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetUserInvoices(userID string, limit int) (*[]models.Invoice, error) {
	args := m.Called(userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.Invoice), args.Error(1)
}

func (m *MockPaymentRepository) GetInvoice(id uuid.UUID) (*models.Invoice, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Invoice), args.Error(1)
}

func (m *MockPaymentRepository) GetSubscriptionPlan(id uuid.UUID) (*models.SubscriptionPlan, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.SubscriptionPlan), args.Error(1)
}

func (m *MockPaymentRepository) GetAllSubscriptionPlans(activeOnly bool) (*[]models.SubscriptionPlan, error) {
	args := m.Called(activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.SubscriptionPlan), args.Error(1)
}

func (m *MockPaymentRepository) GetSubscriptionAnalytics(startDate, endDate time.Time) (*repositories.SubscriptionAnalytics, error) {
	args := m.Called(startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repositories.SubscriptionAnalytics), args.Error(1)
}

func (m *MockPaymentRepository) ResetUsageMetrics(userID string, periodStart, periodEnd time.Time) error {
	args := m.Called(userID, periodStart, periodEnd)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetUserBillingAddresses(userID string) (*[]models.BillingAddress, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.BillingAddress), args.Error(1)
}

func (m *MockPaymentRepository) SetDefaultBillingAddress(userID string, addressID uuid.UUID) error {
	args := m.Called(userID, addressID)
	return args.Error(0)
}

// Ajout des méthodes manquantes pour satisfaire l'interface complète
func (m *MockPaymentRepository) CreateSubscriptionPlan(plan *models.SubscriptionPlan) error {
	args := m.Called(plan)
	return args.Error(0)
}

func (m *MockPaymentRepository) UpdateSubscriptionPlan(plan *models.SubscriptionPlan) error {
	args := m.Called(plan)
	return args.Error(0)
}

func (m *MockPaymentRepository) DeleteSubscriptionPlan(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetUserSubscriptionByStripeID(stripeSubscriptionID string) (*models.UserSubscription, error) {
	args := m.Called(stripeSubscriptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *MockPaymentRepository) GetUserSubscriptions(userID string, includeInactive bool) (*[]models.UserSubscription, error) {
	args := m.Called(userID, includeInactive)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.UserSubscription), args.Error(1)
}

func (m *MockPaymentRepository) UpdateUserSubscription(subscription *models.UserSubscription) error {
	args := m.Called(subscription)
	return args.Error(0)
}

func (m *MockPaymentRepository) CreateInvoice(invoice *models.Invoice) error {
	args := m.Called(invoice)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetInvoiceByStripeID(stripeInvoiceID string) (*models.Invoice, error) {
	args := m.Called(stripeInvoiceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Invoice), args.Error(1)
}

func (m *MockPaymentRepository) UpdateInvoice(invoice *models.Invoice) error {
	args := m.Called(invoice)
	return args.Error(0)
}

func (m *MockPaymentRepository) CreatePaymentMethod(pm *models.PaymentMethod) error {
	args := m.Called(pm)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetPaymentMethod(id uuid.UUID) (*models.PaymentMethod, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.PaymentMethod), args.Error(1)
}

func (m *MockPaymentRepository) UpdatePaymentMethod(pm *models.PaymentMethod) error {
	args := m.Called(pm)
	return args.Error(0)
}

func (m *MockPaymentRepository) DeletePaymentMethod(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockPaymentRepository) CreateBillingAddress(address *models.BillingAddress) error {
	args := m.Called(address)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetDefaultBillingAddress(userID string) (*models.BillingAddress, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.BillingAddress), args.Error(1)
}

func (m *MockPaymentRepository) UpdateBillingAddress(address *models.BillingAddress) error {
	args := m.Called(address)
	return args.Error(0)
}

func (m *MockPaymentRepository) DeleteBillingAddress(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockPaymentRepository) CreateOrUpdateUsageMetrics(metrics *models.UsageMetrics) error {
	args := m.Called(metrics)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetRevenueByPeriod(startDate, endDate time.Time, interval string) (*[]repositories.RevenueByPeriod, error) {
	args := m.Called(startDate, endDate, interval)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]repositories.RevenueByPeriod), args.Error(1)
}

func (m *MockPaymentRepository) CleanupExpiredSubscriptions() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPaymentRepository) ArchiveOldInvoices(daysOld int) error {
	args := m.Called(daysOld)
	return args.Error(0)
}

// Service avec mock pour les tests
type subscriptionServiceWithMock struct {
	repository repositories.PaymentRepository
}

func (s *subscriptionServiceWithMock) HasActiveSubscription(userID string) (bool, error) {
	_, err := s.repository.GetActiveUserSubscription(userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *subscriptionServiceWithMock) GetActiveUserSubscription(userID string) (*models.UserSubscription, error) {
	return s.repository.GetActiveUserSubscription(userID)
}

func (s *subscriptionServiceWithMock) CheckUsageLimit(userID, metricType string, increment int64) (*services.UsageLimitCheck, error) {
	subscription, err := s.repository.GetActiveUserSubscription(userID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &services.UsageLimitCheck{
				Allowed:        false,
				CurrentUsage:   0,
				Limit:          0,
				RemainingUsage: 0,
				Message:        "No active subscription - upgrade required",
				UserID:         userID,
				MetricType:     metricType,
			}, nil
		}
		return nil, err
	}

	sPlan, errSPlan := s.repository.GetSubscriptionPlan(subscription.SubscriptionPlanID)
	if errSPlan != nil {
		return nil, err
	}

	metrics, err := s.repository.GetUserUsageMetrics(userID, metricType)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// First usage
			var limit int64
			switch metricType {
			case "courses_created":
				limit = int64(sPlan.MaxCourses)
			case "lab_sessions":
				limit = int64(sPlan.MaxLabSessions)
			case "concurrent_users":
				limit = int64(sPlan.MaxConcurrentUsers)
			default:
				limit = -1
			}

			return &services.UsageLimitCheck{
				Allowed:        limit == -1 || increment <= limit,
				CurrentUsage:   0,
				Limit:          limit,
				RemainingUsage: limit,
				Message:        "",
				UserID:         userID,
				MetricType:     metricType,
			}, nil
		}
		return nil, err
	}

	newUsage := metrics.CurrentValue + increment
	allowed := metrics.LimitValue == -1 || newUsage <= metrics.LimitValue

	remaining := metrics.LimitValue - metrics.CurrentValue
	if remaining < 0 {
		remaining = 0
	}

	message := ""
	if !allowed {
		message = "Usage limit exceeded"
	}

	return &services.UsageLimitCheck{
		Allowed:        allowed,
		CurrentUsage:   metrics.CurrentValue,
		Limit:          metrics.LimitValue,
		RemainingUsage: remaining,
		Message:        message,
		UserID:         userID,
		MetricType:     metricType,
	}, nil
}

// Setup pour les tests
func setupTestDB() *gorm.DB {
	dbName := fmt.Sprintf("file:memdb%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to test database")
	}

	err = db.AutoMigrate(
		&models.SubscriptionPlan{},
		&models.UserSubscription{},
		&models.UsageMetrics{},
		&models.PaymentMethod{},
		&models.Invoice{},
		&models.BillingAddress{},
	)

	if err != nil {
		panic("Failed to migrate database: " + err.Error())
	}

	return db
}

func TestSubscriptionService_HasActiveSubscription(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		mockSetup      func(*MockPaymentRepository)
		expectedResult bool
		expectError    bool
	}{
		{
			name:   "User has active subscription",
			userID: "user123",
			mockSetup: func(m *MockPaymentRepository) {
				subscription := &models.UserSubscription{
					UserID: "user123",
					Status: "active",
				}
				m.On("GetActiveUserSubscription", "user123").Return(subscription, nil)
			},
			expectedResult: true,
			expectError:    false,
		},
		{
			name:   "User has no active subscription",
			userID: "user456",
			mockSetup: func(m *MockPaymentRepository) {
				m.On("GetActiveUserSubscription", "user456").Return(nil, gorm.ErrRecordNotFound)
			},
			expectedResult: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockPaymentRepository{}
			tt.mockSetup(mockRepo)

			service := &subscriptionServiceWithMock{repository: mockRepo}

			result, err := service.HasActiveSubscription(tt.userID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestSubscriptionService_CheckUsageLimit(t *testing.T) {
	tests := []struct {
		name              string
		userID            string
		metricType        string
		increment         int64
		mockSetup         func(*MockPaymentRepository)
		expectedAllowed   bool
		expectedRemaining int64
	}{
		{
			name:       "User within limits",
			userID:     "user123",
			metricType: "courses_created",
			increment:  1,
			mockSetup: func(m *MockPaymentRepository) {
				subscription := &models.UserSubscription{
					UserID:             "user123",
					SubscriptionPlanID: uuid.New(),
				}
				m.On("GetActiveUserSubscription", "user123").Return(subscription, nil)

				metrics := &models.UsageMetrics{
					CurrentValue: 5,
					LimitValue:   10,
				}
				m.On("GetUserUsageMetrics", "user123", "courses_created").Return(metrics, nil)

				subscriptionPlan := &models.SubscriptionPlan{
					BaseModel: emm.BaseModel{
						ID: uuid.New(),
					},
				}
				m.On("GetSubscriptionPlan", subscription.SubscriptionPlanID).Return(subscriptionPlan, nil)
			},
			expectedAllowed:   true,
			expectedRemaining: 5,
		},
		{
			name:       "User exceeds limits",
			userID:     "user456",
			metricType: "courses_created",
			increment:  1,
			mockSetup: func(m *MockPaymentRepository) {
				subscription := &models.UserSubscription{
					UserID:             "user456",
					SubscriptionPlanID: uuid.New(),
				}
				m.On("GetActiveUserSubscription", "user456").Return(subscription, nil)

				metrics := &models.UsageMetrics{
					CurrentValue: 5,
					LimitValue:   5,
				}
				m.On("GetUserUsageMetrics", "user456", "courses_created").Return(metrics, nil)

				subscriptionPlan := &models.SubscriptionPlan{
					BaseModel: emm.BaseModel{
						ID: uuid.New(),
					},
				}
				m.On("GetSubscriptionPlan", subscription.SubscriptionPlanID).Return(subscriptionPlan, nil)
			},
			expectedAllowed:   false,
			expectedRemaining: 0,
		},
		{
			name:       "No active subscription",
			userID:     "user789",
			metricType: "courses_created",
			increment:  1,
			mockSetup: func(m *MockPaymentRepository) {
				m.On("GetActiveUserSubscription", "user789").Return(nil, gorm.ErrRecordNotFound)
			},
			expectedAllowed:   false,
			expectedRemaining: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := &MockPaymentRepository{}
			tt.mockSetup(mockRepo)

			service := &subscriptionServiceWithMock{repository: mockRepo}

			result, err := service.CheckUsageLimit(tt.userID, tt.metricType, tt.increment)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedAllowed, result.Allowed)
			assert.Equal(t, tt.expectedRemaining, result.RemainingUsage)
			assert.Equal(t, tt.userID, result.UserID)
			assert.Equal(t, tt.metricType, result.MetricType)

			mockRepo.AssertExpectations(t)
		})
	}
}

// Test d'intégration avec vraie DB
func TestSubscriptionService_Integration_CreateAndCheckLimits(t *testing.T) {
	db := setupTestDB()

	assert.True(t, db.Migrator().HasTable("subscription_plans"))
	assert.True(t, db.Migrator().HasTable("user_subscriptions"))
	assert.True(t, db.Migrator().HasTable("usage_metrics"))

	service := services.NewSubscriptionService(db)

	// Créer un plan d'abonnement
	planID := uuid.New()
	plan := &models.SubscriptionPlan{
		Name:         "Test Plan",
		MaxCourses:   5,
		RequiredRole: "member_pro",
		IsActive:     true,
	}
	plan.ID = planID
	result := db.Create(plan)

	assert.NoError(t, result.Error)
	assert.Equal(t, int64(1), result.RowsAffected)

	// Vérifier que le plan a été créé
	var createdPlan models.SubscriptionPlan
	err := db.First(&createdPlan, "id = ?", planID).Error
	assert.NoError(t, err)
	assert.Equal(t, "Test Plan", createdPlan.Name)

	// Créer un abonnement utilisateur
	subscriptionID := uuid.New()
	subscription := &models.UserSubscription{
		UserID:             "integration-test-user",
		SubscriptionPlanID: planID,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
	}
	subscription.ID = subscriptionID
	result = db.Create(subscription)

	assert.NoError(t, result.Error)
	assert.Equal(t, int64(1), result.RowsAffected)

	// Vérifier que l'abonnement a été créé
	var createdSubscription models.UserSubscription
	err = db.First(&createdSubscription, "id = ?", subscriptionID).Error
	assert.NoError(t, err)
	assert.Equal(t, "integration-test-user", createdSubscription.UserID)

	// Test 1: Vérifier qu'il a un abonnement actif
	hasActive, err := service.HasActiveSubscription("integration-test-user")
	if err != nil {
		t.Logf("Error checking active subscription: %v", err)

		// Debug: vérifier manuellement dans la DB
		var count int64
		db.Model(&models.UserSubscription{}).Where("user_id = ? AND status = ?", "integration-test-user", "active").Count(&count)
		t.Logf("Manual count of active subscriptions: %d", count)

		// Lister toutes les subscriptions pour debug
		var allSubs []models.UserSubscription
		db.Find(&allSubs)
		t.Logf("All subscriptions in DB: %+v", allSubs)
	}
	assert.NoError(t, err)
	assert.True(t, hasActive)

	// Test 2: Récupérer l'abonnement actif
	activeSubscription, err := service.GetActiveUserSubscription("integration-test-user")
	assert.NoError(t, err)
	assert.NotNil(t, activeSubscription)
	assert.Equal(t, "integration-test-user", activeSubscription.UserID)
	assert.Equal(t, "active", activeSubscription.Status)

	// Test 3: Vérifier les limites d'usage (première utilisation)
	limitCheck, err := service.CheckUsageLimit("integration-test-user", "courses_created", 1)
	assert.NoError(t, err)
	assert.True(t, limitCheck.Allowed)
	assert.Equal(t, int64(5), limitCheck.Limit)
	assert.Equal(t, int64(0), limitCheck.CurrentUsage)

	// Test 4: Incrémenter l'usage
	err = service.IncrementUsage("integration-test-user", "courses_created", 3)
	assert.NoError(t, err)

	// Test 5: Vérifier que les limites sont mises à jour
	limitCheck, err = service.CheckUsageLimit("integration-test-user", "courses_created", 3)
	assert.NoError(t, err)
	assert.False(t, limitCheck.Allowed) // Dépasserait la limite de 5
	assert.Equal(t, int64(3), limitCheck.CurrentUsage)
	assert.Equal(t, int64(2), limitCheck.RemainingUsage)
}

func TestSubscriptionService_Integration_GetUserUsageMetrics(t *testing.T) {
	db := setupTestDB()
	service := services.NewSubscriptionService(db)

	// Créer un plan et un abonnement comme dans le test précédent
	plan := &models.SubscriptionPlan{
		Name:         "Test Plan 2",
		MaxCourses:   10,
		RequiredRole: "member_pro",
		IsActive:     true,
	}
	plan.ID = uuid.New()
	db.Create(plan)

	subscription := &models.UserSubscription{
		UserID:             "metrics-test-user",
		SubscriptionPlanID: plan.ID,
		Status:             "active",
		CurrentPeriodStart: time.Now(),
		CurrentPeriodEnd:   time.Now().AddDate(0, 1, 0),
	}
	subscription.ID = uuid.New()
	db.Create(subscription)

	// Incrémenter plusieurs métriques
	err := service.IncrementUsage("metrics-test-user", "courses_created", 3)
	assert.NoError(t, err)

	err = service.IncrementUsage("metrics-test-user", "lab_sessions", 5)
	assert.NoError(t, err)

	// Récupérer toutes les métriques
	metrics, err := service.GetUserUsageMetrics("metrics-test-user")
	assert.NoError(t, err)
	assert.NotNil(t, metrics)
	assert.Len(t, *metrics, 2) // 2 types de métriques créées

	// Vérifier le contenu des métriques
	var coursesMetric *models.UsageMetrics
	var labSessionsMetric *models.UsageMetrics

	for _, metric := range *metrics {
		switch metric.MetricType {
		case "courses_created":
			coursesMetric = &metric
		case "lab_sessions":
			labSessionsMetric = &metric
		}
	}

	assert.NotNil(t, coursesMetric)
	assert.Equal(t, int64(3), coursesMetric.CurrentValue)
	assert.Equal(t, int64(10), coursesMetric.LimitValue)

	assert.NotNil(t, labSessionsMetric)
	assert.Equal(t, int64(5), labSessionsMetric.CurrentValue)
}
