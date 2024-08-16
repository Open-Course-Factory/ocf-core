package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type Username struct {
	entityManagementModels.BaseModel
	Username string `gorm:"unique"`
}
