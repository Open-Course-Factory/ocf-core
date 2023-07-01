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
}

type CreatePermissionInput struct {
	User         uuid.UUID `binding:"required"`
	Role         uuid.UUID `binding:"required"`
	Group        uuid.UUID `binding:"required"`
	Organisation uuid.UUID `binding:"required"`
}

type PermissionEditInput struct {
	User         uuid.UUID `json:"userId"`
	Role         uuid.UUID `json:"roleId"`
	Group        uuid.UUID `json:"groupId"`
	Organisation uuid.UUID `json:"organisationId"`
}

type PermissionEditOutput struct {
	User         uuid.UUID `json:"userId"`
	Role         uuid.UUID `json:"roleId"`
	Group        uuid.UUID `json:"groupId"`
	Organisation uuid.UUID `json:"organisationId"`
}

func PermissionModelToPermissionOutput(permissionModel models.Permission) *PermissionOutput {
	return &PermissionOutput{
		ID:           permissionModel.ID,
		User:         permissionModel.User.ID,
		Role:         permissionModel.Role.ID,
		Group:        permissionModel.Group.ID,
		Organisation: permissionModel.Organisation.ID,
	}
}
