package models

import (
	"strconv"
)

type MarpChapterWriter struct {
	Chapter Chapter
}

func (mcw *MarpChapterWriter) SetFrontMatter() string {
	firstLine := "---\n\n"
	footer := createFooterAlone(mcw.Chapter.Footer)
	return firstLine + footer
}

func (mcw *MarpChapterWriter) SetTitlePage() string {
	// Before the chapter, we create a main title page with only the chapter number + title and the header/footer
	titlePage := "<!-- _class: lead hide-header -->\n\n**CHAPITRE " + strconv.Itoa(mcw.Chapter.Number) + "**\n# " + mcw.Chapter.getTitle() + "\n\n"
	return titlePage
}

func (mcw *MarpChapterWriter) SetTitle() string {
	// Second title page with the Table Of Content of the chapter
	title := "\n---\n\n<!-- _class: main-toc -->\n\n<p></p>\n\n# " + mcw.Chapter.getTitle() + "\n\n"
	return title
}

func (mcw *MarpChapterWriter) SetToc() string {
	var toc string
	for _, section := range mcw.Chapter.Sections {
		toc += "- **" + section.Title + "** " + section.Intro + "\n"
	}
	toc += "\n"
	return toc
}

func (mcw *MarpChapterWriter) SetContent() string {
	// Then all the chapter sections are added
	var sections string
	for _, section := range mcw.Chapter.Sections {
		sections += section.String(mcw.Chapter) + "\n\n"
	}
	return sections
}

func (mcw *MarpChapterWriter) SetConclusionPage() string {
	// We finish with a conclusion slide using each section conclusion
	var conclusion string
	conclusion += mcw.SetTitle() + "Dans ce chapitre nous avons :\n"
	for _, section := range mcw.Chapter.Sections {
		conclusion += "- " + section.Conclusion + "\n"
	}
	conclusion += "\n"
	return conclusion
}

func (mcw *MarpChapterWriter) GetChapter() string {
	return mcw.SetFrontMatter() + mcw.SetTitlePage() + mcw.SetTitle() + mcw.SetToc() + mcw.SetContent() + mcw.SetConclusionPage()
}

func createFooterAlone(footer string) string {
	return "<!--\n" + "footer: '" + footer + "'\n-->\n\n"
}

func createHeaderFooter(header string, footer string) string {
	return "<!--\n" + "header: '" + header + "'\n" + "footer: '" + footer + "'\n-->\n\n"
}
