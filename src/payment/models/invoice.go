package models

import (
	"time"

	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

// Invoice represents an invoice. It is a shared table: a row is owned EITHER by
// an individual user (UserID + UserSubscriptionID set) OR by an organization
// (OrganizationID + OrganizationSubscriptionID set). On an organization invoice
// UserID is empty and UserSubscriptionID is NULL (the user-subscription foreign
// key is exempt for NULL, so org rows persist without a matching user sub).
type Invoice struct {
	entityManagementModels.BaseModel
	UserID             string           `gorm:"type:varchar(100);not null;index" json:"user_id"`
	UserSubscriptionID *uuid.UUID       `json:"user_subscription_id"`
	UserSubscription   UserSubscription `gorm:"foreignKey:UserSubscriptionID" json:"user_subscription"`
	// Organization ownership (nullable): set when the invoice is billed to an
	// organization subscription rather than an individual user.
	OrganizationID             *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	OrganizationSubscriptionID *uuid.UUID `gorm:"type:uuid;index" json:"organization_subscription_id,omitempty"`
	StripeInvoiceID            string     `gorm:"type:varchar(100);uniqueIndex" json:"stripe_invoice_id"`
	Amount                     int64      `json:"amount"`          // Montant en centimes
	AmountRefunded             int64      `json:"amount_refunded"` // Montant remboursé en centimes (refunds + credit notes)
	Currency                   string     `gorm:"type:varchar(3)" json:"currency"`
	Status                     string     `gorm:"type:varchar(50)" json:"status"` // paid, open, void, uncollectible, refunded, partially_refunded
	InvoiceNumber              string     `gorm:"type:varchar(100)" json:"invoice_number"`
	InvoiceDate                time.Time  `json:"invoice_date"`
	DueDate                    time.Time  `json:"due_date"`
	PaidAt                     *time.Time `json:"paid_at,omitempty"`
	StripeHostedURL            string     `gorm:"type:varchar(500)" json:"stripe_hosted_url"`
	DownloadURL                string     `gorm:"type:varchar(500)" json:"download_url"`
}

func (i Invoice) GetBaseModel() entityManagementModels.BaseModel {
	return i.BaseModel
}

func (i Invoice) GetReferenceObject() string {
	return "Invoice"
}
