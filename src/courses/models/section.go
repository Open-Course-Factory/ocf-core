package models

import (
	"bufio"
	"os"
	config "soli/formations/src/configuration"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Part of a chapter
type Section struct {
	gorm.Model
	ID                 uuid.UUID `gorm:"type:uuid;primarykey"`
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

func (s *Section) BeforeCreate(tx *gorm.DB) (err error) {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}

	return
}

func (s Section) String() string {
	firstLine := "---\n\n"
	localClass := "<!-- _class: lead -->\n\n"
	title := "# " + strings.ToUpper(s.ParentChapterTitle) + "\n\n"
	var toc string
	for _, lineOfToc := range s.Pages[0].Toc {
		toc += lineOfToc + "\n"
	}
	toc = toc + "\n"

	var pages string
	for _, page := range s.Pages {
		pages += page.String() + "\n"
	}

	return firstLine + localClass + title + toc + pages
}

// Open the file.
// Create a new Scanner for the file.
// Loop over all lines in the file and print them.
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
