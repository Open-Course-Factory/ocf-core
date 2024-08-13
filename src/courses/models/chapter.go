package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"strings"
)

type ChapterWriter interface {
	OCFMdWriter
	SetTitlePage() string
	SetConclusionPage() string
	GetChapter() string
}

// Part of a course
type Chapter struct {
	entityManagementModels.BaseModel
	Title        string
	Number       int
	Footer       string
	Introduction string
	Courses      []*Course `gorm:"many2many:course_chapters;serializer:json" json:"courses"`
	Sections     []Section `json:"sections"`
}

func (c Chapter) String() string {
	cw := SlidevChapterWriter{c}
	return cw.GetChapter()
}

func (c Chapter) getTitle() string {

	return removeAccents(strings.ToUpper(c.Title))
}
