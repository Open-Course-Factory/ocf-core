// tests/payment/shared_mocks.go
package payment_tests

import (
	"time"

	"soli/formations/src/payment/models"
	"soli/formations/src/payment/repositories"
	"soli/formations/src/payment/services"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stripe/stripe-go/v82"
)

// SharedMockPaymentRepository - Mock partagé pour éviter les duplications
type SharedMockPaymentRepository struct {
	mock.Mock
}

// UserSubscription methods
func (m *SharedMockPaymentRepository) GetActiveUserSubscription(userID string) (*models.UserSubscription, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *SharedMockPaymentRepository) GetUserSubscription(id uuid.UUID) (*models.UserSubscription, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *SharedMockPaymentRepository) CreateUserSubscription(subscription *models.UserSubscription) error {
	args := m.Called(subscription)
	return args.Error(0)
}

func (m *SharedMockPaymentRepository) GetUserSubscriptionByStripeID(stripeSubscriptionID string) (*models.UserSubscription, error) {
	args := m.Called(stripeSubscriptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UserSubscription), args.Error(1)
}

func (m *SharedMockPaymentRepository) UpdateUserSubscription(subscription *models.UserSubscription) error {
	args := m.Called(subscription)
	return args.Error(0)
}

func (m *SharedMockPaymentRepository) GetUserSubscriptions(userID string, includeInactive bool) (*[]models.UserSubscription, error) {
	args := m.Called(userID, includeInactive)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.UserSubscription), args.Error(1)
}

// UsageMetrics methods
func (m *SharedMockPaymentRepository) GetUserUsageMetrics(userID, metricType string) (*models.UsageMetrics, error) {
	args := m.Called(userID, metricType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.UsageMetrics), args.Error(1)
}

func (m *SharedMockPaymentRepository) CreateUsageMetrics(metrics *models.UsageMetrics) error {
	args := m.Called(metrics)
	return args.Error(0)
}

func (m *SharedMockPaymentRepository) UpdateUsageMetrics(metrics *models.UsageMetrics) error {
	args := m.Called(metrics)
	return args.Error(0)
}

func (m *SharedMockPaymentRepository) GetAllUserUsageMetrics(userID string) (*[]models.UsageMetrics, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.UsageMetrics), args.Error(1)
}

func (m *SharedMockPaymentRepository) IncrementUsageMetric(userID, metricType string, increment int64) error {
	args := m.Called(userID, metricType, increment)
	return args.Error(0)
}

func (m *SharedMockPaymentRepository) ResetUsageMetrics(userID string, periodStart, periodEnd time.Time) error {
	args := m.Called(userID, periodStart, periodEnd)
	return args.Error(0)
}

// Analytics methods
func (m *SharedMockPaymentRepository) GetSubscriptionAnalytics(startDate, endDate time.Time) (*repositories.SubscriptionAnalytics, error) {
	args := m.Called(startDate, endDate)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repositories.SubscriptionAnalytics), args.Error(1)
}

func (m *SharedMockPaymentRepository) GetRevenueByPeriod(startDate, endDate time.Time, interval string) (*[]repositories.RevenueByPeriod, error) {
	args := m.Called(startDate, endDate, interval)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]repositories.RevenueByPeriod), args.Error(1)
}

// Cleanup methods
func (m *SharedMockPaymentRepository) CleanupExpiredSubscriptions() error {
	args := m.Called()
	return args.Error(0)
}

func (m *SharedMockPaymentRepository) ArchiveOldInvoices(daysOld int) error {
	args := m.Called(daysOld)
	return args.Error(0)
}

// SharedMockGenericService - Mock partagé pour GenericService
type SharedMockGenericService struct {
	mock.Mock
}

func (m *SharedMockGenericService) GetEntity(id uuid.UUID, entityName string, entity interface{}) (interface{}, error) {
	args := m.Called(id, entityName, entity)
	return args.Get(0), args.Error(1)
}

func (m *SharedMockGenericService) CreateEntity(input interface{}, entityName string) (interface{}, error) {
	args := m.Called(input, entityName)
	return args.Get(0), args.Error(1)
}

func (m *SharedMockGenericService) EditEntity(id uuid.UUID, entityName string, entityType interface{}, updates interface{}) error {
	args := m.Called(id, entityName, entityType, updates)
	return args.Error(0)
}

func (m *SharedMockGenericService) DeleteEntity(id uuid.UUID, entityName string, entity interface{}) error {
	args := m.Called(id, entityName, entity)
	return args.Error(0)
}

func (m *SharedMockGenericService) GetEntities(entityName string, entity interface{}, includeInactive bool) (interface{}, error) {
	args := m.Called(entityName, entity, includeInactive)
	return args.Get(0), args.Error(1)
}

// SharedMockSubscriptionService - Mock partagé pour SubscriptionService
type SharedMockSubscriptionService struct {
	mock.Mock
}

