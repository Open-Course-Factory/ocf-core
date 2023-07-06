package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type PermissionAssociationOutput struct {
	ID                           uuid.UUID                            `json:"id"`
	Permission                   PermissionOutput                     `json:"permission"`
	PermissionAssociationObjects []models.PermissionAssociationObject `json:"permissionAssociationObjects"`
}

type PermissionAssociationInput struct {
	PermissionID                    string   `binding:"required"`
	PermissionAssociationObjectsIDs []string `binding:"required"`
}

func PermissionAssociationModelToPermissionAssociationOutput(permissionAssociationModel models.PermissionAssociation) *PermissionAssociationOutput {
	return &PermissionAssociationOutput{
		ID:                           permissionAssociationModel.ID,
		Permission:                   *PermissionModelToPermissionOutput(permissionAssociationModel.Permission),
		PermissionAssociationObjects: permissionAssociationObjectModel.PermissionAssociationObjects,
	}
}
