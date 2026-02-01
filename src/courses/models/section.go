package models

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"strings"

	"github.com/adrg/frontmatter"
	"github.com/go-git/go-billy/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SectionWriter interface {
	OCFMdWriter
	GetSection() string
}

// Part of a chapter
type Section struct {
	entityManagementModels.BaseModel
	Order              int
	FileName           string
	Title              string
	ParentChapterTitle string
	Intro              string
	Conclusion         string
	Number             int

	Chapters    []*Chapter `gorm:"many2many:chapter_sections;"`
	Pages       []*Page    `gorm:"many2many:section_pages;"`
	HiddenPages []int      `gorm:"serializer:json"`
}

type ChapterSections struct {
	ChapterID uuid.UUID `gorm:"primaryKey"`
	SectionID uuid.UUID `gorm:"primaryKey"`
	Order     int
}

func (s *Section) AfterCreate(tx *gorm.DB) (err error) {
	// Create SectionPages entries after the Section and its Pages have been saved
	for _, page := range s.Pages {
		sectionPage := &SectionPages{
			SectionID: s.ID,
			PageID:    page.ID,
			Order:     page.Order, // Assuming order starts from 1
		}
		if err := tx.Save(sectionPage).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s Section) String(chapter Chapter) string {
	sw := SlidevSectionWriter{s, chapter}
	return sw.GetSection()
}

func fillSection(courseFileSystem *billy.Filesystem, currentSection *Section) error {

	if courseFileSystem == nil {
		return errors.New("filesystem is nil")
	}

	f, errFileOpening := (*courseFileSystem).Open(currentSection.FileName)
	if errFileOpening != nil {
		log.Default().Println(errFileOpening.Error())
	}
	defer f.Close()
	scanner, scannerError := getScannerFromFile(f)

	if scannerError != nil {
		return scannerError
	}

	sPages := extractPagesFromSectionsFileScanner(*scanner)
	pages := convertRawPageIntoStruct(currentSection, &sPages)

	currentSection.Pages = pages
	return nil
}

func getScannerFromFile(file billy.File) (*bufio.Scanner, error) {
	// Ensure the read pointer is at the beginning of the file
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	return scanner, nil
}

func convertRawPageIntoStruct(currentSection *Section, sPages *[]string) []*Page {
	var pages []*Page
	pageOrder := 0
	var hide bool

	var sectionFrontMatter struct {
		Title      string `yaml:"title"`
		Intro      string `yaml:"intro"`
		Conclusion string `yaml:"conclusion"`
	}

	var pageFrontMatter struct {
		Layout string `yaml:"layout"`
		Class  string `yaml:"class"`
	}

	beginningIndex := 0
	for index, sPage := range *sPages {

		sectionFrontMatter.Title = ""
		sectionFrontMatter.Intro = ""
		sectionFrontMatter.Conclusion = ""

		pageFrontMatter.Layout = ""

		if index == 1 {
			_, errSectionFrontMatter := frontmatter.Parse(strings.NewReader(sPage), &sectionFrontMatter)
			if errSectionFrontMatter != nil {
				fmt.Println(errSectionFrontMatter.Error())
			}
		}

		if sectionFrontMatter.Title != "" {
			currentSection.Title = sectionFrontMatter.Title
			currentSection.Intro = sectionFrontMatter.Intro
			currentSection.Conclusion = sectionFrontMatter.Conclusion
			beginningIndex = index
		} else {
			if index > beginningIndex {
				pageOrder++
				sPageContent, err := frontmatter.Parse(strings.NewReader(sPage), &pageFrontMatter)

				if err != nil {
					fmt.Println(err.Error())
				}

				if contains(currentSection.HiddenPages, (pageOrder)) {
					hide = true
				}
				page := createPage(pageOrder, strings.Split(string(sPageContent), "\n"), currentSection, hide, pageFrontMatter.Class)
				pages = append(pages, page)
			} else {
				fmt.Println("Front matter for section not found / not formatted as expected")
			}
		}

	}
	return pages
}

func extractPagesFromSectionsFileScanner(scanner bufio.Scanner) []string {
	var sPages []string

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
