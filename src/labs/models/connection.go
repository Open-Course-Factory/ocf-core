package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type Connection struct {
	entityManagementModels.BaseModel
	UsernameID uuid.UUID `gorm:"type:uuid;primarykey"`
	MachineID  uuid.UUID `gorm:"type:uuid;primarykey"`
}
