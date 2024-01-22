package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
)

type Session struct {
	entityManagementModels.BaseModel
	Course    Course
	Title     string
	Group     casdoorsdk.Group
	Beginning time.Time
	End       time.Time
}
