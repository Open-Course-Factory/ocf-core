package models

import entityManagementModels "soli/formations/src/entityManagement/models"

type Machine struct {
	entityManagementModels.BaseModel
	Name     string
	OwnerIDs []string `gorm:"serializer:json"`
}
