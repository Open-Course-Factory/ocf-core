package dto

import "soli/formations/src/courses/models"

type SectionInput struct {
	FileName    string         `json:"fileName"`
	Title       string         `json:"title"`
	Intro       string         `json:"intro"`
	Conclusion  string         `json:"conclusion"`
	Number      int            `json:"number"`
	Pages       []*models.Page `json:"pages"`
	HiddenPages []int          `json:"hiddenPages"`
}

type SectionOutput struct {
	ID        string `json:"id"`
	FileName  string `json:"fileName"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type EditSectionInput struct {
	FileName    string        `json:"fileName"`
	Title       string        `json:"title"`
	Intro       string        `json:"intro"`
	Conclusion  string        `json:"conclusion"`
	Number      int           `json:"number"`
	Pages       []models.Page `json:"pages"`
	HiddenPages []int         `json:"hiddenPages"`
}

func SectionModelToSectionOutput(sectionModel models.Section) *SectionOutput {
	return &SectionOutput{
		ID:        sectionModel.ID.String(),
		FileName:  sectionModel.FileName,
		CreatedAt: sectionModel.CreatedAt.String(),
		UpdatedAt: sectionModel.UpdatedAt.String(),
	}
}
