package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type Generation struct {
	entityManagementModels.BaseModel
	Format     *int
	ThemeID    uuid.UUID
	ScheduleID uuid.UUID
	CourseID   uuid.UUID
}
