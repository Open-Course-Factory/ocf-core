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

