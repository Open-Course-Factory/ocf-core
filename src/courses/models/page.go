package models

import (
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
)

// Part of a Section
type Page struct {
	gorm.Model
	ID        uuid.UUID `gorm:"type:uuid;primarykey"`
	Number    int
	Toc       []string `gorm:"serializer:json"`
	Content   []string `gorm:"serializer:json"`
	Hide      bool
	SectionID uuid.UUID
	Section   Section `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"section"`
}

func (p *Page) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}

	return
}

func (p Page) String() string {
	firstLine := "---\n\n"

	title := "## " + strings.ToUpper(p.Section.Title) + "\n\n"
	var toc string
	toc = toc + "<div class=\"toc\">\n\n"
	for _, lineOfToc := range p.Toc {
		toc += "- " + lineOfToc + "\n"
	}
	toc = toc + "\n</div>\n\n"

	var content string
	for _, line := range p.Content {
		content += line + "\n"
	}

	if p.Hide {
		firstLine += "<!-- _hide: true -->\n\n"
		firstLine += "<!-- _paginate: skip -->\n\n"
	}

	return firstLine + toc + title + content
}

func createPage(number int, pageContent []string, parentSectionTitle string, hide bool) (p Page) {
	p.Number = number
	p.Content = pageContent
	p.Section.Title = parentSectionTitle
	p.Hide = hide
	return
}
