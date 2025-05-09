package dto

type ScheduleInput struct {
	Name               string   `json:"name"`
	FrontMatterContent []string `json:"front_matter_content" mapstructure:"front_matter_content" gorm:"serializer:json"`
}

type ScheduleOutput struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	FrontMatterContent []string `json:"front_matter_content"`
	CreatedAt          string   `json:"createdAt"`
	UpdatedAt          string   `json:"updatedAt"`
}

type EditScheduleInput struct {
	Name               string   `json:"name"`
	FrontMatterContent []string `json:"front_matter_content" mapstructure:"front_matter_content" gorm:"serializer:json"`
}
