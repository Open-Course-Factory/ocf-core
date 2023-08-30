package models

import (
	"reflect"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type Group struct {
	entityManagementModels.BaseModel
	GroupName      string `json:"groupName"`
	ParentGroupID  *uuid.UUID
	ParentGroup    *Group `json:"parentGroup"`
	OrganisationID *uuid.UUID
	Organisation   *Organisation `json:"Organisation"`
}

func (g Group) GetBaseModel() entityManagementModels.BaseModel {
	return g.BaseModel
}

func (g Group) GetReferenceObject() string {
	return reflect.TypeOf(Organisation{}).Name()
}
