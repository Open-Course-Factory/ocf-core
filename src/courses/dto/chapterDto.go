package dto

import "soli/formations/src/courses/models"

type ChapterInput struct {
	OwnerID      string
	Title        string          `json:"title"`
	Number       int             `json:"number"`
	Footer       string          `json:"footer"`
	Introduction string          `json:"introduction"`
	Sections     []*SectionInput `json:"sections"`
}

// ParentCourseOutput contains minimal course information for chapter's parent course
type ParentCourseOutput struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type ChapterOutput struct {
	ID              string               `json:"id"`
	ParentCourseIDs []string             `json:"courseIDs"`
	Courses         []ParentCourseOutput `json:"courses,omitempty"` // Parent course information
	Title           string               `json:"title"`
	Number          int                  `json:"number"`
	Footer          string               `json:"footer"`
	Introduction    string               `json:"introduction"`
	Sections        []SectionOutput      `json:"sections"`
	CreatedAt       string               `json:"createdAt"`
	UpdatedAt       string               `json:"updatedAt"`
}

type EditChapterInput struct {
	Title        *string         `json:"title,omitempty" mapstructure:"title"`
	Number       *int            `json:"number,omitempty" mapstructure:"number"`
	Footer       *string         `json:"footer,omitempty" mapstructure:"footer"`
	Introduction *string         `json:"introduction,omitempty" mapstructure:"introduction"`
	Sections     []*SectionInput `json:"sections,omitempty" mapstructure:"sections"`
}

func ChapterModelToChapterOutput(chapterModel models.Chapter) *ChapterOutput {

	var sectionsOutputs []SectionOutput
	for _, section := range chapterModel.Sections {
		sectionsOutputs = append(sectionsOutputs, *SectionModelToSectionOutput(*section))
	}

	var parentCourseIDs []string
	var parentCourses []ParentCourseOutput
	for _, course := range chapterModel.Courses {
		parentCourseIDs = append(parentCourseIDs, course.ID.String())
		// Include parent course information
		parentCourses = append(parentCourses, ParentCourseOutput{
			ID:      course.ID.String(),
			Name:    course.Name,
			Title:   course.Title,
			Version: course.Version,
		})
	}

	return &ChapterOutput{
		ID:              chapterModel.ID.String(),
		ParentCourseIDs: parentCourseIDs,
		Courses:         parentCourses,
		Title:           chapterModel.Title,
		Number:          chapterModel.Number,
		Footer:          chapterModel.Footer,
		Introduction:    chapterModel.Introduction,
		Sections:        sectionsOutputs,
	}
}

func ChapterModelToChapterInput(chapterModel models.Chapter) *ChapterInput {

	var sectionsInputs []*SectionInput
	for _, section := range chapterModel.Sections {
		sectionsInputs = append(sectionsInputs, SectionModelToSectionInput(*section))
	}

	return &ChapterInput{
		OwnerID:      chapterModel.OwnerIDs[0],
		Title:        chapterModel.Title,
		Number:       chapterModel.Number,
		Footer:       chapterModel.Footer,
		Introduction: chapterModel.Introduction,
		Sections:     sectionsInputs,
	}
}
