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
	OCFMdWriter
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
	Chapter            Chapter   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;serializer:json" json:"chapter"`
	Pages              []Page    `gorm:"foreignKey:SectionID;serializer:json" json:"pages"`
	HiddenPages        []int     `gorm:"serializer:json"`
}

func (s Section) String() string {
	sw := SlidevSectionWriter{s}
	return sw.GetSection()
}

func fillSection(currentSection *Section) {
	filename := config.COURSES_ROOT + currentSection.FileName
	currentSection.FileName = filename

	sPages := extractPagesFromSectionsFiles(filename)
	pages := convertRawPageIntoStruct(currentSection, &sPages)

	currentSection.Pages = pages
}

func convertRawPageIntoStruct(currentSection *Section, sPages *[]string) []Page {
	var pages []Page
	pageCounter := 0
	var hide bool

	var sectionFrontMatter struct {
		Title      string `yaml:"title"`
		Intro      string `yaml:"intro"`
		Conclusion string `yaml:"conclusion"`
	}

	var pageFrontMatter struct {
		Layout string `yaml:"layout"`
	}

	beginningIndex := 0
	for index, sPage := range *sPages {

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
	return pages
}

func extractPagesFromSectionsFiles(filename string) []string {
	var sPages []string
	f, _ := os.Open(filename)
	defer f.Close()
	scanner := bufio.NewScanner(f)

	var currentPageContent []string
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
	return sPages
}
