package dto

type GenerateCourseOutput struct {
	Result bool `json:"result"`
}

type GenerateCourseInput struct {
	Name        string `binding:"required"`
	Theme       string `binding:"required"`
	Format      string `binding:"required"`
	AuthorEmail string `binding:"required"`
}

type CreateCourseOutput struct {
}

type CreateCourseInput struct {
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
	CourseID_str       string `binding:"required"`
	Schedule           string `binding:"required"`
	Prelude            string `binding:"required"`
	LearningObjectives string `json:"learning_objectives"`
	Chapters           []string
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
	CourseID_str       string `binding:"required"`
	Schedule           string `binding:"required"`
	Prelude            string `binding:"required"`
	LearningObjectives string `json:"learning_objectives"`
	Chapters           []string
}

type CreateCourseFromGitOutput struct {
}

type CreateCourseFromGitInput struct {
	Url string `binding:"required"`
}
