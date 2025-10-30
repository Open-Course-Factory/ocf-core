package models

import (
	"strconv"
	"strings"
)

type SlidevChapterWriter struct {
	Chapter Chapter
}

func (scw *SlidevChapterWriter) SetFrontMatter() string {
	firstLine := "---\n"
	firstLine = firstLine + "layout: cover\n"
	firstLine = firstLine + "chapter: " + scw.Chapter.Title + "\n"
	firstLine = firstLine + "---\n\n"
	return firstLine
}

func (scw *SlidevChapterWriter) SetTitlePage() string {
	// Before the chapter, we create a main title page with only the chapter number + title and the header/footer
	titlePage := "**CHAPITRE " + strconv.Itoa(scw.Chapter.Number) + "**\n# " + scw.Chapter.getTitle(true) + "\n\n"
	return titlePage
}

func (scw *SlidevChapterWriter) SetTitle() string {
	// Second title page with the Table Of Content of the chapter
	frontMatter := "\n---\nlayout: maintoc\nchapter: " + scw.Chapter.getTitle(false) + "\n---\n\n"
	title := frontMatter + "# " + scw.Chapter.getTitle(true) + "\n\n"
	return title
}

func (scw *SlidevChapterWriter) SetToc() string {
	var tocBuilder strings.Builder
	for _, section := range scw.Chapter.Sections {
		tocBuilder.WriteString("- **")
		tocBuilder.WriteString(section.Title)
		tocBuilder.WriteString("** ")
		tocBuilder.WriteString(section.Intro)
		tocBuilder.WriteString("\n")
	}
	tocBuilder.WriteString("\n")
	return tocBuilder.String()
}

func (scw *SlidevChapterWriter) SetContent() string {
	// Then all the chapter sections are added
	var sectionsBuilder strings.Builder
	for _, section := range scw.Chapter.Sections {
		sectionsBuilder.WriteString(section.String(scw.Chapter))
		sectionsBuilder.WriteString("\n\n")
	}
	return sectionsBuilder.String()
}

func (scw *SlidevChapterWriter) SetConclusionPage() string {
	// We finish with a conclusion slide using each section conclusion
	var conclusionBuilder strings.Builder
	conclusionBuilder.WriteString(scw.SetTitle())
	conclusionBuilder.WriteString("Dans ce chapitre nous avons :\n")
	for _, section := range scw.Chapter.Sections {
		conclusionBuilder.WriteString("- ")
		conclusionBuilder.WriteString(section.Conclusion)
		conclusionBuilder.WriteString("\n")
	}
	conclusionBuilder.WriteString("\n")
	return conclusionBuilder.String()
}

func (scw *SlidevChapterWriter) GetChapter() string {
	return scw.SetFrontMatter() + scw.SetTitlePage() + scw.SetTitle() + scw.SetToc() + scw.SetContent() + scw.SetConclusionPage()
}
