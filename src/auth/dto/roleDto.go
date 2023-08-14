package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type RoleOutput struct {
	ID       uuid.UUID       `json:"id"`
	RoleName models.RoleType `json:"roleName"`
}

type CreateRoleInput struct {
	RoleName models.RoleType `binding:"required"`
}

type RoleEditInput struct {
	RoleName models.RoleType `json:"roleName"`
}

type RoleEditOutput struct {
	RoleName models.RoleType `json:"roleName"`
}

func RoleModelToRoleOutput(roleModel models.Role) *RoleOutput {
	return &RoleOutput{
		ID:       roleModel.ID,
		RoleName: roleModel.RoleName,
	}
}
