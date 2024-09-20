package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"

	"github.com/google/uuid"
)

type PageWriter interface {
	OCFMdWriter
	GetPage() string
}

// Part of a Section
type Page struct {
	entityManagementModels.BaseModel
	Order    int
	Toc      []string `gorm:"serializer:json"`
	Content  []string `gorm:"serializer:json"`
	Hide     bool
	Sections []*Section `gorm:"many2many:section_pages;"`
}

type SectionPages struct {
	SectionID uuid.UUID `gorm:"primaryKey"`
	PageID    uuid.UUID `gorm:"primaryKey"`
	Order     int
}

func (p Page) String(section Section, chapter Chapter) string {
	pw := SlidevPageWriter{p, section, chapter}
	return pw.GetPage()
}

func createPage(order int, pageContent []string, parentSection *Section, hide bool) (p *Page) {
	pageToReturn := &Page{
		Order:    order,
		Content:  pageContent,
		Sections: []*Section{parentSection},
		Hide:     hide,
	}
	pageToReturn.OwnerIDs = append(pageToReturn.OwnerIDs, parentSection.OwnerIDs[0])
	return pageToReturn
}
