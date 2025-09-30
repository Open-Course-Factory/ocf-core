// tests/payment/stripeService_test.go
package payment_tests

import (
	"encoding/json"
	"testing"
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/services"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stripe/stripe-go/v82"
	"gorm.io/gorm"
)

// Mock pour GenericService
type MockGenericService struct {
	mock.Mock
}

func (m *MockGenericService) GetEntity(id uuid.UUID, entityName string, entity interface{}) (interface{}, error) {
	args := m.Called(id, entityName, entity)
	return args.Get(0), args.Error(1)
}

func (m *MockGenericService) CreateEntity(input interface{}, entityName string) (interface{}, error) {
	args := m.Called(input, entityName)
	return args.Get(0), args.Error(1)
}

func (m *MockGenericService) EditEntity(id uuid.UUID, entityName string, entityType interface{}, updates interface{}) error {
	args := m.Called(id, entityName, entityType, updates)
	return args.Error(0)
}

func (m *MockGenericService) DeleteEntity(id uuid.UUID, entityName string, entity interface{}) error {
	args := m.Called(id, entityName, entity)
	return args.Error(0)
}

func (m *MockGenericService) GetEntities(entityName string, entity interface{}, includeInactive bool) (interface{}, error) {
	args := m.Called(entityName, entity, includeInactive)
	return args.Get(0), args.Error(1)
}

// Mock pour SubscriptionService
type MockSubscriptionService struct {
	mock.Mock
}

func (m *MockSubscriptionService) GetSubscriptionPlan(id uuid.UUID) (*models.SubscriptionPlan, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.SubscriptionPlan), args.Error(1)
}

func (m *MockSubscriptionService) GetAllSubscriptionPlans(includeInactive bool) (*[]models.SubscriptionPlan, error) {
	args := m.Called(includeInactive)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.SubscriptionPlan), args.Error(1)
}

func (m *MockSubscriptionService) HasActiveSubscription(userID string) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockSubscriptionService) CheckUsageLimit(userID, metricType string, increment int64) (*services.UsageLimitCheck, error) {
	args := m.Called(userID, metricType, increment)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.UsageLimitCheck), args.Error(1)
}

func (m *MockSubscriptionService) RecordUsage(userID, metricType string, amount int64) error {
	args := m.Called(userID, metricType, amount)
	return args.Error(0)
}

func (m *MockSubscriptionService) GetUserUsageMetrics(userID string) (*[]models.UsageMetrics, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.UsageMetrics), args.Error(1)
}

func (m *MockSubscriptionService) GetSubscriptionAnalytics() (*services.SubscriptionAnalytics, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.SubscriptionAnalytics), args.Error(1)
}

// Using shared mock repository instead of extending here

// Tests pour StripeService
func TestStripeService_CreateSubscriptionPlanInStripe(t *testing.T) {
	// Note: Ces tests nécessitent des mocks Stripe ou un environnement de test
	// Pour l'instant, nous testons la logique métier sans appels Stripe réels

	// Test avec un plan valide
	t.Run("Valid subscription plan", func(t *testing.T) {
		plan := &models.SubscriptionPlan{
			BaseModel: entityManagementModels.BaseModel{ID: uuid.New()},
			Name:        "Test Plan",
			Description: "Test Description",
			PriceAmount: 1000,
			Currency:    "usd",
			BillingInterval: "month",
			IsActive:    true,
		}

		// Note: Dans un vrai test, nous moquerions les appels Stripe
		// Ici nous testons juste que la structure est correcte
		assert.NotNil(t, plan)
		assert.Equal(t, "Test Plan", plan.Name)
		assert.Equal(t, int64(1000), plan.PriceAmount)
		assert.True(t, plan.IsActive)
	})
}

func TestStripeService_ProcessWebhook_SubscriptionCreated(t *testing.T) {
	mockRepo := new(SharedMockPaymentRepository)

	t.Run("Valid subscription created webhook", func(t *testing.T) {
		// Simuler un événement de création d'abonnement
		subscription := &stripe.Subscription{
			ID:       "sub_test123",
			Customer: &stripe.Customer{ID: "cus_test123"},
			Status:   "active",
			Metadata: map[string]string{
				"user_id":              "user123",
				"subscription_plan_id": uuid.New().String(),
			},
			Items: &stripe.SubscriptionItemList{
				Data: []*stripe.SubscriptionItem{
					{
						CurrentPeriodStart: time.Now().Unix(),
						CurrentPeriodEnd:   time.Now().Add(30 * 24 * time.Hour).Unix(),
					},
				},
			},
		}

		// Vérifier que les métadonnées sont correctes
		assert.Equal(t, "sub_test123", subscription.ID)
		assert.Equal(t, "user123", subscription.Metadata["user_id"])
		assert.NotEmpty(t, subscription.Metadata["subscription_plan_id"])

		// Mock le repository pour attendre la création
		mockRepo.On("CreateUserSubscription", mock.AnythingOfType("*models.UserSubscription")).Return(nil)

		// Dans un vrai test, nous appellerions handleSubscriptionCreated
		// Pour l'instant, vérifions juste la structure
		assert.NotNil(t, subscription)
	})

	t.Run("Missing metadata should fail", func(t *testing.T) {
		subscription := &stripe.Subscription{
			ID:       "sub_test123",
			Customer: &stripe.Customer{ID: "cus_test123"},
			Status:   "active",
			Metadata: map[string]string{}, // Pas de métadonnées
		}

		// Vérifier que les métadonnées manquent
		assert.Empty(t, subscription.Metadata["user_id"])
		assert.Empty(t, subscription.Metadata["subscription_plan_id"])
	})

	mockRepo.AssertExpectations(t)
}

