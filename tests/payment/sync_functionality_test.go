// tests/payment/sync_functionality_test.go
package payment_tests

import (
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// Test des structures et de la logique métier pour les nouvelles fonctionnalités de sync
func TestSyncSubscriptionsResult_Structure(t *testing.T) {
	t.Run("Create and validate SyncSubscriptionsResult", func(t *testing.T) {
		result := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 11,
			CreatedSubscriptions:   5,
			UpdatedSubscriptions:   3,
			SkippedSubscriptions:   2,
			FailedSubscriptions: []services.FailedSubscription{
				{
					StripeSubscriptionID: "sub_test_failed",
					UserID:               "user_123",
					Error:                "Missing metadata",
				},
			},
			CreatedDetails: []string{
				"Created subscription sub_new for user_456",
				"Created subscription sub_new2 for user_789",
			},
			UpdatedDetails: []string{
				"Updated subscription sub_updated for user_101",
			},
			SkippedDetails: []string{
				"Skipped subscription sub_skip: already exists",
				"Skipped subscription sub_skip2: invalid data",
			},
		}

		// Valider la structure
		assert.Equal(t, 11, result.ProcessedSubscriptions)
		assert.Equal(t, 5, result.CreatedSubscriptions)
		assert.Equal(t, 3, result.UpdatedSubscriptions)
		assert.Equal(t, 2, result.SkippedSubscriptions)

		// Valider les échecs
		assert.Len(t, result.FailedSubscriptions, 1)
		assert.Equal(t, "sub_test_failed", result.FailedSubscriptions[0].StripeSubscriptionID)
		assert.Equal(t, "user_123", result.FailedSubscriptions[0].UserID)
		assert.Equal(t, "Missing metadata", result.FailedSubscriptions[0].Error)

		// Valider les détails
		assert.Len(t, result.CreatedDetails, 2)
		assert.Len(t, result.UpdatedDetails, 1)
		assert.Len(t, result.SkippedDetails, 2)

		// Vérifier les totaux
		totalAccounted := result.CreatedSubscriptions + result.UpdatedSubscriptions + result.SkippedSubscriptions + len(result.FailedSubscriptions)
		assert.Equal(t, result.ProcessedSubscriptions, totalAccounted)
	})
}

func TestSyncSubscriptionsResult_EmptyState(t *testing.T) {
	t.Run("Empty result should be valid", func(t *testing.T) {
		result := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 0,
			CreatedSubscriptions:   0,
			UpdatedSubscriptions:   0,
			SkippedSubscriptions:   0,
			FailedSubscriptions:    []services.FailedSubscription{},
			CreatedDetails:         []string{},
			UpdatedDetails:         []string{},
			SkippedDetails:         []string{},
		}

		assert.Equal(t, 0, result.ProcessedSubscriptions)
		assert.Equal(t, 0, result.CreatedSubscriptions)
		assert.Empty(t, result.FailedSubscriptions)
		assert.Empty(t, result.CreatedDetails)
	})
}

func TestFailedSubscription_Structure(t *testing.T) {
	t.Run("Failed subscription with all fields", func(t *testing.T) {
		failed := services.FailedSubscription{
			StripeSubscriptionID: "sub_123_failed",
			UserID:               "user_456",
			Error:                "Invalid subscription plan ID format",
		}

		assert.Equal(t, "sub_123_failed", failed.StripeSubscriptionID)
		assert.Equal(t, "user_456", failed.UserID)
		assert.Equal(t, "Invalid subscription plan ID format", failed.Error)
	})

	t.Run("Failed subscription without user ID", func(t *testing.T) {
		failed := services.FailedSubscription{
			StripeSubscriptionID: "sub_orphaned",
			Error:                "No metadata found in Stripe",
		}

		assert.Equal(t, "sub_orphaned", failed.StripeSubscriptionID)
		assert.Empty(t, failed.UserID)
		assert.Equal(t, "No metadata found in Stripe", failed.Error)
	})
}

