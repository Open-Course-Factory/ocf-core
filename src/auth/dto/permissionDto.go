package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type PermissionOutput struct {
	ID              uuid.UUID               `json:"id"`
	PermissionTypes []models.PermissionType `json:"permissionTypes"`
}

type CreatePermissionInput struct {
	PermissionTypes []models.PermissionType `binding:"required"`
}

type PermissionEditInput struct {
	PermissionTypes []models.PermissionType `json:"permissionTypes"`
}

type PermissionEditOutput struct {
	PermissionTypes []models.PermissionType `json:"permissionTypes"`
}

func PermissionModelToPermissionOutput(permissionModel models.Permission) *PermissionOutput {
	return &PermissionOutput{
		ID:              permissionModel.ID,
		PermissionTypes: permissionModel.PermissionTypes,
	}
}
