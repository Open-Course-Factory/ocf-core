package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type RoleOutput struct {
	ID       uuid.UUID `json:"id"`
	RoleName string    `json:"roleName"`
}

type CreateRoleInput struct {
	RoleName string `binding:"required"`
}

type RoleEditInput struct {
	RoleName string `json:"roleName"`
}

type RoleEditOutput struct {
	RoleName string `json:"roleName"`
}

func RoleModelToRoleOutput(roleModel models.Role) *RoleOutput {
	return &RoleOutput{
		ID:       roleModel.ID,
		RoleName: roleModel.RoleName,
	}
}
