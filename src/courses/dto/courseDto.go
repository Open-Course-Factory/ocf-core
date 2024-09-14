package dto

import (
	"soli/formations/src/courses/models"
)

type GenerateCourseOutput struct {
	Result bool `json:"result"`
}

type GenerateCourseInput struct {
	Id          string `binding:"required"`
	Theme       string `binding:"required"`
	Format      string `binding:"required"`
	AuthorEmail string `binding:"required"`
}

type CourseInput struct {
	Name               string `binding:"required"`
	Theme              string `binding:"required"`
	Format             *int   `binding:"required,gte=0,lte=1"`
	AuthorEmail        string `binding:"required"`
	Category           string `binding:"required"`
	Version            string
	Title              string `binding:"required"`
	Subtitle           string
	Header             string `binding:"required"`
	Footer             string `binding:"required"`
	Logo               string
	Description        string
	Schedule           string          `binding:"required"`
	Prelude            string          `binding:"required"`
	LearningObjectives string          `json:"learning_objectives"`
	ChaptersInput      []*ChapterInput `json:"chapters"`
}

type CourseOutput struct {
	Name               string `binding:"required"`
	Theme              string `binding:"required"`
	Format             int    `binding:"required"`
	AuthorEmail        string `binding:"required"`
	Category           string `binding:"required"`
	Version            string
	Title              string `binding:"required"`
	Subtitle           string
	Header             string `binding:"required"`
	Footer             string `binding:"required"`
	Logo               string
	Description        string
	CourseID_str       string          `binding:"required"`
	Schedule           string          `binding:"required"`
	Prelude            string          `binding:"required"`
	LearningObjectives string          `json:"learning_objectives"`
	ChaptersOutput     []ChapterOutput `json:"chapters"`
}

type EditCourseInput struct {
	Name               string `binding:"required"`
	Theme              string `binding:"required"`
	Format             *int   `binding:"required,gte=0,lte=1"`
	AuthorEmail        string `binding:"required"`
	Category           string `binding:"required"`
	Version            string
	Title              string `binding:"required"`
	Subtitle           string
	Header             string `binding:"required"`
	Footer             string `binding:"required"`
	Logo               string
	Description        string
	Schedule           string            `binding:"required"`
	Prelude            string            `binding:"required"`
	LearningObjectives string            `json:"learning_objectives"`
	Chapters           []*models.Chapter `json:"chapters"`
}

type CreateCourseFromGitOutput struct {
}

type CreateCourseFromGitInput struct {
	Url        string `binding:"required"`
	BranchName string `json:"omitempty"`
	Name       string `binding:"required"`
}

func CourseModelToCourseOutputDto(courseModel models.Course) *CourseOutput {

	var chapterOutputs []ChapterOutput
	for _, chapter := range courseModel.Chapters {
		chapterOutputs = append(chapterOutputs, *ChapterModelToChapterOutput(*chapter))
	}

	return &CourseOutput{
		Name:               courseModel.Name,
		Theme:              courseModel.Theme,
		Version:            courseModel.Version,
		Title:              courseModel.Title,
		Subtitle:           courseModel.Subtitle,
		Header:             courseModel.Header,
		Footer:             courseModel.Footer,
		Logo:               courseModel.Logo,
		Description:        courseModel.Description,
		CourseID_str:       courseModel.ID.String(),
		Schedule:           courseModel.Schedule,
		Prelude:            courseModel.Prelude,
		LearningObjectives: courseModel.LearningObjectives,
		ChaptersOutput:     chapterOutputs,
	}
}

func CourseModelToCourseInputDto(courseModel models.Course) *CourseInput {

	var chapterInputs []*ChapterInput
	for _, chapter := range courseModel.Chapters {
		chapterInputs = append(chapterInputs, ChapterModelToChapterInput(*chapter))
	}

	return &CourseInput{
		Name:               courseModel.Name,
		Theme:              courseModel.Theme,
		Version:            courseModel.Version,
		Title:              courseModel.Title,
		Format:             (*int)(&courseModel.Format),
		Subtitle:           courseModel.Subtitle,
		Header:             courseModel.Header,
		Footer:             courseModel.Footer,
		Logo:               courseModel.Logo,
		Description:        courseModel.Description,
		Schedule:           courseModel.Schedule,
		Prelude:            courseModel.Prelude,
		LearningObjectives: courseModel.LearningObjectives,
		ChaptersInput:      chapterInputs,
	}
}
