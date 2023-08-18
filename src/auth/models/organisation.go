package models

import (
	"reflect"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type Organisation struct {
	entityManagementModels.BaseModel
	OrganisationName string  `json:"organisationName"`
	Groups           []Group `gorm:"foreignKey:OrganisationID" json:"groups"`
}

func (o Organisation) GetBaseModel() entityManagementModels.BaseModel {
	return o.BaseModel
}

func (o Organisation) GetReferenceObject() string {
	return reflect.TypeOf(Organisation{}).Name()
}
