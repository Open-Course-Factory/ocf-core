package models

import (
	"github.com/google/uuid"
)

type PermissionAssociation struct {
	BaseModel
	Permission                   Permission                    `json:"permission"`
	PermissionID                 uuid.UUID                     `json:"permission_id"`
	PermissionAssociationObjects []PermissionAssociationObject `gorm:"many2many:pa_pao;" json:"permission_association_objects"`
}
