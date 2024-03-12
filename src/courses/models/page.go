package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type PageWriter interface {
	OCFWriter
	GetPage() string
}

// Part of a Section
type Page struct {
	entityManagementModels.BaseModel
	Number    int
	Toc       []string `gorm:"serializer:json"`
	Content   []string `gorm:"serializer:json"`
	Hide      bool
	SectionID uuid.UUID
	Section   Section `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"section"`
}

func (p Page) String() string {
	pw := SlidevPageWriter{p}
	return pw.GetPage()
}

func createPage(number int, pageContent []string, parentSection *Section, hide bool) (p Page) {
	p.Number = number
	p.Content = pageContent
	p.Section = *parentSection
	p.Section.Title = parentSection.Title
	p.Hide = hide
	return
}
