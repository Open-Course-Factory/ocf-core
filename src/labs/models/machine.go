package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type Machine struct {
	entityManagementModels.BaseModel
	Name       string
	IP         string
	UsernameId string
	OwnerIDs   []string `gorm:"serializer:json"`
}
