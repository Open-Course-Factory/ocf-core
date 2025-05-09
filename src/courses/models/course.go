package models

import (
	"encoding/json"
	"log"
	"os"
	entityManagementModels "soli/formations/src/entityManagement/models"
	"strings"

	"github.com/go-git/go-billy/v5"
	"gorm.io/gorm"
)

type OCFMdWriter interface {
	SetFrontMatter() string
	SetTitle() string
	SetToc() string
	SetContent() string
}

type CourseMdWriter interface {
	OCFMdWriter
	SetTitlePage() string
	SetIntro() string
	SetLearningObjectives() string
	SetConclusionPage() string
	GetCourse() string
}

type Course struct {
	entityManagementModels.BaseModel
	Category            string
	Name                string
	GitRepository       string
	GitRepositoryBranch string
	FolderName          string
	Version             string
	Title               string
	Subtitle            string
	Header              string
	Footer              string
	Logo                string
	Description         string
	CourseID_str        string
	Schedule            *Schedule `gorm:"-:all" json:"-"`
	Theme               *Theme    `gorm:"-:all" json:"-"`
	Prelude             string
	URL                 string
	LearningObjectives  string     `json:"learning_objectives"`
	Chapters            []*Chapter `gorm:"many2many:course_chapters"`
	Packages            []Generation
}

func (c *Course) AfterCreate(tx *gorm.DB) (err error) {
	for _, chapter := range c.Chapters {
		courseChapter := &CourseChapters{
			CourseID:  c.ID,
			ChapterID: chapter.ID,
			Order:     chapter.Order,
		}

		if err := tx.Save(courseChapter).Error; err != nil {
			return err
		}
	}
	return nil
}

func (c Course) String() string {
	cow := SlidevCourseWriter{c}
	return cow.GetCourse()
}

func (c Course) GetFilename(extensions ...string) string {
	extension := ""
	if len(extensions) > 0 {
		extension = "." + extensions[0]
	}
	return strings.ToLower(c.Category) + "_" + strings.ToLower(c.Name) + "_" + c.Version + extension
}

func (c Course) GetThemes() []string {
	themes := make([]string, 0)

	themes = append(themes, c.Theme.Name)

	for {
		ext, theme := c.Theme.IsThemeExtended(themes[len(themes)-1])

		if !ext {
			break
		}

		themes = append(themes, theme)
	}

	return themes
}

func FillCourseModelFromFiles(courseFileSystem *billy.Filesystem, course *Course) {
	for indexChapter, chapter := range course.Chapters {
		chapter.Number = indexChapter + 1
		chapter.OwnerIDs = append(chapter.OwnerIDs, course.OwnerIDs[0])
		for indexSection, section := range chapter.Sections {
			section.OwnerIDs = append(section.OwnerIDs, chapter.OwnerIDs[0])
			section.Number = indexSection + 1
			section.Chapters = append(section.Chapters, chapter)
			section.ParentChapterTitle = chapter.getTitle(true)
			fillSection(courseFileSystem, section)
			chapter.Sections[indexSection] = section
		}
		course.Chapters[indexChapter] = chapter
	}

	course.InitTocs()
}

func (course *Course) InitTocs() {
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

func ReadJsonCourseFile(jsonCourseFilePath string) *Course {
	jsonFile, err := os.ReadFile(jsonCourseFilePath)

	// should try to download it -> how to standardize the course format ?
	// should we pass it as a param ? (if just a name, look for it locally, either dl ?)
	if err != nil {
		log.Fatal("Error during ReadFile(): ", err)
	}

	var course Course
	err = json.Unmarshal(jsonFile, &course)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
	}
	return &course
}
