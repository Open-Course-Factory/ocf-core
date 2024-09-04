package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Connection struct {
	entityManagementModels.BaseModel
	ID         uuid.UUID `gorm:"type:uuid;unique"`
	UsernameID uuid.UUID `gorm:"type:uuid;primarykey"`
	MachineID  uuid.UUID `gorm:"type:uuid;primarykey"`
}

// override base model ID creation
func (c *Connection) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return
}
