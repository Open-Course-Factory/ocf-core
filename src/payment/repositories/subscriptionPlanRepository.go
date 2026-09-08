// src/payment/repositories/subscriptionPlanRepository.go
package repositories

import (
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionPlanRepository struct {
	db *gorm.DB
}

func NewSubscriptionPlanRepository(db *gorm.DB) *SubscriptionPlanRepository {
	return &SubscriptionPlanRepository{
		db: db,
	}
}

func (r *SubscriptionPlanRepository) GetByID(id uuid.UUID) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := r.db.Where("id = ?", id).First(&plan).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (r *SubscriptionPlanRepository) GetAll(activeOnly bool) (*[]models.SubscriptionPlan, error) {
	var plans []models.SubscriptionPlan
	query := r.db.Model(&models.SubscriptionPlan{})

	if activeOnly {
		query = query.Where("is_active = ?", true)
	}

	err := query.Find(&plans).Error
	if err != nil {
		return nil, err
	}
	return &plans, nil
}

func (r *SubscriptionPlanRepository) GetByStripePriceID(stripePriceID string) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := r.db.Where("stripe_price_id = ?", stripePriceID).First(&plan).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}
