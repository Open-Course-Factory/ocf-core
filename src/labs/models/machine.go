package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
)

type Machine struct {
	entityManagementModels.BaseModel
	Name      string
	IP        string
	Usernames []Username `gorm:"many2many:connections;"`
	Port      int
}
