// tests/payment/shared_mocks.go
package payment_tests

import (
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

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

// SharedMockStripeService - Mock partagé pour StripeService
type SharedMockStripeService struct {
	mock.Mock
}

func (m *SharedMockStripeService) CreateSubscriptionPlanInStripe(plan any) error {
	args := m.Called(plan)
	return args.Error(0)
}