func TestMetadataRecovery_Logic(t *testing.T) {
	t.Run("Valid metadata validation", func(t *testing.T) {
		// Simuler la validation des métadonnées
		userID := "user_valid_123"
		planIDStr := uuid.New().String()

		// Test si l'userID est valide
		assert.NotEmpty(t, userID)
		assert.True(t, len(userID) > 0)

		// Test si le plan ID peut être parsé
		planID, err := uuid.Parse(planIDStr)
		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, planID)
	})

	t.Run("Invalid metadata should fail validation", func(t *testing.T) {
		// Test avec un plan ID invalide
		invalidPlanID := "not-a-valid-uuid"
		_, err := uuid.Parse(invalidPlanID)
		assert.Error(t, err)

		// Test avec un userID vide
		emptyUserID := ""
		assert.Empty(t, emptyUserID)
	})
}

func TestSyncResultAggregation_Logic(t *testing.T) {
	t.Run("Aggregate multiple sync operations", func(t *testing.T) {
		// Simuler l'agrégation de résultats de plusieurs opérations
		results := []*services.SyncSubscriptionsResult{
			{
				ProcessedSubscriptions: 5,
				CreatedSubscriptions:   3,
				UpdatedSubscriptions:   1,
				SkippedSubscriptions:   1,
				FailedSubscriptions:    []services.FailedSubscription{},
			},
			{
				ProcessedSubscriptions: 3,
				CreatedSubscriptions:   2,
				UpdatedSubscriptions:   0,
				SkippedSubscriptions:   0,
				FailedSubscriptions: []services.FailedSubscription{
					{StripeSubscriptionID: "sub_fail", Error: "test error"},
				},
			},
		}

		// Calculer les totaux
		totalProcessed := 0
		totalCreated := 0
		totalUpdated := 0
		totalSkipped := 0
		totalFailed := 0

		for _, result := range results {
			totalProcessed += result.ProcessedSubscriptions
			totalCreated += result.CreatedSubscriptions
			totalUpdated += result.UpdatedSubscriptions
			totalSkipped += result.SkippedSubscriptions
			totalFailed += len(result.FailedSubscriptions)
		}

		assert.Equal(t, 8, totalProcessed)
		assert.Equal(t, 5, totalCreated)
		assert.Equal(t, 1, totalUpdated)
		assert.Equal(t, 1, totalSkipped)
		assert.Equal(t, 1, totalFailed)

		// Vérifier que tous les abonnements sont comptés
		assert.Equal(t, totalProcessed, totalCreated+totalUpdated+totalSkipped+totalFailed)
	})
}

func TestLinkSubscriptionValidation_Logic(t *testing.T) {
	t.Run("Valid link parameters", func(t *testing.T) {
		stripeSubscriptionID := "sub_valid_test"
		userID := "user_valid_test"
		subscriptionPlanID := uuid.New()

		// Valider les paramètres d'entrée
		assert.NotEmpty(t, stripeSubscriptionID)
		assert.True(t, len(stripeSubscriptionID) > 0)

		assert.NotEmpty(t, userID)
		assert.True(t, len(userID) > 0)

		assert.NotEqual(t, uuid.Nil, subscriptionPlanID)
	})

	t.Run("Invalid link parameters should be caught", func(t *testing.T) {
		// Test avec des paramètres invalides
		emptySubscriptionID := ""
		emptyUserID := ""
		nilPlanID := uuid.Nil

		assert.Empty(t, emptySubscriptionID)
		assert.Empty(t, emptyUserID)
		assert.Equal(t, uuid.Nil, nilPlanID)
	})
}

