package models

import (
	"strconv"
	"strings"
)

// Part of a course
type Chapter struct {
	Title        string
	Number       int
	Footer       string
	Introduction string
	Sections     []Section
}

func (c Chapter) String() string {
	firstLine := "---\n\n"
	footer := createFooterAlone(c.Footer)

	// Before the chapter, we create a main title page with only the chapter number + title and the header/footer
	titlePage := "<!-- _class: lead hide-header -->\n\n**CHAPITRE " + strconv.Itoa(c.Number) + "**\n# " + c.getTitle() + "\n\n"

	// Second title page with the Table Of Content of the chapter
	title := "\n---\n\n<!-- _class: main-toc -->\n\n<p></p>\n\n# " + c.getTitle() + "\n\n"
	var toc string
	for _, section := range c.Sections {
		toc += "- **" + section.Title + "** " + section.Intro + "\n"
	}
	toc += "\n"

	// Then all the chapter sections are added
	var sections string
	for _, section := range c.Sections {
		sections += section.String() + "\n\n"
	}

	// We finish with a conclusion slide using each section conclusion
	var conclusion string
	conclusion += title + "Dans ce chapitre nous avons :\n"
	for _, section := range c.Sections {
		conclusion += "- " + section.Conclusion + "\n"
	}
	conclusion += "\n"

	return firstLine + footer + titlePage + title + toc + sections + conclusion
}

func (c Chapter) getTitle() string {

	return removeAccents(strings.ToUpper(c.Title))
}
