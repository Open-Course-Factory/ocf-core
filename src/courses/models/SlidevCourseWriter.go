package models

import (
	"os"
	config "soli/formations/src/configuration"
	"strconv"
	"strings"
)

type SlidevCourseWriter struct {
	Course Course
}

func (scow *SlidevCourseWriter) SetFrontMatter() string {
	return "<!-- _class: lead hide-header -->\n\n"
}

func (scow *SlidevCourseWriter) SetCoverPage() string {
	headfoot := createHeaderFooter(scow.Course.Header, scow.Course.Footer)
	title := "# " + strings.ToUpper(scow.Course.Title) + "\n\n"
	subtitle := "## " + scow.Course.Subtitle + "\n\n"
	logo := "\n" + scow.Course.Logo + "\n"
	return headfoot + title + subtitle + logo
}

func (scow *SlidevCourseWriter) SetIntro() string {
	// ToDo : take data from user
	author := "\n---\n\n@include(./authors/author_tsa.md)\n"
	schedule := "\n---\n\n@include(./schedules/" + scow.Course.Schedule + ")\n"
	prelude := "\n---\n\n@include(./preludes/" + scow.Course.Prelude + ")\n"
	return author + schedule + prelude
}

func (scow *SlidevCourseWriter) SetLearningObjectives() string {
	learningObjectives := ""
	if len(scow.Course.LearningObjectives) > 0 {
		learningObjectivesPathFile := config.COURSES_ROOT + "/learning_objectives/" + scow.Course.LearningObjectives
		_, error := os.Stat(learningObjectivesPathFile)
		if !os.IsNotExist(error) {
			learningObjectives = "\n---\n\n@include(" + learningObjectivesPathFile + ")\n"
		}
	}
	return learningObjectives
}

func (scow *SlidevCourseWriter) SetTitlePage() string {
	headfoot := createHeaderFooter(scow.Course.Header, scow.Course.Footer)
	title := "# " + strings.ToUpper(scow.Course.Title) + "\n\n"
	subtitle := "## " + scow.Course.Subtitle + "\n\n"
	logo := "\n" + scow.Course.Logo + "\n"
	return headfoot + title + subtitle + logo
}

func (scow *SlidevCourseWriter) SetTitle() string {
	// Second title page with the Table Of Content of the chapter

	return ""
}

func (scow *SlidevCourseWriter) SetToc() string {
	var toc string

	frontMatter := "\n---\nlayout: maintoc\n---\n\n"

	toc += frontMatter + "# Thèmes abordés dans le cours\n\n"

	totalChapterNumber := len(scow.Course.Chapters)

	for _, chapter := range scow.Course.Chapters {
		toc += "- Chapitre **" + strconv.Itoa(chapter.Number) + "** : " + chapter.Title + "\n"
		toc += "  - " + chapter.Introduction + "\n"
		if !strings.Contains(scow.Course.Theme, "A4") {
			if totalChapterNumber > 9 && chapter.Number == 6 {
				toc += "- **...**"
				toc += frontMatter + "# Thèmes abordés dans le cours - Suite\n\n"
			}
		}

	}

	toc += "\n"
	return toc
}

func (scow *SlidevCourseWriter) SetContent() string {
	var chapters string

	for _, chapter := range scow.Course.Chapters {
		chapters += chapter.String() + "\n\n"
	}
	return chapters
}

func (scow *SlidevCourseWriter) SetConclusionPage() string {
	// We finish with a conclusion slide using each section conclusion
	var conclusion string
	frontMatter := "\n---\nlayout: cover\nchapter: Conclusion\n---\n\n"
	conclusion += frontMatter + "# Fin\n"
	//conclusion += createFooterAlone("@@author_fullname@@ - Fin")
	conclusion += "\nMerci pour votre attention !\n\n"
	return conclusion
}

func (scow *SlidevCourseWriter) GetCourse() string {
	return scow.SetFrontMatter() + scow.SetTitlePage() + scow.SetIntro() + scow.SetLearningObjectives() + scow.SetTitle() + scow.SetToc() + scow.SetContent() + scow.SetConclusionPage()
}
