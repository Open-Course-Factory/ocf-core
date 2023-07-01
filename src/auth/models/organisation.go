package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Organisation struct {
	gorm.Model
	ID               uuid.UUID `gorm:"type:uuid;primarykey"`
	OrganisationName string    `json:"organisationName"`
	Groups           []Group   `json:"groups"`
}

func (o *Organisation) BeforeCreate(tx *gorm.DB) (err error) {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}

	return
}
