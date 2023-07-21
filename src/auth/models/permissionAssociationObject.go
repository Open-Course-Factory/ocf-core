package models

import (
	"github.com/google/uuid"
)

type PermissionAssociationObject struct {
	BaseModel
	SubObjectID           uuid.UUID               `json:"sub_object_id"`
	SubType               string                  `json:"sub_type"`
	PermissionAssociation []PermissionAssociation `gorm:"many2many:pa_pao;" json:"permission_association"`
}
