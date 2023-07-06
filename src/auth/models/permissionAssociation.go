package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PermissionAssociation struct {
	gorm.Model
	ID                           uuid.UUID                     `json:"id" gorm:"primary_key"`
	Permission                   Permission                    `json:"permission"`
	PermissionID                 uuid.UUID                     `json:"permission_id"`
	PermissionAssociationObjects []PermissionAssociationObject `gorm:"many2many:pa_pao;" json:"permission_association_objects"`
}

func (p *PermissionAssociation) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}

	return
}
