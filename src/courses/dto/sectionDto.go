package dto

import "soli/formations/src/courses/models"

type SectionInput struct {
	FileName           string        `json:"fileName"`
	Title              string        `json:"title"`
	ParentChapterTitle string        `json:"parentChapterTitle"`
	Intro              string        `json:"intro"`
	Conclusion         string        `json:"conclusion"`
	Number             int           `json:"number"`
	Pages              []models.Page `json:"pages"`
	HiddenPages        []int         `json:"hiddenPages"`
}

type SectionOutput struct {
	ID       string `json:"id"`
	FileName string `json:"fileName"`
}

func SectionModelToSectionOutput(sectionModel models.Section) *SectionOutput {
	return &SectionOutput{
		ID:       sectionModel.ID.String(),
		FileName: sectionModel.FileName,
	}
}