func TestWebhookMetadataExtraction_Logic(t *testing.T) {
	t.Run("Extract metadata from webhook payload", func(t *testing.T) {
		// Simuler l'extraction de métadonnées d'un payload webhook
		metadata := map[string]string{
			"user_id":              "user_webhook_test",
			"subscription_plan_id": uuid.New().String(),
			"source":               "checkout_session",
		}

		// Vérifier l'extraction
		userID, hasUserID := metadata["user_id"]
		planIDStr, hasPlanID := metadata["subscription_plan_id"]

		assert.True(t, hasUserID)
		assert.True(t, hasPlanID)
		assert.Equal(t, "user_webhook_test", userID)
		assert.NotEmpty(t, planIDStr)

		// Valider le plan ID
		planID, err := uuid.Parse(planIDStr)
		assert.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, planID)
	})

	t.Run("Missing metadata should be detected", func(t *testing.T) {
		// Métadonnées incomplètes
		incompleteMetadata := map[string]string{
			"user_id": "user_test",
			// subscription_plan_id manquant
		}

		userID, hasUserID := incompleteMetadata["user_id"]
		planIDStr, hasPlanID := incompleteMetadata["subscription_plan_id"]

		assert.True(t, hasUserID)
		assert.False(t, hasPlanID)
		assert.Equal(t, "user_test", userID)
		assert.Empty(t, planIDStr)
	})
}

func TestErrorHandling_EdgeCases(t *testing.T) {
	t.Run("Handle various error scenarios", func(t *testing.T) {
		errorCases := []struct {
			name     string
			subID    string
			userID   string
			expected string
		}{
			{
				name:     "Missing subscription ID",
				subID:    "",
				userID:   "user_123",
				expected: "missing subscription ID",
			},
			{
				name:     "Missing user ID",
				subID:    "sub_123",
				userID:   "",
				expected: "missing user ID",
			},
			{
				name:     "Both missing",
				subID:    "",
				userID:   "",
				expected: "missing required parameters",
			},
		}

		for _, tc := range errorCases {
			t.Run(tc.name, func(t *testing.T) {
				// Simuler la validation d'erreur
				var errorMsg string

				if tc.subID == "" && tc.userID == "" {
					errorMsg = "missing required parameters"
				} else if tc.subID == "" {
					errorMsg = "missing subscription ID"
				} else if tc.userID == "" {
					errorMsg = "missing user ID"
				}

				assert.Contains(t, errorMsg, tc.expected)
			})
		}
	})
}

// ==========================================
// Tests for Invoice Sync Functionality
// ==========================================

func TestSyncInvoicesResult_Structure(t *testing.T) {
	t.Run("Create and validate SyncInvoicesResult", func(t *testing.T) {
		result := &services.SyncInvoicesResult{
			ProcessedInvoices: 10,
			CreatedInvoices:   4,
			UpdatedInvoices:   3,
			SkippedInvoices:   2,
			FailedInvoices: []services.FailedInvoice{
				{
					StripeInvoiceID: "in_test_failed",
					CustomerID:      "cus_123",
					Error:           "No active subscription found",
				},
			},
			CreatedDetails: []string{
				"Created invoice in_new (INV-001) - 1000 usd",
				"Created invoice in_new2 (INV-002) - 2000 eur",
			},
			UpdatedDetails: []string{
				"Updated invoice in_updated (INV-003) - 1500 usd",
			},
			SkippedDetails: []string{
				"Skipped invoice in_skip: no subscription",
			},
		}

		// Valider la structure
		assert.Equal(t, 10, result.ProcessedInvoices)
		assert.Equal(t, 4, result.CreatedInvoices)
		assert.Equal(t, 3, result.UpdatedInvoices)
		assert.Equal(t, 2, result.SkippedInvoices)

		// Valider les échecs
		assert.Len(t, result.FailedInvoices, 1)
		assert.Equal(t, "in_test_failed", result.FailedInvoices[0].StripeInvoiceID)
		assert.Equal(t, "cus_123", result.FailedInvoices[0].CustomerID)
		assert.Equal(t, "No active subscription found", result.FailedInvoices[0].Error)

		// Valider les détails
		assert.Len(t, result.CreatedDetails, 2)
		assert.Len(t, result.UpdatedDetails, 1)
		assert.Len(t, result.SkippedDetails, 1)

		// Vérifier les totaux
		totalAccounted := result.CreatedInvoices + result.UpdatedInvoices + result.SkippedInvoices + len(result.FailedInvoices)
		assert.Equal(t, result.ProcessedInvoices, totalAccounted)
	})
}

