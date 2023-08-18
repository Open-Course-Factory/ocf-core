package models

import (
	"reflect"
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type User struct {
	entityManagementModels.BaseModel
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Email        string `json:"email"`
	Password     string
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	SshKeys      []SshKey
	Roles        []Role `gorm:"many2many:user_roles;"`
}

func (u User) GetBaseModel() entityManagementModels.BaseModel {
	return u.BaseModel
}

func (u User) GetReferenceObject() string {
	return reflect.TypeOf(User{}).Name()
}
