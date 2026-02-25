package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// PaymentMethod represents a payment method
type PaymentMethod struct {
	entityManagementModels.BaseModel
	UserID                string `gorm:"type:varchar(100);not null;index" json:"user_id"`
	StripePaymentMethodID string `gorm:"type:varchar(100);uniqueIndex" json:"stripe_payment_method_id"`
	Type                  string `gorm:"type:varchar(50)" json:"type"` // card, sepa_debit, etc.
	CardBrand             string `gorm:"type:varchar(20)" json:"card_brand,omitempty"`
	CardLast4             string `gorm:"type:varchar(4)" json:"card_last4,omitempty"`
	CardExpMonth          int    `json:"card_exp_month,omitempty"`
	CardExpYear           int    `json:"card_exp_year,omitempty"`
	IsDefault             bool   `gorm:"default:false" json:"is_default"`
	IsActive              bool   `gorm:"default:true" json:"is_active"`
}

func (p PaymentMethod) GetBaseModel() entityManagementModels.BaseModel {
	return p.BaseModel
}

func (p PaymentMethod) GetReferenceObject() string {
	return "PaymentMethod"
}
