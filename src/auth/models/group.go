package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Group struct {
	gorm.Model
	ID             uuid.UUID `gorm:"type:uuid;primarykey"`
	GroupName      string    `json:"groupName"`
	ParentGroupID  *uuid.UUID
	ParentGroup    *Group `json:"parentGroup"`
	OrganisationID *uuid.UUID
	Organisation   *Organisation `json:"Organisation"`
}

func (g *Group) BeforeCreate(tx *gorm.DB) (err error) {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}

	return
}
