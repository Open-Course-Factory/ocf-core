package models

import (
	"errors"
	entityManagementModels "soli/formations/src/entityManagement/models"

	"gorm.io/gorm"
)

type Username struct {
	entityManagementModels.BaseModel
	Username string `gorm:"unique"`
}

func (u *Username) BeforeDelete(tx *gorm.DB) (err error) {
	//First method fires the request, all paremeters must be set before
	result := tx.Where("username_id = ?", u.ID).First(&Connection{})
	if result.RowsAffected > 0 {
		return errors.New("used in connection")
	}
	return nil
}

func (u *Username) AfterDelete(tx *gorm.DB) (err error) {
	tx.Model(u).Unscoped().Update("username", gorm.Expr("NULL"))

	return
}
