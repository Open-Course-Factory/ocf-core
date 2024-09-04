package models

import (
	"strconv"
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
	titlePage := "**CHAPITRE " + strconv.Itoa(scw.Chapter.Number) + "**\n# " + scw.Chapter.getTitle() + "\n\n"
	return titlePage
}

func (scw *SlidevChapterWriter) SetTitle() string {
	// Second title page with the Table Of Content of the chapter
	frontMatter := "\n---\nlayout: maintoc\nchapter: " + scw.Chapter.Title + "\n---\n\n"
	title := frontMatter + "# " + scw.Chapter.getTitle() + "\n\n"
	return title
}

func (scw *SlidevChapterWriter) SetToc() string {
	var toc string
	for _, section := range scw.Chapter.Sections {
		toc += "- **" + section.Title + "** " + section.Intro + "\n"
	}
	toc += "\n"
	return toc
}

func (scw *SlidevChapterWriter) SetContent() string {
	// Then all the chapter sections are added
	var sections string
	for _, section := range scw.Chapter.Sections {
		sections += section.String(scw.Chapter) + "\n\n"
	}
	return sections
}

func (scw *SlidevChapterWriter) SetConclusionPage() string {
	// We finish with a conclusion slide using each section conclusion
	var conclusion string
	conclusion += scw.SetTitle() + "Dans ce chapitre nous avons :\n"
	for _, section := range scw.Chapter.Sections {
		conclusion += "- " + section.Conclusion + "\n"
	}
	conclusion += "\n"
	return conclusion
}

func (scw *SlidevChapterWriter) GetChapter() string {
	return scw.SetFrontMatter() + scw.SetTitlePage() + scw.SetTitle() + scw.SetToc() + scw.SetContent() + scw.SetConclusionPage()
}
