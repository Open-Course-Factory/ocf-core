package models

import (
	"github.com/google/uuid"
)

type PermissionType string

const (
	PermissionTypeRead   PermissionType = "read"
	PermissionTypeWrite  PermissionType = "write"
	PermissionTypeDelete PermissionType = "delete"
	PermissionTypeAll    PermissionType = "all"
)

func ContainsPermissionType(enumArray []PermissionType, value PermissionType) bool {
	for _, v := range enumArray {
		if v == value {
			return true
		}
	}
	return false
}

// ToDo : maybe the group should not be in permission table
type Permission struct {
	BaseModel
	UserID          *uuid.UUID
	User            *User `json:"user"`
	GroupID         *uuid.UUID
	Group           *Group `json:"group"`
	RoleID          *uuid.UUID
	Role            *Role `json:"role"`
	OrganisationID  *uuid.UUID
	Organisation    *Organisation    `json:"organisation"`
	PermissionTypes []PermissionType `gorm:"serializer:json" json:"permission_types"`
}
