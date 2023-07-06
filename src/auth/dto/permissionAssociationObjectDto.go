package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type PermissionAssociationObjectOutput struct {
	ID          uuid.UUID `json:"id"`
	SubObjectID uuid.UUID `json:"sub_object_id"`
	SubType     string    `json:"sub_type"`
}

type PermissionAssociationObjectInput struct {
	SubObjectID uuid.UUID `json:"sub_object_id"`
	SubType     string    `json:"sub_type"`
}

func PermissionAssociationObjectModelToPermissionAssociationObjectOutput(permissionAssociationObjectModel models.PermissionAssociationObject) *PermissionAssociationObjectOutput {
	return &PermissionAssociationObjectOutput{
		ID:          permissionAssociationObjectModel.ID,
		SubObjectID: permissionAssociationObjectModel.SubObjectID,
		SubType:     permissionAssociationObjectModel.SubType,
	}
}
