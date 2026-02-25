package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// Invoice represents an invoice
type Invoice struct {
	entityManagementModels.BaseModel
	UserID             string           `gorm:"type:varchar(100);not null;index" json:"user_id"`
	UserSubscriptionID uuid.UUID        `gorm:"not null" json:"user_subscription_id"`
	UserSubscription   UserSubscription `gorm:"foreignKey:UserSubscriptionID" json:"user_subscription"`
	StripeInvoiceID    string           `gorm:"type:varchar(100);uniqueIndex" json:"stripe_invoice_id"`
	Amount             int64            `json:"amount"` // Montant en centimes
	Currency           string           `gorm:"type:varchar(3)" json:"currency"`
	Status             string           `gorm:"type:varchar(50)" json:"status"` // paid, open, void, uncollectible
	InvoiceNumber      string           `gorm:"type:varchar(100)" json:"invoice_number"`
	InvoiceDate        time.Time        `json:"invoice_date"`
	DueDate            time.Time        `json:"due_date"`
	PaidAt             *time.Time       `json:"paid_at,omitempty"`
	StripeHostedURL    string           `gorm:"type:varchar(500)" json:"stripe_hosted_url"`
	DownloadURL        string           `gorm:"type:varchar(500)" json:"download_url"`
}

func (i Invoice) GetBaseModel() entityManagementModels.BaseModel {
	return i.BaseModel
}

func (i Invoice) GetReferenceObject() string {
	return "Invoice"
}
