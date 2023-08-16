package dto

import (
	"soli/formations/src/auth/models"
	"soli/formations/src/auth/types"

	"github.com/google/uuid"
)

type RoleOutput struct {
	ID          uuid.UUID          `json:"id"`
	RoleName    models.RoleType    `json:"roleName"`
	Permissions []types.Permission `json:"permissions"`
}

type CreateRoleInput struct {
	RoleName    models.RoleType    `binding:"required"`
	Permissions []types.Permission `binding:"required"`
}

type RoleEditInput struct {
	RoleName    models.RoleType    `json:"roleName"`
	Permissions []types.Permission `json:"permissions"`
}

type RoleEditOutput struct {
	RoleName    models.RoleType    `json:"roleName"`
	Permissions []types.Permission `json:"permissions"`
}

func RoleModelToRoleOutput(roleModel models.Role) *RoleOutput {
	return &RoleOutput{
		ID:          roleModel.ID,
		RoleName:    roleModel.RoleName,
		Permissions: roleModel.Permissions,
	}
}
