package dto

import "soli/formations/src/courses/models"

type SectionInput struct {
	FileName    string       `json:"fileName"`
	Title       string       `json:"title"`
	Intro       string       `json:"intro"`
	Conclusion  string       `json:"conclusion"`
	Number      int          `json:"number"`
	Pages       []*PageInput `json:"pages"`
	HiddenPages []int        `json:"hiddenPages"`
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

func SectionModelToSectionInput(sectionModel models.Section) *SectionInput {
	var pages []*PageInput

	for _, page := range sectionModel.Pages {
		pageInput := PageModelToPageInput(*page)
		pages = append(pages, pageInput)
	}

	return &SectionInput{
		FileName:    sectionModel.FileName,
		Title:       sectionModel.Title,
		Intro:       sectionModel.Intro,
		Conclusion:  sectionModel.Conclusion,
		Number:      sectionModel.Number,
		Pages:       pages,
		HiddenPages: sectionModel.HiddenPages,
	}
}
