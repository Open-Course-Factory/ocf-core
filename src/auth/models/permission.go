package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
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

type Permission struct {
	gorm.Model
	ID              uuid.UUID `gorm:"type:uuid;primarykey"`
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

func (p *Permission) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}

	return
}
