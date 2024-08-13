package models

import (
	"reflect"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type Sshkey struct {
	entityManagementModels.BaseModel
	KeyName    string `gorm:"type:varchar(255)"`
	PrivateKey string `gorm:"type:text"`
	OwnerID    string
	Owner      *casdoorsdk.User `json:"owner" gorm:"serializer:json"`
}

func (s Sshkey) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}

func (s Sshkey) GetReferenceObject() string {
	return reflect.TypeOf(casdoorsdk.User{}).Name()
}
