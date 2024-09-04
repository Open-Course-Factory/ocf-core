package dto

import "soli/formations/src/courses/models"

type ChapterInput struct {
	Title        string           `json:"title"`
	Number       int              `json:"number"`
	Footer       string           `json:"footer"`
	Introduction string           `json:"introduction"`
	Sections     []models.Section `json:"sections"`
}

type ChapterOutput struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Number       int             `json:"number"`
	Footer       string          `json:"footer"`
	Introduction string          `json:"introduction"`
	Sections     []SectionOutput `json:"sections"`
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