func (m *SharedMockSubscriptionService) GetSubscriptionPlan(id uuid.UUID) (*models.SubscriptionPlan, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.SubscriptionPlan), args.Error(1)
}

func (m *SharedMockSubscriptionService) GetAllSubscriptionPlans(includeInactive bool) (*[]models.SubscriptionPlan, error) {
	args := m.Called(includeInactive)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.SubscriptionPlan), args.Error(1)
}

func (m *SharedMockSubscriptionService) HasActiveSubscription(userID string) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

func (m *SharedMockSubscriptionService) CheckUsageLimit(userID, metricType string, increment int64) (*services.UsageLimitCheck, error) {
	args := m.Called(userID, metricType, increment)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.UsageLimitCheck), args.Error(1)
}

func (m *SharedMockSubscriptionService) RecordUsage(userID, metricType string, amount int64) error {
	args := m.Called(userID, metricType, amount)
	return args.Error(0)
}

func (m *SharedMockSubscriptionService) GetUserUsageMetrics(userID string) (*[]models.UsageMetrics, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*[]models.UsageMetrics), args.Error(1)
}

func (m *SharedMockSubscriptionService) GetSubscriptionAnalytics() (*services.SubscriptionAnalytics, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.SubscriptionAnalytics), args.Error(1)
}

// SharedMockStripeService - Mock partagé pour StripeService
type SharedMockStripeService struct {
	mock.Mock
}

func (m *SharedMockStripeService) ValidateWebhookSignature(payload []byte, signature string) (*stripe.Event, error) {
	args := m.Called(payload, signature)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*stripe.Event), args.Error(1)
}

func (m *SharedMockStripeService) ProcessWebhook(payload []byte, signature string) error {
	args := m.Called(payload, signature)
	return args.Error(0)
}

func (m *SharedMockStripeService) CreateOrGetCustomer(userID, email, name string) (string, error) {
	args := m.Called(userID, email, name)
	return args.String(0), args.Error(1)
}

func (m *SharedMockStripeService) UpdateCustomer(customerID string, params *stripe.CustomerParams) error {
	args := m.Called(customerID, params)
	return args.Error(0)
}

func (m *SharedMockStripeService) CreateCheckoutSession(userID string, input interface{}) (interface{}, error) {
	args := m.Called(userID, input)
	return args.Get(0), args.Error(1)
}

func (m *SharedMockStripeService) CreatePortalSession(userID string, input interface{}) (interface{}, error) {
	args := m.Called(userID, input)
	return args.Get(0), args.Error(1)
}

func (m *SharedMockStripeService) CreateSubscriptionPlanInStripe(plan interface{}) error {
	args := m.Called(plan)
	return args.Error(0)
}

func (m *SharedMockStripeService) UpdateSubscriptionPlanInStripe(plan interface{}) error {
	args := m.Called(plan)
	return args.Error(0)
}

func (m *SharedMockStripeService) CancelSubscription(subscriptionID string, cancelAtPeriodEnd bool) error {
	args := m.Called(subscriptionID, cancelAtPeriodEnd)
	return args.Error(0)
}

func (m *SharedMockStripeService) ReactivateSubscription(subscriptionID string) error {
	args := m.Called(subscriptionID)
	return args.Error(0)
}

func (m *SharedMockStripeService) SyncExistingSubscriptions() (*services.SyncSubscriptionsResult, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.SyncSubscriptionsResult), args.Error(1)
}

func (m *SharedMockStripeService) SyncUserSubscriptions(userID string) (*services.SyncSubscriptionsResult, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.SyncSubscriptionsResult), args.Error(1)
}

func (m *SharedMockStripeService) SyncSubscriptionsWithMissingMetadata() (*services.SyncSubscriptionsResult, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.SyncSubscriptionsResult), args.Error(1)
}

func (m *SharedMockStripeService) LinkSubscriptionToUser(stripeSubscriptionID, userID string, subscriptionPlanID interface{}) error {
	args := m.Called(stripeSubscriptionID, userID, subscriptionPlanID)
	return args.Error(0)
}

func (m *SharedMockStripeService) AttachPaymentMethod(paymentMethodID, customerID string) error {
	args := m.Called(paymentMethodID, customerID)
	return args.Error(0)
}

func (m *SharedMockStripeService) DetachPaymentMethod(paymentMethodID string) error {
	args := m.Called(paymentMethodID)
	return args.Error(0)
}

func (m *SharedMockStripeService) SetDefaultPaymentMethod(customerID, paymentMethodID string) error {
	args := m.Called(customerID, paymentMethodID)
	return args.Error(0)
}

func (m *SharedMockStripeService) GetInvoice(invoiceID string) (*stripe.Invoice, error) {
	args := m.Called(invoiceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*stripe.Invoice), args.Error(1)
}

func (m *SharedMockStripeService) SendInvoice(invoiceID string) error {
	args := m.Called(invoiceID)
	return args.Error(0)
}