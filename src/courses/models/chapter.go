package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	Order        int
	Title        string
	Number       int
	Footer       string
	Introduction string
	Courses      []*Course  `gorm:"many2many:course_chapters"`
	Sections     []*Section `gorm:"many2many:chapter_sections"`
}

func (c *Chapter) AfterCreate(tx *gorm.DB) (err error) {
	for _, section := range c.Sections {
		chapterSection := &ChapterSections{
			ChapterID: c.ID,
			SectionID: section.ID,
			Order:     section.Order, // Assuming order starts from 1
		}
		if err := tx.Save(chapterSection).Error; err != nil {
			return err
		}
	}
	return nil
}

func (c Chapter) String() string {
	cw := SlidevChapterWriter{c}
	return cw.GetChapter()
}

func (c Chapter) getTitle(toUpper bool) string {
	title := c.Title
	if toUpper {
		title = strings.ToUpper(title)
	}
	return removeAccents(title)
}

type CourseChapters struct {
	CourseID  uuid.UUID `gorm:"primaryKey"`
	ChapterID uuid.UUID `gorm:"primaryKey"`
	Order     int
}
