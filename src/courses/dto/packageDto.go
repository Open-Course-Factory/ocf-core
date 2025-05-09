package dto

import "soli/formations/src/courses/models"

type GenerationInput struct {
	OwnerID    string
	Format     *int
	ThemeId    string
	ScheduleId string
	CourseId   string
}

type GenerationOutput struct {
	ID                       string   `json:"id"`
	OwnerIDs                 []string `gorm:"serializer:json"`
	Format                   *int
	ThemeGitRepository       string
	ThemeGitRepositoryBranch string
	ThemeId                  string
	ScheduleId               string
	CourseId                 string
}

func GenerationModelToGenerationOutput(generationModel models.Generation) *GenerationOutput {

	return &GenerationOutput{
		ID:         generationModel.ID.String(),
		OwnerIDs:   generationModel.OwnerIDs,
		ThemeId:    generationModel.ThemeID.String(),
		ScheduleId: generationModel.ScheduleID.String(),
		CourseId:   generationModel.CourseID.String(),
		Format:     generationModel.Format,
	}
}

func GenerationModelToGenerationInput(generationModel models.Generation) *GenerationInput {

	return &GenerationInput{
		OwnerID:    generationModel.OwnerIDs[0],
		ThemeId:    generationModel.ThemeID.String(),
		ScheduleId: generationModel.ScheduleID.String(),
		CourseId:   generationModel.CourseID.String(),
		Format:     generationModel.Format,
	}
}
