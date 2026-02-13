// src/payment/repositories/subscriptionBatchRepository.go
package repositories

import (
	"soli/formations/src/payment/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SubscriptionBatchRepository interface {
	Create(batch *models.SubscriptionBatch) error
	GetByID(id uuid.UUID) (*models.SubscriptionBatch, error)
	GetByStripeSubscriptionID(stripeSubID string) (*models.SubscriptionBatch, error)
	GetByPurchaser(purchaserUserID string) (*[]models.SubscriptionBatch, error)
	GetByGroup(groupID uuid.UUID) (*[]models.SubscriptionBatch, error)
	GetAccessibleByUser(userID string) (*[]models.SubscriptionBatch, error)
	Update(batch *models.SubscriptionBatch) error
	IncrementAssignedQuantity(batchID uuid.UUID, increment int) error
	DecrementAssignedQuantity(batchID uuid.UUID, decrement int) error
}

type subscriptionBatchRepository struct {
	db *gorm.DB
}

func NewSubscriptionBatchRepository(db *gorm.DB) SubscriptionBatchRepository {
	return &subscriptionBatchRepository{
		db: db,
	}
}

func (r *subscriptionBatchRepository) Create(batch *models.SubscriptionBatch) error {
	return r.db.Create(batch).Error
}

func (r *subscriptionBatchRepository) GetByID(id uuid.UUID) (*models.SubscriptionBatch, error) {
	var batch models.SubscriptionBatch
	err := r.db.Preload("SubscriptionPlan").Where("id = ?", id).First(&batch).Error
	if err != nil {
		return nil, err
	}
	return &batch, nil
}

func (r *subscriptionBatchRepository) GetByStripeSubscriptionID(stripeSubID string) (*models.SubscriptionBatch, error) {
	var batch models.SubscriptionBatch
	err := r.db.Preload("SubscriptionPlan").
		Where("stripe_subscription_id = ?", stripeSubID).
		First(&batch).Error
	if err != nil {
		return nil, err
	}
	return &batch, nil
}

func (r *subscriptionBatchRepository) GetByPurchaser(purchaserUserID string) (*[]models.SubscriptionBatch, error) {
	var batches []models.SubscriptionBatch
	err := r.db.Preload("SubscriptionPlan").
		Where("purchaser_user_id = ?", purchaserUserID).
		Order("created_at DESC").
		Find(&batches).Error
	if err != nil {
		return nil, err
	}
	return &batches, nil
}

func (r *subscriptionBatchRepository) GetByGroup(groupID uuid.UUID) (*[]models.SubscriptionBatch, error) {
	var batches []models.SubscriptionBatch
	err := r.db.Preload("SubscriptionPlan").
		Where("group_id = ?", groupID).
		Order("created_at DESC").
		Find(&batches).Error
	if err != nil {
		return nil, err
	}
	return &batches, nil
}

func (r *subscriptionBatchRepository) Update(batch *models.SubscriptionBatch) error {
	return r.db.Save(batch).Error
}

func (r *subscriptionBatchRepository) IncrementAssignedQuantity(batchID uuid.UUID, increment int) error {
	return r.db.Model(&models.SubscriptionBatch{}).
		Where("id = ?", batchID).
		UpdateColumn("assigned_quantity", gorm.Expr("assigned_quantity + ?", increment)).
		Error
}

func (r *subscriptionBatchRepository) DecrementAssignedQuantity(batchID uuid.UUID, decrement int) error {
	return r.db.Model(&models.SubscriptionBatch{}).
		Where("id = ?", batchID).
		UpdateColumn("assigned_quantity", gorm.Expr("assigned_quantity - ?", decrement)).
		Error
}

// GetAccessibleByUser returns all batches accessible to a user through:
// 1. Direct purchase (user is the purchaser)
// 2. Organization membership (batches purchased by other members of their team organizations)
func (r *subscriptionBatchRepository) GetAccessibleByUser(userID string) (*[]models.SubscriptionBatch, error) {
	var batches []models.SubscriptionBatch

	// Build query to get batches:
	// 1. Where user is the direct purchaser
	// 2. OR where purchaser is a member of user's team organizations
	query := r.db.Preload("SubscriptionPlan").
		Distinct().
		Joins("LEFT JOIN organization_members om1 ON subscription_batches.purchaser_user_id = om1.user_id").
		Joins("LEFT JOIN organizations org ON om1.organization_id = org.id AND org.organization_type = 'team'").
		Joins("LEFT JOIN organization_members om2 ON org.id = om2.organization_id AND om2.user_id = ?", userID).
		Where("subscription_batches.purchaser_user_id = ? OR om2.user_id IS NOT NULL", userID).
		Order("subscription_batches.created_at DESC")

	err := query.Find(&batches).Error
	if err != nil {
		return nil, err
	}

	return &batches, nil
}
