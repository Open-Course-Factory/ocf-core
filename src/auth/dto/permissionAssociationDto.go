package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type PermissionAssociationOutput struct {
	ID                           uuid.UUID                           `json:"id"`
	Permission                   PermissionOutput                    `json:"permission"`
	PermissionAssociationObjects []PermissionAssociationObjectOutput `json:"permissionAssociationObjects"`
}

type PermissionAssociationInput struct {
	PermissionID                 string                             `binding:"required"`
	PermissionAssociationObjects []PermissionAssociationObjectInput `binding:"required"`
}

func PermissionAssociationModelToPermissionAssociationOutput(permissionAssociationModel models.PermissionAssociation) *PermissionAssociationOutput {

	var PermissionAssociationObjectOutputs []PermissionAssociationObjectOutput

	for _, permissionAssociationObjectModel := range permissionAssociationModel.PermissionAssociationObjects {
		permissionAssociationObjectOutput := PermissionAssociationObjectModelToPermissionAssociationObjectOutput(permissionAssociationObjectModel)
		PermissionAssociationObjectOutputs = append(PermissionAssociationObjectOutputs, *permissionAssociationObjectOutput)
	}

	return &PermissionAssociationOutput{
		ID:                           permissionAssociationModel.ID,
		Permission:                   *PermissionModelToPermissionOutput(permissionAssociationModel.Permission),
		PermissionAssociationObjects: PermissionAssociationObjectOutputs,
	}
}
