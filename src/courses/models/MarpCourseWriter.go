package models

import (
	"os"
	config "soli/formations/src/configuration"
	"strconv"
	"strings"
)

type MarpCourseWriter struct {
	Course Course
}

func (mcow *MarpCourseWriter) SetFrontMatter() string {
	return "<!-- _class: lead hide-header -->\n\n"
}

func (mcow *MarpCourseWriter) SetCoverPage() string {
	headfoot := createHeaderFooter(mcow.Course.Header, mcow.Course.Footer)
	title := "# " + strings.ToUpper(mcow.Course.Title) + "\n\n"
	subtitle := "## " + mcow.Course.Subtitle + "\n\n"
	logo := "\n" + mcow.Course.Logo + "\n"
	return headfoot + title + subtitle + logo
}

func (mcow *MarpCourseWriter) SetIntro() string {
	// ToDo : take data from user
	author := "\n---\n\n@include(./authors/author_tsa.md)\n"
	// ToDo : take data from schedule db
	//schedule := "\n---\n\n@include(./schedules/" + mcow.Course.Schedule + ")\n"
	schedule := ""
	prelude := "\n---\n\n@include(./preludes/" + mcow.Course.Prelude + ")\n"
	return author + schedule + prelude
}

func (mcow *MarpCourseWriter) SetLearningObjectives() string {
	learningObjectives := ""
	if len(mcow.Course.LearningObjectives) > 0 {
		learningObjectivesPathFile := config.COURSES_ROOT + "learning_objectives/" + mcow.Course.LearningObjectives
		_, error := os.Stat(learningObjectivesPathFile)
		if !os.IsNotExist(error) {
			learningObjectives = "\n---\n\n@include(" + learningObjectivesPathFile + ")\n"
		}
	}
	return learningObjectives
}

func (mcow *MarpCourseWriter) SetTitlePage() string {
	headfoot := createHeaderFooter(mcow.Course.Header, mcow.Course.Footer)
	title := "# " + strings.ToUpper(mcow.Course.Title) + "\n\n"
	subtitle := "## " + mcow.Course.Subtitle + "\n\n"
	logo := "\n" + mcow.Course.Logo + "\n"
	return headfoot + title + subtitle + logo
}

func (mcow *MarpCourseWriter) SetTitle() string {
	// Second title page with the Table Of Content of the chapter

	return ""
}

func (mcow *MarpCourseWriter) SetToc() string {
	var toc string

	toc += "\n\n---\n\n<!-- _class: main-toc -->\n\n# Thèmes abordés dans le cours\n\n"

	totalChapterNumber := len(mcow.Course.Chapters)

	for _, chapter := range mcow.Course.Chapters {
		toc += "- Chapitre **" + strconv.Itoa(chapter.Number) + "** : " + chapter.Title + "\n"
		toc += "  - " + chapter.Introduction + "\n"
		if !strings.Contains(mcow.Course.Theme.Name, "A4") {
			if totalChapterNumber > 9 && chapter.Number == 6 {
				toc += "- **...**"
				toc += "\n\n---\n\n<!-- _class: main-toc -->\n\n# Thèmes abordés dans le cours - Suite\n\n"
			}
		}

	}

	toc += "\n"
	return toc
}

func (mcow *MarpCourseWriter) SetContent() string {
	var chapters string

	for _, chapter := range mcow.Course.Chapters {
		chapters += chapter.String() + "\n\n"
	}
	return chapters
}

func (mcow *MarpCourseWriter) SetConclusionPage() string {
	// We finish with a conclusion slide using each section conclusion
	var conclusion string
	conclusion += "\n---\n\n<!-- _class: lead hide-header -->\n\n# Fin\n"
	conclusion += createFooterAlone("@@author_fullname@@ - Fin")
	conclusion += "\nMerci pour votre attention !\n\n"
	return conclusion
}

func (mcow *MarpCourseWriter) GetCourse() string {
	return mcow.SetFrontMatter() + mcow.SetTitlePage() + mcow.SetIntro() + mcow.SetLearningObjectives() + mcow.SetTitle() + mcow.SetToc() + mcow.SetContent() + mcow.SetConclusionPage()
}
