package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type Generation struct {
	entityManagementModels.BaseModel
	Name       string
	Format     *int
	ThemeID    uuid.UUID
	ScheduleID uuid.UUID
	CourseID   uuid.UUID
}
