package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type SshKey struct {
	entityManagementModels.BaseModel
	KeyName    string    `gorm:"type:varchar(255)"`
	PrivateKey string    `gorm:"type:text"`
	UserID     uuid.UUID `gorm:"type:uuid;primarykey"`
}

func (s SshKey) GetBaseModel() entityManagementModels.BaseModel {
	return s.BaseModel
}
