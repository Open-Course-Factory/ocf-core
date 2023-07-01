package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Permission struct {
	gorm.Model
	ID             uuid.UUID `gorm:"type:uuid;primarykey"`
	UserID         *uuid.UUID
	User           *User `json:"user"`
	GroupID        *uuid.UUID
	Group          *Group `json:"group"`
	RoleID         *uuid.UUID
	Role           *Role `json:"role"`
	OrganisationID *uuid.UUID
	Organisation   *Organisation `json:"organisation"`
}

func (p *Permission) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}

	return
}
