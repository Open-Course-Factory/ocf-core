package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type Package struct {
	entityManagementModels.BaseModel
	Format                   *int
	ThemeGitRepository       string
	ThemeGitRepositoryBranch string
	ThemeId                  string
	ScheduleID               uuid.UUID
	CourseID                 uuid.UUID
}
