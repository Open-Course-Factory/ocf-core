package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type UserRoles struct {
	entityManagementModels.BaseModel
	Role        *Role      `json:"role"`
	RoleID      *uuid.UUID `json:"role_id"`
	User        *User      `json:"user"`
	UserID      *uuid.UUID `json:"user_id"`
	SubObjectID *uuid.UUID `json:"sub_object_id"`
	SubType     string     `json:"sub_type"`
}

func (ur UserRoles) GetBaseModel() entityManagementModels.BaseModel {
	return ur.BaseModel
}
