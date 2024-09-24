package dto

import (
	"soli/formations/src/courses/models"
)

type GenerateCourseOutput struct {
	Result bool `json:"result"`
}

type GenerateCourseInput struct {
	Id                       string `binding:"required"`
	ThemeId                  string
	ThemeGitRepository       string
	ThemeGitRepositoryBranch string
	Format                   string `binding:"required"`
	AuthorEmail              string `binding:"required"`
	ScheduleId               string
}

type CourseInput struct {
	OwnerID                  string
	Name                     string `binding:"required"`
	Theme                    string `binding:"required"`
	Format                   *int   `binding:"required,gte=0,lte=1"`
	AuthorEmail              string `binding:"required"`
	Category                 string `binding:"required"`
	Version                  string
	Title                    string `binding:"required"`
	Subtitle                 string
	Header                   string `binding:"required"`
	Footer                   string `binding:"required"`
	Logo                     string
	Description              string
	ScheduleId               string          `binding:"required"`
	Prelude                  string          `binding:"required"`
	LearningObjectives       string          `json:"learning_objectives"`
	ChaptersInput            []*ChapterInput `json:"chapters"`
	GitRepository            string
	GitRepositoryBranch      string
	ThemeGitRepository       string
	ThemeGitRepositoryBranch string
}

type CourseOutput struct {
	Name               string          `binding:"required" json:"name"`
	Theme              string          `binding:"required" json:"theme"`
	Format             int             `binding:"required" json:"format"`
	AuthorEmail        string          `binding:"required" json:"author_email"`
	Category           string          `binding:"required" json:"category"`
	Version            string          `json:"version"`
	Title              string          `binding:"required" json:"title"`
	Subtitle           string          `json:"subtitles"`
	Header             string          `binding:"required" json:"header"`
	Footer             string          `binding:"required" json:"footer"`
	Logo               string          `json:"logo"`
	Description        string          `json:"description"`
	CourseID_str       string          `binding:"required" json:"course_id_str"`
	ScheduleId         string          `binding:"required"`
	Prelude            string          `binding:"required" json:"prelude"`
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
	ScheduleId         string            `binding:"required"`
	Prelude            string            `binding:"required"`
	LearningObjectives string            `json:"learning_objectives"`
	Chapters           []*models.Chapter `json:"chapters"`
}

type CreateCourseFromGitOutput struct {
}

type CreateCourseFromGitInput struct {
	Url        string `binding:"required"`
	BranchName string `json:"branch_name,omitempty"`
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
		ScheduleId:         courseModel.Schedule.ID.String(),
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
		OwnerID:                  courseModel.OwnerIDs[0],
		Name:                     courseModel.Name,
		Theme:                    courseModel.Theme,
		Version:                  courseModel.Version,
		Title:                    courseModel.Title,
		Format:                   (*int)(&courseModel.Format),
		Subtitle:                 courseModel.Subtitle,
		Header:                   courseModel.Header,
		Footer:                   courseModel.Footer,
		Logo:                     courseModel.Logo,
		Description:              courseModel.Description,
		ScheduleId:               courseModel.Schedule.ID.String(),
		Prelude:                  courseModel.Prelude,
		LearningObjectives:       courseModel.LearningObjectives,
		ChaptersInput:            chapterInputs,
		GitRepository:            courseModel.GitRepository,
		GitRepositoryBranch:      courseModel.GitRepositoryBranch,
		ThemeGitRepository:       courseModel.ThemeGitRepository,
		ThemeGitRepositoryBranch: courseModel.ThemeGitRepositoryBranch,
	}
}
