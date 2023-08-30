package dto

import (
	"soli/formations/src/auth/models"

	"github.com/google/uuid"
)

type UserRoleObjectAssociationOutput struct {
	ID          uuid.UUID `json:"id"`
	RoleID      uuid.UUID `json:"role_id"`
	UserID      uuid.UUID `json:"user_id"`
	SubObjectID uuid.UUID `json:"sub_object_id"`
	SubType     string    `json:"sub_type"`
}

type UserRoleObjectAssociationInput struct {
	RoleId      uuid.UUID `binding:"required"`
	UserId      uuid.UUID `binding:"required"`
	SubObjectID uuid.UUID
	SubType     string
}

func RolePermissionAssociationObjectModelToRolePermissionAssociationObjectOutput(userRoleObjectAssociationModel models.UserRoles) *UserRoleObjectAssociationOutput {
	return &UserRoleObjectAssociationOutput{
		ID:          userRoleObjectAssociationModel.ID,
		RoleID:      userRoleObjectAssociationModel.Role.ID,
		UserID:      userRoleObjectAssociationModel.User.ID,
		SubObjectID: *userRoleObjectAssociationModel.SubObjectID,
		SubType:     userRoleObjectAssociationModel.SubType,
	}
}
