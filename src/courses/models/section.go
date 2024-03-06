package models

import (
	"bufio"
	"os"
	config "soli/formations/src/configuration"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"strings"

	"github.com/google/uuid"
)

type SectionWriter interface {
	OCFWriter
	GetSection() string
}

// Part of a chapter
type Section struct {
	entityManagementModels.BaseModel
	FileName           string
	Title              string
	ParentChapterTitle string
	Intro              string
	Conclusion         string
	Number             int
	ChapterID          uuid.UUID `gorm:"foreignKey:ID"`
	Chapter            Chapter   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"chapter"`
	Pages              []Page    `gorm:"foreignKey:SectionID" json:"pages"`
	HiddenPages        []int     `gorm:"serializer:json"`
}

func (s Section) String() string {
	sw := MarpSectionWriter{s}
	return sw.GetSection()
}

func fillSection(currentSection *Section) {
	var pages []Page
	filename := config.COURSES_ROOT + currentSection.FileName
	f, _ := os.Open(filename)
	defer f.Close()
	scanner := bufio.NewScanner(f)

	pageCounter := 0
	titleFound := false
	introFound := false
	concluFound := false
	var currentPageContent []string
	var hide bool

	currentSection.FileName = filename
	for scanner.Scan() {
		line := scanner.Text()
		hide = false

		// To refactor
		if strings.HasPrefix(line, "title") && !titleFound {
			titleFound = true
			titleLine := strings.Split(line, ":")
			currentSection.Title = titleLine[1]
			continue
		}
		if strings.HasPrefix(line, "intro") && !introFound {
			introFound = true
			introLine := strings.Split(line, ":")
			currentSection.Intro = introLine[1]
			continue
		}
		if strings.HasPrefix(line, "conclusion") && !concluFound {
			concluFound = true
			conclusionLine := strings.Split(line, ":")
			currentSection.Conclusion = conclusionLine[1]
			continue
		}

		if line == "---" {
			if pageCounter > 0 {
				if contains(currentSection.HiddenPages, (pageCounter)) {
					hide = true
				}
				pages = append(pages, createPage(pageCounter, currentPageContent, currentSection.Title, hide))
				currentPageContent = nil
			}
			pageCounter++
		} else {
			currentPageContent = append(currentPageContent, line)
		}
	}
	if contains(currentSection.HiddenPages, pageCounter) {
		hide = true
	}
	pages = append(pages, createPage(pageCounter, currentPageContent, currentSection.Title, hide))
	currentSection.Pages = pages
}
