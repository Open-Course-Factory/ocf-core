package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type SshKey struct {
	entityManagementModels.BaseModel
	KeyName    string `gorm:"type:varchar(255)"`
	PrivateKey string `gorm:"type:text"`
}
