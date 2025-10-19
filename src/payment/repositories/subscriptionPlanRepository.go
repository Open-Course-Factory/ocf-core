// src/payment/repositories/subscriptionPlanRepository.go
package repositories

import (
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionPlanRepository interface {
	GetByID(id uuid.UUID) (*models.SubscriptionPlan, error)
	GetAll(activeOnly bool) (*[]models.SubscriptionPlan, error)
	GetByStripePriceID(stripePriceID string) (*models.SubscriptionPlan, error)
}

type subscriptionPlanRepository struct {
	db *gorm.DB
}

func NewSubscriptionPlanRepository(db *gorm.DB) SubscriptionPlanRepository {
	return &subscriptionPlanRepository{
		db: db,
	}
}

func (r *subscriptionPlanRepository) GetByID(id uuid.UUID) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := r.db.Where("id = ?", id).First(&plan).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}

func (r *subscriptionPlanRepository) GetAll(activeOnly bool) (*[]models.SubscriptionPlan, error) {
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

func (r *subscriptionPlanRepository) GetByStripePriceID(stripePriceID string) (*models.SubscriptionPlan, error) {
	var plan models.SubscriptionPlan
	err := r.db.Where("stripe_price_id = ?", stripePriceID).First(&plan).Error
	if err != nil {
		return nil, err
	}
	return &plan, nil
}
