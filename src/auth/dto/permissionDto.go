package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type PermissionOutput struct {
	ID           uuid.UUID `json:"id"`
	User         uuid.UUID `json:"userId"`
	Role         uuid.UUID `json:"roleId"`
	Group        uuid.UUID `json:"groupId"`
	Organisation uuid.UUID `json:"organisationId"`
	PermissionTypes []PermissionType `json:"permissionTypes"`
}

type CreatePermissionInput struct {
	User         uuid.UUID `binding:"required"`
	Role         uuid.UUID `binding:"required"`
	Group        uuid.UUID `binding:"required"`
	Organisation uuid.UUID `binding:"required"`
	PermissionTypes []PermissionType `binding:"required"`
}

type PermissionEditInput struct {
	User         uuid.UUID `json:"userId"`
	Role         uuid.UUID `json:"roleId"`
	Group        uuid.UUID `json:"groupId"`
	Organisation uuid.UUID `json:"organisationId"`
	PermissionTypes []PermissionType `json:"permissionTypes"`
}

type PermissionEditOutput struct {
	User         uuid.UUID `json:"userId"`
	Role         uuid.UUID `json:"roleId"`
	Group        uuid.UUID `json:"groupId"`
	Organisation uuid.UUID `json:"organisationId"`
	PermissionTypes []PermissionType `json:"permissionTypes"`
}

func PermissionModelToPermissionOutput(permissionModel models.Permission) *PermissionOutput {
	return &PermissionOutput{
		ID:           permissionModel.ID,
		User:         permissionModel.User.ID,
		Role:         permissionModel.Role.ID,
		Group:        permissionModel.Group.ID,
		Organisation: permissionModel.Organisation.ID,
		PermissionTypes: permissionModel.PermissionTypes,
	}
}
