package dto

import (
	"soli/formations/src/courses/models"

	"github.com/lib/pq"
)

type SectionInput struct {
	OwnerID     string
	FileName    string       `json:"fileName"`
	Title       string       `json:"title"`
	Intro       string       `json:"intro"`
	Conclusion  string       `json:"conclusion"`
	Number      int          `json:"number"`
	Pages       []*PageInput `json:"pages"`
	HiddenPages []int        `json:"hiddenPages"`
}

type SectionOutput struct {
	ID          string         `json:"id"`
	FileName    string         `json:"fileName"`
	OwnerIDs    pq.StringArray `gorm:"type:text[]"`
	Title       string         `json:"title"`
	Intro       string         `json:"intro"`
	Conclusion  string         `json:"conclusion"`
	Number      int            `json:"number"`
	Pages       []*PageInput   `json:"pages"`
	HiddenPages []int          `json:"hiddenPages"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
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
	var pages []*PageInput

	for _, page := range sectionModel.Pages {
		pageInput := PageModelToPageInput(*page)
		pages = append(pages, pageInput)
	}

	return &SectionOutput{
		ID:          sectionModel.ID.String(),
		FileName:    sectionModel.FileName,
		OwnerIDs:    sectionModel.OwnerIDs,
		Title:       sectionModel.Title,
		Intro:       sectionModel.Intro,
		Conclusion:  sectionModel.Conclusion,
		Number:      sectionModel.Number,
		Pages:       pages,
		HiddenPages: sectionModel.HiddenPages,
		CreatedAt:   sectionModel.CreatedAt.String(),
		UpdatedAt:   sectionModel.UpdatedAt.String(),
	}
}

func SectionModelToSectionInput(sectionModel models.Section) *SectionInput {
	var pages []*PageInput

	for _, page := range sectionModel.Pages {
		pageInput := PageModelToPageInput(*page)
		pages = append(pages, pageInput)
	}

	return &SectionInput{
		OwnerID:     sectionModel.OwnerIDs[0],
		FileName:    sectionModel.FileName,
		Title:       sectionModel.Title,
		Intro:       sectionModel.Intro,
		Conclusion:  sectionModel.Conclusion,
		Number:      sectionModel.Number,
		Pages:       pages,
		HiddenPages: sectionModel.HiddenPages,
	}
}
