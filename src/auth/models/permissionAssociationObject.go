package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionAssociationObject struct {
	gorm.Model
	ID          uuid.UUID `json:"id" gorm:"primary_key"`
	SubObjectID uuid.UUID `json:"sub_object_id"`
	SubType     string    `json:"sub_type"`
}

func (p *PermissionAssociationObject) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}

	return
}
