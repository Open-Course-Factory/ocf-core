package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

// BillingAddress represents a billing address
type BillingAddress struct {
	entityManagementModels.BaseModel
	UserID     string `gorm:"type:varchar(100);not null;index" json:"user_id"`
	Line1      string `gorm:"type:varchar(255)" json:"line1"`
	Line2      string `gorm:"type:varchar(255)" json:"line2,omitempty"`
	City       string `gorm:"type:varchar(100)" json:"city"`
	State      string `gorm:"type:varchar(100)" json:"state,omitempty"`
	PostalCode string `gorm:"type:varchar(20)" json:"postal_code"`
	Country    string `gorm:"type:varchar(2)" json:"country"` // Code ISO 2 lettres
	// B2B facturation fields (issue #383): optional, empty for B2C buyers.
	CompanyName string `gorm:"type:varchar(255)" json:"company_name,omitempty"` // Raison sociale
	Siret       string `gorm:"type:varchar(14)" json:"siret,omitempty"`         // SIRET (14 chiffres)
	VatNumber   string `gorm:"type:varchar(20)" json:"vat_number,omitempty"`    // N° TVA intracommunautaire
	IsDefault   bool   `gorm:"default:false" json:"is_default"`
}

func (b BillingAddress) GetBaseModel() entityManagementModels.BaseModel {
	return b.BaseModel
}

func (b BillingAddress) GetReferenceObject() string {
	return "BillingAddress"
}
