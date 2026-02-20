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

// ParentChapterOutput contains minimal chapter information for section's parent chapter
type ParentChapterOutput struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Number       int    `json:"number"`
	Introduction string `json:"introduction"`
}

type SectionOutput struct {
	ID          string                `json:"id"`
	FileName    string                `json:"fileName"`
	OwnerIDs    pq.StringArray        `gorm:"type:text[]" json:"ownerIDs" swaggertype:"array,string"`
	Chapters    []ParentChapterOutput `json:"chapters,omitempty"` // Parent chapter information
	Title       string                `json:"title"`
	Intro       string                `json:"intro"`
	Conclusion  string                `json:"conclusion"`
	Number      int                   `json:"number"`
	Pages       []*PageInput          `json:"pages"`
	HiddenPages []int                 `json:"hiddenPages"`
	CreatedAt   string                `json:"createdAt"`
	UpdatedAt   string                `json:"updatedAt"`
}

type EditSectionInput struct {
	FileName    *string       `json:"fileName,omitempty" mapstructure:"fileName"`
	Title       *string       `json:"title,omitempty" mapstructure:"title"`
	Intro       *string       `json:"intro,omitempty" mapstructure:"intro"`
	Conclusion  *string       `json:"conclusion,omitempty" mapstructure:"conclusion"`
	Number      *int          `json:"number,omitempty" mapstructure:"number"`
	Pages       []models.Page `json:"pages,omitempty" mapstructure:"pages"`
	HiddenPages []int         `json:"hiddenPages,omitempty" mapstructure:"hiddenPages"`
}

func SectionModelToSectionOutput(sectionModel models.Section) *SectionOutput {
	var pages []*PageInput

	for _, page := range sectionModel.Pages {
		pageInput := PageModelToPageInput(*page)
		pages = append(pages, pageInput)
	}

	var parentChapters []ParentChapterOutput
	for _, chapter := range sectionModel.Chapters {
		// Include parent chapter information
		parentChapters = append(parentChapters, ParentChapterOutput{
			ID:           chapter.ID.String(),
			Title:        chapter.Title,
			Number:       chapter.Number,
			Introduction: chapter.Introduction,
		})
	}

	return &SectionOutput{
		ID:          sectionModel.ID.String(),
		FileName:    sectionModel.FileName,
		OwnerIDs:    sectionModel.OwnerIDs,
		Chapters:    parentChapters,
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