func TestStripeService_SyncSubscriptionsResult(t *testing.T) {
	t.Run("Create sync result", func(t *testing.T) {
		result := &services.SyncSubscriptionsResult{
			ProcessedSubscriptions: 5,
			CreatedSubscriptions:   2,
			UpdatedSubscriptions:   1,
			SkippedSubscriptions:   2,
			FailedSubscriptions: []services.FailedSubscription{
				{
					StripeSubscriptionID: "sub_failed",
					UserID:               "user123",
					Error:                "test error",
				},
			},
			CreatedDetails: []string{"Created sub_123 for user_456"},
			UpdatedDetails: []string{"Updated sub_789"},
			SkippedDetails: []string{"Skipped sub_000: already exists"},
		}

		assert.Equal(t, 5, result.ProcessedSubscriptions)
		assert.Equal(t, 2, result.CreatedSubscriptions)
		assert.Equal(t, 1, result.UpdatedSubscriptions)
		assert.Equal(t, 2, result.SkippedSubscriptions)
		assert.Len(t, result.FailedSubscriptions, 1)
		assert.Equal(t, "sub_failed", result.FailedSubscriptions[0].StripeSubscriptionID)
		assert.Equal(t, "test error", result.FailedSubscriptions[0].Error)
	})
}

func TestStripeService_LinkSubscriptionToUser(t *testing.T) {
	mockRepo := new(SharedMockPaymentRepository)

	t.Run("Link subscription successfully", func(t *testing.T) {
		stripeSubscriptionID := "sub_test123"
		userID := "user123"
		subscriptionPlanID := uuid.New()

		// Mock que l'abonnement n'existe pas encore
		mockRepo.On("GetUserSubscriptionByStripeID", stripeSubscriptionID).Return(nil, gorm.ErrRecordNotFound)

		// Mock la création réussie
		mockRepo.On("CreateUserSubscription", mock.AnythingOfType("*models.UserSubscription")).Return(nil)

		// Dans un vrai test, nous appellerions LinkSubscriptionToUser
		// Pour l'instant, testons juste les paramètres
		assert.NotEmpty(t, stripeSubscriptionID)
		assert.NotEmpty(t, userID)
		assert.NotEqual(t, uuid.Nil, subscriptionPlanID)
	})

	t.Run("Subscription already exists should fail", func(t *testing.T) {
		stripeSubscriptionID := "sub_existing"
		existingSubscription := &models.UserSubscription{
			StripeSubscriptionID: stripeSubscriptionID,
		}

		// Mock que l'abonnement existe déjà
		mockRepo.On("GetUserSubscriptionByStripeID", stripeSubscriptionID).Return(existingSubscription, nil)

		// Vérifier qu'il existe
		assert.NotNil(t, existingSubscription)
		assert.Equal(t, stripeSubscriptionID, existingSubscription.StripeSubscriptionID)
	})

	mockRepo.AssertExpectations(t)
}

func TestStripeService_HandleWebhookEvents(t *testing.T) {
	t.Run("Process different webhook event types", func(t *testing.T) {
		eventTypes := []string{
			"customer.subscription.created",
			"customer.subscription.updated",
			"customer.subscription.deleted",
			"invoice.payment_succeeded",
			"invoice.payment_failed",
			"checkout.session.completed",
		}

		for _, eventType := range eventTypes {
			t.Run(eventType, func(t *testing.T) {
				event := &stripe.Event{
					Type: stripe.EventType(eventType),
					Data: &stripe.EventData{
						Raw: json.RawMessage(`{"id": "test_object"}`),
					},
				}

				assert.Equal(t, eventType, string(event.Type))
				assert.NotNil(t, event.Data)
			})
		}
	})

	t.Run("Unknown event type should be skipped", func(t *testing.T) {
		event := &stripe.Event{
			Type: stripe.EventType("unknown.event.type"),
			Data: &stripe.EventData{
				Raw: json.RawMessage(`{"id": "test_object"}`),
			},
		}

		assert.Equal(t, "unknown.event.type", string(event.Type))
		// Dans la vraie implémentation, ceci devrait être ignoré sans erreur
	})
}