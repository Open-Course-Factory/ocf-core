package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"gorm.io/gorm"
)

type Username struct {
	entityManagementModels.BaseModel
	Username string `gorm:"unique"`
}

func (u *Username) AfterDelete(tx *gorm.DB) (err error) {
	tx.Model(u).Unscoped().Update("username", gorm.Expr("NULL"))

	return
}
