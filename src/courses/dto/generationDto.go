package dto

import "soli/formations/src/courses/models"

type GenerationInput struct {
	OwnerID    string
	Name       string `json:"name"`
	Format     *int   `json:"format"`
	ThemeId    string `json:"themes" mapstructure:"themes"`
	ScheduleId string `json:"schedules" mapstructure:"schedules"`
	CourseId   string `json:"courses" mapstructure:"courses"`
}

type GenerationOutput struct {
	ID         string   `json:"id"`
	OwnerIDs   []string `gorm:"serializer:json"`
	Name       string   `json:"name"`
	Format     *int     `json:"format"`
	ThemeId    string   `json:"themes"`
	ScheduleId string   `json:"schedules"`
	CourseId   string   `json:"courses"`
}

func GenerationModelToGenerationOutput(generationModel models.Generation) *GenerationOutput {

	return &GenerationOutput{
		ID:         generationModel.ID.String(),
		Name:       generationModel.Name,
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
		Name:       generationModel.Name,
		ThemeId:    generationModel.ThemeID.String(),
		ScheduleId: generationModel.ScheduleID.String(),
		CourseId:   generationModel.CourseID.String(),
		Format:     generationModel.Format,
	}
}
