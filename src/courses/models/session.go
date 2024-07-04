package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"
)

type Session struct {
	entityManagementModels.BaseModel
	CourseId  string
	Title     string
	GroupId   string
	Beginning time.Time
	End       time.Time
}
