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
	Update(batch *models.SubscriptionBatch) error
	Delete(id uuid.UUID) error
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

func (r *subscriptionBatchRepository) Delete(id uuid.UUID) error {
	return r.db.Delete(&models.SubscriptionBatch{}, id).Error
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
