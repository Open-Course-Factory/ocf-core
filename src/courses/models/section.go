package models

import (
	"bufio"
	"fmt"
	"os"
	config "soli/formations/src/configuration"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"strings"

	"github.com/adrg/frontmatter"
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
	sw := SlidevSectionWriter{s}
	return sw.GetSection()
}

func fillSection(currentSection *Section) {
	var pages []Page
	filename := config.COURSES_ROOT + currentSection.FileName
	f, _ := os.Open(filename)
	defer f.Close()
	scanner := bufio.NewScanner(f)

	pageCounter := 0

	var sPages []string
	var currentPageContent []string
	var hide bool

	currentSection.FileName = filename

	bIgnoreFrontMatterEnd := false
	for scanner.Scan() {
		line := scanner.Text()

		if !bIgnoreFrontMatterEnd {
			if line == "---" {
				bIgnoreFrontMatterEnd = true
				sPages = append(sPages, strings.Join(currentPageContent[:], "\n"))
				currentPageContent = nil
				currentPageContent = append(currentPageContent, line)
			} else {
				currentPageContent = append(currentPageContent, line)
			}

		} else {
			if line == "---" {
				bIgnoreFrontMatterEnd = false
			}
			currentPageContent = append(currentPageContent, line)
		}
	}
	sPages = append(sPages, strings.Join(currentPageContent[:], "\n"))

	var sectionFrontMatter struct {
		Title      string `yaml:"title"`
		Intro      string `yaml:"intro"`
		Conclusion string `yaml:"conclusion"`
	}

	var pageFrontMatter struct {
		Layout string `yaml:"layout"`
	}

	beginningIndex := 0
	for index, sPage := range sPages {

		sectionFrontMatter.Title = ""
		sectionFrontMatter.Intro = ""
		sectionFrontMatter.Conclusion = ""

		pageFrontMatter.Layout = ""

		sPageContent, err := frontmatter.Parse(strings.NewReader(sPage), &sectionFrontMatter)
		if sectionFrontMatter.Title != "" {
			currentSection.Title = sectionFrontMatter.Title
			currentSection.Intro = sectionFrontMatter.Intro
			currentSection.Conclusion = sectionFrontMatter.Conclusion
			beginningIndex = index
		} else {
			if index > beginningIndex {
				pageCounter++
				sPageContent, err = frontmatter.Parse(strings.NewReader(sPage), &pageFrontMatter)
				fmt.Printf("%+v\n", pageFrontMatter)

				if contains(currentSection.HiddenPages, (pageCounter)) {
					hide = true
				}
				pages = append(pages, createPage(pageCounter, strings.Split(string(sPageContent), "\n"), currentSection, hide))
			}
		}

		if err != nil {
			fmt.Println(err.Error())
		}

	}

	currentSection.Pages = pages
}
