package models

import (
	"encoding/json"
	"log"
	"os"
	"soli/formations/src/auth/models"
	config "soli/formations/src/configuration"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type Format int

const (
	HTML Format = iota
	PDF
)

func (s Format) String() string {
	switch s {
	case HTML:
		return "html"
	case PDF:
		return "pdf"
	}
	return "unknown"
}

type Course struct {
	entityManagementModels.BaseModel
	Category           string
	Name               string
	Version            string
	Title              string
	Subtitle           string
	Header             string
	Footer             string
	Logo               string
	OwnerID            *uuid.UUID
	Owner              *models.User `json:"owner"`
	Description        string
	Format             Format
	CourseID_str       string
	Schedule           string
	Prelude            string
	Theme              string
	URL                string
	LearningObjectives string    `json:"learning_objectives"`
	Chapters           []Chapter `gorm:"many2many:course_chapters;" json:"chapters"`
}

func (c Course) String() string {
	localClass := "<!-- _class: lead hide-header -->\n\n"
	headfoot := createHeaderFooter(c.Header, c.Footer)
	title := "# " + strings.ToUpper(c.Title) + "\n\n"
	subtitle := "## " + c.Subtitle + "\n\n"
	logo := "\n" + c.Logo + "\n"
	// ToDo : take data from user
	author := "\n---\n\n@include(./authors/author_tsa.md)\n"
	schedule := "\n---\n\n@include(./schedules/" + c.Schedule + ")\n"
	prelude := "\n---\n\n@include(./preludes/" + c.Prelude + ")\n"

	learningObjectives := ""
	if len(c.LearningObjectives) > 0 {
		learningObjectivesPathFile := config.COURSES_ROOT + "/learning_objectives/" + c.LearningObjectives
		_, error := os.Stat(learningObjectivesPathFile)
		if !os.IsNotExist(error) {
			learningObjectives = "\n---\n\n@include(" + learningObjectivesPathFile + ")\n"
		}
	}

	var chapters string

	chapters += "\n\n---\n\n<!-- _class: main-toc -->\n\n# Thèmes abordés dans le cours\n\n"

	totalChapterNumber := len(c.Chapters)

	for _, chapter := range c.Chapters {
		chapters += "- Chapitre **" + strconv.Itoa(chapter.Number) + "** : " + chapter.Title + "\n"
		chapters += "  - " + chapter.Introduction + "\n"
		if !strings.Contains(c.Theme, "A4") {
			if totalChapterNumber > 9 && chapter.Number == 6 {
				chapters += "- **...**"
				chapters += "\n\n---\n\n<!-- _class: main-toc -->\n\n# Thèmes abordés dans le cours - Suite\n\n"
			}
		}

	}

	chapters += "\n"

	for _, chapter := range c.Chapters {
		chapters += chapter.String() + "\n\n"
	}

	chapters += "\n---\n\n<!-- _class: lead hide-header -->\n\n# Fin\n"
	chapters += createFooterAlone("@@author_fullname@@ - Fin")
	chapters += "\nMerci pour votre attention !\n\n"

	cover_slide := localClass + headfoot + title + subtitle + logo
	preamble := author + schedule + learningObjectives + prelude

	return cover_slide + preamble + chapters
}

func (c Course) GetFilename(extensions ...string) string {
	extension := ""
	if len(extensions) > 0 {
		extension = "." + extensions[0]
	}
	return strings.ToLower(c.Category) + "_" + strings.ToLower(c.Name) + "_" + c.Version + extension
}

func (c Course) IsThemeExtended(themes ...string) (bool, string) {
	theme := c.Theme
	res := false
	from := ""

	if len(themes) > 0 {
		theme = themes[0]
	}

	extendsFilePath := config.THEMES_ROOT + "/" + theme + "/extends.json"
	if fileExists(extendsFilePath) {
		extends := LoadExtends(extendsFilePath)
		from = extends.Theme
		res = true
	}

	return res, from
}

func (c Course) GetThemes() []string {
	themes := make([]string, 0)

	themes = append(themes, c.Theme)

	for {
		ext, theme := c.IsThemeExtended(themes[len(themes)-1])

		if !ext {
			break
		}

		themes = append(themes, theme)
	}

	return themes
}

func CreateCourse(course *Course) {
	for indexChapter, chapter := range course.Chapters {
		chapter.Number = indexChapter + 1
		for indexSection, section := range chapter.Sections {
			fillSection(&section)
			section.Number = indexSection + 1
			section.ParentChapterTitle = chapter.getTitle()
			chapter.Sections[indexSection] = section
		}
		course.Chapters[indexChapter] = chapter
	}

	initTocs(course)
}

func (c *Course) CompileResources(configuration *config.Configuration) error {
	outputDir := config.COURSES_OUTPUT_DIR + c.Theme
	outputFolders := [2]string{"images", "theme"}

	for _, f := range outputFolders {
		err := os.MkdirAll(outputDir+"/"+f, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Copy Themes
	for _, t := range c.GetThemes() {
		themeSrc := config.THEMES_ROOT + t
		cptErr := CopyDir(themeSrc, outputDir+"/theme")
		if cptErr != nil {
			log.Fatal(cptErr)
		}
	}

	// Copy global images
	if _, err := os.Stat(config.IMAGES_ROOT); !os.IsNotExist(err) {
		cpiErr := CopyDir(config.IMAGES_ROOT, outputDir+"/images")
		if cpiErr != nil {
			log.Fatal(cpiErr)
		}
	}

	// Copy course specifique images
	courseImages := config.COURSES_ROOT + c.Category + "/images"
	if _, ciiErr := os.Stat(courseImages); !os.IsNotExist(ciiErr) {
		cpic_err := CopyDir(courseImages, outputDir+"/images")
		if cpic_err != nil {
			log.Fatal(cpic_err)
		}
	}

	return nil
}

func (c *Course) WriteMd(configuration *config.Configuration) (string, error) {
	outputDir := config.COURSES_OUTPUT_DIR + c.Theme

	err := os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}

	fileToCreate := outputDir + "/" + c.GetFilename("md")
	f, err := os.Create(fileToCreate)

	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	courseReplaceTrigram := strings.ReplaceAll(c.String(), "@@author@@", configuration.AuthorTrigram)
	courseReplaceFullname := strings.ReplaceAll(courseReplaceTrigram, "@@author_fullname@@", configuration.AuthorFullname)
	courseReplaceEmail := strings.ReplaceAll(courseReplaceFullname, "@@author_email@@", configuration.AuthorEmail)
	courseReplaceVersion := strings.ReplaceAll(courseReplaceEmail, "@@version@@", c.Version)

	_, err2 := f.WriteString(courseReplaceVersion)

	if err2 != nil {
		log.Fatal(err2)
	}

	return fileToCreate, err
}

func initTocs(course *Course) {
	tocsChapter := make(map[int][]string)
	for _, chapter := range course.Chapters {
		for _, section := range chapter.Sections {
			tocsChapter[chapter.Number] = append(tocsChapter[chapter.Number], section.Title)
		}
	}
	for indexChapter, chapter := range course.Chapters {
		for indexSection, section := range chapter.Sections {
			for indexPage, page := range section.Pages {
				for _, toc := range tocsChapter[chapter.Number] {
					if toc == section.Title {
						page.Toc = append(page.Toc, "**"+toc+"**")
					} else {
						page.Toc = append(page.Toc, toc)
					}
				}
				section.Pages[indexPage] = page
			}
			chapter.Sections[indexSection] = section
		}
		course.Chapters[indexChapter] = chapter
	}
}

func ReadJsonCourseFile(jsonCourseFilePath string) Course {
	jsonFile, err := os.ReadFile(jsonCourseFilePath)

	if err != nil {
		log.Fatal("Error during ReadFile(): ", err)
	}

	var course Course
	err = json.Unmarshal(jsonFile, &course)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
	}
	return course
}
