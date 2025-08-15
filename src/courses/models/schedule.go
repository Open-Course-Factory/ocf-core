package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type Schedule struct {
	entityManagementModels.BaseModel
	Name               string
	FrontMatterContent []string `gorm:"serializer:json"`
	Generations        []Generation
}
