package models

import (
	"github.com/google/uuid"
)

type UserRole struct {
	BaseModel
	Role        *Role      `json:"role"`
	RoleID      *uuid.UUID `gorm:"primaryKey"`
	User        *User      `json:"user"`
	UserID      *uuid.UUID `gorm:"primaryKey"`
	SubObjectID *uuid.UUID `gorm:"primaryKey" json:"sub_object_id"`
	SubType     string     `json:"sub_type"`
}
