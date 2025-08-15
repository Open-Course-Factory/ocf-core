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
	// var globalCourseInfo string

	layout := "layout: intro\n"
	author := "author: @@author_fullname@@\n"

	lineNumbers := "lineNumbers: false\n"
	download := "download: true\n"
	exportFilename := "exportFilename: slides-exported\n"
	routerMode := "routerMode: hash\n"
	highlighter := "highlighter: shiki\n"

	globalCourseInfo := layout + author + lineNumbers + download + exportFilename + routerMode + highlighter

	drawings := "drawings:\n"
	drawingsPresenter := "  presenterOnly: true\n"
	drawingPersistence := "  persist: true\n"
	drawingsOptions := drawings + drawingsPresenter + drawingPersistence

	theme := "theme: ./theme\nthemeConfig:\n"
	themeTitle := "  title: " + scow.Course.Title + "\n"
	themeFooterTitle := "  footerTitle: " + scow.Course.Title + "\n"
	themeVersion := "  version: @@version@@\n"
	themeAuthor := "  author: @@author_fullname@@\n"
	themeAuthorEmail := "  email: @@author_email@@\n"
	themeOptions := theme + themeTitle + themeFooterTitle + themeVersion + themeAuthor + themeAuthorEmail

	return "---\n" + globalCourseInfo + drawingsOptions + themeOptions + "---\n\n"
}

func (scow *SlidevCourseWriter) SetCoverPage() string {
	title := "# " + strings.ToUpper(scow.Course.Title) + "\n\n"
	subtitle := "## " + scow.Course.Subtitle + "\n\n"
	logo := "\n" + scow.Course.Logo + "\n"
	return title + subtitle + logo
}

func (scow *SlidevCourseWriter) SetIntro() string {
	// ToDo : take data from user
	//author := "\n---\nlayout: twocols\nchapter: " + scow.Course.Chapters[0].Title + "\nsrc: theme/authors/author_@@author@@.md\n---\n\n"
	author := ""
	schedule := scow.fillSchedule()
	chapterTitle := "default title"
	chapterTitle = getChapterTitle(scow, chapterTitle)
	prelude := "\n---\nlayout: cover\nchapter: " + chapterTitle + "\nsrc: theme/preludes/" + scow.Course.Prelude + "\n---\n\n"
	return author + schedule + prelude
}

func getChapterTitle(scow *SlidevCourseWriter, chapterTitle string) string {
	if scow.Course.Chapters != nil {
		if len(scow.Course.Chapters) > 0 {
			chapterTitle = scow.Course.Chapters[0].Title
		}
	}
	return chapterTitle
}

func (scow *SlidevCourseWriter) fillSchedule() string {

	schedule := "---\nlayout: schedule\n"
	chapterTitle := "default title"
	chapterTitle = getChapterTitle(scow, chapterTitle)
	schedule = schedule + "chapter: " + chapterTitle + "\n"
	if scow.Course.Schedule != nil {
		for _, line := range scow.Course.Schedule.FrontMatterContent {
			schedule = schedule + line + "\n"
		}
		schedule = schedule + "---\n"
	} else {
		schedule = ""
	}

	return schedule
}

func (scow *SlidevCourseWriter) SetLearningObjectives() string {
	learningObjectives := ""
	if len(scow.Course.LearningObjectives) > 0 {
		learningObjectivesPathFile := config.COURSES_ROOT + "learning_objectives/" + scow.Course.LearningObjectives
		_, error := os.Stat(learningObjectivesPathFile)
		if !os.IsNotExist(error) {
			learningObjectives = "\n---\n\n@include(" + learningObjectivesPathFile + ")\n"
		}
	}
	return learningObjectives
}

func (scow *SlidevCourseWriter) SetTitlePage() string {
	title := "# " + strings.ToUpper(scow.Course.Title) + "\n\n"
	subtitle := "## " + scow.Course.Subtitle + "\n\n"
	logo := "\n" + scow.Course.Logo + "\n"
	return title + subtitle + logo
}

func (scow *SlidevCourseWriter) SetTitle() string {
	// Second title page with the Table Of Content of the chapter

	return ""
}

func (scow *SlidevCourseWriter) SetToc() string {
	var toc string

	frontMatter := "\n---\nlayout: maintoc\nchapter: " + scow.Course.Title + "\n---\n\n"

	toc += frontMatter + "# Thèmes abordés dans le cours\n\n"

	totalChapterNumber := len(scow.Course.Chapters)

	for _, chapter := range scow.Course.Chapters {
		toc += "- Chapitre **" + strconv.Itoa(chapter.Number) + "** : " + chapter.Title + "\n"
		toc += "  - " + chapter.Introduction + "\n"
		if !strings.Contains(scow.Course.Theme.Name, "A4") {
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
	conclusion += "\nMerci pour votre attention !\n\n"
	return conclusion
}

func (scow *SlidevCourseWriter) GetCourse() string {
	return scow.SetFrontMatter() + scow.SetTitlePage() + scow.SetIntro() + scow.SetLearningObjectives() + scow.SetTitle() + scow.SetToc() + scow.SetContent() + scow.SetConclusionPage()
}
