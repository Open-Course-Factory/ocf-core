package dto

import "soli/formations/src/courses/models"

type ChapterInput struct {
	Title        string          `json:"title"`
	Number       int             `json:"number"`
	Footer       string          `json:"footer"`
	Introduction string          `json:"introduction"`
	Sections     []*SectionInput `json:"sections"`
}

type ChapterOutput struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Number       int             `json:"number"`
	Footer       string          `json:"footer"`
	Introduction string          `json:"introduction"`
	Sections     []SectionOutput `json:"sections"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
}

type EditChapterInput struct {
	Title        string          `json:"title"`
	Number       int             `json:"number"`
	Footer       string          `json:"footer"`
	Introduction string          `json:"introduction"`
	Sections     []*SectionInput `json:"sections"`
}

func ChapterModelToChapterOutput(chapterModel models.Chapter) *ChapterOutput {

	var sectionsOutputs []SectionOutput
	for _, section := range chapterModel.Sections {
		sectionsOutputs = append(sectionsOutputs, *SectionModelToSectionOutput(*section))
	}

	return &ChapterOutput{
		ID:           chapterModel.ID.String(),
		Title:        chapterModel.Title,
		Number:       chapterModel.Number,
		Footer:       chapterModel.Footer,
		Introduction: chapterModel.Introduction,
		Sections:     sectionsOutputs,
	}
}

func ChapterModelToChapterInput(chapterModel models.Chapter) *ChapterInput {

	var sectionsInputs []*SectionInput
	for _, section := range chapterModel.Sections {
		sectionsInputs = append(sectionsInputs, SectionModelToSectionInput(*section))
	}

	return &ChapterInput{
		Title:        chapterModel.Title,
		Number:       chapterModel.Number,
		Footer:       chapterModel.Footer,
		Introduction: chapterModel.Introduction,
		Sections:     sectionsInputs,
	}
}
