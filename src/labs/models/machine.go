package models

import (
	"errors"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"gorm.io/gorm"
)

type Machine struct {
	entityManagementModels.BaseModel
	Name      string
	IP        string
	Usernames []Username `gorm:"many2many:connections;"`
	Port      int
}

func (m *Machine) BeforeDelete(tx *gorm.DB) (err error) {
	//First method fires the request, all paremeters must be set before
	result := tx.Where("machine_id = ?", m.ID).First(&Connection{})
	if result.RowsAffected > 0 {
		return errors.New("used in connection")
	}
	return nil
}