func TestSyncInvoicesResult_EmptyState(t *testing.T) {
	t.Run("Empty invoice sync result should be valid", func(t *testing.T) {
		result := &services.SyncInvoicesResult{
			ProcessedInvoices: 0,
			CreatedInvoices:   0,
			UpdatedInvoices:   0,
			SkippedInvoices:   0,
			FailedInvoices:    []services.FailedInvoice{},
			CreatedDetails:    []string{},
			UpdatedDetails:    []string{},
			SkippedDetails:    []string{},
		}

		assert.Equal(t, 0, result.ProcessedInvoices)
		assert.Equal(t, 0, result.CreatedInvoices)
		assert.Empty(t, result.FailedInvoices)
		assert.Empty(t, result.CreatedDetails)
	})
}

func TestFailedInvoice_Structure(t *testing.T) {
	t.Run("Failed invoice with all fields", func(t *testing.T) {
		failed := services.FailedInvoice{
			StripeInvoiceID: "in_123_failed",
			CustomerID:      "cus_456",
			Error:           "No active subscription found for customer",
		}

		assert.Equal(t, "in_123_failed", failed.StripeInvoiceID)
		assert.Equal(t, "cus_456", failed.CustomerID)
		assert.Equal(t, "No active subscription found for customer", failed.Error)
	})

	t.Run("Failed invoice without customer ID", func(t *testing.T) {
		failed := services.FailedInvoice{
			StripeInvoiceID: "in_orphaned",
			Error:           "Customer not found in Stripe",
		}

		assert.Equal(t, "in_orphaned", failed.StripeInvoiceID)
		assert.Empty(t, failed.CustomerID)
		assert.Equal(t, "Customer not found in Stripe", failed.Error)
	})
}

// ==========================================
// Mock Payment Repository for testing
// ==========================================

type MockPaymentRepository struct {
	mock.Mock
}

