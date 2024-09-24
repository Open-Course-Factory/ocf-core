package models

import (
	entityManagementModels "soli/formations/src/entityManagement/models"
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

func (p Page) String(section Section, chapter Chapter) string {
	pw := SlidevPageWriter{p, section, chapter}
	return pw.GetPage()
}

func createPage(order int, pageContent []string, parentSection *Section, hide bool) (p *Page) {
	return &Page{
		Order:    order,
		Content:  pageContent,
		Sections: []*Section{parentSection},
		Hide:     hide,
	}
}
