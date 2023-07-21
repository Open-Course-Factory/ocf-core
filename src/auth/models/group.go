package models

import (
	"github.com/google/uuid"
)

type Group struct {
	BaseModel
	GroupName      string `json:"groupName"`
	ParentGroupID  *uuid.UUID
	ParentGroup    *Group `json:"parentGroup"`
	OrganisationID *uuid.UUID
	Organisation   *Organisation `json:"Organisation"`
}

// func (g *Group) BeforeCreate(tx *gorm.DB) (err error) {
// 	if g.ID == uuid.Nil {
// 		g.ID = uuid.New()
// 	}

// 	return
// }