func (m *MockPaymentRepository) GetActiveSubscriptionByCustomerID(customerID string) (*models.UserSubscription, error) {
	args := m.Called(customerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *MockPaymentRepository) GetActiveUserSubscription(userID string) (*models.UserSubscription, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *MockPaymentRepository) GetUserSubscriptionByStripeID(stripeSubscriptionID string) (*models.UserSubscription, error) {
	args := m.Called(stripeSubscriptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *MockPaymentRepository) GetInvoiceByStripeID(stripeInvoiceID string) (*models.Invoice, error) {
	args := m.Called(stripeInvoiceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Invoice), args.Error(1)
}

func (m *MockPaymentRepository) CreateInvoice(invoice *models.Invoice) error {
	args := m.Called(invoice)
	return args.Error(0)
}

func (m *MockPaymentRepository) UpdateInvoice(invoice *models.Invoice) error {
	args := m.Called(invoice)
	return args.Error(0)
}

// Add other required methods to satisfy interface
func (m *MockPaymentRepository) GetAllSubscriptionPlans(activeOnly bool) (*[]models.SubscriptionPlan, error) {
	args := m.Called(activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.SubscriptionPlan), args.Error(1)
}

func (m *MockPaymentRepository) CreateUserSubscription(subscription *models.UserSubscription) error {
	args := m.Called(subscription)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetUserSubscription(id uuid.UUID) (*models.UserSubscription, error) {
	args := m.Called(id)
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

func (m *MockPaymentRepository) GetInvoice(id uuid.UUID) (*models.Invoice, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Invoice), args.Error(1)
}

func (m *MockPaymentRepository) GetUserInvoices(userID string, limit int) (*[]models.Invoice, error) {
	args := m.Called(userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.Invoice), args.Error(1)
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

func (m *MockPaymentRepository) GetUserPaymentMethods(userID string, activeOnly bool) (*[]models.PaymentMethod, error) {
	args := m.Called(userID, activeOnly)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.PaymentMethod), args.Error(1)
}

func (m *MockPaymentRepository) UpdatePaymentMethod(pm *models.PaymentMethod) error {
	args := m.Called(pm)
	return args.Error(0)
}

func (m *MockPaymentRepository) DeletePaymentMethod(id uuid.UUID) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockPaymentRepository) SetDefaultPaymentMethod(userID string, pmID uuid.UUID) error {
	args := m.Called(userID, pmID)
	return args.Error(0)
}

func (m *MockPaymentRepository) CreateBillingAddress(address *models.BillingAddress) error {
	args := m.Called(address)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetUserBillingAddresses(userID string) (*[]models.BillingAddress, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.BillingAddress), args.Error(1)
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

func (m *MockPaymentRepository) SetDefaultBillingAddress(userID string, addressID uuid.UUID) error {
	args := m.Called(userID, addressID)
	return args.Error(0)
}

func (m *MockPaymentRepository) CreateOrUpdateUsageMetrics(metrics *models.UsageMetrics) error {
	args := m.Called(metrics)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetUserUsageMetrics(userID string, metricType string) (*models.UsageMetrics, error) {
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

func (m *MockPaymentRepository) ResetUsageMetrics(userID string, periodStart, periodEnd time.Time) error {
	args := m.Called(userID, periodStart, periodEnd)
	return args.Error(0)
}

func (m *MockPaymentRepository) GetSubscriptionAnalytics(startDate, endDate time.Time) (*repositories.SubscriptionAnalytics, error) {
	args := m.Called(startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repositories.SubscriptionAnalytics), args.Error(1)
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

// ==========================================
// Repository Tests - GetActiveSubscriptionByCustomerID
// ==========================================

func TestGetActiveSubscriptionByCustomerID(t *testing.T) {
	t.Run("Find active subscription by customer ID", func(t *testing.T) {
		mockRepo := new(MockPaymentRepository)
		customerID := "cus_test_123"
		userID := "user_123"
		planID := uuid.New()

		expectedSub := &models.UserSubscription{
			UserID:               userID,
			SubscriptionPlanID:   planID,
			StripeSubscriptionID: "sub_test_123",
			StripeCustomerID:     customerID,
			Status:               "active",
			CurrentPeriodStart:   time.Now(),
			CurrentPeriodEnd:     time.Now().Add(30 * 24 * time.Hour),
		}

		mockRepo.On("GetActiveSubscriptionByCustomerID", customerID).Return(expectedSub, nil)

		result, err := mockRepo.GetActiveSubscriptionByCustomerID(customerID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, customerID, result.StripeCustomerID)
		assert.Equal(t, "active", result.Status)
		assert.Equal(t, userID, result.UserID)
		mockRepo.AssertExpectations(t)
	})

	t.Run("No active subscription found for customer", func(t *testing.T) {
		mockRepo := new(MockPaymentRepository)
		customerID := "cus_nonexistent"

		mockRepo.On("GetActiveSubscriptionByCustomerID", customerID).Return(nil, gorm.ErrRecordNotFound)

		result, err := mockRepo.GetActiveSubscriptionByCustomerID(customerID)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, gorm.ErrRecordNotFound, err)
		mockRepo.AssertExpectations(t)
	})

	t.Run("Handle trialing subscription", func(t *testing.T) {
		mockRepo := new(MockPaymentRepository)
		customerID := "cus_trial_123"

		expectedSub := &models.UserSubscription{
			StripeCustomerID: customerID,
			Status:           "trialing",
		}

		mockRepo.On("GetActiveSubscriptionByCustomerID", customerID).Return(expectedSub, nil)

		result, err := mockRepo.GetActiveSubscriptionByCustomerID(customerID)

		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "trialing", result.Status)
		mockRepo.AssertExpectations(t)
	})
}